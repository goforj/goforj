package stacks

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// verifyPrivateFileUntracked permits non-Git projects but requires a successful index inspection whenever Git context exists.
func verifyPrivateFileUntracked(root, name string) error {
	configured, err := hasGitDirectory(root)
	if err != nil {
		return fmt.Errorf("inspect Git context for %s: %w", name, err)
	}
	if !configured {
		explicit := false
		for _, key := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE"} {
			if _, set := os.LookupEnv(key); set {
				explicit = true
			}
		}
		if !explicit {
			return nil
		}
	}
	command := exec.Command("git", "ls-files", "--cached", "-z", "--", name)
	command.Dir = root
	if configured {
		// Git subprocess overrides must not redirect this check away from the project's real index.
		command.Env = []string{}
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, "GIT_") {
				command.Env = append(command.Env, entry)
			}
		}
	}
	tracked, err := command.Output()
	if err != nil {
		return fmt.Errorf("cannot verify Git tracking for %s: %w", name, err)
	}
	if len(tracked) != 0 {
		return fmt.Errorf("%s is tracked by Git; remove it from the index before storing private stack settings", name)
	}
	return nil
}

// hasGitDirectory recognizes normal repositories and linked worktrees through physical ancestors without relying on Git being executable.
func hasGitDirectory(root string) (bool, error) {
	directory, err := filepath.Abs(root)
	if err != nil {
		return false, err
	}
	directory, err = filepath.EvalSymlinks(directory)
	if err != nil {
		return false, err
	}
	for {
		if _, err := os.Lstat(filepath.Join(directory, ".git")); err == nil {
			return true, nil
		} else if !os.IsNotExist(err) {
			return false, err
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return false, nil
		}
		directory = parent
	}
}
