package managedsession

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// SchemaVersion identifies the first managed-session message schema.
	SchemaVersion uint16 = 1
	// CapabilityV1 identifies the first bounded managed-session feature set.
	CapabilityV1 Capability = "managed-session.v1"
	// MethodRegister attaches one authenticated GoForj process to a Harbor session.
	MethodRegister = "managed-session.v1.register"
	// MethodReplacePublications replaces every private publication observed by a session.
	MethodReplacePublications = "managed-session.v1.publications.replace"
	// MethodBarrier asks Harbor whether a named lifecycle barrier has been acknowledged.
	MethodBarrier = "managed-session.v1.barrier"
	// MaximumFrameSize bounds each length-prefixed JSON frame.
	MaximumFrameSize uint32 = 1 << 20
	// MaximumSequence is the largest generation representable exactly by JSON number clients.
	MaximumSequence uint64 = 1<<53 - 1
)

const (
	maximumManagedSessionApps         = 256
	maximumManagedSessionRuntimes     = 256
	maximumManagedSessionCapabilities = 128
	maximumManagedSessionTokenBytes   = 512
	maximumManagedSessionRootBytes    = 4096
	maximumManagedPublications        = 256
	maximumIdentifierBytes            = 256
	maximumEndpointIDBytes            = 128
	maximumWireTokenBytes             = 128
)

// Capability is an independently negotiated managed-session feature name.
type Capability string

// Role identifies a peer's authorization boundary on the Harbor IPC connection.
type Role string

const (
	// RoleDaemon identifies Harbor's daemon.
	RoleDaemon Role = "daemon"
	// RoleGoForjSession identifies this managed GoForj process.
	RoleGoForjSession Role = "goforj_session"
)

// ProjectID identifies one registered Harbor project.
type ProjectID string

// SessionID identifies one durable project lifecycle.
type SessionID string

// SessionOwner identifies the user surface retaining lifecycle authority.
type SessionOwner string

const (
	// SessionOwnerHarbor means Harbor launched and supervises GoForj.
	SessionOwnerHarbor SessionOwner = "harbor"
	// SessionOwnerTerminal means the foreground terminal owns GoForj.
	SessionOwnerTerminal SessionOwner = "terminal"
)

// Validate verifies that a session owner is recognized by the v1 contract.
func (owner SessionOwner) Validate() error {
	switch owner {
	case SessionOwnerHarbor, SessionOwnerTerminal:
		return nil
	default:
		return fmt.Errorf("unknown session owner %q", owner)
	}
}

// Version identifies one compatible Harbor IPC protocol revision.
type Version struct {
	Major uint16 `json:"major"`
	Minor uint16 `json:"minor"`
}

// Compare orders one protocol version relative to another.
func (version Version) Compare(other Version) int {
	if version.Major != other.Major {
		if version.Major < other.Major {
			return -1
		}
		return 1
	}
	if version.Minor < other.Minor {
		return -1
	}
	if version.Minor > other.Minor {
		return 1
	}
	return 0
}

// Validate verifies that a protocol version is negotiated rather than zero.
func (version Version) Validate() error {
	if version.Major == 0 {
		return errors.New("protocol major must be greater than zero")
	}
	return nil
}

// VersionRange declares a contiguous set of minor protocol revisions.
type VersionRange struct {
	Min Version `json:"min"`
	Max Version `json:"max"`
}

// Validate verifies that a protocol range is ordered and stays within one major.
func (versionRange VersionRange) Validate() error {
	if err := versionRange.Min.Validate(); err != nil {
		return fmt.Errorf("minimum version: %w", err)
	}
	if err := versionRange.Max.Validate(); err != nil {
		return fmt.Errorf("maximum version: %w", err)
	}
	if versionRange.Min.Major != versionRange.Max.Major {
		return errors.New("a protocol range cannot span major versions")
	}
	if versionRange.Min.Compare(versionRange.Max) > 0 {
		return errors.New("minimum protocol version exceeds maximum")
	}
	return nil
}

