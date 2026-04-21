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
	if err := os.MkdirAll(filepath.Join(root, "internal", "storages"), 0o755); err != nil {
		t.Fatalf("mkdir storage package: %v", err)
	}

	written, err := GenerateStorageFiles(root)
	if err != nil {
		t.Fatalf("GenerateStorageFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated storage files to be written")
	}

	for _, generatedPath := range []string{
		filepath.Join(root, "internal", "storages", "manager_gen.go"),
	} {
		if _, err := os.Stat(generatedPath); err != nil {
			t.Fatalf("expected generated file %s: %v", generatedPath, err)
		}
	}

	disksGen, err := os.ReadFile(filepath.Join(root, "internal", "storages", "manager_gen.go"))
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}
	for _, snippet := range []string{
		"func (m *Manager) Public()",
		"func (m *Manager) Avatars()",
		`Name: "storage_public"`,
		`func (m *Manager) ReadinessChecks() []ReadinessCheck`,
	} {
		if !strings.Contains(string(disksGen), snippet) {
			t.Fatalf("expected generated accessors to contain %q", snippet)
		}
	}

	testSource := `package storages

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

	checks := mgr.ReadinessChecks()
	if len(checks) != 3 {
		t.Fatalf("len(ReadinessChecks()) = %d, want 3", len(checks))
	}
	for _, check := range checks {
		if err := check.Check(t.Context()); err != nil {
			t.Fatalf("readiness check %s returned error: %v", check.Name, err)
		}
	}
}
`
	if err := os.WriteFile(filepath.Join(root, "internal", "storages", "generated_accessors_test.go"), []byte(testSource), 0o644); err != nil {
		t.Fatalf("write generated test: %v", err)
	}

	relRoot, err := filepath.Rel(repoRoot, root)
	if err != nil {
		t.Fatalf("relative temp path: %v", err)
	}
	pkgPath := "./" + filepath.ToSlash(filepath.Join(relRoot, "internal", "storages"))
	cmd := exec.Command("go", "test", pkgPath, "-run", "TestGeneratedAccessors", "-count=1")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "GOCACHE=/tmp/goforj-go-cache")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated storage package test failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}

func TestGenerateStorageFilesUsesSupportedDriverImports(t *testing.T) {
	t.Setenv("STORAGE_DRIVER", "local")
	t.Setenv("STORAGE_ROOT", "storage/app/private")
	t.Setenv("STORAGE_SUPPORTED_DRIVERS", "local,s3")

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "storages"), 0o755); err != nil {
		t.Fatalf("mkdir storage package: %v", err)
	}

	if _, err := GenerateStorageFiles(root); err != nil {
		t.Fatalf("GenerateStorageFiles returned error: %v", err)
	}

	managerGen, err := os.ReadFile(filepath.Join(root, "internal", "storages", "manager_gen.go"))
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}
	source := string(managerGen)
	if !strings.Contains(source, `"github.com/goforj/storage/driver/s3storage"`) {
		t.Fatal("expected generated storage manager to import s3storage from STORAGE_SUPPORTED_DRIVERS")
	}
}

func TestGenerateStorageFilesTracksOptionalDiskWarnings(t *testing.T) {
	t.Setenv("STORAGE_DRIVER", "local")
	t.Setenv("STORAGE_ROOT", "storage/app/private")
	t.Setenv("STORAGE_REDIS_BACKED_DRIVER", "redis")

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "storages"), 0o755); err != nil {
		t.Fatalf("mkdir storage package: %v", err)
	}

	if _, err := GenerateStorageFiles(root); err != nil {
		t.Fatalf("GenerateStorageFiles returned error: %v", err)
	}

	managerGen, err := os.ReadFile(filepath.Join(root, "internal", "storages", "manager_gen.go"))
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}
	source := string(managerGen)
	for _, snippet := range []string{
		`OptionalDiskWarning`,
		`type OptionalDiskWarning struct {`,
		`func (m *Manager) Warnings() []OptionalDiskWarning {`,
		`diskRedisBacked, warningRedisBacked, err := optionalDiskFromScope(storageScope, storage.DiskName("redis_backed"))`,
		`manager.warnings = append(manager.warnings, *warningRedisBacked)`,
		`func optionalDiskFromScope(storageScope env.Scope, name storage.DiskName) (storage.Storage, *OptionalDiskWarning, error) {`,
		`Error:  err.Error(),`,
	} {
		if !strings.Contains(source, snippet) {
			t.Fatalf("expected generated storage manager to contain %q", snippet)
		}
	}
}

