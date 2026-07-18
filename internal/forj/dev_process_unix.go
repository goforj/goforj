//go:build unix

package forj

import (
	"errors"
	"os"
	"syscall"
)

// isProcessRunning checks whether a PID still identifies a Unix process.
func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	return unixProcessRunningResult(syscall.Kill(pid, 0))
}

// unixProcessRunningResult treats permission denial as proof that the process exists.
func unixProcessRunningResult(err error) bool {
	return err == nil || errors.Is(err, syscall.EPERM)
}

// signalDevInterrupt forwards the TUI interrupt through the process signal path owned by DevCmd.
func signalDevInterrupt() error {
	return signalUnixDevInterrupt(syscall.Kill)
}

// signalUnixDevInterrupt targets the current process with the interrupt DevCmd already observes.
func signalUnixDevInterrupt(kill func(int, syscall.Signal) error) error {
	return kill(os.Getpid(), syscall.SIGINT)
}
