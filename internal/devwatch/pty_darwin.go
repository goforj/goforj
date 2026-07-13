//go:build darwin

package devwatch

import (
	"bytes"
	"os"
	"syscall"
	"unsafe"
)

// openDevWatchPTY opens one Darwin pseudo-terminal pair for merged child output.
func openDevWatchPTY() (*os.File, *os.File, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, err
	}
	if err := devWatchPTYIoctl(master.Fd(), syscall.TIOCPTYGRANT, 0); err != nil {
		_ = master.Close()
		return nil, nil, err
	}
	if err := devWatchPTYIoctl(master.Fd(), syscall.TIOCPTYUNLK, 0); err != nil {
		_ = master.Close()
		return nil, nil, err
	}
	var nameBuffer [128]byte
	if err := devWatchPTYIoctl(master.Fd(), syscall.TIOCPTYGNAME, uintptr(unsafe.Pointer(&nameBuffer[0]))); err != nil {
		_ = master.Close()
		return nil, nil, err
	}
	name := string(bytes.TrimRight(nameBuffer[:], "\x00"))
	slave, err := os.OpenFile(name, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		_ = master.Close()
		return nil, nil, err
	}
	return master, slave, nil
}

// devWatchPTYIoctl applies the platform terminal control operation without hiding errno.
func devWatchPTYIoctl(fd uintptr, request uintptr, argument uintptr) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, request, argument)
	if errno != 0 {
		return errno
	}
	return nil
}