func TestGenerateStorageFilesRejectsUnknownEnvVars(t *testing.T) {
	t.Setenv("STORAGE_DRIVER", "local")
	t.Setenv("STORAGE_PUBLIC_ROOOT", "storage/app/public")

	_, err := GenerateStorageFiles(t.TempDir())
	if err == nil {
		t.Fatal("expected GenerateStorageFiles to reject unknown storage env vars")
	}
	if !strings.Contains(err.Error(), "STORAGE_PUBLIC_ROOOT") {
		t.Fatalf("expected error to mention unknown env var, got: %v", err)
	}
}

func TestGenerateStorageFilesRejectsWrongDriverEnvVars(t *testing.T) {
	t.Setenv("STORAGE_DRIVER", "local")
	t.Setenv("STORAGE_PUBLIC_DRIVER", "local")
	t.Setenv("STORAGE_PUBLIC_BUCKET", "public-assets")

	_, err := GenerateStorageFiles(t.TempDir())
	if err == nil {
		t.Fatal("expected GenerateStorageFiles to reject wrong-driver storage env vars")
	}
	if !strings.Contains(err.Error(), "STORAGE_PUBLIC_BUCKET") {
		t.Fatalf("expected error to mention wrong-driver env var, got: %v", err)
	}
	if !strings.Contains(err.Error(), `driver "local"`) {
		t.Fatalf("expected error to mention local driver, got: %v", err)
	}
}

func TestGenerateStorageFilesAllowsInactiveRootDriverEnvVars(t *testing.T) {
	t.Setenv("STORAGE_DRIVER", "local")
	t.Setenv("STORAGE_BUCKET", "assets")
	t.Setenv("STORAGE_REGION", "us-east-1")

	if _, err := GenerateStorageFiles(t.TempDir()); err != nil {
		t.Fatalf("expected GenerateStorageFiles to allow documented inactive root storage env vars, got %v", err)
	}
}

func TestGenerateStorageFilesLocalManagerCreatesMissingRoots(t *testing.T) {
	t.Setenv("STORAGE_DRIVER", "local")
	t.Setenv("STORAGE_ROOT", filepath.Join(t.TempDir(), "private"))
	t.Setenv("STORAGE_FAVICONS_DRIVER", "local")
	t.Setenv("STORAGE_FAVICONS_ROOT", filepath.Join(t.TempDir(), "favicons"))

	repoRoot := repoRoot(t)
	root, err := os.MkdirTemp(repoRoot, ".tmp-storage-generation-*")
	if err != nil {
		t.Fatalf("mkdir temp generation root: %v", err)
	}
	defer os.RemoveAll(root)
	if err := os.MkdirAll(filepath.Join(root, "internal", "storages"), 0o755); err != nil {
		t.Fatalf("mkdir storage package: %v", err)
	}

	written, err := GenerateStorageFiles(root)
	if err != nil {
		t.Fatalf("GenerateStorageFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated storage files to be written")
	}

	testSource := `package storages

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewManagerCreatesMissingLocalRoots(t *testing.T) {
	defaultRoot := filepath.Join(t.TempDir(), "private")
	faviconRoot := filepath.Join(t.TempDir(), "favicons")

	t.Setenv("STORAGE_DRIVER", "local")
	t.Setenv("STORAGE_ROOT", defaultRoot)
	t.Setenv("STORAGE_FAVICONS_DRIVER", "local")
	t.Setenv("STORAGE_FAVICONS_ROOT", faviconRoot)

	if _, err := os.Stat(defaultRoot); !os.IsNotExist(err) {
		t.Fatalf("expected default root to be absent before NewManager, got %v", err)
	}
	if _, err := os.Stat(faviconRoot); !os.IsNotExist(err) {
		t.Fatalf("expected favicon root to be absent before NewManager, got %v", err)
	}

	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	if mgr.Default() == nil {
		t.Fatal("expected default disk")
	}
	if mgr.Favicons() == nil {
		t.Fatal("expected favicons disk")
	}
	for _, path := range []string{defaultRoot, faviconRoot} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected root %s to exist: %v", path, err)
		}
		if !info.IsDir() {
			t.Fatalf("expected root %s to be a directory", path)
		}
	}
}
`
	if err := os.WriteFile(filepath.Join(root, "internal", "storages", "generated_local_root_test.go"), []byte(testSource), 0o644); err != nil {
		t.Fatalf("write generated test: %v", err)
	}

	relRoot, err := filepath.Rel(repoRoot, root)
	if err != nil {
		t.Fatalf("relative temp path: %v", err)
	}
	pkgPath := "./" + filepath.ToSlash(filepath.Join(relRoot, "internal", "storages"))
	cmd := exec.Command("go", "test", pkgPath, "-run", "TestNewManagerCreatesMissingLocalRoots", "-count=1")
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

	root := mustTempGeneratedModuleRoot(t, ".tmp-storage-driver-imports-*", filepath.Join("internal", "storages"))
	writeFixtureGoMod(t, root, fixtureModuleSpec(
		"example.com/storageimporttest",
		[]string{
			"github.com/goforj/env/v2",
			"github.com/goforj/storage",
			"github.com/goforj/storage/driver/dropboxstorage",
			"github.com/goforj/storage/driver/ftpstorage",
			"github.com/goforj/storage/driver/gcsstorage",
			"github.com/goforj/storage/driver/localstorage",
			"github.com/goforj/storage/driver/memorystorage",
			"github.com/goforj/storage/driver/rclonestorage",
			"github.com/goforj/storage/driver/redisstorage",
			"github.com/goforj/storage/driver/s3storage",
			"github.com/goforj/storage/driver/sftpstorage",
			"github.com/goforj/str",
		},
		nil,
		nil,
	))
	written, err := GenerateStorageFiles(root)
	if err != nil {
		t.Fatalf("GenerateStorageFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated storage files to be written")
	}

	configGenPath := filepath.Join(root, "internal", "storages", "manager_gen.go")
	configGen, err := os.ReadFile(configGenPath)
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}
	for _, importPath := range []string{
		`"github.com/goforj/storage/driver/localstorage"`,
		`"github.com/goforj/storage/driver/memorystorage"`,
	} {
		if !strings.Contains(string(configGen), importPath) {
			t.Fatalf("expected manager_gen.go to import %s", importPath)
		}
	}

	runFixtureGoModTidy(t, root, nil)
	assertFixtureGoModContains(t, root,
		"github.com/goforj/storage/driver/localstorage",
		"github.com/goforj/storage/driver/memorystorage",
	)
	runFixtureGoTest(t, root, "./internal/storages", "TestDoesNotExist", nil)
}

