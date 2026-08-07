//go:build darwin

package forj

import "golang.org/x/sys/unix"

// setTTYSingleKeyMode centralizes set ttysingle key mode behavior so callers follow the same contract.
func setTTYSingleKeyMode(fd int) (func(), error) {
	orig, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return nil, err
	}
	next := *orig
	next.Lflag &^= unix.ICANON | unix.ECHO
	next.Cc[unix.VMIN] = 1
	next.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, unix.TIOCSETA, &next); err != nil {
		return nil, err
	}
	return func() {
		_ = unix.IoctlSetTermios(fd, unix.TIOCSETA, orig)
	}, nil
}

// drainTTYInput centralizes drain ttyinput behavior so callers follow the same contract.
func drainTTYInput(fd int) {
	if err := unix.SetNonblock(fd, true); err != nil {
		return
	}
	defer func() {
		_ = unix.SetNonblock(fd, false)
	}()

	buf := make([]byte, 256)
	for {
		_, err := unix.Read(fd, buf)
		if err == nil {
			continue
		}
		if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
			return
		}
		return
	}
}
