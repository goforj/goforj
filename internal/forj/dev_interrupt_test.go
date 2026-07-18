package forj

import (
	"errors"
	"testing"
)

// TestDevForwardInterruptCmdInvokesPlatformSignal verifies Bubble Tea dispatches the platform interrupt once.
func TestDevForwardInterruptCmdInvokesPlatformSignal(t *testing.T) {
	t.Parallel()
	calls := 0
	command := devForwardInterruptCmdWith(func() error {
		calls++
		return errors.New("ignored signal failure")
	})
	if message := command(); message != nil {
		t.Fatalf("devForwardInterruptCmdWith() message = %#v, want nil", message)
	}
	if calls != 1 {
		t.Fatalf("devForwardInterruptCmdWith() calls = %d, want 1", calls)
	}
}
