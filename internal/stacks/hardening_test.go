package stacks

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStackParseErrorsDoNotExposeCredentials exercises error redaction across active, saved, private, and ancestor dotenv inputs.
func TestStackParseErrorsDoNotExposeCredentials(t *testing.T) {
	for _, name := range []string{".env", ".env.example", ".env.stack.broken", ".env.stack.broken.local", "../.env.production"} {
		t.Run(name, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".env.stack.broken", "DB_DRIVER=mysql\n")
			put(t, root, name, "DB_PASSWORD='must-never-appear-in-errors")
			s, err := Open(root)
			if err == nil {
				if strings.Contains(name, "broken") {
					_, err = s.Load("broken", true)
				} else {
					_, err = s.Overrides()
				}
			}
			if err == nil || !strings.Contains(err.Error(), "invalid dotenv document") || strings.Contains(err.Error(), "must-never-appear") {
				t.Fatalf("expected a redacted parse error, got %v", err)
			}
		})
	}
}

// TestPrivateStackWritesRestrictExistingPermissions exercises save, recovery, and kept working copies with permissive existing destinations.
func TestPrivateStackWritesRestrictExistingPermissions(t *testing.T) {
	root := fixture(t)
	if err := os.Chmod(filepath.Join(root, ".env"), 0640); err != nil {
		t.Fatal(err)
	}
	private := ".env.stack.services.local"
	put(t, root, private, "# existing file\n")
	if err := os.Chmod(filepath.Join(root, private), 0644); err != nil {
		t.Fatal(err)
	}
	s := openTest(t, root)
	if err := s.Save("services", s.Current); err != nil {
		t.Fatal(err)
	}
	assertFileMode(t, root, private, 0600)
	s = openTest(t, root)
	if err := s.Activate("services", s.Current, false); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{private, stateName} {
		if err := os.Chmod(filepath.Join(root, name), 0664); err != nil {
			t.Fatal(err)
		}
	}
	put(t, root, ".env", "DB_DRIVER=mysql\nDB_PASSWORD=changed-private-value\n")
	s = openTest(t, root)
	portable, err := s.Portable()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Activate("", portable, true); err != nil {
		t.Fatal(err)
	}
	assertFileMode(t, root, ".env", 0640)
	for _, name := range []string{private, stateName} {
		assertFileMode(t, root, name, 0600)
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || !bytes.Contains(data, []byte("changed-private-value")) {
			t.Fatalf("missing saved credential in %s: %v", name, err)
		}
	}
}

// TestPrivatePermissionHardeningRespectsReadOnlyFiles prevents permission tightening from bypassing the owner's write protection.
func TestPrivatePermissionHardeningRespectsReadOnlyFiles(t *testing.T) {
	root := fixture(t)
	name := ".env.stack.services.local"
	put(t, root, name, "original")
	if err := os.Chmod(filepath.Join(root, name), 0400); err != nil {
		t.Fatal(err)
	}
	s := openTest(t, root)
	if err := s.Save("services", s.Current); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("expected read-only rejection, got %v", err)
	}
	assertFileMode(t, root, name, 0400)
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil || string(data) != "original" {
		t.Fatalf("failed save changed private contents: %v", err)
	}
}

// assertFileMode checks the published file rather than a permission value prepared in memory.
func assertFileMode(t *testing.T, root, name string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != want {
		t.Fatalf("%s mode = %04o, want %04o", name, info.Mode().Perm(), want)
	}
}

// TestPrivatePermissionsRollbackPreservesOriginalState keeps temporary secrets restricted while restoring an unsuccessful transaction exactly.
func TestPrivatePermissionsRollbackPreservesOriginalState(t *testing.T) {
	root := t.TempDir()
	name := ".env.stack.services.local"
	put(t, root, name, "original")
	if err := os.Chmod(filepath.Join(root, name), 0644); err != nil {
		t.Fatal(err)
	}
	put(t, root, ".env", "unchanged")
	private, err := readFile(root, name)
	if err != nil {
		t.Fatal(err)
	}
	active, err := readFile(root, ".env")
	if err != nil {
		t.Fatal(err)
	}
	private.after, active.after = []byte("new secret"), []byte("new active")
	err = commit(root, []file{private, active}, func(from, to string) error {
		if filepath.Base(to) == ".env" {
			return errors.New("injected publication failure")
		}
		assertFileMode(t, filepath.Dir(from), filepath.Base(from), 0600)
		return os.Rename(from, to)
	})
	if err == nil {
		t.Fatal("expected publication failure")
	}
	assertFileMode(t, root, name, 0644)
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil || string(data) != "original" {
		t.Fatalf("rollback failed: %v", err)
	}
}

// TestKeepWorkingCopyRejectsConcurrentChanges covers every destination transition while the activation prompt is open.
func TestKeepWorkingCopyRejectsConcurrentChanges(t *testing.T) {
	for _, change := range []string{"create", "edit", "remove", "chmod"} {
		t.Run(change, func(t *testing.T) {
			root := fixture(t)
			s := openTest(t, root)
			if err := s.Save("services", s.Current); err != nil {
				t.Fatal(err)
			}
			s = openTest(t, root)
			if err := s.Activate("services", s.Current, false); err != nil {
				t.Fatal(err)
			}
			put(t, root, ".env", "DB_DRIVER=mysql\nDB_PASSWORD=manual-change\n")
			private := filepath.Join(root, ".env.stack.services.local")
			if change == "create" {
				if err := os.Remove(private); err != nil {
					t.Fatal(err)
				}
			}
			s = openTest(t, root)
			portable, err := s.Portable()
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "create", "edit":
				put(t, root, filepath.Base(private), "DB_PASSWORD=external-refresh\n")
			case "remove":
				if err := os.Remove(private); err != nil {
					t.Fatal(err)
				}
			case "chmod":
				if err := os.Chmod(private, 0644); err != nil {
					t.Fatal(err)
				}
			}
			before, err := readFile(root, filepath.Base(private))
			if err != nil {
				t.Fatal(err)
			}
			err = s.Activate("portable", portable, true)
			if err == nil || !strings.Contains(err.Error(), "changed while") {
				t.Fatalf("expected concurrent-edit rejection, got %v", err)
			}
			after, err := readFile(root, filepath.Base(private))
			if err != nil || before.exists != after.exists || before.mode != after.mode || !bytes.Equal(before.before, after.before) {
				t.Fatalf("changed private destination: %v", err)
			}
			for _, name := range []string{".env", stateName} {
				data, err := os.ReadFile(filepath.Join(root, name))
				if err != nil || !bytes.Equal(data, s.files[name].before) {
					t.Fatalf("failed activation changed %s: %v", name, err)
				}
			}
		})
	}
}

// TestOpenRejectsUnsafeActiveWorkingCopy prevents prompts from offering to overwrite a non-regular private destination.
func TestOpenRejectsUnsafeActiveWorkingCopy(t *testing.T) {
	root := fixture(t)
	s := openTest(t, root)
	if err := s.Activate("services", s.Current, false); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".env.stack.services.local"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(root); err == nil {
		t.Fatal("accepted a directory as the active working copy")
	}
}
