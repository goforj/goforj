package testkit

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// RepoRoot centralizes repo root behavior so callers follow the same contract.
func RepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	if root, ok := findRepoRoot(wd); ok {
		return root, nil
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		if root, found := findRepoRoot(filepath.Dir(file)); found {
			return root, nil
		}
	}
	return "", fmt.Errorf("resolve repo root from %q", wd)
}

// findRepoRoot centralizes find repo root behavior so callers follow the same contract.
func findRepoRoot(start string) (string, bool) {
	dir := start
	for {
		if isRepoRoot(dir) {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

// isRepoRoot centralizes the is repo root decision for its callers.
func isRepoRoot(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "cmd", "forj")); err != nil {
		return false
	}
	return true
}
