package managedsession

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode"
	"unicode/utf8"
)

// Kind identifies the stable semantic shape carried by one envelope.
type Kind string

const (
	kindHello    Kind = "hello"
	kindWelcome  Kind = "welcome"
	kindReject   Kind = "reject"
	kindRequest  Kind = "request"
	kindResponse Kind = "response"
	kindEvent    Kind = "event"
)

// Hello starts protocol negotiation from the GoForj side.
type Hello struct {
	ProtocolRanges []VersionRange `json:"protocol_ranges"`
	Role           Role           `json:"role"`
	ClientVersion  string         `json:"client_version"`
	Capabilities   []Capability   `json:"capabilities"`
}

// Validate verifies client-controlled handshake input.
func (hello Hello) Validate() error {
	if _, err := CanonicalVersionRanges(hello.ProtocolRanges); err != nil {
		return fmt.Errorf("protocol ranges: %w", err)
	}
	if hello.Role != RoleGoForjSession {
		return fmt.Errorf("unsupported client role %q", hello.Role)
	}
	if err := validateWireToken("client version", hello.ClientVersion, maximumWireTokenBytes); err != nil {
		return err
	}
	if _, err := CanonicalCapabilities(hello.Capabilities); err != nil {
		return err
	}
	return nil
}

// Welcome completes protocol negotiation from Harbor's daemon.
type Welcome struct {
	Protocol       Version        `json:"protocol"`
	ProtocolRanges []VersionRange `json:"protocol_ranges"`
	Role           Role           `json:"role"`
	DaemonVersion  string         `json:"daemon_version"`
	Capabilities   []Capability   `json:"capabilities"`
}

// Validate verifies that a welcome selects a protocol advertised by Harbor.
func (welcome Welcome) Validate() error {
	if err := welcome.Protocol.Validate(); err != nil {
		return fmt.Errorf("selected protocol: %w", err)
	}
	ranges, err := CanonicalVersionRanges(welcome.ProtocolRanges)
	if err != nil {
		return fmt.Errorf("protocol ranges: %w", err)
	}
	selected, err := NegotiateVersion([]VersionRange{{Min: welcome.Protocol, Max: welcome.Protocol}}, ranges)
	if err != nil || selected.Compare(welcome.Protocol) != 0 {
		return errors.New("selected protocol is outside the daemon ranges")
	}
	if welcome.Role != RoleDaemon {
		return errors.New("welcome role must be daemon")
	}
	if err := validateWireToken("daemon version", welcome.DaemonVersion, maximumWireTokenBytes); err != nil {
		return err
	}
	if _, err := CanonicalCapabilities(welcome.Capabilities); err != nil {
		return err
	}
	return nil
}

// Reject terminates a failed handshake with a redaction-safe error.
type Reject struct {
	ProtocolRanges []VersionRange `json:"protocol_ranges,omitempty"`
	Role           Role           `json:"role"`
	DaemonVersion  string         `json:"daemon_version,omitempty"`
	Error          WireError      `json:"error"`
}

// Validate verifies a handshake rejection's safe shape.
func (reject Reject) Validate() error {
	if len(reject.ProtocolRanges) > 0 {
		if _, err := CanonicalVersionRanges(reject.ProtocolRanges); err != nil {
			return fmt.Errorf("protocol ranges: %w", err)
		}
	}
	if reject.Role != RoleDaemon {
		return errors.New("rejection role must be daemon")
	}
	if reject.DaemonVersion != "" {
		if err := validateWireToken("daemon version", reject.DaemonVersion, maximumWireTokenBytes); err != nil {
			return err
		}
	}
	return reject.Error.Validate()
}

// ErrorCode identifies a stable machine-readable remote failure category.
type ErrorCode string

// WireError is Harbor's bounded peer-facing error shape.
type WireError struct {
	Code      ErrorCode `json:"code"`
	Message   string    `json:"message"`
	Retryable bool      `json:"retryable"`
}

// Validate verifies that a remote error is bounded and safe to display.
func (wireError WireError) Validate() error {
	if err := validateWireToken("error code", string(wireError.Code), maximumWireTokenBytes); err != nil {
		return err
	}
	if wireError.Message == "" {
		return errors.New("error message is required")
	}
	if len(wireError.Message) > 256 || !utf8.ValidString(wireError.Message) {
		return errors.New("error message is not bounded UTF-8")
	}
	for _, character := range wireError.Message {
		if unicode.IsControl(character) || unicode.In(character, unicode.Cf, unicode.Zl, unicode.Zp) {
			return errors.New("error message contains a control character")
		}
	}
	return nil
}

