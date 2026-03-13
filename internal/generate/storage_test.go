package generate

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGenerateStorageFilesSupportsDefaultAndNamedAccessors(t *testing.T) {
	t.Setenv("STORAGE_DRIVER", "local")
	t.Setenv("STORAGE_ROOT", "storage/app/private")
	t.Setenv("STORAGE_PUBLIC_DRIVER", "local")
	t.Setenv("STORAGE_PUBLIC_ROOT", "storage/app/public")
	t.Setenv("STORAGE_AVATARS_DRIVER", "local")
	t.Setenv("STORAGE_AVATARS_ROOT", "storage/app/avatars")

	repoRoot := repoRoot(t)
	root, err := os.MkdirTemp(repoRoot, ".tmp-storage-generation-*")
	if err != nil {
		t.Fatalf("mkdir temp generation root: %v", err)
	}
	defer os.RemoveAll(root)
	if err := os.MkdirAll(filepath.Join(root, "internal", "storage"), 0o755); err != nil {
		t.Fatalf("mkdir storage package: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "internal", "storage", "manager.go"), loadStorageManagerFixture(t), 0o644); err != nil {
		t.Fatalf("write manager.go: %v", err)
	}

	written, err := GenerateStorageFiles(root)
	if err != nil {
		t.Fatalf("GenerateStorageFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated storage files to be written")
	}

	for _, generatedPath := range []string{
		filepath.Join(root, "internal", "storage", "disks_gen.go"),
		filepath.Join(root, "internal", "storage", "config_gen.go"),
	} {
		if _, err := os.Stat(generatedPath); err != nil {
			t.Fatalf("expected generated file %s: %v", generatedPath, err)
		}
	}

	disksGen, err := os.ReadFile(filepath.Join(root, "internal", "storage", "disks_gen.go"))
	if err != nil {
		t.Fatalf("read disks_gen.go: %v", err)
	}
	for _, snippet := range []string{
		"func (m *Manager) Public()",
		"func (m *Manager) Avatars()",
	} {
		if !strings.Contains(string(disksGen), snippet) {
			t.Fatalf("expected generated accessors to contain %q", snippet)
		}
	}

	testSource := `package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratedAccessors(t *testing.T) {
	defaultRoot := t.TempDir()
	publicRoot := t.TempDir()
	avatarsRoot := t.TempDir()

	t.Setenv("STORAGE_DRIVER", "local")
	t.Setenv("STORAGE_ROOT", defaultRoot)
	t.Setenv("STORAGE_PUBLIC_DRIVER", "local")
	t.Setenv("STORAGE_PUBLIC_ROOT", publicRoot)
	t.Setenv("STORAGE_AVATARS_DRIVER", "local")
	t.Setenv("STORAGE_AVATARS_ROOT", avatarsRoot)

	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	if err := mgr.Default().Put("default.txt", []byte("default")); err != nil {
		t.Fatalf("default Put returned error: %v", err)
	}
	if err := mgr.Public().Put("public.txt", []byte("public")); err != nil {
		t.Fatalf("public Put returned error: %v", err)
	}
	if err := mgr.Avatars().Put("avatar.txt", []byte("avatar")); err != nil {
		t.Fatalf("avatars Put returned error: %v", err)
	}

	for _, tc := range []struct {
		name     string
		root     string
		filename string
		want     string
	}{
		{name: "default", root: defaultRoot, filename: "default.txt", want: "default"},
		{name: "public", root: publicRoot, filename: "public.txt", want: "public"},
		{name: "avatars", root: avatarsRoot, filename: "avatar.txt", want: "avatar"},
	} {
		content, err := os.ReadFile(filepath.Join(tc.root, tc.filename))
		if err != nil {
			t.Fatalf("%s file missing: %v", tc.name, err)
		}
		if string(content) != tc.want {
			t.Fatalf("%s file content = %q, want %q", tc.name, string(content), tc.want)
		}
	}
}
`
	if err := os.WriteFile(filepath.Join(root, "internal", "storage", "generated_accessors_test.go"), []byte(testSource), 0o644); err != nil {
		t.Fatalf("write generated test: %v", err)
	}

	relRoot, err := filepath.Rel(repoRoot, root)
	if err != nil {
		t.Fatalf("relative temp path: %v", err)
	}
	pkgPath := "./" + filepath.ToSlash(filepath.Join(relRoot, "internal", "storage"))
	cmd := exec.Command("go", "test", pkgPath, "-run", "TestGeneratedAccessors", "-count=1")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "GOCACHE=/tmp/goforj-go-cache")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated storage package test failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}

