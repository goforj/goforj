package managedsession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultClientVersion    = "goforj"
	defaultHandshakeTimeout = 5 * time.Second
	defaultRequestTimeout   = 30 * time.Second
)

var defaultProtocolRanges = []VersionRange{{Min: Version{Major: 1}, Max: Version{Major: 1}}}

// Dialer opens one transport-neutral connection to Harbor's managed-session endpoint.
type Dialer func(context.Context) (net.Conn, error)

// ClientConfig defines one immutable managed-session client policy.
type ClientConfig struct {
	// ClientVersion is advertised during handshake; empty uses the GoForj default token.
	ClientVersion string
	// ProtocolRanges lists protocol revisions understood by this client; empty uses v1.0.
	ProtocolRanges []VersionRange
	// Capabilities lists features to advertise; empty advertises managed-session.v1.
	Capabilities []Capability
	// HandshakeTimeout bounds unauthenticated negotiation; zero uses five seconds.
	HandshakeTimeout time.Duration
	// RequestTimeout supplies a deadline when an operation context has none; zero uses 30 seconds.
	RequestTimeout time.Duration
}

// Peer describes the daemon identity established during negotiation.
type Peer struct {
	// Role is the daemon's negotiated role.
	Role Role
	// DaemonVersion is Harbor's advertised build token.
	DaemonVersion string
	// Protocol is the exact selected protocol revision.
	Protocol Version
	// Capabilities is the negotiated capability intersection.
	Capabilities []Capability
}

// Client sends serialized v1 managed-session calls over one negotiated connection.
type Client struct {
	connection net.Conn
	reader     *frameReader
	writer     *frameWriter
	config     ClientConfig
	peer       Peer
	// launchContext is retained only for an explicitly inherited Harbor session so a new IPC connection can replay
	// the same durable process identity after Harbor restarts. It never leaves this process or enters a request log.
	launchContext *LaunchContext
	requestID     atomic.Uint64
	callMutex     sync.Mutex
	closeOnce     sync.Once
	closed        chan struct{}
}

