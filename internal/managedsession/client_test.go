package managedsession

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestClientRegisterReplaceAndBarrierOverPipe(t *testing.T) {
	clientConnection, serverConnection := net.Pipe()
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- runTestManagedSessionPeer(serverConnection, func(reader *frameReader, writer *frameWriter, protocol Version) error {
			registerEnvelope, err := readTestEnvelope(reader)
			if err != nil {
				return err
			}
			if registerEnvelope.Kind != kindRequest || registerEnvelope.Method != MethodRegister {
				return errors.New("first request was not managed-session.register")
			}
			registerRequest, err := DecodeRegisterRequest(registerEnvelope.Payload)
			if err != nil {
				return err
			}
			if registerRequest.DescriptorDigest != strings.Repeat("a", 64) {
				return errors.New("register digest retained sha256 prefix")
			}
			registerResponse := RegisterResponse{
				SchemaVersion: SchemaVersion,
				Fence: ManagedPublicationFence{
					ProjectID:         registerRequest.ProjectID,
					SessionID:         registerRequest.SessionID,
					SessionGeneration: registerRequest.ExpectedSessionGeneration + 1,
				},
				AttachmentTicket: "ticket-1",
			}
			if err := writeTestResponse(writer, protocol, registerEnvelope.RequestID, registerResponse); err != nil {
				return err
			}

			replaceEnvelope, err := readTestEnvelope(reader)
			if err != nil {
				return err
			}
			if replaceEnvelope.Method != MethodReplacePublications {
				return errors.New("second request was not managed-session.publications.replace")
			}
			replaceRequest, err := DecodeReplacePublicationsRequest(replaceEnvelope.Payload)
			if err != nil {
				return err
			}
			if len(replaceRequest.Publications) != 1 || replaceRequest.Publications[0].Upstream != netip.MustParseAddrPort("127.0.0.1:43106") {
				return errors.New("replace request publication was not preserved")
			}
			replaceResponse := ReplacePublicationsResponse{
				SchemaVersion:    SchemaVersion,
				Fence:            replaceRequest.Fence,
				Accepted:         true,
				PublicationCount: 1,
			}
			if err := writeTestResponse(writer, protocol, replaceEnvelope.RequestID, replaceResponse); err != nil {
				return err
			}

			barrierEnvelope, err := readTestEnvelope(reader)
			if err != nil {
				return err
			}
			if barrierEnvelope.Method != MethodBarrier {
				return errors.New("third request was not managed-session.barrier")
			}
			barrierRequest, err := DecodeBarrierRequest(barrierEnvelope.Payload)
			if err != nil {
				return err
			}
			if barrierRequest.Phase != BarrierPhaseCompose || barrierRequest.AcceptedProjectIdentity != "orders" {
				return errors.New("barrier request did not preserve compose identity")
			}
			return writeTestResponse(writer, protocol, barrierEnvelope.RequestID, BarrierResponse{
				SchemaVersion: SchemaVersion,
				Fence:         barrierRequest.Fence,
				Phase:         barrierRequest.Phase,
				Acknowledged:  true,
			})
		})
	}()

	client, err := NewClient(context.Background(), clientConnection, ClientConfig{ClientVersion: "0.19.0"})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()
	digest := strings.Repeat("a", 64)
	registerResponse, err := client.Register(context.Background(), RegisterRequest{
		SchemaVersion:             SchemaVersion,
		ProjectID:                 "project-orders",
		SessionID:                 "session-orders",
		ProjectRoot:               "/tmp/orders",
		ExpectedSessionGeneration: 1,
		DescriptorDigest:          "sha256:" + digest,
		ClientNonce:               "nonce-1",
		Owner:                     SessionOwnerHarbor,
		Capabilities:              []Capability{CapabilityV1},
		ActiveApps:                []ActiveApp{{ID: "app", RuntimeIDs: []string{"http"}}},
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if registerResponse.Fence.SessionGeneration != 2 {
		t.Fatalf("Register() fence generation = %d, want 2", registerResponse.Fence.SessionGeneration)
	}

	replaceResponse, err := client.ReplacePublications(context.Background(), ReplacePublicationsRequest{
		SchemaVersion: SchemaVersion,
		Fence:         registerResponse.Fence,
		Publications: []ManagedEndpointPublication{{
			Fence:                 registerResponse.Fence,
			EndpointID:            "service:api",
			ReservationGeneration: 1,
			Upstream:              netip.MustParseAddrPort("127.0.0.1:43106"),
		}},
	})
	if err != nil {
		t.Fatalf("ReplacePublications() error = %v", err)
	}
	if !replaceResponse.Accepted || replaceResponse.PublicationCount != 1 {
		t.Fatalf("ReplacePublications() response = %#v", replaceResponse)
	}

	barrierResponse, err := client.Barrier(context.Background(), BarrierRequest{
		SchemaVersion:           SchemaVersion,
		Fence:                   registerResponse.Fence,
		Phase:                   BarrierPhaseCompose,
		AcceptedProjectIdentity: "orders",
	})
	if err != nil {
		t.Fatalf("Barrier() error = %v", err)
	}
	if !barrierResponse.Acknowledged {
		t.Fatal("Barrier() was not acknowledged")
	}
	client.Close()
	if err := <-serverDone; err != nil {
		t.Fatalf("fake Harbor peer error = %v", err)
	}
}

func TestDialUsesInjectedDialer(t *testing.T) {
	clientConnection, serverConnection := net.Pipe()
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- runTestManagedSessionPeer(serverConnection, func(_ *frameReader, _ *frameWriter, _ Version) error {
			return nil
		})
	}()
	called := false
	client, err := Dial(context.Background(), func(context.Context) (net.Conn, error) {
		called = true
		return clientConnection, nil
	}, ClientConfig{})
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	if !called {
		t.Fatal("Dial() did not call injected dialer")
	}
	client.Close()
	if err := <-serverDone; err != nil {
		t.Fatalf("fake Harbor peer error = %v", err)
	}
}

