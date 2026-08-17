//go:build windows

package atlas

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// lockEvaluationLease holds one command root until its owner has completed cleanup.
func lockEvaluationLease(file *os.File) error {
	var overlapped windows.Overlapped
	return windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &overlapped)
}

// tryLockEvaluationLease distinguishes abandoned roots from work owned by another live command.
func tryLockEvaluationLease(file *os.File) (bool, error) {
	var overlapped windows.Overlapped
	err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return false, nil
	}
	return err == nil, err
}

// unlockEvaluationLease releases the command-root liveness claim before deletion.
func unlockEvaluationLease(file *os.File) error {
	var overlapped windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped)
}