// CanonicalVersionRanges validates, sorts, and merges protocol ranges.
func CanonicalVersionRanges(ranges []VersionRange) ([]VersionRange, error) {
	if len(ranges) == 0 {
		return nil, errors.New("at least one protocol range is required")
	}
	canonical := append([]VersionRange(nil), ranges...)
	for index, candidate := range canonical {
		if err := candidate.Validate(); err != nil {
			return nil, fmt.Errorf("protocol range %d: %w", index, err)
		}
	}
	slices.SortFunc(canonical, func(left, right VersionRange) int {
		comparison := left.Min.Compare(right.Min)
		if comparison != 0 {
			return comparison
		}
		return left.Max.Compare(right.Max)
	})
	merged := make([]VersionRange, 0, len(canonical))
	for _, candidate := range canonical {
		if len(merged) == 0 {
			merged = append(merged, candidate)
			continue
		}
		last := &merged[len(merged)-1]
		if last.Max.Major == candidate.Min.Major && uint32(candidate.Min.Minor) <= uint32(last.Max.Minor)+1 {
			if candidate.Max.Compare(last.Max) > 0 {
				last.Max = candidate.Max
			}
			continue
		}
		merged = append(merged, candidate)
	}
	return merged, nil
}

// NegotiateVersion selects the highest protocol version supported by both peers.
func NegotiateVersion(clientRanges, serverRanges []VersionRange) (Version, error) {
	client, err := CanonicalVersionRanges(clientRanges)
	if err != nil {
		return Version{}, fmt.Errorf("client protocol ranges: %w", err)
	}
	server, err := CanonicalVersionRanges(serverRanges)
	if err != nil {
		return Version{}, fmt.Errorf("server protocol ranges: %w", err)
	}
	var selected Version
	found := false
	for _, clientRange := range client {
		for _, serverRange := range server {
			if clientRange.Min.Major != serverRange.Min.Major {
				continue
			}
			lower := clientRange.Min
			if serverRange.Min.Compare(lower) > 0 {
				lower = serverRange.Min
			}
			upper := clientRange.Max
			if serverRange.Max.Compare(upper) < 0 {
				upper = serverRange.Max
			}
			if lower.Compare(upper) > 0 || found && upper.Compare(selected) <= 0 {
				continue
			}
			selected = upper
			found = true
		}
	}
	if !found {
		return Version{}, errors.New("no compatible protocol version")
	}
	return selected, nil
}

// CanonicalCapabilities validates, de-duplicates, and sorts capability names.
func CanonicalCapabilities(capabilities []Capability) ([]Capability, error) {
	unique := make(map[Capability]struct{}, len(capabilities))
	for index, capability := range capabilities {
		if err := validateWireToken(fmt.Sprintf("capability %d", index), string(capability), maximumWireTokenBytes); err != nil {
			return nil, err
		}
		unique[capability] = struct{}{}
	}
	canonical := make([]Capability, 0, len(unique))
	for capability := range unique {
		canonical = append(canonical, capability)
	}
	slices.Sort(canonical)
	return canonical, nil
}

// ActiveApp identifies one App and the runtime IDs selected for the session.
type ActiveApp struct {
	ID         string   `json:"id"`
	RuntimeIDs []string `json:"runtime_ids"`
}

// Validate verifies that an App/runtime set is deterministic and bounded.
func (app ActiveApp) Validate() error {
	if err := validateIdentifier("managed session App ID", app.ID); err != nil {
		return err
	}
	if app.RuntimeIDs == nil {
		return fmt.Errorf("managed session App %q runtime IDs must be initialized", app.ID)
	}
	if len(app.RuntimeIDs) > maximumManagedSessionRuntimes {
		return fmt.Errorf("managed session App %q contains more than %d runtimes", app.ID, maximumManagedSessionRuntimes)
	}
	for index, runtimeID := range app.RuntimeIDs {
		if err := validateWireToken(fmt.Sprintf("managed session App %q runtime %d", app.ID, index+1), runtimeID, maximumManagedSessionTokenBytes); err != nil {
			return err
		}
		if index > 0 && app.RuntimeIDs[index-1] >= runtimeID {
			return fmt.Errorf("managed session App %q runtime IDs must be sorted and unique", app.ID)
		}
	}
	return nil
}

