//go:build windows

package forj

import (
	"errors"

	"golang.org/x/sys/windows"
)

// isProcessRunning checks whether a PID still identifies a Windows process.
func isProcessRunning(pid int) bool {
	return windowsProcessRunning(pid, windows.OpenProcess, windows.GetExitCodeProcess, windows.CloseHandle)
}

// windowsProcessRunning probes process state without requiring termination rights.
func windowsProcessRunning(
	pid int,
	openProcess func(uint32, bool, uint32) (windows.Handle, error),
	getExitCode func(windows.Handle, *uint32) error,
	closeHandle func(windows.Handle) error,
) bool {
	if pid <= 0 || uint64(pid) > uint64(^uint32(0)) {
		return false
	}
	handle, err := openProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// A protected process can deny inspection while still owning the dev lock.
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer func() { _ = closeHandle(handle) }()

	var exitCode uint32
	if err := getExitCode(handle, &exitCode); err != nil {
		// Once a handle opens successfully, uncertainty must not make an active lock look stale.
		return true
	}
	return exitCode == uint32(windows.STATUS_PENDING)
}

// signalDevInterrupt forwards the TUI interrupt through the console signal path owned by DevCmd.
func signalDevInterrupt() error {
	return signalWindowsDevInterrupt(windows.GenerateConsoleCtrlEvent)
}

// signalWindowsDevInterrupt emits Ctrl+C to the current console process group.
func signalWindowsDevInterrupt(generateControlEvent func(uint32, uint32) error) error {
	return generateControlEvent(windows.CTRL_C_EVENT, 0)
}
