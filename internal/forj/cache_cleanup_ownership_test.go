package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestValidateCacheRenderTransitionRejectsUnmarkedCleanupArtifact verifies owner edits cannot be mistaken for disposable Cache output.
func TestValidateCacheRenderTransitionRejectsUnmarkedCleanupArtifact(t *testing.T) {
	t.Chdir(t.TempDir())
	path := filepath.Join("internal", "cmd", "cache_shell_cmd.go")
	writeCacheCleanupFixture(t, path, "package cmd\n\nfunc customCacheCommand() {}\n")

	err := validateCacheRenderTransition(project.Components{})
	if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "ownership marker") {
		t.Fatalf("validate Cache transition error = %v, want ownership error for %s", err, path)
	}
	contents, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read preserved Cache artifact: %v", readErr)
	}
	if !strings.Contains(string(contents), "customCacheCommand") {
		t.Fatalf("Cache validation changed owner artifact %s", path)
	}
}

// TestCleanupDisabledCacheGeneratedFilesRequiresAndRemovesMarkers verifies cleanup is independently safe and removes only marked output.
func TestCleanupDisabledCacheGeneratedFilesRequiresAndRemovesMarkers(t *testing.T) {
	t.Chdir(t.TempDir())
	artifacts := cacheGeneratedCleanupArtifacts()
	for _, artifact := range artifacts {
		writeCacheCleanupFixture(t, artifact.path, artifact.marker+"\n")
	}

	if err := cleanupDisabledCacheGeneratedFiles(); err != nil {
		t.Fatalf("clean marked Cache artifacts: %v", err)
	}
	for _, artifact := range artifacts {
		if _, err := os.Stat(artifact.path); !os.IsNotExist(err) {
			t.Fatalf("generated Cache artifact %s still exists: %v", artifact.path, err)
		}
	}
}

// writeCacheCleanupFixture writes one ownership fixture beneath the isolated test root.
func writeCacheCleanupFixture(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create Cache fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write Cache fixture %s: %v", path, err)
	}
}