// Dial opens and negotiates a managed-session client through an injected transport dialer.
func Dial(ctx context.Context, dialer Dialer, config ClientConfig) (*Client, error) {
	if dialer == nil {
		return nil, errors.New("managed session dialer is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	connection, err := dialer(ctx)
	if err != nil {
		return nil, fmt.Errorf("dial managed session endpoint: %w", err)
	}
	if connection == nil {
		return nil, errors.New("managed session dialer returned a nil connection")
	}
	client, err := NewClient(ctx, connection, config)
	if err != nil {
		return nil, err
	}
	return client, nil
}

// NewClient negotiates one already-authenticated transport and starts a typed client.
func NewClient(ctx context.Context, connection net.Conn, config ClientConfig) (*Client, error) {
	if connection == nil {
		return nil, errors.New("managed session connection is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	normalized, err := normalizeClientConfig(config)
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	client := &Client{
		connection: connection,
		reader:     &frameReader{reader: connection, limit: MaximumFrameSize},
		writer:     &frameWriter{writer: connection, limit: MaximumFrameSize},
		config:     normalized,
		closed:     make(chan struct{}),
	}
	if err := client.negotiate(ctx); err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("negotiate managed session: %w", err)
	}
	return client, nil
}

// Peer returns a copy of the negotiated daemon identity.
func (client *Client) Peer() Peer {
	peer := client.peer
	peer.Capabilities = append([]Capability(nil), peer.Capabilities...)
	return peer
}

// Close terminates the transport and wakes future calls.
func (client *Client) Close() error {
	client.closeOnce.Do(func() {
		close(client.closed)
		_ = client.connection.Close()
	})
	return nil
}

// ReconnectLaunchSession opens a fresh authenticated connection and replays the retained Harbor launch identity.
//
// Harbor accepts the replay only when the durable attached session still names this process, checkout, descriptor,
// generation, and launch credential. The original client remains usable until the caller replaces it with the
// returned client, which lets a recovery loop preserve the current fence while the new connection is negotiated.
func (client *Client) ReconnectLaunchSession(ctx context.Context) (*Client, RegisterResponse, error) {
	if client == nil {
		return nil, RegisterResponse{}, errors.New("managed session client is required")
	}
	if client.launchContext == nil {
		return nil, RegisterResponse{}, errors.New("managed session client has no inherited launch context")
	}
	select {
	case <-client.closed:
		return nil, RegisterResponse{}, ErrClosed
	default:
	}
	launch := *client.launchContext
	return OpenLaunchSession(ctx, launch)
}

// Register attaches the process and validates the returned session fence.
func (client *Client) Register(ctx context.Context, request RegisterRequest) (RegisterResponse, error) {
	normalized, err := NormalizeRegisterRequest(request)
	if err != nil {
		return RegisterResponse{}, fmt.Errorf("normalize managed session registration: %w", err)
	}
	if normalized.LaunchTicket != "" && !containsCapability(client.peer.Capabilities, CapabilityLaunchContextV1) {
		return RegisterResponse{}, errors.New("managed session launch context capability was not negotiated")
	}
	payload, err := MarshalRegisterRequest(normalized)
	if err != nil {
		return RegisterResponse{}, err
	}
	responsePayload, err := client.call(ctx, MethodRegister, payload)
	if err != nil {
		return RegisterResponse{}, err
	}
	response, err := DecodeRegisterResponse(responsePayload)
	if err != nil {
		return RegisterResponse{}, err
	}
	if err := ValidateRegisterCorrelation(normalized, response); err != nil {
		return RegisterResponse{}, err
	}
	return response, nil
}

// RegisterUntilAttached retries only Harbor's explicit startup race until the caller context ends.
func (client *Client) RegisterUntilAttached(ctx context.Context, request RegisterRequest) (RegisterResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		response, err := client.Register(ctx, request)
		if err == nil {
			return response, nil
		}
		var remoteError RemoteError
		if !errors.As(err, &remoteError) || remoteError.Failure.Code != ErrorCodeUnavailable || !remoteError.Failure.Retryable {
			return RegisterResponse{}, err
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return RegisterResponse{}, ctx.Err()
		case <-timer.C:
		}
	}
}

// ReplacePublications sends a complete observed publication replacement and validates its acknowledgement.
func (client *Client) ReplacePublications(ctx context.Context, request ReplacePublicationsRequest) (ReplacePublicationsResponse, error) {
	payload, err := MarshalReplacePublicationsRequest(request)
	if err != nil {
		return ReplacePublicationsResponse{}, err
	}
	responsePayload, err := client.call(ctx, MethodReplacePublications, payload)
	if err != nil {
		return ReplacePublicationsResponse{}, err
	}
	response, err := DecodeReplacePublicationsResponse(responsePayload)
	if err != nil {
		return ReplacePublicationsResponse{}, err
	}
	if err := ValidateReplacePublicationsCorrelation(request, response); err != nil {
		return ReplacePublicationsResponse{}, err
	}
	return response, nil
}

// Barrier asks Harbor to acknowledge a named lifecycle phase and validates its correlation.
func (client *Client) Barrier(ctx context.Context, request BarrierRequest) (BarrierResponse, error) {
	payload, err := MarshalBarrierRequest(request)
	if err != nil {
		return BarrierResponse{}, err
	}
	responsePayload, err := client.call(ctx, MethodBarrier, payload)
	if err != nil {
		return BarrierResponse{}, err
	}
	response, err := DecodeBarrierResponse(responsePayload)
	if err != nil {
		return BarrierResponse{}, err
	}
	if err := ValidateBarrierCorrelation(request, response); err != nil {
		return BarrierResponse{}, err
	}
	return response, nil
}

// RuntimePlan requests one semantic endpoint-assignment plan when the optional capability was negotiated.
func (client *Client) RuntimePlan(ctx context.Context, request RuntimePlanRequest) (RuntimePlanResponse, error) {
	if !containsCapability(client.peer.Capabilities, CapabilityRuntimePlanV1) {
		return RuntimePlanResponse{}, errors.New("managed session runtime-plan capability was not negotiated")
	}
	payload, err := MarshalRuntimePlanRequest(request)
	if err != nil {
		return RuntimePlanResponse{}, err
	}
	responsePayload, err := client.call(ctx, MethodRuntimePlan, payload)
	if err != nil {
		return RuntimePlanResponse{}, err
	}
	response, err := DecodeRuntimePlanResponse(responsePayload)
	if err != nil {
		return RuntimePlanResponse{}, err
	}
	if err := ValidateRuntimePlanCorrelation(request, response); err != nil {
		return RuntimePlanResponse{}, err
	}
	return response, nil
}

// PublishEvent sends one negotiated managed-session event without waiting for
// a response. Event delivery is best-effort at the transport boundary; the
// Harbor sink remains responsible for applying its exact session fence.
func (client *Client) PublishEvent(ctx context.Context, event Event) error {
	if !containsCapability(client.peer.Capabilities, CapabilityEventsV1) {
		return errors.New("managed session events capability was not negotiated")
	}
	payload, err := MarshalEvent(event)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline, cancel, err := operationContext(ctx, client.config.RequestTimeout)
	if err != nil {
		return err
	}
	defer cancel()
	client.callMutex.Lock()
	defer client.callMutex.Unlock()
	select {
	case <-client.closed:
		return ErrClosed
	default:
	}
	if err := client.connection.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set event deadline: %w", err)
	}
	defer func() { _ = client.connection.SetDeadline(time.Time{}) }()
	sequence := event.Sequence
	message := envelope{
		Kind:     kindEvent,
		Protocol: &client.peer.Protocol,
		Name:     string(event.Kind),
		Sequence: &sequence,
		Payload:  json.RawMessage(payload),
	}
	if err := client.writeEnvelope(message); err != nil {
		return wrapManagedSessionTransportError("write event", err)
	}
	return nil
}

// negotiate performs the unauthenticated Hello/Welcome exchange before calls are allowed.
func (client *Client) negotiate(ctx context.Context) error {
	deadline, cancel, err := operationContext(ctx, client.config.HandshakeTimeout)
	if err != nil {
		return err
	}
	defer cancel()
	if err := client.connection.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set handshake deadline: %w", err)
	}
	defer func() { _ = client.connection.SetDeadline(time.Time{}) }()
	hello := Hello{
		ProtocolRanges: append([]VersionRange(nil), client.config.ProtocolRanges...),
		Role:           RoleGoForjSession,
		ClientVersion:  client.config.ClientVersion,
		Capabilities:   append([]Capability(nil), client.config.Capabilities...),
	}
	message, err := newEnvelopePayload(kindHello, hello)
	if err != nil {
		return fmt.Errorf("create hello: %w", err)
	}
	if err := message.Validate(); err != nil {
		return fmt.Errorf("validate hello: %w", err)
	}
	if err := client.writeEnvelope(message); err != nil {
		return fmt.Errorf("write hello: %w", err)
	}
	response, err := client.readEnvelope()
	if err != nil {
		return fmt.Errorf("read handshake: %w", err)
	}
	switch response.Kind {
	case kindReject:
		var reject Reject
		if err := json.Unmarshal(response.Payload, &reject); err != nil {
			return fmt.Errorf("decode handshake rejection: %w", err)
		}
		if err := reject.Validate(); err != nil {
			return fmt.Errorf("validate handshake rejection: %w", err)
		}
		return HandshakeError{Failure: reject.Error}
	case kindWelcome:
		var welcome Welcome
		if err := json.Unmarshal(response.Payload, &welcome); err != nil {
			return fmt.Errorf("decode welcome: %w", err)
		}
		if err := welcome.Validate(); err != nil {
			return fmt.Errorf("validate welcome: %w", err)
		}
		if response.Protocol == nil || response.Protocol.Compare(welcome.Protocol) != 0 {
			return errors.New("welcome envelope protocol does not match its payload")
		}
		selected, err := NegotiateVersion(client.config.ProtocolRanges, welcome.ProtocolRanges)
		if err != nil || selected.Compare(welcome.Protocol) != 0 {
			return errors.New("daemon selected an unadvertised protocol")
		}
		if !containsCapability(welcome.Capabilities, CapabilityV1) {
			return errors.New("daemon did not select managed-session.v1")
		}
		for _, capability := range welcome.Capabilities {
			if !containsCapability(client.config.Capabilities, capability) {
				return fmt.Errorf("daemon selected unadvertised capability %q", capability)
			}
		}
		client.peer = Peer{
			Role:          welcome.Role,
			DaemonVersion: welcome.DaemonVersion,
			Protocol:      welcome.Protocol,
			Capabilities:  append([]Capability(nil), welcome.Capabilities...),
		}
		return nil
	default:
		return fmt.Errorf("daemon handshake must welcome or reject, got %q", response.Kind)
	}
}

