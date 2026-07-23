// Package launcher retains process state captured at the CLI boundary for later command execution.
package launcher

import (
	"os"
	"strings"
	"sync"
)

// Environment stores a launcher environment snapshot without exposing its mutable backing map.
type Environment struct {
	mutex  sync.RWMutex
	values map[string]string
}

// sharedEnvironment is the single launcher snapshot shared by CLI initialization and Wire.
var sharedEnvironment = &Environment{
	values: captureProcessEnvironment(),
}

// Provide returns the shared launcher environment dependency for application wiring.
func Provide() *Environment {
	return sharedEnvironment
}

// Set replaces the stored environment with a defensive copy of values.
func (environment *Environment) Set(values map[string]string) {
	environment.mutex.Lock()
	defer environment.mutex.Unlock()
	environment.values = copyValues(values)
}

// Snapshot returns a defensive copy of the launcher environment captured at initialization.
func (environment *Environment) Snapshot() map[string]string {
	environment.mutex.RLock()
	defer environment.mutex.RUnlock()
	return copyValues(environment.values)
}

// captureProcessEnvironment seeds compatibility callers that initialize Wire outside the CLI entry point.
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
