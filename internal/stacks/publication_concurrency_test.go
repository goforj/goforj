package stacks

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCommitRechecksLaterDestinations preserves edits that arrive while earlier files are being published.
func TestCommitRechecksLaterDestinations(t *testing.T) {
	for _, name := range []string{".env", ".env.stack.services.local"} {
		for _, change := range []string{"edit", "create", "remove", "permissions", "symlink"} {
			t.Run(name+"/"+change, func(t *testing.T) {
				root := t.TempDir()
				if change != "create" {
					put(t, root, name, "DB_PASSWORD=original\n")
				}
				earlier, err := readFile(root, ".env.stack.services")
				if err != nil {
					t.Fatal(err)
				}
				later, err := readFile(root, name)
				if err != nil {
					t.Fatal(err)
				}
				earlier.after = []byte("DB_DRIVER=mysql\n")
				later.after = []byte("DB_PASSWORD=wizard\n")
				err = commit(root, []file{earlier, later}, func(from, to string) error {
					if err := os.Rename(from, to); err != nil {
						return err
					}
					if filepath.Base(to) != earlier.name {
						return nil
					}
					path := filepath.Join(root, name)
					switch change {
					case "edit", "create":
						return os.WriteFile(path, []byte("DB_PASSWORD=concurrent\n"), 0600)
					case "remove":
						return os.Remove(path)
					case "permissions":
						return os.Chmod(path, 0400)
					default:
						put(t, root, "external", "DB_PASSWORD=concurrent\n")
						if err := os.Remove(path); err != nil {
							return err
						}
						return os.Symlink(filepath.Join(root, "external"), path)
					}
				})
				if err == nil {
					t.Fatal("published over a concurrent change")
				}
				if _, err := os.Stat(filepath.Join(root, earlier.name)); !os.IsNotExist(err) {
					t.Fatalf("earlier publication was not rolled back: %v", err)
				}
				path := filepath.Join(root, name)
				switch change {
				case "remove":
					if _, err := os.Stat(path); !os.IsNotExist(err) {
						t.Fatal("recreated removed destination")
					}
				case "permissions":
					assertFileMode(t, root, name, 0400)
				default:
					data, err := os.ReadFile(path)
					if err != nil || string(data) != "DB_PASSWORD=concurrent\n" {
						t.Fatalf("lost concurrent content: %q, %v", data, err)
					}
				}
				leftovers, err := filepath.Glob(filepath.Join(root, ".env.stack-tmp-*.local"))
				if err != nil || len(leftovers) != 0 {
					t.Fatalf("temporary files remain: %v, %v", leftovers, err)
				}
			})
		}
	}
}

// TestRollbackPreservesEditsToPublishedFiles prevents recovery from clobbering a newer private value.
func TestRollbackPreservesEditsToPublishedFiles(t *testing.T) {
	root := t.TempDir()
	ignore, _ := readFile(root, ".gitignore")
	private, _ := readFile(root, ".env.stack.services.local")
	active, _ := readFile(root, ".env")
	ignore.after = []byte(".env.stack.*.local\n.env.stack-tmp-*.local\n")
	private.after = []byte("DB_PASSWORD=wizard\n")
	active.after = []byte("DB_DRIVER=mysql\n")
	err := commit(root, []file{ignore, private, active}, func(from, to string) error {
		if filepath.Base(to) == active.name {
			put(t, root, private.name, "DB_PASSWORD=concurrent\n")
			return errors.New("injected active publication failure")
		}
		return os.Rename(from, to)
	})
	if err == nil || !strings.Contains(err.Error(), "changed while the wizard was open") {
		t.Fatalf("expected a rollback conflict, got %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, private.name))
	if err != nil || string(data) != "DB_PASSWORD=concurrent\n" {
		t.Fatalf("rollback lost the newer private edit: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(root, ignore.name)); err != nil {
		t.Fatalf("removed ignore rules protecting the retained private file: %v", err)
	}
}

// TestPrivatePublicationConflictKeepsIgnoreRules protects a private file created by another writer after ignore publication.
func TestPrivatePublicationConflictKeepsIgnoreRules(t *testing.T) {
	root := t.TempDir()
	runStackGit(t, root, "init", "-q")
	ignore, _ := readFile(root, ".gitignore")
	private, _ := readFile(root, ".env.stack.services.local")
	ignore.after = []byte(".env.stack.*.local\n.env.stack-tmp-*.local\n")
	private.after = []byte("DB_PASSWORD=wizard\n")
	err := commit(root, []file{ignore, private}, func(from, to string) error {
		if err := os.Rename(from, to); err != nil {
			return err
		}
		if filepath.Base(to) == ignore.name {
			put(t, root, private.name, "DB_PASSWORD=concurrent\n")
		}
		return nil
	})
	if err == nil {
		t.Fatal("overwrote a concurrently created private file")
	}
	data, err := os.ReadFile(filepath.Join(root, private.name))
	if err != nil || string(data) != "DB_PASSWORD=concurrent\n" {
		t.Fatal("concurrent private file was not preserved")
	}
	runStackGit(t, root, "check-ignore", "-q", private.name)
}
