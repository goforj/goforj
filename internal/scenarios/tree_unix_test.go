//go:build !windows

package scenarios

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestCopyScenarioFilePreservesModeAcrossUmask keeps prepared-tree identity independent from the invoking shell.
func TestCopyScenarioFilePreservesModeAcrossUmask(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	destination := filepath.Join(root, "destination")
	if err := os.WriteFile(source, []byte("fixture"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.Chmod(source, 0o644); err != nil {
		t.Fatalf("set source mode: %v", err)
	}
	previous := syscall.Umask(0o077)
	t.Cleanup(func() { syscall.Umask(previous) })
	if err := copyScenarioFile(source, destination, 0o644); err != nil {
		t.Fatalf("copyScenarioFile(): %v", err)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatalf("inspect destination: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("destination mode = %o, want 644", info.Mode().Perm())
	}
}
