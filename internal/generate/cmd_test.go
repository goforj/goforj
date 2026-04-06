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

func TestGenerateProjectFilesRunsGoModTidyWhenDBGenerationRuns(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module example.com/test\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "internal", "database"), 0o755); err != nil {
		t.Fatalf("mkdir database dir: %v", err)
	}

	t.Setenv("DB_DRIVER", "mysql")

	called := 0
	orig := goModTidyRunner
	goModTidyRunner = func(dir string) error {
		called++
		if dir != projectDir {
			t.Fatalf("goModTidyRunner dir = %q, want %q", dir, projectDir)
		}
		return nil
	}
	defer func() { goModTidyRunner = orig }()

	total, changed, err := GenerateProjectFiles(projectDir, false, false, false, false, true)
	if err != nil {
		t.Fatalf("GenerateProjectFiles returned error: %v", err)
	}
	if total != 1 {
		t.Fatalf("total files = %d, want %d", total, 1)
	}
	if changed == 0 {
		t.Fatal("expected generated db file to be written")
	}
	if called != 1 {
		t.Fatalf("goModTidyRunner called %d times, want 1", called)
	}
}

func TestGenerateProjectFilesSkipsGoModTidyWhenNothingChanged(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module example.com/test\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "internal", "database"), 0o755); err != nil {
		t.Fatalf("mkdir database dir: %v", err)
	}

	t.Setenv("DB_DRIVER", "mysql")

	if _, err := GenerateDBFiles(projectDir); err != nil {
		t.Fatalf("seed generated db file: %v", err)
	}

	called := 0
	orig := goModTidyRunner
	goModTidyRunner = func(dir string) error {
		called++
		return nil
	}
	defer func() { goModTidyRunner = orig }()

	total, changed, err := GenerateProjectFiles(projectDir, false, false, false, false, true)
	if err != nil {
		t.Fatalf("GenerateProjectFiles returned error: %v", err)
	}
	if total != 1 {
		t.Fatalf("total files = %d, want %d", total, 1)
	}
	if changed != 0 {
		t.Fatalf("changed files = %d, want 0", changed)
	}
	if called != 0 {
		t.Fatalf("goModTidyRunner called %d times, want 0", called)
	}
}

func TestCmdRunRunsGoModTidyWhenDBGenerationRuns(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "internal", "database"), 0o755); err != nil {
		t.Fatalf("mkdir database dir: %v", err)
	}

	t.Setenv("DB_DRIVER", "mysql")

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir root: %v", err)
	}
	defer func() { _ = os.Chdir(origWD) }()

	called := 0
	orig := goModTidyRunner
	goModTidyRunner = func(dir string) error {
		called++
		if dir != "." {
			t.Fatalf("goModTidyRunner dir = %q, want %q", dir, ".")
		}
		return nil
	}
	defer func() { goModTidyRunner = orig }()

	cmd := &Cmd{DB: true}
	if err := cmd.Run(); err != nil {
		t.Fatalf("Cmd.Run returned error: %v", err)
	}
	if called != 1 {
		t.Fatalf("goModTidyRunner called %d times, want 1", called)
	}
}
