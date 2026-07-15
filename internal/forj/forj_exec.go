package forj

import (
	"fmt"
	"os"

	"github.com/goforj/goforj/internal/testkit"
)

func currentForjExecutable() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve current forj executable: %w", err)
	}
	if path == "" {
		return "", fmt.Errorf("resolve current forj executable: empty path")
	}
	return path, nil
}

func resolveForjRepoRoot() (string, error) {
	return testkit.RepoRoot()
}

func isForjRepoRoot(dir string) bool {
	root, err := testkit.RepoRoot()
	return err == nil && root == dir
}
