package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRedisShellCleanupRemovesOnlyGeneratedArtifacts verifies disabling Redis does not leave a stale command surface.
func TestRedisShellCleanupRemovesOnlyGeneratedArtifacts(t *testing.T) {
	t.Chdir(t.TempDir())
	workspace := currentProjectRenderWorkspace(t)
	for _, artifact := range redisShellGeneratedCleanupArtifacts() {
		path := workspace.path(artifact.path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create artifact directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(artifact.marker+"\npackage cmd\n"), 0o644); err != nil {
			t.Fatalf("write generated artifact: %v", err)
		}
	}
	if err := workspace.cleanupDisabledRedisShellGeneratedFiles(); err != nil {
		t.Fatalf("cleanupDisabledRedisShellGeneratedFiles returned error: %v", err)
	}
	for _, artifact := range redisShellGeneratedCleanupArtifacts() {
		if _, err := os.Stat(workspace.path(artifact.path)); !os.IsNotExist(err) {
			t.Fatalf("generated artifact %s still exists after cleanup", artifact.path)
		}
	}
}

// TestRedisShellCleanupRejectsOwnerArtifact verifies the transition fails before deleting an unmarked file.
func TestRedisShellCleanupRejectsOwnerArtifact(t *testing.T) {
	t.Chdir(t.TempDir())
	workspace := currentProjectRenderWorkspace(t)
	artifact := redisShellGeneratedCleanupArtifacts()[0]
	path := workspace.path(artifact.path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create artifact directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("package cmd\n"), 0o644); err != nil {
		t.Fatalf("write owner artifact: %v", err)
	}
	err := workspace.cleanupDisabledRedisShellGeneratedFiles()
	if err == nil || !strings.Contains(err.Error(), "does not carry the generated ownership marker") {
		t.Fatalf("cleanupDisabledRedisShellGeneratedFiles error = %v, want ownership rejection", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("owner artifact was removed: %v", statErr)
	}
}
