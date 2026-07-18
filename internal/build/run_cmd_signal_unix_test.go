//go:build !windows

package build

import (
	"os/exec"
	"syscall"
	"testing"
)

// TestExitCodeFromErrorNormalizesSignalStatus verifies Unix child signals retain the conventional shell exit code.
func TestExitCodeFromErrorNormalizesSignalStatus(t *testing.T) {
	child := exec.Command("sh", "-c", "kill -TERM $$")
	err := child.Run()
	if err == nil {
		t.Fatal("signal-terminated child returned nil error")
	}
	code, ok := exitCodeFromError(err)
	if !ok || code != 128+int(syscall.SIGTERM) {
		t.Fatalf("exitCodeFromError() = %d, %v; want %d, true", code, ok, 128+int(syscall.SIGTERM))
	}
}
