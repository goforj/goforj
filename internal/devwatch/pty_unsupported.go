//go:build !linux && !darwin

package devwatch

import (
	"errors"
	"os"
)

// openDevWatchPTY reports that native terminal attachment is unavailable on this platform.
func openDevWatchPTY() (*os.File, *os.File, error) {
	return nil, nil, errors.New("pseudo-terminal output is not supported on this platform")
}
