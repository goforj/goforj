//go:build !linux && !darwin

package forj

import "fmt"

// setTTYSingleKeyMode centralizes set ttysingle key mode behavior so callers follow the same contract.
func setTTYSingleKeyMode(fd int) (func(), error) {
	return nil, fmt.Errorf("single-key tty mode not supported on this platform")
}

// drainTTYInput centralizes drain ttyinput behavior so callers follow the same contract.
func drainTTYInput(fd int) {}
