package commanddiag

import (
	"errors"
	"fmt"
	"testing"
)

// TestWrapFormatsAllDistinctOutput keeps stderr and stdout visible without flattening multiline diagnostics.
func TestWrapFormatsAllDistinctOutput(t *testing.T) {
	cause := errors.New("exit status 2")
	err := Wrap("run tool", cause, "stderr line one\nstderr line two", "stdout detail", "stderr line one\nstderr line two")
	want := "run tool: exit status 2\n  stderr line one\n  stderr line two\n  stdout detail"
	if err.Error() != want {
		t.Fatalf("command error = %q, want %q", err.Error(), want)
	}
	if !HasAction(fmt.Errorf("render: %w", err), "run tool") {
		t.Fatal("HasAction() did not find wrapped command diagnostic")
	}
	if !errors.Is(err, cause) {
		t.Fatal("Wrap() did not preserve the process failure")
	}
}