// RegisterRequest carries the non-secret identity and proposed runtime set for one attachment.
type RegisterRequest struct {
	SchemaVersion             uint16       `json:"schema_version"`
	ProjectID                 ProjectID    `json:"project_id"`
	SessionID                 SessionID    `json:"session_id"`
	ProjectRoot               string       `json:"project_root"`
	ExpectedSessionGeneration uint64       `json:"expected_session_generation"`
	DescriptorDigest          string       `json:"descriptor_digest"`
	ClientNonce               string       `json:"client_nonce"`
	Owner                     SessionOwner `json:"owner"`
	Capabilities              []Capability `json:"capabilities"`
	ActiveApps                []ActiveApp  `json:"active_apps"`
}

// Validate verifies a registration against the exact Harbor v1 identity shape.
func (request RegisterRequest) Validate() error {
	if request.SchemaVersion != SchemaVersion {
		return fmt.Errorf("managed session registration schema version %d is unsupported", request.SchemaVersion)
	}
	if err := validateIdentifier("project ID", string(request.ProjectID)); err != nil {
		return err
	}
	if err := validateIdentifier("session ID", string(request.SessionID)); err != nil {
		return err
	}
	if err := validateManagedSessionRoot(request.ProjectRoot); err != nil {
		return err
	}
	if request.ExpectedSessionGeneration == 0 || request.ExpectedSessionGeneration >= MaximumSequence {
		return fmt.Errorf("managed session expected generation must be between 1 and %d", MaximumSequence-1)
	}
	if err := validateManagedSessionDigest(request.DescriptorDigest); err != nil {
		return err
	}
	if err := validateWireToken("managed session client nonce", request.ClientNonce, maximumManagedSessionTokenBytes); err != nil {
		return err
	}
	if err := request.Owner.Validate(); err != nil {
		return err
	}
	if request.Capabilities == nil {
		return errors.New("managed session capabilities must be initialized")
	}
	if len(request.Capabilities) > maximumManagedSessionCapabilities {
		return fmt.Errorf("managed session contains more than %d capabilities", maximumManagedSessionCapabilities)
	}
	canonical, err := CanonicalCapabilities(request.Capabilities)
	if err != nil {
		return err
	}
	if !slices.Equal(canonical, request.Capabilities) {
		return errors.New("managed session capabilities must be sorted and unique")
	}
	if request.ActiveApps == nil {
		return errors.New("managed session active Apps must be initialized")
	}
	if len(request.ActiveApps) > maximumManagedSessionApps {
		return fmt.Errorf("managed session contains more than %d Apps", maximumManagedSessionApps)
	}
	for index, app := range request.ActiveApps {
		if err := app.Validate(); err != nil {
			return fmt.Errorf("managed session active App %d: %w", index+1, err)
		}
		if index > 0 && request.ActiveApps[index-1].ID >= app.ID {
			return errors.New("managed session active Apps must be sorted and unique")
		}
	}
	return nil
}

// NormalizeDescriptorDigest converts GoForj descriptor output into Harbor's wire form.
func NormalizeDescriptorDigest(digest string) (string, error) {
	digest = strings.TrimPrefix(digest, "sha256:")
	if err := validateManagedSessionDigest(digest); err != nil {
		return "", err
	}
	return digest, nil
}

// NormalizeRegisterRequest copies a request and removes the descriptor's exact sha256 prefix.
func NormalizeRegisterRequest(request RegisterRequest) (RegisterRequest, error) {
	normalized := request
	digest, err := NormalizeDescriptorDigest(request.DescriptorDigest)
	if err != nil {
		return RegisterRequest{}, err
	}
	normalized.DescriptorDigest = digest
	return normalized, nil
}

