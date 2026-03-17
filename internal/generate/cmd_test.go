package generate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateProjectFilesUsesPluralServicePackageDirs(t *testing.T) {
	projectDir := t.TempDir()

	for _, dir := range []string{
		filepath.Join(projectDir, "internal", "caches"),
		filepath.Join(projectDir, "internal", "queues"),
		filepath.Join(projectDir, "internal", "storages"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	t.Setenv("CACHE_DRIVER", "memory")
	t.Setenv("QUEUE_DRIVER", "null")
	t.Setenv("STORAGE_DRIVER", "local")

	total, changed, err := GenerateProjectFiles(projectDir, true, true, true, false, false)
	if err != nil {
		t.Fatalf("GenerateProjectFiles returned error: %v", err)
	}
	if total != 6 {
		t.Fatalf("total files = %d, want %d", total, 6)
	}
	if changed == 0 {
		t.Fatal("expected generated files to be written")
	}

	for _, path := range []string{
		filepath.Join(projectDir, "internal", "caches", "manager_gen.go"),
		filepath.Join(projectDir, "internal", "queues", "manager_gen.go"),
		filepath.Join(projectDir, "internal", "storages", "manager_gen.go"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
}
