package backup

import (
	"archive/tar"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestStorageDirectoryArchiveRoundTrip(t *testing.T) {
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "note.txt"), []byte("hello"), 0o640); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(t.TempDir(), "storage.tar.zst")
	if err := ArchiveDirectory(source, artifact); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "restored")
	if err := RestoreDirectoryArchive(artifact, target); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "nested", "note.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("restored data = %q", data)
	}
}

func TestSafeExtractPathRejectsTraversal(t *testing.T) {
	if _, err := safeExtractPath("/tmp/restore", "../../secret"); err == nil {
		t.Fatal("expected traversal rejection")
	}
}

// TestRestoreDirectoryArchiveRejectsExpandedByteLimit verifies compressed input cannot consume unbounded destination storage.
func TestRestoreDirectoryArchiveRejectsExpandedByteLimit(t *testing.T) {
	artifact := writeStorageArchive(t, []storageArchiveTestEntry{{name: "large.txt", body: "12345"}})
	err := restoreDirectoryArchive(artifact, t.TempDir(), archiveRestoreLimits{entries: 10, bytes: 4})
	if err == nil || !strings.Contains(err.Error(), "exceeds 4 expanded bytes") {
		t.Fatalf("restore error = %v, want expanded byte limit", err)
	}
}

// TestRestoreDirectoryArchiveRejectsEntryLimit verifies many small entries cannot exhaust filesystem metadata.
func TestRestoreDirectoryArchiveRejectsEntryLimit(t *testing.T) {
	artifact := writeStorageArchive(t, []storageArchiveTestEntry{{name: "one.txt", body: "1"}, {name: "two.txt", body: "2"}})
	err := restoreDirectoryArchive(artifact, t.TempDir(), archiveRestoreLimits{entries: 1, bytes: 10})
	if err == nil || !strings.Contains(err.Error(), "exceeds 1 entries") {
		t.Fatalf("restore error = %v, want entry limit", err)
	}
}

// TestRestoreDirectoryArchiveContainsDestinationSymlinks verifies writes cannot follow a destination link outside the restore root.
func TestRestoreDirectoryArchiveContainsDestinationSymlinks(t *testing.T) {
	outside := t.TempDir()
	target := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(target, "linked")); err != nil {
		t.Skipf("create destination symlink: %v", err)
	}
	artifact := writeStorageArchive(t, []storageArchiveTestEntry{{name: "linked/escaped.txt", body: "blocked"}})
	if err := RestoreDirectoryArchive(artifact, target); err == nil {
		t.Fatal("expected destination symlink rejection")
	}
	if _, err := os.Stat(filepath.Join(outside, "escaped.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside file stat error = %v, want not found", err)
	}
}

// storageArchiveTestEntry describes one regular file in a test archive.
type storageArchiveTestEntry struct {
	name string
	body string
}

// writeStorageArchive creates a valid compressed tar artifact for restore security tests.
func writeStorageArchive(t *testing.T, entries []storageArchiveTestEntry) string {
	t.Helper()
	artifact := filepath.Join(t.TempDir(), "storage.tar.zst")
	file, err := os.Create(artifact)
	if err != nil {
		t.Fatal(err)
	}
	compressor, err := zstd.NewWriter(file)
	if err != nil {
		t.Fatal(err)
	}
	archive := tar.NewWriter(compressor)
	for _, entry := range entries {
		header := &tar.Header{Name: entry.name, Mode: 0o600, Size: int64(len(entry.body)), Typeflag: tar.TypeReg}
		if err := archive.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(archive, entry.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressor.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return artifact
}
