//go:build linux

package forj

import "golang.org/x/sys/unix"

func setTTYSingleKeyMode(fd int) (func(), error) {
	orig, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return nil, err
	}
	next := *orig
	next.Lflag &^= unix.ICANON | unix.ECHO
	next.Cc[unix.VMIN] = 1
	next.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, &next); err != nil {
		return nil, err
	}
	return func() {
		_ = unix.IoctlSetTermios(fd, unix.TCSETS, orig)
	}, nil
}
