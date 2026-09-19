package stacks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runStackGit prepares real repository state for private-file protection tests.
func runStackGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s: %v", args, output, err)
	}
}

// TestSaveRequiresSuccessfulGitInspection prevents repository failures from authorizing writes of private settings.
func TestSaveRequiresSuccessfulGitInspection(t *testing.T) {
	for _, failure := range []string{"invalid directory", "alternate index", "alternate repository", "missing executable", "corrupt index", "broken worktree"} {
		t.Run(failure, func(t *testing.T) {
			root := fixture(t)
			runStackGit(t, root, "init", "-q")
			name := ".env.stack.services.local"
			put(t, root, name, "")
			runStackGit(t, root, "add", name)
			s := openTest(t, root)
			switch failure {
			case "invalid directory":
				t.Setenv("GIT_DIR", filepath.Join(root, "missing"))
			case "alternate index":
				t.Setenv("GIT_INDEX_FILE", filepath.Join(root, "new-index"))
			case "alternate repository":
				other := t.TempDir()
				runStackGit(t, other, "init", "-q")
				t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
				t.Setenv("GIT_WORK_TREE", root)
			case "missing executable":
				t.Setenv("PATH", t.TempDir())
			case "corrupt index":
				put(t, root, ".git/index", "invalid index")
			case "broken worktree":
				if err := os.Rename(filepath.Join(root, ".git"), filepath.Join(root, "original-git")); err != nil {
					t.Fatal(err)
				}
				put(t, root, ".git", "gitdir: missing\n")
			}
			err := s.Save("services", s.Current)
			if err == nil || (!strings.Contains(err.Error(), "cannot verify Git tracking") && !strings.Contains(err.Error(), "is tracked")) {
				t.Fatalf("expected failed verification, got %v", err)
			}
			if content, err := os.ReadFile(filepath.Join(root, name)); err != nil || len(content) != 0 {
				t.Fatalf("private file changed: %q, %v", content, err)
			}
			for _, name := range []string{".env.stack.services", ".gitignore", stateName} {
				if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
					t.Fatalf("published %s before Git verification: %v", name, err)
				}
			}
		})
	}
}

// TestPrivateFileGitContexts covers non-Git projects, inherited repositories, and linked worktrees.
func TestPrivateFileGitContexts(t *testing.T) {
	t.Run("non-Git without executable", func(t *testing.T) {
		root := fixture(t)
		t.Setenv("PATH", t.TempDir())
		s := openTest(t, root)
		if err := s.Save("services", s.Current); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("explicit invalid context", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("GIT_DIR", filepath.Join(root, "missing"))
		if err := verifyPrivateFileUntracked(root, ".env.stack.services.local"); err == nil {
			t.Fatal("ignored explicit Git context")
		}
	})
	t.Run("unreadable context path", func(t *testing.T) {
		root := t.TempDir()
		put(t, root, "file", "")
		if err := verifyPrivateFileUntracked(filepath.Join(root, "file", "project"), ".env.stack.services.local"); err == nil || !strings.Contains(err.Error(), "inspect Git context") {
			t.Fatalf("ignored filesystem inspection error: %v", err)
		}
	})
	t.Run("valid explicit worktree", func(t *testing.T) {
		root := fixture(t)
		repository := t.TempDir()
		runStackGit(t, repository, "init", "--bare", "-q")
		t.Setenv("GIT_DIR", repository)
		t.Setenv("GIT_WORK_TREE", root)
		s := openTest(t, root)
		if err := s.Save("services", s.Current); err != nil {
			t.Fatal(err)
		}
		runStackGit(t, root, "add", "-f", ".env.stack.services.local")
		s = openTest(t, root)
		if err := s.Save("services", s.Current); err == nil || !strings.Contains(err.Error(), "is tracked") {
			t.Fatalf("missed tracked file in explicit repository: %v", err)
		}
	})
	for _, layout := range []string{"ancestor", "worktree", "symlinked subdirectory"} {
		t.Run(layout, func(t *testing.T) {
			repository := t.TempDir()
			runStackGit(t, repository, "init", "-q")
			root := filepath.Join(repository, "project")
			if layout == "worktree" {
				runStackGit(t, repository, "-c", "user.name=Stack Test", "-c", "user.email=stack@example.org", "commit", "--allow-empty", "-qm", "test: initialize fixture")
				runStackGit(t, repository, "worktree", "add", "-q", "-b", "fixture", root)
			} else if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			name := ".env.stack.services.local"
			put(t, root, name, "")
			if err := verifyPrivateFileUntracked(root, name); err != nil {
				t.Fatal(err)
			}
			runStackGit(t, root, "add", name)
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

// TestTrackedRecoveryFileBlocksActivation prevents secrets in the recovery snapshot from entering an already tracked file.
func TestTrackedRecoveryFileBlocksActivation(t *testing.T) {
	root := fixture(t)
	runStackGit(t, root, "init", "-q")
	put(t, root, stateName, `{"version":1}`)
	runStackGit(t, root, "add", stateName)
	s := openTest(t, root)
	values, err := s.Portable()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Activate("", values, false); err == nil || !strings.Contains(err.Error(), "is tracked") {
		t.Fatalf("tracked recovery state accepted: %v", err)
	}
	fresh := openTest(t, root)
	if !equalValues(fresh.Current, s.Current) || fresh.HasPrevious() {
		t.Fatal("failed Git verification changed the active configuration or recovery state")
	}
}

// TestTrackedLocalDefinitionRemainsEditable distinguishes the public local definition from its private working copy.
func TestTrackedLocalDefinitionRemainsEditable(t *testing.T) {
	root := fixture(t)
	runStackGit(t, root, "init", "-q")
	s := openTest(t, root)
	if err := s.Save("local", s.Current); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(root, ".env.stack.local"), 0644); err != nil {
		t.Fatal(err)
	}
	runStackGit(t, root, "add", ".env.stack.local")
	s = openTest(t, root)
	values, err := s.Portable()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save("local", values); err != nil {
		t.Fatal(err)
	}
	assertFileMode(t, root, ".env.stack.local", 0644)
	assertFileMode(t, root, ".env.stack.local.local", 0600)
	runStackGit(t, root, "check-ignore", "-q", ".env.stack.local.local")
	s = openTest(t, root)
	loaded, err := s.Load("local", true)
	if err != nil || loaded["DB_DRIVER"] != "sqlite" {
		t.Fatalf("replacement did not persist: %v, %v", loaded, err)
	}
	runStackGit(t, root, "add", "-f", ".env.stack.local.local")
	if err := s.Save("local", values); err == nil || !strings.Contains(err.Error(), "is tracked") {
		t.Fatalf("private local working copy was not protected: %v", err)
	}
}
