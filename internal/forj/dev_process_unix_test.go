//go:build unix

package forj

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

// TestUnixProcessRunningResultClassifiesProbeErrors verifies stale locks are removed only when absence is known.
func TestUnixProcessRunningResultClassifiesProbeErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "successful probe", want: true},
		{name: "permission denied", err: syscall.EPERM, want: true},
		{name: "wrapped permission denied", err: errors.Join(errors.New("probe"), syscall.EPERM), want: true},
		{name: "missing process", err: syscall.ESRCH, want: false},
		{name: "unexpected failure", err: errors.New("probe failed"), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := unixProcessRunningResult(test.err); got != test.want {
				t.Fatalf("unixProcessRunningResult(%v) = %t, want %t", test.err, got, test.want)
			}
		})
	}
}

// TestIsProcessRunningRecognizesCurrentUnixProcess verifies the host probe reaches the current process.
func TestIsProcessRunningRecognizesCurrentUnixProcess(t *testing.T) {
	t.Parallel()
	if !isProcessRunning(os.Getpid()) {
		t.Fatalf("isProcessRunning(%d) = false, want true", os.Getpid())
	}
	for _, pid := range []int{0, -1} {
		if isProcessRunning(pid) {
			t.Fatalf("isProcessRunning(%d) = true, want false", pid)
		}
	}
}

// TestSignalUnixDevInterruptTargetsCurrentProcess verifies the TUI uses the signal DevCmd observes.
func TestSignalUnixDevInterruptTargetsCurrentProcess(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("interrupt sent")
	var gotPID int
	var gotSignal syscall.Signal
	err := signalUnixDevInterrupt(func(pid int, signal syscall.Signal) error {
		gotPID = pid
		gotSignal = signal
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("signalUnixDevInterrupt() error = %v, want %v", err, wantErr)
	}
	if gotPID != os.Getpid() || gotSignal != syscall.SIGINT {
		t.Fatalf("signalUnixDevInterrupt() target = (%d, %v), want (%d, %v)", gotPID, gotSignal, os.Getpid(), syscall.SIGINT)
	}
}
