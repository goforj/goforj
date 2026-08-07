package testkit

import (
	"fmt"
	"os"
	"strings"
)

// TempRoot centralizes temp root behavior so callers follow the same contract.
func TempRoot(envKey string) (string, error) {
	if override := strings.TrimSpace(os.Getenv(envKey)); override != "" {
		if err := os.MkdirAll(override, 0o755); err != nil {
			return "", fmt.Errorf("create %s: %w", envKey, err)
		}
		return override, nil
	}

	root := os.TempDir()
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("create os temp dir: %w", err)
	}
	return root, nil
}