// Envelope is the stable outer IPC message.
type envelope struct {
	Kind      Kind            `json:"kind"`
	Protocol  *Version        `json:"protocol,omitempty"`
	RequestID string          `json:"request_id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Deadline  *time.Time      `json:"deadline,omitempty"`
	Name      string          `json:"name,omitempty"`
	Sequence  *uint64         `json:"sequence,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Error     *WireError      `json:"error,omitempty"`
}

// Validate verifies an envelope's discriminator and bounded metadata.
func (message envelope) Validate() error {
	switch message.Kind {
	case kindHello, kindReject:
		if message.Protocol != nil {
			return errors.New("unnegotiated handshake envelope cannot set protocol")
		}
		if err := validatePayload(message.Payload); err != nil {
			return err
		}
		if message.RequestID != "" || message.Method != "" || message.Deadline != nil || message.Error != nil {
			return errors.New("handshake contains fields belonging to another envelope kind")
		}
	case kindWelcome:
		if err := message.validateNegotiated(); err != nil {
			return err
		}
		if err := validatePayload(message.Payload); err != nil {
			return err
		}
		if message.RequestID != "" || message.Method != "" || message.Deadline != nil || message.Error != nil {
			return errors.New("handshake contains fields belonging to another envelope kind")
		}
	case kindRequest:
		if err := message.validateNegotiated(); err != nil {
			return err
		}
		if err := validateWireToken("request ID", message.RequestID, maximumWireTokenBytes); err != nil {
			return err
		}
		if err := validateWireToken("method", message.Method, maximumWireTokenBytes); err != nil {
			return err
		}
		if message.Deadline == nil || message.Deadline.IsZero() {
			return errors.New("request deadline is required")
		}
		if _, offset := message.Deadline.Zone(); offset != 0 {
			return errors.New("request deadline must use UTC")
		}
		if err := validatePayload(message.Payload); err != nil {
			return err
		}
		if message.Error != nil {
			return errors.New("request contains an error")
		}
	case kindResponse:
		if err := message.validateNegotiated(); err != nil {
			return err
		}
		if err := validateWireToken("request ID", message.RequestID, maximumWireTokenBytes); err != nil {
			return err
		}
		if message.Method != "" || message.Deadline != nil {
			return errors.New("response contains fields belonging to another envelope kind")
		}
		hasPayload := len(message.Payload) > 0
		if hasPayload == (message.Error != nil) {
			return errors.New("response must contain exactly one payload or error")
		}
		if hasPayload {
			return validatePayload(message.Payload)
		}
		return message.Error.Validate()
	case kindEvent:
		if err := message.validateNegotiated(); err != nil {
			return err
		}
		if err := validateWireToken("event name", message.Name, maximumWireTokenBytes); err != nil {
			return err
		}
		if message.Sequence == nil || *message.Sequence == 0 || *message.Sequence > MaximumSequence {
			return fmt.Errorf("event sequence must be between 1 and %d", MaximumSequence)
		}
		if err := validatePayload(message.Payload); err != nil {
			return err
		}
		if message.RequestID != "" || message.Method != "" || message.Deadline != nil || message.Error != nil {
			return errors.New("event contains fields belonging to another envelope kind")
		}
	default:
		return fmt.Errorf("unsupported envelope kind %q", message.Kind)
	}
	return nil
}

// validateNegotiated verifies that a post-handshake envelope identifies a protocol.
func (message envelope) validateNegotiated() error {
	if message.Protocol == nil {
		return errors.New("negotiated protocol is required")
	}
	return message.Protocol.Validate()
}

// validatePayload verifies that a frame carries one complete JSON value.
func validatePayload(payload json.RawMessage) error {
	if len(payload) == 0 {
		return errors.New("envelope payload is required")
	}
	if !json.Valid(payload) {
		return errors.New("envelope payload is not valid JSON")
	}
	return nil
}

// newEnvelopePayload serializes one typed payload into an envelope.
func newEnvelopePayload(kind Kind, payload any) (envelope, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return envelope{}, fmt.Errorf("encode envelope payload: %w", err)
	}
	return envelope{Kind: kind, Payload: encoded}, nil
}