// RegisterResponse returns the attached-session fence and one short-lived credential.
type RegisterResponse struct {
	SchemaVersion    uint16                  `json:"schema_version"`
	Fence            ManagedPublicationFence `json:"fence"`
	AttachmentTicket string                  `json:"attachment_ticket"`
}

// Validate verifies a registration response contains bounded ephemeral authority.
func (response RegisterResponse) Validate() error {
	if response.SchemaVersion != SchemaVersion {
		return fmt.Errorf("managed session registration response schema version %d is unsupported", response.SchemaVersion)
	}
	if err := response.Fence.Validate(); err != nil {
		return err
	}
	return validateWireToken("managed session attachment ticket", response.AttachmentTicket, maximumManagedSessionTokenBytes)
}

// ValidateRegisterCorrelation binds a response to the requested project, session, and next generation.
func ValidateRegisterCorrelation(request RegisterRequest, response RegisterResponse) error {
	if err := request.Validate(); err != nil {
		return fmt.Errorf("validate managed session registration request: %w", err)
	}
	if err := response.Validate(); err != nil {
		return fmt.Errorf("validate managed session registration response: %w", err)
	}
	if response.Fence.ProjectID != request.ProjectID || response.Fence.SessionID != request.SessionID {
		return errors.New("managed session registration response does not match the requested project and session")
	}
	if request.ExpectedSessionGeneration == MaximumSequence || response.Fence.SessionGeneration != request.ExpectedSessionGeneration+1 {
		return errors.New("managed session registration response does not match the requested next generation")
	}
	return nil
}

// ManagedPublicationFence binds one publication to the authorized project session.
type ManagedPublicationFence struct {
	ProjectID         ProjectID `json:"project_id"`
	SessionID         SessionID `json:"session_id"`
	SessionGeneration uint64    `json:"session_generation"`
}

// Validate verifies that a publication fence contains complete session identity.
func (fence ManagedPublicationFence) Validate() error {
	if err := validateIdentifier("managed publication project fence", string(fence.ProjectID)); err != nil {
		return err
	}
	if err := validateIdentifier("managed publication session fence", string(fence.SessionID)); err != nil {
		return err
	}
	if fence.SessionGeneration == 0 {
		return fmt.Errorf("managed publication session generation must be positive")
	}
	return nil
}

// ManagedEndpointPublication is one private host publication observed for a session endpoint.
type ManagedEndpointPublication struct {
	Fence                 ManagedPublicationFence `json:"fence"`
	EndpointID            string                  `json:"endpoint_id"`
	ReservationGeneration uint64                  `json:"reservation_generation"`
	Upstream              netip.AddrPort          `json:"upstream"`
}

// Validate verifies that a publication contains a bounded loopback high-port upstream.
func (publication ManagedEndpointPublication) Validate() error {
	if err := publication.Fence.Validate(); err != nil {
		return err
	}
	if err := validateEndpointID(publication.EndpointID); err != nil {
		return err
	}
	if publication.ReservationGeneration == 0 {
		return fmt.Errorf("managed publication endpoint %q reservation generation must be positive", publication.EndpointID)
	}
	if !publication.Upstream.IsValid() || publication.Upstream.Port() < 1024 {
		return fmt.Errorf("managed publication endpoint %q upstream %s must use a high port", publication.EndpointID, publication.Upstream)
	}
	address := publication.Upstream.Addr()
	if !address.Is4() || !address.IsLoopback() || address != address.Unmap() {
		return fmt.Errorf("managed publication endpoint %q upstream %s must use canonical IPv4 loopback", publication.EndpointID, publication.Upstream)
	}
	return nil
}

