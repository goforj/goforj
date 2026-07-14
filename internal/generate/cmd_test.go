package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateProjectFilesUsesPluralServicePackageDirs(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module example.com/test\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	for _, dir := range []string{
		filepath.Join(projectDir, "internal", "caches"),
		filepath.Join(projectDir, "internal", "mail"),
		filepath.Join(projectDir, "internal", "queues"),
		filepath.Join(projectDir, "internal", "runtime"),
		filepath.Join(projectDir, "internal", "storages"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	writeQueueRuntimeFixture(t, projectDir)
	if err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte(strings.Join([]string{
		"CACHE_DRIVER=memory",
		"CACHE_SUPPORTED_DRIVERS=memory",
		"MAIL_DRIVER=log",
		"MAIL_SUPPORTED_DRIVERS=log",
		"QUEUE_DRIVER=null",
		"QUEUE_SUPPORTED_DRIVERS=null",
		"STORAGE_DRIVER=local",
		"STORAGE_SUPPORTED_DRIVERS=local",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write environment: %v", err)
	}

	orig := goModTidyRunner
	goModTidyRunner = func(dir string) error { return nil }
	defer func() { goModTidyRunner = orig }()

	total, changed, err := GenerateProjectFiles(projectDir, true, true, true, false, false, false)
	if err != nil {
		t.Fatalf("GenerateProjectFiles returned error: %v", err)
	}
	if total != 8 {
		t.Fatalf("total files = %d, want %d", total, 8)
	}
	if changed == 0 {
		t.Fatal("expected generated files to be written")
	}

	for _, path := range []string{
		filepath.Join(projectDir, "internal", "caches", "manager_gen.go"),
		filepath.Join(projectDir, "internal", "mail", "manager_gen.go"),
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

	if err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte("DB_DRIVER=mysql\nDB_SUPPORTED_DRIVERS=mysql\n"), 0o644); err != nil {
		t.Fatalf("write environment: %v", err)
	}

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

	total, changed, err := GenerateProjectFiles(projectDir, false, false, false, false, true, false)
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
	if err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte("DB_DRIVER=mysql\nDB_SUPPORTED_DRIVERS=mysql\n"), 0o644); err != nil {
		t.Fatalf("write environment: %v", err)
	}

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

	total, changed, err := GenerateProjectFiles(projectDir, false, false, false, false, true, false)
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

	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("DB_DRIVER=mysql\nDB_SUPPORTED_DRIVERS=mysql\n"), 0o644); err != nil {
		t.Fatalf("write environment: %v", err)
	}

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

func TestCmdRunGeneratesObservabilityTargetsWithoutGoModTidy(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "containers", "observability", "vmagent"), 0o755); err != nil {
		t.Fatalf("mkdir vmagent dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "internal", "http"), 0o755); err != nil {
		t.Fatalf("mkdir http dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(strings.Join([]string{
		"APP_NAME=Test App",
		"APP_ENV=local",
		"OBSERVABILITY_METRICS_TARGET_HOST=localhost",
		"METRICS_API_PORT=9100",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write environment: %v", err)
	}

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
		return nil
	}
	defer func() { goModTidyRunner = orig }()

	cmd := &Cmd{Observability: true}
	if err := cmd.Run(); err != nil {
		t.Fatalf("Cmd.Run returned error: %v", err)
	}
	if called != 0 {
		t.Fatalf("goModTidyRunner called %d times, want 0", called)
	}

	content, err := os.ReadFile(filepath.Join(root, "containers", "observability", "vmagent", "metrics-targets.json"))
	if err != nil {
		t.Fatalf("read metrics-targets.json: %v", err)
	}
	if string(content) == "" {
		t.Fatal("expected generated metrics targets content")
	}
}

func TestGenerateProjectFilesSkipsGoModTidyForObservabilityOnlyChanges(t *testing.T) {
	projectDir := t.TempDir()
	for _, dir := range []string{
		filepath.Join(projectDir, "internal", "storages"),
		filepath.Join(projectDir, "containers", "observability", "vmagent"),
		filepath.Join(projectDir, "internal", "http"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module example.com/test\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	t.Setenv("STORAGE_DRIVER", "local")
	if err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte(strings.Join([]string{
		"STORAGE_DRIVER=local",
		"STORAGE_SUPPORTED_DRIVERS=local",
		"APP_NAME=Test App",
		"OBSERVABILITY_METRICS_TARGET_HOST=host.docker.internal",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write environment: %v", err)
	}

	if _, err := GenerateStorageFiles(projectDir); err != nil {
		t.Fatalf("seed generated storage file: %v", err)
	}

	called := 0
	orig := goModTidyRunner
	goModTidyRunner = func(dir string) error {
		called++
		return nil
	}
	defer func() { goModTidyRunner = orig }()

	total, changed, err := GenerateProjectFiles(projectDir, true, false, false, false, false, true)
	if err != nil {
		t.Fatalf("GenerateProjectFiles returned error: %v", err)
	}
	if total != 3 {
		t.Fatalf("total files = %d, want %d", total, 3)
	}
	if changed != 1 {
		t.Fatalf("changed files = %d, want %d", changed, 1)
	}
	if called != 0 {
		t.Fatalf("goModTidyRunner called %d times, want 0", called)
	}
}