func TestGenerateStorageFilesWithPinnedDriverModules(t *testing.T) {
	t.Setenv("STORAGE_DRIVER", "local")
	t.Setenv("STORAGE_ROOT", "storage/app/private")
	t.Setenv("STORAGE_CACHE_DRIVER", "memory")

	root := mustTempGeneratedModuleRoot(t, ".tmp-storage-driver-pins-*", filepath.Join("internal", "storages"))
	writeFixtureGoMod(t, root, fixtureModuleSpec(
		"example.com/storagepinnedtest",
		[]string{
			"github.com/goforj/env/v2",
			"github.com/goforj/storage",
			"github.com/goforj/str",
		},
		[]string{
			"github.com/goforj/storage/driver/localstorage",
			"github.com/goforj/storage/driver/memorystorage",
		},
		nil,
	))
	written, err := GenerateStorageFiles(root)
	if err != nil {
		t.Fatalf("GenerateStorageFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated storage files to be written")
	}

	configGenPath := filepath.Join(root, "internal", "storages", "manager_gen.go")
	configGen, err := os.ReadFile(configGenPath)
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}
	for _, importPath := range []string{
		`"github.com/goforj/storage/driver/localstorage"`,
		`"github.com/goforj/storage/driver/memorystorage"`,
	} {
		if !strings.Contains(string(configGen), importPath) {
			t.Fatalf("expected manager_gen.go to import %s", importPath)
		}
	}

	runFixtureGoModTidy(t, root, nil)
	assertFixtureGoModContains(t, root,
		"github.com/goforj/storage/driver/localstorage",
		"github.com/goforj/storage/driver/memorystorage",
	)
	runFixtureGoTest(t, root, "./internal/storages", "TestDoesNotExist", nil)
}

