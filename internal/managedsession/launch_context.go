package managedsession

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"
)

const (
	// ManagedLaunchContextSchemaVersion identifies the owner-only inherited launch context shape.
	ManagedLaunchContextSchemaVersion = "managed-launch-context.v1"
	// ManagedLaunchContextEnvironment names the reserved environment value carrying the context file path.
	ManagedLaunchContextEnvironment   = "FORJ_INTERNAL_MANAGED_CONTEXT"
	managedLaunchContextOwner         = SessionOwnerHarbor
	managedLaunchContextFilePrefix    = "managed-launch-"
	managedLaunchContextMaximumBytes  = 16 << 10
	managedLaunchContextMaximumPath   = 4096
	managedLaunchContextMinimumTicket = 32
	managedLaunchContextMaximumTicket = 512
)

// LaunchContext binds one inherited GoForj process to its exact Harbor session authority.
//
// The ticket is read only from the owner-private, one-use file. It is never accepted from argv or
// project dotenv files, which keeps normal project configuration from impersonating Harbor.
type LaunchContext struct {
	// SchemaVersion identifies the context shape understood by both launchers.
	SchemaVersion string `json:"schema_version"`
	// ProjectID binds the launch to one Harbor project admission.
	ProjectID ProjectID `json:"project_id"`
	// SessionID binds the launch to one durable project lifecycle.
	SessionID SessionID `json:"session_id"`
	// ProjectRoot binds the launch to the canonical checkout selected by Harbor.
	ProjectRoot string `json:"project_root"`
	// ExpectedSessionGeneration prevents a context from attaching a later replacement session.
	ExpectedSessionGeneration uint64 `json:"expected_session_generation"`
	// DescriptorDigest binds the child to the descriptor preflight used for this launch.
	DescriptorDigest string `json:"descriptor_digest"`
	// EndpointReference identifies the authenticated Harbor IPC endpoint without carrying a socket secret.
	EndpointReference string `json:"endpoint_reference"`
	// Owner identifies the lifecycle authority that issued the context.
	Owner SessionOwner `json:"owner"`
	// Ticket is the one-use credential presented by the future managed-session handshake.
	Ticket string `json:"ticket"`
}

// Validate rejects incomplete or ambiguous inherited launch authority before project configuration is loaded.
func (context LaunchContext) Validate() error {
	if context.SchemaVersion != ManagedLaunchContextSchemaVersion {
		return fmt.Errorf("managed launch context schema version %q is unsupported", context.SchemaVersion)
	}
	if err := validateIdentifier("managed launch context project ID", string(context.ProjectID)); err != nil {
		return err
	}
	if err := validateIdentifier("managed launch context session ID", string(context.SessionID)); err != nil {
		return err
	}
	if err := validateManagedSessionRoot(context.ProjectRoot); err != nil {
		return fmt.Errorf("managed launch context project root: %w", err)
	}
	if context.ExpectedSessionGeneration == 0 || context.ExpectedSessionGeneration >= MaximumSequence {
		return fmt.Errorf("managed launch context session generation must be between 1 and %d", MaximumSequence-1)
	}
	if err := validateManagedSessionDigest(context.DescriptorDigest); err != nil {
		return fmt.Errorf("managed launch context descriptor digest: %w", err)
	}
	if err := validateManagedLaunchEndpoint(context.EndpointReference); err != nil {
		return err
	}
	if context.Owner != managedLaunchContextOwner {
		return fmt.Errorf("managed launch context owner %q is not Harbor", context.Owner)
	}
	return validateManagedLaunchTicket(context.Ticket)
}

// CaptureInheritedLaunchContext consumes the reserved owner-only context before any project env load.
func CaptureInheritedLaunchContext() (*LaunchContext, error) {
	path, present := os.LookupEnv(ManagedLaunchContextEnvironment)
	if !present {
		return nil, nil
	}
	// The carrier is one-use authority; clearing the inherited name prevents nested project tools from replaying a retired path.
	if err := os.Unsetenv(ManagedLaunchContextEnvironment); err != nil {
		return nil, fmt.Errorf("clear managed launch context environment: %w", err)
	}
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("managed launch context path is empty")
	}
	context, err := readAndConsumeLaunchContext(path)
	if err != nil {
		return nil, err
	}
	return &context, nil
}

