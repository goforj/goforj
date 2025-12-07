package env

import (
	"os"
	"testing"
)

func TestIsHostEnvironment_Host(t *testing.T) {
	t.Cleanup(reset)

	// No container indicators
	statFile = func(path string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}
	readFile = func(path string) ([]byte, error) {
		return []byte(""), nil
	}
	getEnv = func(s string) string { return "" }

	if !IsHostEnvironment() {
		t.Fatalf("expected IsHostEnvironment == true for host")
	}
}
