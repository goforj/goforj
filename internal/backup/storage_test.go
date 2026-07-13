package backup

import (
	"os"
	"path/filepath"
	"testing"
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
