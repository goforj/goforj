package stacks

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestCommitProtectsPrivateTemporaryFiles exercises concurrent Git staging during first publication and failure recovery.
func TestCommitProtectsPrivateTemporaryFiles(t *testing.T) {
	for _, outcome := range []string{"success", "rename failure", "restore failure", "cleanup failure"} {
		t.Run(outcome, func(t *testing.T) {
			root := t.TempDir()
			runStackGit(t, root, "init", "-q")
			put(t, root, ".env", "APP_NAME=original\n")
			ignore, err := readFile(root, ".gitignore")
			if err != nil {
				t.Fatal(err)
			}
			ignore.after = []byte(".env.stack.*.local\n.env.stack-tmp-*.local\n")
			private, err := readFile(root, ".env.stack.services.local")
			if err != nil {
				t.Fatal(err)
			}
			secret := "synthetic-stack-publication-secret"
			private.after = []byte("DB_PASSWORD=" + secret + "\n")
			active, err := readFile(root, ".env")
			if err != nil {
				t.Fatal(err)
			}
			active.after = []byte("APP_NAME=changed\n")
			leftover := ""
			err = commit(root, []file{active, private, ignore}, func(from, to string) error {
				assertStackSecretUntracked(t, root, secret)
				if filepath.Base(to) != ".env" || outcome == "success" {
					return os.Rename(from, to)
				}
				if outcome == "restore failure" || outcome == "cleanup failure" {
					leftover = filepath.Join(root, private.name)
					if outcome == "cleanup failure" {
						leftover = from
					}
					if err := os.Remove(leftover); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(leftover, 0700); err != nil {
						t.Fatal(err)
					}
					put(t, leftover, "retained", secret)
				}
				return errors.New("injected publication failure")
			})
			if (err == nil) != (outcome == "success") {
				t.Fatalf("unexpected commit result: %v", err)
			}
			if outcome == "rename failure" {
				for _, name := range []string{ignore.name, private.name} {
					if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
						t.Fatalf("rollback retained %s: %v", name, err)
					}
				}
			}
			if leftover != "" {
				if _, err := os.Stat(filepath.Join(root, ".gitignore")); err != nil {
					t.Fatalf("removed ignore protection after incomplete rollback: %v", err)
				}
				runStackGit(t, root, "check-ignore", "-q", leftover)
			}
			assertStackSecretUntracked(t, root, secret)
		})
	}
}

// assertStackSecretUntracked simulates an owner staging everything while a Stack write is in progress.
func assertStackSecretUntracked(t *testing.T, root, secret string) {
	t.Helper()
	runStackGit(t, root, "add", "-A")
	command := exec.Command("git", "grep", "--cached", "-q", "--", secret)
	command.Dir = root
	err := command.Run()
	var status *exec.ExitError
	if !errors.As(err, &status) || status.ExitCode() != 1 {
		t.Fatalf("private value entered Git's index or inspection failed: %v", err)
	}
}
