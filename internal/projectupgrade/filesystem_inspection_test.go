package projectupgrade

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestInspectionFSRetainsReadOnlyFilesystemSemantics checks discovery metadata, directory entries, and invalid paths.
func TestInspectionFSRetainsReadOnlyFilesystemSemantics(t *testing.T) {
	directory := t.TempDir()
	writeFixture(t, directory, "managed/source.go", "SAFE")
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	files := InspectionFS(root)
	data, err := fs.ReadFile(files, "managed/source.go")
	if err != nil || string(data) != "SAFE" {
		t.Fatalf("read regular file: %q, %v", data, err)
	}
	entries, err := fs.ReadDir(files, "managed")
	if err != nil || len(entries) != 1 || entries[0].Name() != "source.go" {
		t.Fatalf("read directory: %v, %v", entries, err)
	}
	if _, err := fs.ReadDir(files, "managed/source.go"); err == nil {
		t.Fatal("accepted regular file as a directory")
	}
	if err := os.Chmod(filepath.Join(directory, "managed/source.go"), 0); err != nil {
		t.Fatal(err)
	}
	if info, err := fs.Stat(files, "managed/source.go"); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("metadata for unreadable marker: %v, %v", info, err)
	}
	invalid := []string{"", "..", "../outside", "/managed", "managed/../source.go"}
	if runtime.GOOS == "windows" {
		invalid = append(invalid, `managed\source.go`)
	}
	for _, name := range invalid {
		if file, err := files.Open(name); !errors.Is(err, fs.ErrInvalid) {
			if file != nil {
				file.Close()
			}
			t.Errorf("Open(%q): %v", name, err)
		}
		if _, err := fs.Stat(files, name); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("Stat(%q): %v", name, err)
		}
	}
	if _, err := files.Open("missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing open: %v", err)
	}
	if _, err := fs.Stat(files, "missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing stat: %v", err)
	}
}
