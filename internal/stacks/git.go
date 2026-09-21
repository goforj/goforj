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
	explicit := false
	for _, key := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE"} {
		if _, set := os.LookupEnv(key); set {
			explicit = true
		}
	}
	if configured {
		if err := inspectPrivateFileIndex(root, name, false, false); err != nil {
			return err
		}
	}
	if explicit {
		if err := inspectPrivateFileIndex(root, name, true, false); err != nil {
			return err
		}
		if _, selected := os.LookupEnv("GIT_INDEX_FILE"); selected {
			return inspectPrivateFileIndex(root, name, true, true)
		}
	}
	return nil
}

// inspectPrivateFileIndex checks a normal or explicitly selected index without allowing unrelated Git configuration overrides to redirect inspection.
func inspectPrivateFileIndex(root, name string, external, alternate bool) error {
	command := exec.Command("git", "ls-files", "--cached", "-z", "--", name)
	command.Dir = root
	command.Env = []string{}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		location := key == "GIT_DIR" || key == "GIT_WORK_TREE" || key == "GIT_COMMON_DIR"
		if !strings.HasPrefix(key, "GIT_") || (external && location) || (alternate && key == "GIT_INDEX_FILE") {
			command.Env = append(command.Env, entry)
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
