package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveBackupSourceRetainsExistingLocalDirectory verifies local backups need no remote repository resolution.
func TestResolveBackupSourceRetainsExistingLocalDirectory(t *testing.T) {
	root := t.TempDir()
	source, err := resolveBackupSource(context.Background(), root)
	if err != nil {
		t.Fatalf("resolve local backup source: %v", err)
	}
	defer source.cleanup()
	if source.path != root {
		t.Fatalf("resolved path = %q, want %q", source.path, root)
	}
}

// TestResolveBackupSourceReturnsLocalInspectionFailure prevents inaccessible paths from being mistaken for remote names.
func TestResolveBackupSourceReturnsLocalInspectionFailure(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write path blocker: %v", err)
	}

	_, err := resolveBackupSource(context.Background(), filepath.Join(blocker, "backup"))
	if err == nil || !strings.Contains(err.Error(), "inspect backup source") {
		t.Fatalf("resolveBackupSource() error = %v, want local inspection failure", err)
	}
}
