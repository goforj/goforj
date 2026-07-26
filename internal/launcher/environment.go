// Package launcher retains process state captured at the CLI boundary for later command execution.
package launcher

import (
	"os"
	"strings"
	"sync"
)

var (
	environmentMutex  sync.RWMutex
	environmentValues = captureProcessEnvironment()
)

// Capture replaces the launcher environment with the current process values.
func Capture() {
	set(captureProcessEnvironment())
}

// set replaces the launcher environment with a defensive copy.
func set(values map[string]string) {
	environmentMutex.Lock()
	defer environmentMutex.Unlock()
	environmentValues = copyValues(values)
}

// Snapshot returns a defensive copy of the environment captured at the CLI boundary.
func Snapshot() map[string]string {
	environmentMutex.RLock()
	defer environmentMutex.RUnlock()
	return copyValues(environmentValues)
}

// captureProcessEnvironment seeds callers that execute runtime commands outside the CLI entry point.
func captureProcessEnvironment() map[string]string {
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok && key != "" {
			values[key] = value
		}
	}
	return values
}

// copyValues prevents callers from mutating the shared launcher's backing map.
func copyValues(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	copy := make(map[string]string, len(values))
	for key, value := range values {
		copy[key] = value
	}
	return copy
}
