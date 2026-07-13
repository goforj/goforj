//go:build windows

package devwatch

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// processTree owns a Windows Job Object that remains capable of terminating descendants after the leader exits.
type processTree struct {
	mu  sync.Mutex
	job windows.Handle
}

// newProcessTree prepares an isolated process group and kill-on-close Job Object before launch.
func newProcessTree(cmd *exec.Cmd) (*processTree, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)),
		uint32(unsafe.Sizeof(limits)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return nil, err
	}
	return &processTree{job: job}, nil
}

// attach assigns the started leader to the Job Object so future descendants inherit containment.
func (t *processTree) attach(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.job == 0 {
		return os.ErrProcessDone
	}
	var assignErr error
	if err := cmd.Process.WithHandle(func(handle uintptr) {
		assignErr = windows.AssignProcessToJobObject(t.job, windows.Handle(handle))
	}); err != nil {
		return err
	}
	return assignErr
}

// terminate requests a graceful CTRL_BREAK from the managed process group.
func (t *processTree) terminate(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(cmd.Process.Pid))
}

// kill forcibly terminates every process retained by the Job Object.
func (t *processTree) kill(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}
	t.mu.Lock()
	job := t.job
	t.mu.Unlock()
	if job != 0 {
		if err := windows.TerminateJobObject(job, 1); err == nil {
			return nil
		}
	}
	return cmd.Process.Kill()
}

// cleanupAfterExit closes the kill-on-close Job Object so descendants cannot outlive their leader.
func (t *processTree) cleanupAfterExit(_ *exec.Cmd) error {
	return t.close()
}

// release closes Job Object resources when process setup does not complete.
func (t *processTree) release() {
	_ = t.close()
}

// close releases the Job Object exactly once while preserving kill-on-close containment.
func (t *processTree) close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.job == 0 {
		return nil
	}
	err := windows.CloseHandle(t.job)
	t.job = 0
	return err
}

// ignoreProcessDoneError treats a process that already exited as a successful stop.
func ignoreProcessDoneError(err error) error {
	if errors.Is(err, os.ErrProcessDone) || errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return nil
	}
	return err
}

// shellArgs selects cmd.exe because it is present on every supported Windows host.
func shellArgs(command string) (string, []string) {
	return "cmd.exe", []string{"/d", "/s", "/c", command}
}
