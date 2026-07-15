package apiindex

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/goforj/web/webindex"
)

// TestRunnerStrictFailurePreservesActiveArtifacts verifies diagnostics never cross the staged publication boundary.
func TestRunnerStrictFailurePreservesActiveArtifacts(t *testing.T) {
	root := t.TempDir()
	brokenPath := filepath.Join(root, "internal", "broken", "broken.go")
	if err := os.MkdirAll(filepath.Dir(brokenPath), 0o755); err != nil {
		t.Fatalf("create broken package directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/strict\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(brokenPath, []byte("package broken\nfunc Broken("), 0o644); err != nil {
		t.Fatalf("write broken source: %v", err)
	}
	paths := paths{
		root:        root,
		appName:     "app",
		out:         filepath.Join(root, "build", "api_index.json"),
		diagnostics: filepath.Join(root, "build", "api_index.diagnostics.json"),
		openAPI:     filepath.Join(root, "build", "openapi.json"),
	}
	for _, path := range artifactPaths(paths) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create active artifact directory: %v", err)
		}
		if err := os.WriteFile(path, []byte("{\"generation\":\"active\"}\n"), 0o644); err != nil {
			t.Fatalf("write active artifact: %v", err)
		}
	}

	prepared, err := newTestRunner().prepareDefaultPaths(paths, runOptions{strict: true})
	if prepared.candidate != nil {
		t.Fatal("strict diagnostics unexpectedly returned a publishable candidate")
	}
	var diagnosticsErr *webindex.DiagnosticsError
	if !errors.As(err, &diagnosticsErr) {
		t.Fatalf("strict runner error = %T %v, want DiagnosticsError", err, err)
	}
	if prepared.report.appName != "app" || prepared.report.outcome != outcomeRejected || prepared.report.diagnostics == 0 {
		t.Fatalf("strict failure report = %#v, want active app and diagnostic count", prepared.report)
	}
	for _, path := range artifactPaths(paths) {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read preserved artifact: %v", readErr)
		}
		if string(content) != "{\"generation\":\"active\"}\n" {
			t.Fatalf("strict failure replaced active artifact %s: %s", path, content)
		}
	}
	assertNoStagingDirectories(t, filepath.Dir(paths.out))
}

// TestReportStatusFormatsSkippedOutcome verifies zero-count non-participants remain explicit to standalone users.
func TestReportStatusFormatsSkippedOutcome(t *testing.T) {
	report := runReport{appName: "ship", outcome: outcomeSkipped, reason: noWebAPIReason}
	want := "app ship, skipped (no web API), 0 operations, 0 schemas, 0 diagnostics"
	if got := report.status(); got != want {
		t.Fatalf("skipped API index status = %q, want %q", got, want)
	}
}
