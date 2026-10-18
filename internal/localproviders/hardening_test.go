package localproviders

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// assertFileMode checks the published file rather than a permission value prepared in memory.
func assertFileMode(t *testing.T, root, name string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != want {
		t.Fatalf("%s mode = %04o, want %04o", name, info.Mode().Perm(), want)
	}
}

// TestPrivatePermissionsRollbackPreservesOriginalState keeps temporary secrets restricted while restoring an unsuccessful transaction exactly.
func TestPrivatePermissionsRollbackPreservesOriginalState(t *testing.T) {
	root := t.TempDir()
	name := ".env.local-providers.backup"
	put(t, root, name, "original")
	if err := os.Chmod(filepath.Join(root, name), 0644); err != nil {
		t.Fatal(err)
	}
	put(t, root, ".env", "unchanged")
	private, err := readFile(root, name)
	if err != nil {
		t.Fatal(err)
	}
	active, err := readFile(root, ".env")
	if err != nil {
		t.Fatal(err)
	}
	private.after, active.after = []byte("new secret"), []byte("new active")
	err = commit(root, []file{private, active}, func(from, to string) error {
		if filepath.Base(to) == ".env" {
			return errors.New("injected publication failure")
		}
		assertFileMode(t, filepath.Dir(from), filepath.Base(from), 0600)
		return os.Rename(from, to)
	})
	if err == nil {
		t.Fatal("expected publication failure")
	}
	assertFileMode(t, root, name, 0644)
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil || string(data) != "original" {
		t.Fatalf("rollback failed: %v", err)
	}
}