func TestDecodeRegisterRequestRejectsUnknownDuplicateAndTrailingFields(t *testing.T) {
	request := validRegisterRequest()
	payload, err := MarshalRegisterRequest(request)
	if err != nil {
		t.Fatalf("MarshalRegisterRequest() error = %v", err)
	}
	for _, test := range []struct {
		name    string
		payload string
		wantErr string
	}{
		{name: "unknown", payload: strings.TrimSuffix(string(payload), "}") + `,"future":true}`, wantErr: "unknown field"},
		{name: "duplicate", payload: strings.TrimSuffix(string(payload), "}") + `,"schema_version":1}`, wantErr: "duplicate field"},
		{name: "trailing", payload: string(payload) + ` {}`, wantErr: "trailing"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeRegisterRequest([]byte(test.payload)); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("DecodeRegisterRequest() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestManagedSessionValidationRejectsUnsafePublicationAndDigest(t *testing.T) {
	if _, err := NormalizeDescriptorDigest(strings.Repeat("A", 64)); err == nil {
		t.Fatal("uppercase digest unexpectedly accepted")
	}
	request := validReplaceRequest()
	request.Publications[0].Upstream = netip.MustParseAddrPort("192.168.1.4:43106")
	if err := request.Validate(); err == nil || !strings.Contains(err.Error(), "canonical IPv4 loopback") {
		t.Fatalf("unsafe publication validation error = %v", err)
	}
}

// TestManagedSessionLaunchTicketRequiresNegotiatedCapability keeps ticket use bound to negotiated protocol authority.
func TestManagedSessionLaunchTicketRequiresNegotiatedCapability(t *testing.T) {
	request := validRegisterRequest()
	request.LaunchTicket = strings.Repeat("b", 64)
	if err := request.Validate(); err == nil {
		t.Fatal("launch ticket without capability passed validation")
	}
	request.Capabilities = []Capability{CapabilityLaunchContextV1, CapabilityV1}
	if err := request.Validate(); err != nil {
		t.Fatalf("negotiated launch ticket rejected: %v", err)
	}
	request.LaunchTicket = ""
	if err := request.Validate(); err == nil {
		t.Fatal("negotiated launch capability without ticket passed validation")
	}
	request.LaunchTicket = strings.Repeat(" ", 64)
	if err := request.Validate(); err == nil {
		t.Fatal("whitespace launch ticket passed validation")
	}
}

// TestClientRejectsLaunchTicketWhenTheDaemonDidNotNegotiateIt keeps the secret off the wire for older Harbor peers.
func TestClientRejectsLaunchTicketWhenTheDaemonDidNotNegotiateIt(t *testing.T) {
	clientConnection, serverConnection := net.Pipe()
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- runTestManagedSessionPeer(serverConnection, func(_ *frameReader, _ *frameWriter, _ Version) error {
			return nil
		})
	}()
	client, err := NewClient(context.Background(), clientConnection, ClientConfig{Capabilities: []Capability{CapabilityV1, CapabilityLaunchContextV1}})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	request := validRegisterRequest()
	request.LaunchTicket = strings.Repeat("b", 64)
	if _, err := client.Register(context.Background(), request); err == nil || !strings.Contains(err.Error(), "was not negotiated") {
		t.Fatalf("Register() error = %v, want negotiation rejection", err)
	}
	_ = client.Close()
	if err := <-serverDone; err != nil {
		t.Fatalf("fake Harbor peer error = %v", err)
	}
}

// TestClientRegisterUntilAttachedRetriesOnlyUnavailable proves the startup race does not broaden retries to invalid requests.
func TestClientRegisterUntilAttachedRetriesOnlyUnavailable(t *testing.T) {
	clientConnection, serverConnection := net.Pipe()
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- runTestManagedSessionPeerWithCapabilities(serverConnection, []Capability{CapabilityLaunchContextV1, CapabilityV1}, func(reader *frameReader, writer *frameWriter, protocol Version) error {
			for attempt := 1; ; attempt++ {
				message, err := readTestEnvelope(reader)
				if err != nil {
					return err
				}
				if attempt == 1 {
					failure := WireError{Code: ErrorCodeUnavailable, Message: "Harbor is temporarily unavailable.", Retryable: true}
					if err := writer.writeFrame(mustJSON(envelope{Kind: kindResponse, Protocol: &protocol, RequestID: message.RequestID, Error: &failure})); err != nil {
						return err
					}
					continue
				}
				request, err := DecodeRegisterRequest(message.Payload)
				if err != nil {
					return err
				}
				return writeTestResponse(writer, protocol, message.RequestID, RegisterResponse{
					SchemaVersion:    SchemaVersion,
					Fence:            ManagedPublicationFence{ProjectID: request.ProjectID, SessionID: request.SessionID, SessionGeneration: request.ExpectedSessionGeneration + 1},
					AttachmentTicket: "ticket-1",
				})
			}
		})
	}()
	client, err := NewClient(context.Background(), clientConnection, ClientConfig{Capabilities: []Capability{CapabilityV1, CapabilityLaunchContextV1}})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	request := validRegisterRequest()
	request.Capabilities = []Capability{CapabilityLaunchContextV1, CapabilityV1}
	request.LaunchTicket = strings.Repeat("b", 64)
	if _, err := client.RegisterUntilAttached(context.Background(), request); err != nil {
		t.Fatalf("RegisterUntilAttached() error = %v", err)
	}
	_ = client.Close()
	if err := <-serverDone; err != nil {
		t.Fatalf("fake Harbor peer error = %v", err)
	}
}

func TestClientRejectsRegisterCorrelationMismatch(t *testing.T) {
	clientConnection, serverConnection := net.Pipe()
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- runTestManagedSessionPeer(serverConnection, func(reader *frameReader, writer *frameWriter, protocol Version) error {
			requestEnvelope, err := readTestEnvelope(reader)
			if err != nil {
				return err
			}
			request, err := DecodeRegisterRequest(requestEnvelope.Payload)
			if err != nil {
				return err
			}
			return writeTestResponse(writer, protocol, requestEnvelope.RequestID, RegisterResponse{
				SchemaVersion: SchemaVersion,
				Fence: ManagedPublicationFence{
					ProjectID:         request.ProjectID,
					SessionID:         "session-other",
					SessionGeneration: request.ExpectedSessionGeneration + 1,
				},
				AttachmentTicket: "ticket-1",
			})
		})
	}()
	client, err := NewClient(context.Background(), clientConnection, ClientConfig{})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if _, err := client.Register(context.Background(), validRegisterRequest()); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("Register() error = %v, want correlation mismatch", err)
	}
	client.Close()
	if err := <-serverDone; err != nil {
		t.Fatalf("fake Harbor peer error = %v", err)
	}
}