// readAndConsumeLaunchContext validates and retires a context path even when its contents are malformed.
func readAndConsumeLaunchContext(path string) (LaunchContext, error) {
	if err := validateManagedLaunchContextPath(path); err != nil {
		return LaunchContext{}, err
	}
	contents, readErr := readManagedLaunchContextFile(path)
	removeErr := os.Remove(path)
	if errors.Is(removeErr, os.ErrNotExist) {
		removeErr = nil
	}
	if readErr != nil || removeErr != nil {
		return LaunchContext{}, errors.Join(readErr, removeErr)
	}
	var context LaunchContext
	decoder := json.NewDecoder(strings.NewReader(string(contents)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&context); err != nil {
		return LaunchContext{}, fmt.Errorf("decode managed launch context: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return LaunchContext{}, errors.New("managed launch context contains trailing JSON")
		}
		return LaunchContext{}, fmt.Errorf("decode managed launch context trailer: %w", err)
	}
	if err := context.Validate(); err != nil {
		return LaunchContext{}, fmt.Errorf("validate managed launch context: %w", err)
	}
	return context, nil
}

// validateManagedLaunchContextPath limits inherited reads to an absolute owner-private file reference.
func validateManagedLaunchContextPath(path string) error {
	if path == "" || !utf8.ValidString(path) || len([]byte(path)) > managedLaunchContextMaximumPath {
		return errors.New("managed launch context path is invalid")
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("managed launch context path must be a canonical absolute path")
	}
	if strings.IndexByte(path, 0) >= 0 {
		return errors.New("managed launch context path contains NUL")
	}
	base := filepath.Base(path)
	if !strings.HasPrefix(base, managedLaunchContextFilePrefix) || !strings.HasSuffix(base, ".json") {
		return errors.New("managed launch context path has an invalid file name")
	}
	return nil
}

// readManagedLaunchContextFile checks owner-only permissions before reading bounded JSON content.
func readManagedLaunchContextFile(path string) ([]byte, error) {
	parent, err := os.Lstat(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("inspect managed launch context directory: %w", err)
	}
	if parent.Mode()&os.ModeSymlink != 0 || !parent.IsDir() || runtime.GOOS != "windows" && parent.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("managed launch context directory is not owner-only")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect managed launch context file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("managed launch context file is not a regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		return nil, errors.New("managed launch context file is not owner-only")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open managed launch context: %w", err)
	}
	defer file.Close()
	contents, err := io.ReadAll(io.LimitReader(file, managedLaunchContextMaximumBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read managed launch context: %w", err)
	}
	if len(contents) > managedLaunchContextMaximumBytes {
		return nil, fmt.Errorf("managed launch context exceeds %d bytes", managedLaunchContextMaximumBytes)
	}
	return contents, nil
}

// validateManagedLaunchEndpoint accepts only platform-local absolute endpoint references.
func validateManagedLaunchEndpoint(endpoint string) error {
	if endpoint == "" || !utf8.ValidString(endpoint) || len([]byte(endpoint)) > managedLaunchContextMaximumPath {
		return errors.New("managed launch context endpoint reference is invalid")
	}
	if !filepath.IsAbs(endpoint) && !strings.HasPrefix(endpoint, `\\.\pipe\`) {
		return errors.New("managed launch context endpoint reference must be local and absolute")
	}
	if strings.IndexByte(endpoint, 0) >= 0 {
		return errors.New("managed launch context endpoint reference contains NUL")
	}
	return nil
}

// validateManagedLaunchTicket bounds opaque credential material without exposing its value in errors.
func validateManagedLaunchTicket(ticket string) error {
	if len([]byte(ticket)) < managedLaunchContextMinimumTicket || len([]byte(ticket)) > managedLaunchContextMaximumTicket {
		return fmt.Errorf("managed launch context ticket must contain between %d and %d bytes", managedLaunchContextMinimumTicket, managedLaunchContextMaximumTicket)
	}
	if strings.TrimSpace(ticket) != ticket || !utf8.ValidString(ticket) || strings.IndexByte(ticket, 0) >= 0 {
		return errors.New("managed launch context ticket is invalid")
	}
	return nil
}
