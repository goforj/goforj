//go:build unix

package devwatch

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// processTree addresses a managed Unix process through its isolated process group.
type processTree struct{}

// newProcessTree gives each managed command an isolated process group before it starts.
func newProcessTree(cmd *exec.Cmd) (*processTree, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return &processTree{}, nil
}

// attach is a no-op because Unix process-group membership is established atomically at process creation.
func (t *processTree) attach(_ *exec.Cmd) error {
	return nil
}

// terminate sends SIGTERM to the managed process group.
func (t *processTree) terminate(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
}

// kill sends SIGKILL to the managed process group.
func (t *processTree) kill(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if err != nil && !errors.Is(err, syscall.ESRCH) {
		return errors.Join(err, cmd.Process.Kill())
	}
	return err
}

// cleanupAfterExit removes descendants that outlive the managed process-group leader.
func (t *processTree) cleanupAfterExit(cmd *exec.Cmd) error {
	return ignoreProcessDoneError(t.kill(cmd))
}

// release is a no-op because Unix process groups do not hold supervisor-owned resources.
func (t *processTree) release() {}

// ignoreProcessDoneError treats a process that already exited as a successful stop.
func ignoreProcessDoneError(err error) error {
	if errors.Is(err, syscall.ESRCH) || errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}

// shellArgs selects the conventional POSIX shell for compatibility commands.
func shellArgs(command string) (string, []string) {
	return "sh", []string{"-c", command}
}