// ReplacePublicationsRequest carries a complete replacement set for one attached session.
type ReplacePublicationsRequest struct {
	SchemaVersion uint16                       `json:"schema_version"`
	Fence         ManagedPublicationFence      `json:"fence"`
	Publications  []ManagedEndpointPublication `json:"publications"`
}

// Validate verifies every publication is bounded by the exact attached-session fence.
func (request ReplacePublicationsRequest) Validate() error {
	if request.SchemaVersion != SchemaVersion {
		return fmt.Errorf("managed session publication schema version %d is unsupported", request.SchemaVersion)
	}
	if err := request.Fence.Validate(); err != nil {
		return err
	}
	if request.Publications == nil {
		return errors.New("managed session publications must be initialized")
	}
	if len(request.Publications) > maximumManagedPublications {
		return fmt.Errorf("managed session contains more than %d publications", maximumManagedPublications)
	}
	seen := make(map[string]struct{}, len(request.Publications))
	for index, publication := range request.Publications {
		if err := publication.Validate(); err != nil {
			return fmt.Errorf("managed session publication %d: %w", index+1, err)
		}
		if publication.Fence != request.Fence {
			return fmt.Errorf("managed session publication %q does not match the request fence", publication.EndpointID)
		}
		if _, duplicate := seen[publication.EndpointID]; duplicate {
			return fmt.Errorf("managed session publication endpoint %q is duplicated", publication.EndpointID)
		}
		seen[publication.EndpointID] = struct{}{}
	}
	return nil
}

// ReplacePublicationsResponse acknowledges one complete publication replacement.
type ReplacePublicationsResponse struct {
	SchemaVersion    uint16                  `json:"schema_version"`
	Fence            ManagedPublicationFence `json:"fence"`
	Accepted         bool                    `json:"accepted"`
	PublicationCount uint16                  `json:"publication_count"`
}

// Validate verifies a publication acknowledgement is tied to one session fence.
func (response ReplacePublicationsResponse) Validate() error {
	if response.SchemaVersion != SchemaVersion {
		return fmt.Errorf("managed session publication response schema version %d is unsupported", response.SchemaVersion)
	}
	if err := response.Fence.Validate(); err != nil {
		return err
	}
	if response.PublicationCount > maximumManagedPublications {
		return fmt.Errorf("managed session publication count exceeds %d", maximumManagedPublications)
	}
	if !response.Accepted && response.PublicationCount != 0 {
		return errors.New("rejected managed session publication acknowledgement must not report publications")
	}
	return nil
}

// ValidateReplacePublicationsCorrelation binds an acknowledgement to one replacement request.
func ValidateReplacePublicationsCorrelation(request ReplacePublicationsRequest, response ReplacePublicationsResponse) error {
	if err := request.Validate(); err != nil {
		return fmt.Errorf("validate managed session publication request: %w", err)
	}
	if err := response.Validate(); err != nil {
		return fmt.Errorf("validate managed session publication response: %w", err)
	}
	if response.Fence != request.Fence {
		return errors.New("managed session publication response does not match the request fence")
	}
	if response.Accepted && int(response.PublicationCount) != len(request.Publications) {
		return errors.New("managed session publication response count does not match the replacement")
	}
	return nil
}

// BarrierPhase identifies a lifecycle synchronization point in v1.
type BarrierPhase string

const (
	// BarrierPhaseCompose is the point after Compose starts and before setup or migrations continue.
	BarrierPhaseCompose BarrierPhase = "compose"
)

// Validate verifies that a barrier phase is known to this protocol version.
func (phase BarrierPhase) Validate() error {
	if phase != BarrierPhaseCompose {
		return fmt.Errorf("unsupported managed session barrier phase %q", phase)
	}
	return nil
}

// BarrierRequest asks Harbor to acknowledge a lifecycle barrier.
type BarrierRequest struct {
	SchemaVersion           uint16                  `json:"schema_version"`
	Fence                   ManagedPublicationFence `json:"fence"`
	Phase                   BarrierPhase            `json:"phase"`
	AcceptedProjectIdentity string                  `json:"accepted_project_identity"`
}