func TestGenerateStorageFilesAddsDriverImportsToGoMod(t *testing.T) {
	t.Setenv("STORAGE_DRIVER", "local")
	t.Setenv("STORAGE_ROOT", "storage/app/private")
	t.Setenv("STORAGE_CACHE_DRIVER", "memory")

	repoRoot := repoRoot(t)
	root, err := os.MkdirTemp(repoRoot, ".tmp-storage-driver-imports-*")
	if err != nil {
		t.Fatalf("mkdir temp module root: %v", err)
	}
	defer os.RemoveAll(root)
	if err := os.MkdirAll(filepath.Join(root, "internal", "storage"), 0o755); err != nil {
		t.Fatalf("mkdir storage package: %v", err)
	}

	goMod := `module example.com/storageimporttest

go 1.24

require (
	github.com/goforj/env/v2 v2.3.0
	github.com/goforj/storage v0.2.5
	github.com/goforj/str v1.2.0
)
`
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "storage", "manager.go"), loadStorageManagerFixture(t), 0o644); err != nil {
		t.Fatalf("write manager.go: %v", err)
	}

	written, err := GenerateStorageFiles(root)
	if err != nil {
		t.Fatalf("GenerateStorageFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated storage files to be written")
	}

	configGenPath := filepath.Join(root, "internal", "storage", "config_gen.go")
	configGen, err := os.ReadFile(configGenPath)
	if err != nil {
		t.Fatalf("read config_gen.go: %v", err)
	}
	for _, importPath := range []string{
		`"github.com/goforj/storage/driver/localstorage"`,
		`"github.com/goforj/storage/driver/memorystorage"`,
	} {
		if !strings.Contains(string(configGen), importPath) {
			t.Fatalf("expected config_gen.go to import %s", importPath)
		}
	}

	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = root
	tidy.Env = append(os.Environ(),
		"GOCACHE=/tmp/goforj-go-cache",
		"GOMODCACHE=/tmp/goforj-go-modcache",
	)
	output, err := tidy.CombinedOutput()
	if err != nil {
		t.Fatalf("go mod tidy failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}

	goModAfter, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod after tidy: %v", err)
	}
	for _, module := range []string{
		"github.com/goforj/storage/driver/localstorage",
		"github.com/goforj/storage/driver/memorystorage",
	} {
		if !strings.Contains(string(goModAfter), module) {
			t.Fatalf("expected go.mod to contain %s after tidy", module)
		}
	}

	goTest := exec.Command("go", "test", "./internal/storage", "-run", "TestDoesNotExist", "-count=1")
	goTest.Dir = root
	goTest.Env = append(os.Environ(),
		"GOCACHE=/tmp/goforj-go-cache",
		"GOMODCACHE=/tmp/goforj-go-modcache",
	)
	output, err = goTest.CombinedOutput()
	if err != nil {
		t.Fatalf("generated storage package compile failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}

func TestGenerateStorageFilesWithPinnedDriverModules(t *testing.T) {
	t.Setenv("STORAGE_DRIVER", "local")
	t.Setenv("STORAGE_ROOT", "storage/app/private")
	t.Setenv("STORAGE_CACHE_DRIVER", "memory")

	repoRoot := repoRoot(t)
	root, err := os.MkdirTemp(repoRoot, ".tmp-storage-driver-pins-*")
	if err != nil {
		t.Fatalf("mkdir temp module root: %v", err)
	}
	defer os.RemoveAll(root)
	if err := os.MkdirAll(filepath.Join(root, "internal", "storage"), 0o755); err != nil {
		t.Fatalf("mkdir storage package: %v", err)
	}

	goMod := `module example.com/storagepinnedtest

go 1.24

require (
	github.com/goforj/env/v2 v2.3.0
	github.com/goforj/storage v0.2.5
	github.com/goforj/storage/driver/localstorage v0.2.5
	github.com/goforj/storage/driver/memorystorage v0.2.5
	github.com/goforj/str v1.2.0
)
`
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "storage", "manager.go"), loadStorageManagerFixture(t), 0o644); err != nil {
		t.Fatalf("write manager.go: %v", err)
	}

	written, err := GenerateStorageFiles(root)
	if err != nil {
		t.Fatalf("GenerateStorageFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated storage files to be written")
	}

	configGenPath := filepath.Join(root, "internal", "storage", "config_gen.go")
	configGen, err := os.ReadFile(configGenPath)
	if err != nil {
		t.Fatalf("read config_gen.go: %v", err)
	}
	for _, importPath := range []string{
		`"github.com/goforj/storage/driver/localstorage"`,
		`"github.com/goforj/storage/driver/memorystorage"`,
	} {
		if !strings.Contains(string(configGen), importPath) {
			t.Fatalf("expected config_gen.go to import %s", importPath)
		}
	}

	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = root
	tidy.Env = append(os.Environ(),
		"GOCACHE=/tmp/goforj-go-cache",
		"GOMODCACHE=/tmp/goforj-go-modcache",
	)
	output, err := tidy.CombinedOutput()
	if err != nil {
		t.Fatalf("go mod tidy failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}

	goModAfter, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod after tidy: %v", err)
	}
	for _, module := range []string{
		"github.com/goforj/storage/driver/localstorage",
		"github.com/goforj/storage/driver/memorystorage",
	} {
		if !strings.Contains(string(goModAfter), module) {
			t.Fatalf("expected pinned go.mod to retain %s after tidy", module)
		}
	}

	goTest := exec.Command("go", "test", "./internal/storage", "-run", "TestDoesNotExist", "-count=1")
	goTest.Dir = root
	goTest.Env = append(os.Environ(),
		"GOCACHE=/tmp/goforj-go-cache",
		"GOMODCACHE=/tmp/goforj-go-modcache",
	)
	output, err = goTest.CombinedOutput()
	if err != nil {
		t.Fatalf("generated storage package compile failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}

func loadStorageManagerFixture(t *testing.T) []byte {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve test file path")
	}
	fixturePath := filepath.Join(filepath.Dir(currentFile), "..", "forj", "internal", "storage", "manager.go")
	content, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read storage manager fixture: %v", err)
	}
	return content
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve test file path")
	}
	return filepath.Join(filepath.Dir(currentFile), "..", "..")
}
