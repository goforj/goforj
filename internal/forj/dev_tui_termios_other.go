//go:build !linux && !darwin

package forj

import "fmt"

func setTTYSingleKeyMode(fd int) (func(), error) {
	return nil, fmt.Errorf("single-key tty mode not supported on this platform")
}

func drainTTYInput(fd int) {}
