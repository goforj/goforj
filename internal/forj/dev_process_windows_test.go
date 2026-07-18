//go:build windows

package forj

import (
	"errors"
	"strconv"
	"testing"

	"golang.org/x/sys/windows"
)

// TestDevWindowsProcessRunningRejectsInvalidPIDs verifies invalid lock contents never reach OpenProcess.
func TestDevWindowsProcessRunningRejectsInvalidPIDs(t *testing.T) {
	t.Parallel()
	openCalled := false
	openProcess := func(uint32, bool, uint32) (windows.Handle, error) {
		openCalled = true
		return 0, nil
	}
	for _, pid := range []int{0, -1} {
		if windowsProcessRunning(pid, openProcess, nil, nil) {
			t.Fatalf("windowsProcessRunning(%d) = true, want false", pid)
		}
	}
	if strconv.IntSize > 32 {
		pid := int(int64(^uint32(0)) + 1)
		if windowsProcessRunning(pid, openProcess, nil, nil) {
			t.Fatalf("windowsProcessRunning(%d) = true, want false", pid)
		}
	}
	if openCalled {
		t.Fatal("windowsProcessRunning() opened an invalid PID")
	}
}

// TestDevWindowsProcessRunningClassifiesOpenFailure verifies protected processes retain their locks.
func TestDevWindowsProcessRunningClassifiesOpenFailure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "access denied", err: windows.ERROR_ACCESS_DENIED, want: true},
		{name: "wrapped access denied", err: errors.Join(errors.New("open"), windows.ERROR_ACCESS_DENIED), want: true},
		{name: "missing process", err: windows.ERROR_INVALID_PARAMETER, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			openProcess := func(access uint32, inherit bool, pid uint32) (windows.Handle, error) {
				if access != windows.PROCESS_QUERY_LIMITED_INFORMATION || inherit || pid != 42 {
					t.Fatalf("OpenProcess() arguments = (%d, %t, %d)", access, inherit, pid)
				}
				return 0, test.err
			}
			if got := windowsProcessRunning(42, openProcess, nil, nil); got != test.want {
				t.Fatalf("windowsProcessRunning() = %t, want %t", got, test.want)
			}
		})
	}
}

// TestDevWindowsProcessRunningReadsAndClosesHandle verifies active, exited, and uncertain process states.
func TestDevWindowsProcessRunningReadsAndClosesHandle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		exitCode uint32
		queryErr error
		want     bool
	}{
		{name: "active", exitCode: uint32(windows.STATUS_PENDING), want: true},
		{name: "exited", exitCode: 0, want: false},
		{name: "query failed", queryErr: errors.New("query failed"), want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			const handle windows.Handle = 17
			closed := false
			openProcess := func(access uint32, inherit bool, pid uint32) (windows.Handle, error) {
				if access != windows.PROCESS_QUERY_LIMITED_INFORMATION || inherit || pid != 42 {
					t.Fatalf("OpenProcess() arguments = (%d, %t, %d)", access, inherit, pid)
				}
				return handle, nil
			}
			getExitCode := func(gotHandle windows.Handle, exitCode *uint32) error {
				if gotHandle != handle {
					t.Fatalf("GetExitCodeProcess() handle = %d, want %d", gotHandle, handle)
				}
				*exitCode = test.exitCode
				return test.queryErr
			}
			closeHandle := func(gotHandle windows.Handle) error {
				if gotHandle != handle {
					t.Fatalf("CloseHandle() handle = %d, want %d", gotHandle, handle)
				}
				closed = true
				return nil
			}
			if got := windowsProcessRunning(42, openProcess, getExitCode, closeHandle); got != test.want {
				t.Fatalf("windowsProcessRunning() = %t, want %t", got, test.want)
			}
			if !closed {
				t.Fatal("windowsProcessRunning() did not close its process handle")
			}
		})
	}
}

// TestDevSignalWindowsInterruptUsesCurrentConsoleGroup verifies Ctrl+C reaches DevCmd's console signal listener.
func TestDevSignalWindowsInterruptUsesCurrentConsoleGroup(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("interrupt sent")
	var gotEvent uint32
	var gotGroup uint32
	err := signalWindowsDevInterrupt(func(event uint32, group uint32) error {
		gotEvent = event
		gotGroup = group
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("signalWindowsDevInterrupt() error = %v, want %v", err, wantErr)
	}
	if gotEvent != windows.CTRL_C_EVENT || gotGroup != 0 {
		t.Fatalf("signalWindowsDevInterrupt() target = (%d, %d), want (%d, 0)", gotEvent, gotGroup, windows.CTRL_C_EVENT)
	}
}