func TestClientReturnsRemoteError(t *testing.T) {
	clientConnection, serverConnection := net.Pipe()
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- runTestManagedSessionPeer(serverConnection, func(reader *frameReader, writer *frameWriter, protocol Version) error {
			requestEnvelope, err := readTestEnvelope(reader)
			if err != nil {
				return err
			}
			message := WireError{Code: ErrorCode("unavailable"), Message: "Harbor is temporarily unavailable.", Retryable: true}
			if err := message.Validate(); err != nil {
				return err
			}
			response := envelope{Kind: kindResponse, Protocol: &protocol, RequestID: requestEnvelope.RequestID, Error: &message}
			return writer.writeFrame(mustJSON(response))
		})
	}()
	client, err := NewClient(context.Background(), clientConnection, ClientConfig{})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	_, err = client.Register(context.Background(), validRegisterRequest())
	var remoteError RemoteError
	if !errors.As(err, &remoteError) || !remoteError.Failure.Retryable {
		t.Fatalf("Register() error = %v, want retryable RemoteError", err)
	}
	client.Close()
	if err := <-serverDone; err != nil {
		t.Fatalf("fake Harbor peer error = %v", err)
	}
}

func TestClientCallHonorsContextDeadline(t *testing.T) {
	clientConnection, serverConnection := net.Pipe()
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- runTestManagedSessionPeer(serverConnection, func(reader *frameReader, _ *frameWriter, _ Version) error {
			if _, err := readTestEnvelope(reader); err != nil {
				return err
			}
			time.Sleep(100 * time.Millisecond)
			return nil
		})
	}()
	client, err := NewClient(context.Background(), clientConnection, ClientConfig{})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := client.Register(ctx, validRegisterRequest()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Register() error = %v, want context deadline exceeded", err)
	}
	client.Close()
	if err := <-serverDone; err != nil {
		t.Fatalf("fake Harbor peer error = %v", err)
	}
}

