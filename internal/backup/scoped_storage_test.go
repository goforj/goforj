package backup

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/goforj/storage/driver/localstorage"
)

// scopedStorageContract restricts fixture discovery to the configured local disk.
func scopedStorageContract(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	t.Setenv("FORJ_APP", "app")
	if err := os.Mkdir("bin", 0700); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nprintf '%s\\n' '{\"version\":1,\"app\":\"app\",\"resources\":[{\"id\":\"storage.private\",\"kind\":\"storage\",\"name\":\"private\",\"driver\":\"local\"}]}'\n"
	if err := os.WriteFile("bin/app", []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STORAGE_PRIVATE_DRIVER", "local")
}

// TestSelectedLocalPrefixMatchesAppStorage preserves the local driver's supported prefix syntax on capture and restore.
func TestSelectedLocalPrefixMatchesAppStorage(t *testing.T) {
	for _, prefix := range []string{"/tenant/", " tenant/nested/ ", `tenant\nested`, "/", ""} {
		t.Run(prefix, func(t *testing.T) {
			scopedStorageContract(t)
			root := filepath.Join("storage", "source")
			disk, err := localstorage.New(localstorage.Config{Root: root, Prefix: prefix})
			if err != nil {
				t.Fatal(err)
			}
			if err := disk.Put("note.txt", []byte("App data")); err != nil {
				t.Fatal(err)
			}
			t.Setenv("STORAGE_PRIVATE_ROOT", root)
			t.Setenv("STORAGE_PRIVATE_PREFIX", prefix)
			created, err := NewService().CreateSelected(context.Background(), t.TempDir(), CreateOptions{Resources: []string{"storage.private"}})
			if err != nil {
				t.Fatal(err)
			}
			target := filepath.Join("storage", "target")
			t.Setenv("STORAGE_PRIVATE_ROOT", target)
			t.Setenv("STORAGE_PRIVATE_PREFIX", ` /restored\scope/ `)
			if err := NewService().Restore(context.Background(), created.Directory, "storage.private", "restore-production"); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(target, "restored", "scope", "note.txt"))
			if err != nil || string(data) != "App data" {
				t.Fatalf("restored data = %q, error = %v", data, err)
			}
		})
	}
}

// TestScopedStorageRejectsPrefixSymlinkEscape protects both capture and restore from prefix links outside the App root.
func TestScopedStorageRejectsPrefixSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture requires Unix symlink creation")
	}
	for _, prefix := range []string{"tenant", "tenant/nested"} {
		t.Run(prefix, func(t *testing.T) {
			scopedStorageContract(t)
			source := t.TempDir()
			if err := os.MkdirAll(filepath.Join(source, "tenant"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(source, "tenant", "note.txt"), []byte("backup contents"), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("STORAGE_PRIVATE_ROOT", source)
			t.Setenv("STORAGE_PRIVATE_PREFIX", "tenant")
			created, err := NewService().CreateSelected(context.Background(), t.TempDir(), CreateOptions{Resources: []string{"storage.private"}})
			if err != nil {
				t.Fatal(err)
			}
			root, outside := t.TempDir(), t.TempDir()
			if err := os.MkdirAll(filepath.Join(outside, "nested"), 0700); err != nil {
				t.Fatal(err)
			}
			victim := filepath.Join(outside, "note.txt")
			if prefix == "tenant/nested" {
				victim = filepath.Join(outside, "nested", "note.txt")
			}
			if err := os.WriteFile(victim, []byte("outside original"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(root, "tenant")); err != nil {
				t.Fatal(err)
			}
			t.Setenv("STORAGE_PRIVATE_ROOT", root)
			t.Setenv("STORAGE_PRIVATE_PREFIX", prefix)
			if err := NewService().Restore(context.Background(), created.Directory, "storage.private", "restore-production"); err == nil {
				t.Fatal("restore followed an escaping prefix symlink")
			}
			data, err := os.ReadFile(victim)
			if err != nil || string(data) != "outside original" {
				t.Fatalf("outside file changed: %q, %v", data, err)
			}
			if _, err := NewService().CreateSelected(context.Background(), t.TempDir(), CreateOptions{Resources: []string{"storage.private"}}); err == nil {
				t.Fatal("capture followed an escaping prefix symlink")
			}
		})
	}
}

// TestScopedStorageRejectsInvalidPrefix fails before creating destination directories for invalid logical paths.
func TestScopedStorageRejectsInvalidPrefix(t *testing.T) {
	for _, prefix := range []string{"../outside", "a\x00b"} {
		root := filepath.Join(t.TempDir(), "absent")
		if _, err := openStorageArchiveRoot(root, prefix, true); err == nil {
			t.Fatalf("accepted invalid prefix %q", prefix)
		}
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Fatal("invalid prefix created a destination", err)
		}
	}
}

// TestScopedStorageHandleSurvivesPrefixSwap keeps later I/O confined when the prefix pathname changes after opening.
func TestScopedStorageHandleSurvivesPrefixSwap(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture requires Unix symlink and open-directory rename semantics")
	}
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "tenant"), 0700); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(outside, "note.txt")
	if err := os.WriteFile(victim, []byte("outside original"), 0600); err != nil {
		t.Fatal(err)
	}
	directory, err := openStorageArchiveRoot(root, "tenant", true)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	if err := os.Rename(filepath.Join(root, "tenant"), filepath.Join(root, "original")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "tenant")); err != nil {
		t.Fatal(err)
	}
	file, err := directory.OpenFile("note.txt", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("restored data"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(victim)
	if err != nil || string(data) != "outside original" {
		t.Fatalf("outside path changed: %q, %v", data, err)
	}
	data, err = os.ReadFile(filepath.Join(root, "original", "note.txt"))
	if err != nil || string(data) != "restored data" {
		t.Fatalf("pinned directory changed: %q, %v", data, err)
	}
}

// TestNamedLocalDiskDoesNotInheritDefaultPrefix keeps selection aligned with the generated named disk configuration.
func TestNamedLocalDiskDoesNotInheritDefaultPrefix(t *testing.T) {
	scopedStorageContract(t)
	source := t.TempDir()
	disk, err := localstorage.New(localstorage.Config{Root: source})
	if err != nil {
		t.Fatal(err)
	}
	if err := disk.Put("note.txt", []byte("named disk contents")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STORAGE_PREFIX", "default-only")
	t.Setenv("STORAGE_PRIVATE_PREFIX", "")
	t.Setenv("STORAGE_PRIVATE_ROOT", source)
	plan, err := BuildPlan()
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Storage) != 1 || plan.Storage[0].Prefix != "" {
		t.Fatalf("named storage inherited default prefix: %#v", plan.Storage)
	}
	created, err := NewService().CreateSelected(context.Background(), t.TempDir(), CreateOptions{Resources: []string{"storage.private"}})
	if err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	t.Setenv("STORAGE_PRIVATE_ROOT", target)
	if err := NewService().Restore(context.Background(), created.Directory, "storage.private", "restore-production"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "note.txt"))
	if err != nil || string(data) != "named disk contents" {
		t.Fatalf("named disk restore = %q, %v", data, err)
	}
}
