package managedsession

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validLaunchContext returns one deterministic context with the same shape Harbor emits.
func validLaunchContext(t *testing.T) LaunchContext {
	t.Helper()
	return LaunchContext{
		SchemaVersion:             ManagedLaunchContextSchemaVersion,
		ProjectID:                 "project-context",
		SessionID:                 "session-context",
		ProjectRoot:               filepath.Clean(t.TempDir()),
		ExpectedSessionGeneration: 1,
		DescriptorDigest:          strings.Repeat("a", 64),
		EndpointReference:         filepath.Join(string(filepath.Separator), "tmp", "harbord.sock"),
		Owner:                     SessionOwnerHarbor,
		Ticket:                    strings.Repeat("b", 64),
	}
}

// TestLaunchContextValidationRejectsAmbiguousValues keeps inherited authority fail-closed before env loading.
func TestLaunchContextValidationRejectsAmbiguousValues(t *testing.T) {
	base := validLaunchContext(t)
	tests := []struct {
		name   string
		mutate func(*LaunchContext)
	}{
		{name: "schema", mutate: func(context *LaunchContext) { context.SchemaVersion = "managed-launch-context.v2" }},
		{name: "project root", mutate: func(context *LaunchContext) { context.ProjectRoot = "relative" }},
		{name: "generation", mutate: func(context *LaunchContext) { context.ExpectedSessionGeneration = 0 }},
		{name: "digest", mutate: func(context *LaunchContext) { context.DescriptorDigest = strings.Repeat("A", 64) }},
		{name: "endpoint", mutate: func(context *LaunchContext) { context.EndpointReference = "harbord.sock" }},
		{name: "owner", mutate: func(context *LaunchContext) { context.Owner = SessionOwnerTerminal }},
		{name: "ticket", mutate: func(context *LaunchContext) { context.Ticket = "short" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := base
			test.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("Validate() unexpectedly accepted ambiguous context")
			}
		})
	}
}

// TestCaptureInheritedLaunchContextConsumesOwnerOnlyFile verifies the ticket is read before configuration and cannot be replayed.
func TestCaptureInheritedLaunchContextConsumesOwnerOnlyFile(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatalf("secure context directory: %v", err)
	}
	path := filepath.Join(root, "managed-launch-test.json")
	context := validLaunchContext(t)
	encoded, err := json.Marshal(context)
	if err != nil {
		t.Fatalf("marshal launch context: %v", err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatalf("write launch context: %v", err)
	}
	t.Setenv(ManagedLaunchContextEnvironment, path)
	captured, err := CaptureInheritedLaunchContext()
	if err != nil {
		t.Fatalf("CaptureInheritedLaunchContext() error = %v", err)
	}
	if captured == nil || *captured != context {
		t.Fatalf("captured context = %#v, want %#v", captured, context)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("context file stat after capture = %v, want not exist", err)
	}
	if _, err := CaptureInheritedLaunchContext(); err == nil {
		t.Fatal("replayed context unexpectedly succeeded")
	}
}

// TestCaptureInheritedLaunchContextConsumesMalformedFile verifies malformed authority cannot remain available for retry.
func TestCaptureInheritedLaunchContextConsumesMalformedFile(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatalf("secure context directory: %v", err)
	}
	path := filepath.Join(root, "managed-launch-test.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":"managed-launch.v1","unknown":true}`), 0o600); err != nil {
		t.Fatalf("write malformed launch context: %v", err)
	}
	t.Setenv(ManagedLaunchContextEnvironment, path)
	if _, err := CaptureInheritedLaunchContext(); err == nil {
		t.Fatal("malformed context unexpectedly succeeded")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("malformed context stat after capture = %v, want not exist", err)
	}
}

// TestCaptureInheritedLaunchContextRejectsNonPrivateDirectory prevents a context from being read through a shared path.
func TestCaptureInheritedLaunchContextRejectsNonPrivateDirectory(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "managed-launch.json")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write launch context: %v", err)
	}
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatalf("make context directory shared: %v", err)
	}
	t.Setenv(ManagedLaunchContextEnvironment, path)
	if _, err := CaptureInheritedLaunchContext(); err == nil {
		t.Fatal("shared context directory unexpectedly accepted")
	}
}
