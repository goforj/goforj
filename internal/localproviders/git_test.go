package localproviders

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runLocalGit prepares real repository state for private-file protection tests.
func runLocalGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s: %v", args, output, err)
	}
}

// TestPrivateFileGitContexts covers non-Git projects, inherited repositories, and linked worktrees.
func TestPrivateFileGitContexts(t *testing.T) {
	t.Run("non-Git without executable", func(t *testing.T) {
		root := fixture(t)
		t.Setenv("PATH", t.TempDir())
		s := openTest(t, root)
		values, err := s.Portable()
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Apply(values); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("explicit invalid context", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("GIT_DIR", filepath.Join(root, "missing"))
		if err := verifyPrivateFileUntracked(root, ".env.local-providers.backup"); err == nil {
			t.Fatal("ignored explicit Git context")
		}
	})
	t.Run("unreadable context path", func(t *testing.T) {
		root := t.TempDir()
		put(t, root, "file", "")
		if err := verifyPrivateFileUntracked(filepath.Join(root, "file", "project"), ".env.local-providers.backup"); err == nil || !strings.Contains(err.Error(), "inspect Git context") {
			t.Fatalf("ignored filesystem inspection error: %v", err)
		}
	})
	t.Run("valid explicit worktree", func(t *testing.T) {
		root := fixture(t)
		repository := t.TempDir()
		runLocalGit(t, repository, "init", "--bare", "-q")
		t.Setenv("GIT_DIR", repository)
		t.Setenv("GIT_WORK_TREE", root)
		s := openTest(t, root)
		values, err := s.Portable()
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Apply(values); err != nil {
			t.Fatal(err)
		}
		runLocalGit(t, root, "add", "-f", ".env.local-providers.backup")
		s = openTest(t, root)
		if err := s.Restore(); err == nil || !strings.Contains(err.Error(), "is tracked") {
			t.Fatalf("missed tracked file in explicit repository: %v", err)
		}
	})
	for _, layout := range []string{"ancestor", "worktree", "symlinked subdirectory"} {
		t.Run(layout, func(t *testing.T) {
			repository := t.TempDir()
			runLocalGit(t, repository, "init", "-q")
			root := filepath.Join(repository, "project")
			if layout == "worktree" {
				runLocalGit(t, repository, "-c", "user.name=Local Test", "-c", "user.email=local@example.org", "commit", "--allow-empty", "-qm", "test: initialize fixture")
				runLocalGit(t, repository, "worktree", "add", "-q", "-b", "fixture", root)
			} else if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			name := ".env.local-providers.backup"
			put(t, root, name, "")
			if err := verifyPrivateFileUntracked(root, name); err != nil {
				t.Fatal(err)
			}
			runLocalGit(t, root, "add", name)
			if layout == "symlinked subdirectory" {
				alias := filepath.Join(t.TempDir(), "project")
				if err := os.Symlink(root, alias); err != nil {
					t.Fatal(err)
				}
				root = alias
			}
			if err := verifyPrivateFileUntracked(root, name); err == nil || !strings.Contains(err.Error(), "is tracked") {
				t.Fatalf("missed tracked private file: %v", err)
			}
		})
	}
}
