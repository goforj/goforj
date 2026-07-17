package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestApplyRenderedIntegrationEnvironmentPublishesTimezone verifies database variants start outside UTC.
func TestApplyRenderedIntegrationEnvironmentPublishesTimezone(t *testing.T) {
	for _, variant := range []string{"mysql", "postgres"} {
		t.Run(variant, func(t *testing.T) {
			projectDir := t.TempDir()
			envPath := filepath.Join(projectDir, ".env")
			if err := os.WriteFile(envPath, []byte("TZ=UTC\n"), 0o644); err != nil {
				t.Fatalf("write env: %v", err)
			}
			if err := applyRenderedIntegrationEnvironment(projectDir, dbIntegrationVariantSpecs[variant].testEnv); err != nil {
				t.Fatalf("apply integration environment: %v", err)
			}
			body, err := os.ReadFile(envPath)
			if err != nil {
				t.Fatalf("read env: %v", err)
			}
			if !strings.Contains(string(body), "TZ=America/Los_Angeles") {
				t.Fatalf("expected non-UTC timezone, got:\n%s", body)
			}
		})
	}
}

// TestApplyRenderedIntegrationEnvironmentSkipsMissingTimezone verifies unrelated variants need no env file mutation.
func TestApplyRenderedIntegrationEnvironmentSkipsMissingTimezone(t *testing.T) {
	if err := applyRenderedIntegrationEnvironment(t.TempDir(), nil); err != nil {
		t.Fatalf("empty integration environment: %v", err)
	}
}

// TestApplyRenderedIntegrationEnvironmentReturnsEnvWriteFailure verifies setup errors are not hidden.
func TestApplyRenderedIntegrationEnvironmentReturnsEnvWriteFailure(t *testing.T) {
	err := applyRenderedIntegrationEnvironment(t.TempDir(), map[string]string{"TZ": "America/Los_Angeles"})
	if err == nil {
		t.Fatal("expected missing env file error")
	}
}