func TestGenerateStorageFilesDriverMatrixCompiles(t *testing.T) {
	t.Setenv("STORAGE_DRIVER", "local")
	t.Setenv("STORAGE_ROOT", "storage/app/private")
	t.Setenv("STORAGE_MEMORY_DRIVER", "memory")
	t.Setenv("STORAGE_REDIS_DRIVER", "redis")
	t.Setenv("STORAGE_REDIS_ADDR", "127.0.0.1:6379")
	t.Setenv("STORAGE_REDIS_PASSWORD", "secret")
	t.Setenv("STORAGE_REDIS_DB", "2")
	t.Setenv("STORAGE_FTP_DRIVER", "ftp")
	t.Setenv("STORAGE_FTP_HOST", "127.0.0.1")
	t.Setenv("STORAGE_FTP_PORT", "21")
	t.Setenv("STORAGE_FTP_USER", "test")
	t.Setenv("STORAGE_FTP_PASSWORD", "secret")
	t.Setenv("STORAGE_FTP_TLS", "true")
	t.Setenv("STORAGE_FTP_INSECURE_SKIP_VERIFY", "true")
	t.Setenv("STORAGE_SFTP_DRIVER", "sftp")
	t.Setenv("STORAGE_SFTP_HOST", "127.0.0.1")
	t.Setenv("STORAGE_SFTP_PORT", "22")
	t.Setenv("STORAGE_SFTP_USER", "root")
	t.Setenv("STORAGE_SFTP_PASSWORD", "secret")
	t.Setenv("STORAGE_SFTP_KEY_PATH", "/tmp/id_rsa")
	t.Setenv("STORAGE_SFTP_KNOWN_HOSTS_PATH", "/tmp/known_hosts")
	t.Setenv("STORAGE_SFTP_INSECURE_IGNORE_HOST_KEY", "true")
	t.Setenv("STORAGE_S3_DRIVER", "s3")
	t.Setenv("STORAGE_S3_BUCKET", "app-bucket")
	t.Setenv("STORAGE_S3_ENDPOINT", "http://127.0.0.1:9000")
	t.Setenv("STORAGE_S3_REGION", "us-east-1")
	t.Setenv("STORAGE_S3_ACCESS_KEY_ID", "access")
	t.Setenv("STORAGE_S3_SECRET_ACCESS_KEY", "secret")
	t.Setenv("STORAGE_S3_USE_PATH_STYLE", "true")
	t.Setenv("STORAGE_S3_UNSIGNED_PAYLOAD", "true")
	t.Setenv("STORAGE_GCS_DRIVER", "gcs")
	t.Setenv("STORAGE_GCS_BUCKET", "gcs-bucket")
	t.Setenv("STORAGE_GCS_CREDENTIALS_JSON", `{"type":"service_account"}`)
	t.Setenv("STORAGE_GCS_ENDPOINT", "http://127.0.0.1:4443")
	t.Setenv("STORAGE_DROPBOX_DRIVER", "dropbox")
	t.Setenv("STORAGE_DROPBOX_TOKEN", "token")
	t.Setenv("STORAGE_DROPBOX_PREFIX", "uploads")
	t.Setenv("STORAGE_RCLONE_DRIVER", "rclone")
	t.Setenv("STORAGE_RCLONE_REMOTE", "remote:")
	t.Setenv("STORAGE_RCLONE_RCLONE_CONFIG_PATH", "/tmp/rclone.conf")
	t.Setenv("STORAGE_RCLONE_RCLONE_CONFIG_DATA", "[remote]")

	root := mustTempGeneratedModuleRoot(t, ".tmp-storage-driver-matrix-*", filepath.Join("internal", "storages"))
	writeFixtureGoMod(t, root, fixtureModuleSpec(
		"example.com/storagedrivermatrix",
		[]string{
			"github.com/goforj/env/v2",
			"github.com/goforj/storage",
			"github.com/goforj/str",
		},
		nil,
		nil,
	))
	written, err := GenerateStorageFiles(root)
	if err != nil {
		t.Fatalf("GenerateStorageFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated storage files to be written")
	}

	configGenPath := filepath.Join(root, "internal", "storages", "manager_gen.go")
	configGen, err := os.ReadFile(configGenPath)
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}
	for _, importPath := range []string{
		`"github.com/goforj/storage/driver/localstorage"`,
		`"github.com/goforj/storage/driver/memorystorage"`,
		`"github.com/goforj/storage/driver/redisstorage"`,
		`"github.com/goforj/storage/driver/ftpstorage"`,
		`"github.com/goforj/storage/driver/sftpstorage"`,
		`"github.com/goforj/storage/driver/s3storage"`,
		`"github.com/goforj/storage/driver/gcsstorage"`,
		`"github.com/goforj/storage/driver/dropboxstorage"`,
		`"github.com/goforj/storage/driver/rclonestorage"`,
	} {
		if !strings.Contains(string(configGen), importPath) {
			t.Fatalf("expected manager_gen.go to import %s", importPath)
		}
	}

	runFixtureGoModTidy(t, root, nil)
	assertFixtureGoModContains(t, root,
		"github.com/goforj/storage/driver/localstorage",
		"github.com/goforj/storage/driver/memorystorage",
		"github.com/goforj/storage/driver/redisstorage",
		"github.com/goforj/storage/driver/ftpstorage",
		"github.com/goforj/storage/driver/sftpstorage",
		"github.com/goforj/storage/driver/s3storage",
		"github.com/goforj/storage/driver/gcsstorage",
		"github.com/goforj/storage/driver/dropboxstorage",
		"github.com/goforj/storage/driver/rclonestorage",
	)
	runFixtureGoTest(t, root, "./internal/storages", "TestDoesNotExist", nil)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve test file path")
	}
	return filepath.Join(filepath.Dir(currentFile), "..", "..")
}