// call sends one bounded request and waits for its correlated response.
func (client *Client) call(ctx context.Context, method string, payload []byte) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	deadline, cancel, err := operationContext(ctx, client.config.RequestTimeout)
	if err != nil {
		return nil, err
	}
	defer cancel()
	select {
	case <-client.closed:
		return nil, ErrClosed
	default:
	}
	client.callMutex.Lock()
	defer client.callMutex.Unlock()
	select {
	case <-client.closed:
		return nil, ErrClosed
	default:
	}
	if err := client.connection.SetDeadline(deadline); err != nil {
		return nil, fmt.Errorf("set request deadline: %w", err)
	}
	defer func() { _ = client.connection.SetDeadline(time.Time{}) }()
	requestID := "managed-session-request-" + strconv.FormatUint(client.requestID.Add(1), 10)
	message, err := newEnvelopePayload(kindRequest, json.RawMessage(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	message.Protocol = &client.peer.Protocol
	message.RequestID = requestID
	message.Method = method
	deadlineUTC := deadline.UTC()
	message.Deadline = &deadlineUTC
	if err := message.Validate(); err != nil {
		return nil, fmt.Errorf("validate request: %w", err)
	}
	if err := client.writeEnvelope(message); err != nil {
		return nil, wrapManagedSessionTransportError("write request", err)
	}
	response, err := client.readEnvelope()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !time.Now().Before(deadline) {
			return nil, context.DeadlineExceeded
		}
		return nil, wrapManagedSessionTransportError("read response", err)
	}
	if response.Kind != kindResponse {
		return nil, fmt.Errorf("managed session response kind %q is not response", response.Kind)
	}
	if response.Protocol == nil || response.Protocol.Compare(client.peer.Protocol) != 0 {
		return nil, errors.New("managed session response protocol does not match negotiation")
	}
	if response.RequestID != requestID {
		return nil, fmt.Errorf("managed session response request ID %q does not match %q", response.RequestID, requestID)
	}
	if response.Error != nil {
		return nil, RemoteError{Failure: *response.Error}
	}
	return append([]byte(nil), response.Payload...), nil
}