func TestClientRejectsWelcomeProtocolMismatch(t *testing.T) {
	clientConnection, serverConnection := net.Pipe()
	serverDone := make(chan error, 1)
	go func() {
		reader := &frameReader{reader: serverConnection, limit: MaximumFrameSize}
		writer := &frameWriter{writer: serverConnection, limit: MaximumFrameSize}
		defer serverConnection.Close()
		if _, err := readTestEnvelope(reader); err != nil {
			serverDone <- err
			return
		}
		payload, err := newEnvelopePayload(kindWelcome, Welcome{
			Protocol:       Version{Major: 1},
			ProtocolRanges: []VersionRange{{Min: Version{Major: 1}, Max: Version{Major: 1}}},
			Role:           RoleDaemon,
			DaemonVersion:  "0.19.0",
			Capabilities:   []Capability{CapabilityV1},
		})
		if err != nil {
			serverDone <- err
			return
		}
		payload.Protocol = &Version{Major: 1, Minor: 1}
		serverDone <- writer.writeFrame(mustJSON(payload))
	}()
	if _, err := NewClient(context.Background(), clientConnection, ClientConfig{}); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("NewClient() error = %v, want welcome protocol mismatch", err)
	}
	if err := <-serverDone; err != nil {
		t.Fatalf("fake Harbor peer error = %v", err)
	}
}