// Validate verifies a barrier request contains a bounded Compose identity and exact fence.
func (request BarrierRequest) Validate() error {
	if request.SchemaVersion != SchemaVersion {
		return fmt.Errorf("managed session barrier schema version %d is unsupported", request.SchemaVersion)
	}
	if err := request.Fence.Validate(); err != nil {
		return err
	}
	if err := request.Phase.Validate(); err != nil {
		return err
	}
	return validateWireToken("managed session accepted project identity", request.AcceptedProjectIdentity, maximumManagedSessionTokenBytes)
}

// BarrierResponse acknowledges whether Harbor has completed the requested barrier.
type BarrierResponse struct {
	SchemaVersion uint16                  `json:"schema_version"`
	Fence         ManagedPublicationFence `json:"fence"`
	Phase         BarrierPhase            `json:"phase"`
	Acknowledged  bool                    `json:"acknowledged"`
}

// Validate verifies that a barrier response contains a known phase and fence.
func (response BarrierResponse) Validate() error {
	if response.SchemaVersion != SchemaVersion {
		return fmt.Errorf("managed session barrier response schema version %d is unsupported", response.SchemaVersion)
	}
	if err := response.Fence.Validate(); err != nil {
		return err
	}
	return response.Phase.Validate()
}

// ValidateBarrierCorrelation binds a barrier acknowledgement to one exact request.
func ValidateBarrierCorrelation(request BarrierRequest, response BarrierResponse) error {
	if err := request.Validate(); err != nil {
		return fmt.Errorf("validate managed session barrier request: %w", err)
	}
	if err := response.Validate(); err != nil {
		return fmt.Errorf("validate managed session barrier response: %w", err)
	}
	if response.Fence != request.Fence || response.Phase != request.Phase {
		return errors.New("managed session barrier response does not match the request")
	}
	return nil
}

// MarshalRegisterRequest validates and encodes one registration object.
func MarshalRegisterRequest(request RegisterRequest) ([]byte, error) {
	return marshalManagedObject("managed session registration", request, request.Validate)
}

// DecodeRegisterRequest strictly decodes and validates one registration object.
func DecodeRegisterRequest(payload []byte) (RegisterRequest, error) {
	var request RegisterRequest
	if err := decodeManagedObject(payload, "managed session registration", &request); err != nil {
		return RegisterRequest{}, err
	}
	if err := request.Validate(); err != nil {
		return RegisterRequest{}, err
	}
	return request, nil
}

// MarshalRegisterResponse validates and encodes one registration response.
func MarshalRegisterResponse(response RegisterResponse) ([]byte, error) {
	return marshalManagedObject("managed session registration response", response, response.Validate)
}

// DecodeRegisterResponse strictly decodes and validates one registration response.
func DecodeRegisterResponse(payload []byte) (RegisterResponse, error) {
	var response RegisterResponse
	if err := decodeManagedObject(payload, "managed session registration response", &response); err != nil {
		return RegisterResponse{}, err
	}
	if err := response.Validate(); err != nil {
		return RegisterResponse{}, err
	}
	return response, nil
}

// MarshalReplacePublicationsRequest validates and encodes one complete replacement.
func MarshalReplacePublicationsRequest(request ReplacePublicationsRequest) ([]byte, error) {
	return marshalManagedObject("managed session publication replacement", request, request.Validate)
}

// DecodeReplacePublicationsRequest strictly decodes and validates one replacement.
func DecodeReplacePublicationsRequest(payload []byte) (ReplacePublicationsRequest, error) {
	var request ReplacePublicationsRequest
	if err := decodeManagedObject(payload, "managed session publication replacement", &request); err != nil {
		return ReplacePublicationsRequest{}, err
	}
	if err := request.Validate(); err != nil {
		return ReplacePublicationsRequest{}, err
	}
	return request, nil
}