// wrapManagedSessionTransportError marks only stream failures as reconnectable while preserving protocol errors as terminal.
func wrapManagedSessionTransportError(operation string, err error) error {
	wrapped := fmt.Errorf("%s: %w", operation, err)
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, net.ErrClosed) {
		return errors.Join(ErrDisconnected, wrapped)
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return errors.Join(ErrDisconnected, wrapped)
	}
	return wrapped
}

// writeEnvelope validates and frames one envelope.
func (client *Client) writeEnvelope(message envelope) error {
	if err := message.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode envelope: %w", err)
	}
	return client.writer.writeFrame(payload)
}

// readEnvelope reads and validates one envelope while leaving typed payload validation to its method.
func (client *Client) readEnvelope() (envelope, error) {
	payload, err := client.reader.readFrame()
	if err != nil {
		return envelope{}, err
	}
	var message envelope
	if err := json.Unmarshal(payload, &message); err != nil {
		return envelope{}, fmt.Errorf("decode envelope: %w", err)
	}
	if err := message.Validate(); err != nil {
		return envelope{}, fmt.Errorf("validate envelope: %w", err)
	}
	return message, nil
}

// normalizeClientConfig supplies the one protocol and capability required by this adapter.
func normalizeClientConfig(config ClientConfig) (ClientConfig, error) {
	if config.ClientVersion == "" {
		config.ClientVersion = defaultClientVersion
	}
	if config.HandshakeTimeout < 0 || config.RequestTimeout < 0 {
		return ClientConfig{}, errors.New("managed session client timeouts must not be negative")
	}
	if config.HandshakeTimeout == 0 {
		config.HandshakeTimeout = defaultHandshakeTimeout
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = defaultRequestTimeout
	}
	if len(config.ProtocolRanges) == 0 {
		config.ProtocolRanges = append([]VersionRange(nil), defaultProtocolRanges...)
	}
	ranges, err := CanonicalVersionRanges(config.ProtocolRanges)
	if err != nil {
		return ClientConfig{}, fmt.Errorf("client protocol ranges: %w", err)
	}
	config.ProtocolRanges = ranges
	if len(config.Capabilities) == 0 {
		config.Capabilities = []Capability{CapabilityV1}
	}
	capabilities, err := CanonicalCapabilities(config.Capabilities)
	if err != nil {
		return ClientConfig{}, fmt.Errorf("client capabilities: %w", err)
	}
	if !containsCapability(capabilities, CapabilityV1) {
		return ClientConfig{}, errors.New("client capabilities must include managed-session.v1")
	}
	config.Capabilities = capabilities
	return config, nil
}

// operationContext supplies a bounded absolute deadline while preserving caller cancellation.
func operationContext(ctx context.Context, timeout time.Duration) (time.Time, context.CancelFunc, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if deadline, ok := ctx.Deadline(); ok {
		if err := ctx.Err(); err != nil {
			return time.Time{}, func() {}, err
		}
		return deadline, func() {}, nil
	}
	bounded, cancel := context.WithTimeout(ctx, timeout)
	deadline, _ := bounded.Deadline()
	return deadline, cancel, bounded.Err()
}

// containsCapability checks one negotiated capability without exposing slice ownership.
func containsCapability(capabilities []Capability, wanted Capability) bool {
	return slices.Contains(capabilities, wanted)
}

// HandshakeError reports Harbor's redaction-safe handshake rejection.
type HandshakeError struct {
	Failure WireError
}

// Error returns the safe handshake failure message.
func (errorValue HandshakeError) Error() string {
	return fmt.Sprintf("managed session handshake rejected: %s", errorValue.Failure.Message)
}

// RemoteError reports one redaction-safe method failure returned by Harbor.
type RemoteError struct {
	Failure WireError
}

// Error returns the safe method failure message.
func (errorValue RemoteError) Error() string {
	return fmt.Sprintf("managed session request failed: %s", errorValue.Failure.Message)
}
