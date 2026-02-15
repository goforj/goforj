//go:build darwin

package forj

import "golang.org/x/sys/unix"

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