// MarshalReplacePublicationsResponse validates and encodes one replacement acknowledgement.
func MarshalReplacePublicationsResponse(response ReplacePublicationsResponse) ([]byte, error) {
	return marshalManagedObject("managed session publication response", response, response.Validate)
}

// DecodeReplacePublicationsResponse strictly decodes and validates one replacement acknowledgement.
func DecodeReplacePublicationsResponse(payload []byte) (ReplacePublicationsResponse, error) {
	var response ReplacePublicationsResponse
	if err := decodeManagedObject(payload, "managed session publication response", &response); err != nil {
		return ReplacePublicationsResponse{}, err
	}
	if err := response.Validate(); err != nil {
		return ReplacePublicationsResponse{}, err
	}
	return response, nil
}

// MarshalBarrierRequest validates and encodes one lifecycle barrier request.
func MarshalBarrierRequest(request BarrierRequest) ([]byte, error) {
	return marshalManagedObject("managed session barrier", request, request.Validate)
}

// DecodeBarrierRequest strictly decodes and validates one lifecycle barrier request.
func DecodeBarrierRequest(payload []byte) (BarrierRequest, error) {
	var request BarrierRequest
	if err := decodeManagedObject(payload, "managed session barrier", &request); err != nil {
		return BarrierRequest{}, err
	}
	if err := request.Validate(); err != nil {
		return BarrierRequest{}, err
	}
	return request, nil
}

// MarshalBarrierResponse validates and encodes one lifecycle barrier response.
func MarshalBarrierResponse(response BarrierResponse) ([]byte, error) {
	return marshalManagedObject("managed session barrier response", response, response.Validate)
}

// DecodeBarrierResponse strictly decodes and validates one lifecycle barrier response.
func DecodeBarrierResponse(payload []byte) (BarrierResponse, error) {
	var response BarrierResponse
	if err := decodeManagedObject(payload, "managed session barrier response", &response); err != nil {
		return BarrierResponse{}, err
	}
	if err := response.Validate(); err != nil {
		return BarrierResponse{}, err
	}
	return response, nil
}

// validateIdentifier keeps project and session IDs bounded without assigning path semantics.
func validateIdentifier(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s must not be empty", name)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s must be valid UTF-8", name)
	}
	if len(value) > maximumIdentifierBytes {
		return fmt.Errorf("%s must not exceed %d bytes", name, maximumIdentifierBytes)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must not contain surrounding whitespace", name)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf("%s must not contain control characters", name)
		}
	}
	return nil
}

// validateManagedSessionRoot keeps path authority canonical without touching filesystem state.
func validateManagedSessionRoot(root string) error {
	if root == "" {
		return errors.New("managed session project root is required")
	}
	if !utf8.ValidString(root) || len(root) > maximumManagedSessionRootBytes {
		return fmt.Errorf("managed session project root must be valid UTF-8 of at most %d bytes", maximumManagedSessionRootBytes)
	}
	for _, character := range root {
		if unicode.IsControl(character) {
			return errors.New("managed session project root must not contain control characters")
		}
	}
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return errors.New("managed session project root must be a canonical absolute path")
	}
	return nil
}

// validateManagedSessionDigest accepts Harbor's bare lowercase SHA-256 form.
func validateManagedSessionDigest(digest string) error {
	if len(digest) != 64 {
		return errors.New("managed session descriptor digest must be 64 lowercase hexadecimal characters")
	}
	if _, err := hex.DecodeString(digest); err != nil || strings.ToLower(digest) != digest {
		return errors.New("managed session descriptor digest must be 64 lowercase hexadecimal characters")
	}
	return nil
}

// validateWireToken rejects whitespace and control text before it reaches logs or authority decisions.
func validateWireToken(name, value string, maximum int) error {
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if !utf8.ValidString(value) || len(value) > maximum {
		return fmt.Errorf("%s must be valid UTF-8 of at most %d bytes", name, maximum)
	}
	for _, character := range value {
		if character > unicode.MaxASCII || !isWireTokenCharacter(byte(character)) {
			return fmt.Errorf("%s contains an unsupported character", name)
		}
	}
	return nil
}