// validRegisterRequest returns the canonical registration fixture used by client tests.
func validRegisterRequest() RegisterRequest {
	return RegisterRequest{
		SchemaVersion:             SchemaVersion,
		ProjectID:                 "project-orders",
		SessionID:                 "session-orders",
		ProjectRoot:               "/tmp/orders",
		ExpectedSessionGeneration: 1,
		DescriptorDigest:          strings.Repeat("a", 64),
		ClientNonce:               "nonce-1",
		Owner:                     SessionOwnerHarbor,
		Capabilities:              []Capability{CapabilityV1},
		ActiveApps:                []ActiveApp{{ID: "app", RuntimeIDs: []string{"http"}}},
	}
}

// validReplaceRequest returns one canonical publication replacement fixture.
func validReplaceRequest() ReplacePublicationsRequest {
	fence := ManagedPublicationFence{ProjectID: "project-orders", SessionID: "session-orders", SessionGeneration: 2}
	return ReplacePublicationsRequest{
		SchemaVersion: SchemaVersion,
		Fence:         fence,
		Publications: []ManagedEndpointPublication{{
			Fence:                 fence,
			EndpointID:            "service:api",
			ReservationGeneration: 1,
			Upstream:              netip.MustParseAddrPort("127.0.0.1:43106"),
		}},
	}
}

// runTestManagedSessionPeer performs the minimal Harbor handshake for net.Pipe tests.
func runTestManagedSessionPeer(connection net.Conn, handle func(*frameReader, *frameWriter, Version) error) error {
	return runTestManagedSessionPeerWithCapabilities(connection, []Capability{CapabilityV1}, handle)
}

// runTestManagedSessionPeerWithCapabilities performs the minimal Harbor handshake with an explicit capability set.
func runTestManagedSessionPeerWithCapabilities(connection net.Conn, capabilities []Capability, handle func(*frameReader, *frameWriter, Version) error) error {
	reader := &frameReader{reader: connection, limit: MaximumFrameSize}
	writer := &frameWriter{writer: connection, limit: MaximumFrameSize}
	defer connection.Close()
	helloEnvelope, err := readTestEnvelope(reader)
	if err != nil {
		return err
	}
	if helloEnvelope.Kind != kindHello {
		return errors.New("client did not send hello")
	}
	var hello Hello
	if err := json.Unmarshal(helloEnvelope.Payload, &hello); err != nil {
		return err
	}
	if err := hello.Validate(); err != nil {
		return err
	}
	protocol := Version{Major: 1}
	welcome, err := newEnvelopePayload(kindWelcome, Welcome{
		Protocol:       protocol,
		ProtocolRanges: []VersionRange{{Min: protocol, Max: protocol}},
		Role:           RoleDaemon,
		DaemonVersion:  "0.19.0",
		Capabilities:   capabilities,
	})
	if err != nil {
		return err
	}
	welcome.Protocol = &protocol
	if err := writer.writeFrame(mustJSON(welcome)); err != nil {
		return err
	}
	return handle(reader, writer, protocol)
}

// readTestEnvelope reads and validates one fake-peer envelope.
func readTestEnvelope(reader *frameReader) (envelope, error) {
	payload, err := reader.readFrame()
	if err != nil {
		return envelope{}, err
	}
	var message envelope
	if err := json.Unmarshal(payload, &message); err != nil {
		return envelope{}, err
	}
	if err := message.Validate(); err != nil {
		return envelope{}, err
	}
	return message, nil
}

// writeTestResponse writes one correlated successful fake-peer response.
func writeTestResponse(writer *frameWriter, protocol Version, requestID string, payload any) error {
	message, err := newEnvelopePayload(kindResponse, payload)
	if err != nil {
		return err
	}
	message.Protocol = &protocol
	message.RequestID = requestID
	return writer.writeFrame(mustJSON(message))
}

// mustJSON encodes a test envelope and fails immediately on an impossible fixture error.
func mustJSON(value any) []byte {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return payload
}

func TestClientOperationContextUsesCallerDeadline(t *testing.T) {
	deadline := time.Now().Add(time.Second)
	contextWithDeadline, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	got, _, err := operationContext(contextWithDeadline, time.Minute)
	if err != nil {
		t.Fatalf("operationContext() error = %v", err)
	}
	if !got.Equal(deadline) {
		t.Fatalf("operationContext() deadline = %v, want %v", got, deadline)
	}
}
