package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestServiceRestoreRejectsUnknownResource verifies a filtered restore cannot report success without changing a resource.
func TestServiceRestoreRejectsUnknownResource(t *testing.T) {
	t.Setenv("FORJ_APP", "")
	t.Setenv("DB_DRIVER", "sqlite")
	dir := t.TempDir()
	artifact := filepath.Join(dir, "databases", "default.sqlite.backup")
	if err := os.MkdirAll(filepath.Dir(artifact), 0o755); err != nil {
		t.Fatalf("create artifact directory: %v", err)
	}
	if err := os.WriteFile(artifact, []byte("backup fixture"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	fingerprint, err := Checksum(artifact)
	if err != nil {
		t.Fatalf("checksum artifact: %v", err)
	}
	manifest := Manifest{Version: 1, Resources: []Resource{{
		ID: "db.default", Kind: "database", Name: "default", Driver: "sqlite",
		Strategy: "sqlite-vacuum-into", Artifact: "databases/default.sqlite.backup",
		Checksum: fingerprint.Checksum, Size: fingerprint.Size,
	}}}
	if err := WriteManifest(dir, manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	afterRestoreCalled := false
	service := &Service{Hooks: HookRegistry{AfterRestore: []Hook{func(context.Context, HookEvent) error {
		afterRestoreCalled = true
		return nil
	}}}}

	err = service.Restore(context.Background(), dir, "missing", "restore-production")
	if err == nil || !strings.Contains(err.Error(), `restore resource "missing" was not found`) {
		t.Fatalf("restore error = %v, want missing-resource failure", err)
	}
	if afterRestoreCalled {
		t.Fatal("after-restore hook ran despite restoring no resource")
	}
}