// isWireTokenCharacter defines the portable ASCII vocabulary shared by Harbor's handshake fields.
func isWireTokenCharacter(character byte) bool {
	if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' {
		return true
	}
	switch character {
	case '.', '_', '-', ':', '+':
		return true
	default:
		return false
	}
}

// validateEndpointID mirrors Harbor's bounded network reservation identity grammar.
func validateEndpointID(endpointID string) error {
	if endpointID == "" {
		return errors.New("managed publication endpoint ID is required")
	}
	if len(endpointID) > maximumEndpointIDBytes {
		return fmt.Errorf("managed publication endpoint ID %q exceeds %d bytes", endpointID, maximumEndpointIDBytes)
	}
	if strings.TrimSpace(endpointID) != endpointID {
		return fmt.Errorf("managed publication endpoint ID %q must not contain surrounding whitespace", endpointID)
	}
	for _, character := range endpointID {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '.' || character == '_' || character == ':' || character == '-' {
			continue
		}
		return fmt.Errorf("managed publication endpoint ID %q contains an unsupported character", endpointID)
	}
	return nil
}

// decodeManagedObject rejects duplicate, unknown, trailing, or oversized JSON fields.
func decodeManagedObject(payload []byte, label string, target any) error {
	if len(payload) == 0 || len(payload) > int(MaximumFrameSize) {
		return fmt.Errorf("%s exceeds its bounded object shape", label)
	}
	if err := rejectDuplicateJSON(payload, label); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", label, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("decode %s: trailing JSON value", label)
		}
		return fmt.Errorf("decode %s trailing data: %w", label, err)
	}
	return nil
}

// marshalManagedObject validates a typed payload before writing it to the wire.
func marshalManagedObject(label string, value any, validate func() error) ([]byte, error) {
	if err := validate(); err != nil {
		return nil, fmt.Errorf("validate %s: %w", label, err)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", label, err)
	}
	if len(payload) > int(MaximumFrameSize) {
		return nil, fmt.Errorf("encode %s exceeds its bounded object shape", label)
	}
	return payload, nil
}

// rejectDuplicateJSON scans nested objects so duplicate keys cannot hide under normal decoding.
func rejectDuplicateJSON(payload []byte, label string) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := inspectJSONValue(decoder, label, true); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("decode %s: trailing JSON value", label)
		}
		return fmt.Errorf("decode %s trailing data: %w", label, err)
	}
	return nil
}

// inspectJSONValue walks one JSON value and rejects duplicate object members.
func inspectJSONValue(decoder *json.Decoder, label string, requireObject bool) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("decode %s: %w", label, err)
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		if requireObject {
			return fmt.Errorf("%s must be an object", label)
		}
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			fieldToken, err := decoder.Token()
			if err != nil {
				return fmt.Errorf("decode %s field: %w", label, err)
			}
			field, ok := fieldToken.(string)
			if !ok {
				return fmt.Errorf("%s field name must be a string", label)
			}
			if _, duplicate := seen[field]; duplicate {
				return fmt.Errorf("%s contains duplicate field %q", label, field)
			}
			seen[field] = struct{}{}
			if err := inspectJSONValue(decoder, label, false); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("decode %s object end: %w", label, err)
		}
		if closing != json.Delim('}') {
			return fmt.Errorf("decode %s object is not terminated", label)
		}
	case '[':
		if requireObject {
			return fmt.Errorf("%s must be an object", label)
		}
		for decoder.More() {
			if err := inspectJSONValue(decoder, label, false); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("decode %s array end: %w", label, err)
		}
		if closing != json.Delim(']') {
			return fmt.Errorf("decode %s array is not terminated", label)
		}
	default:
		return fmt.Errorf("decode %s contains unsupported JSON delimiter %q", label, delimiter)
	}
	return nil
}
