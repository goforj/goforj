//go:build !windows

package atlas

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// lockEvaluationLease holds one command root until its owner has completed cleanup.
func lockEvaluationLease(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX)
}

// tryLockEvaluationLease distinguishes abandoned roots from work owned by another live command.
func tryLockEvaluationLease(file *os.File) (bool, error) {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return false, nil
	}
	return err == nil, err
}

// unlockEvaluationLease releases the command-root liveness claim before deletion.
func unlockEvaluationLease(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
