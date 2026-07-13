//go:build linux

package devwatch

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// openDevWatchPTY opens one Linux pseudo-terminal pair for merged child output.
func openDevWatchPTY() (*os.File, *os.File, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, err
	}
	fd := master.Fd()
	unlock := int32(0)
	if err := devWatchPTYIoctl(fd, syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); err != nil {
		_ = master.Close()
		return nil, nil, err
	}
	var ptyNumber uint32
	if err := devWatchPTYIoctl(fd, syscall.TIOCGPTN, uintptr(unsafe.Pointer(&ptyNumber))); err != nil {
		_ = master.Close()
		return nil, nil, err
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", ptyNumber), os.O_RDWR|syscall.O_NOCTTY, 0)
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
