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
		filepath.Join(root, "internal", "storages", "accessors_gen.go"),
	} {
		if _, err := os.Stat(generatedPath); err != nil {
			t.Fatalf("expected generated file %s: %v", generatedPath, err)
		}
	}

	disksGen, err := os.ReadFile(filepath.Join(root, "internal", "storages", "accessors_gen.go"))
	if err != nil {
		t.Fatalf("read accessors_gen.go: %v", err)
	}
	for _, snippet := range []string{
		"func (m *Manager) Public()",
		"func (m *Manager) Avatars()",
	} {
		if !strings.Contains(string(disksGen), snippet) {
			t.Fatalf("expected generated accessors to contain %q", snippet)
		}
	}
	managerGen, err := os.ReadFile(filepath.Join(root, "internal", "storages", "manager_gen.go"))
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}
	for _, snippet := range []string{
		`Name: "storage_public"`,
		`func (m *Manager) ReadinessChecks() []ReadinessCheck`,
	} {
		if !strings.Contains(string(managerGen), snippet) {
			t.Fatalf("expected generated manager to contain %q", snippet)
		}
	}

	testSource := `package storages

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/goforj/storage"
	"github.com/goforj/storage/driver/localstorage"
)

// trackedStorage records generated-manager ownership without reimplementing storage operations.
type trackedStorage struct {
	storage.Storage
	closeCalls *atomic.Int32
	closeErr   error
}

// Close records one underlying resource release and returns its configured failure.
func (s *trackedStorage) Close() error {
	s.closeCalls.Add(1)
	return s.closeErr
}

func TestGeneratedAccessors(t *testing.T) {
	defaultRoot := t.TempDir()
	publicRoot := t.TempDir()
	avatarsRoot := t.TempDir()
	archiveLogsRoot := t.TempDir()

	t.Setenv("STORAGE_DRIVER", "local")
	t.Setenv("STORAGE_ROOT", defaultRoot)
	t.Setenv("STORAGE_PUBLIC_DRIVER", "local")
	t.Setenv("STORAGE_PUBLIC_ROOT", publicRoot)
	t.Setenv("STORAGE_AVATARS_DRIVER", "local")
	t.Setenv("STORAGE_AVATARS_ROOT", avatarsRoot)
	t.Setenv("STORAGE_ARCHIVE_LOGS_DRIVER", "local")
	t.Setenv("STORAGE_ARCHIVE_LOGS_ROOT", archiveLogsRoot)

	config, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv returned error: %v", err)
	}
	if _, ok := config.Disks["archive_logs"]; !ok {
		t.Fatalf("LoadConfigFromEnv disks = %#v, want multiword archive_logs disk", config.Disks)
	}

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

	var observed []string
	mgr = mgr.WithObserver(ObserverFunc(func(_ context.Context, event StorageOpEvent) {
		if event.Err != nil {
			t.Fatalf("observer saw error: %v", event.Err)
		}
		observed = append(observed, event.Disk+":"+event.Operation+":"+event.Driver)
	}))

	if _, err := mgr.Public().Get("public.txt"); err != nil {
		t.Fatalf("observer Get returned error: %v", err)
	}
	if len(observed) == 0 {
		t.Fatal("expected observer to capture storage operations")
	}

	t.Run("manager close is idempotent and deduplicates shared disks", func(t *testing.T) {
		firstInner, err := storage.Build(localstorage.Config{Root: t.TempDir()})
		if err != nil {
			t.Fatalf("build first disk: %v", err)
		}
		secondInner, err := storage.Build(localstorage.Config{Root: t.TempDir()})
		if err != nil {
			t.Fatalf("build second disk: %v", err)
		}
		firstErr := errors.New("first close failed")
		secondErr := errors.New("second close failed")
		var firstCalls atomic.Int32
		var secondCalls atomic.Int32
		first := &trackedStorage{Storage: firstInner, closeCalls: &firstCalls, closeErr: firstErr}
		second := &trackedStorage{Storage: secondInner, closeCalls: &secondCalls, closeErr: secondErr}
		owned := &Manager{ownedDisks: []ownedStorageDisk{
			{name: "default", disk: first},
			{name: "shared", disk: first},
			{name: "archive", disk: second},
		}}
		closeErr := owned.Close()
		if !errors.Is(closeErr, firstErr) || !errors.Is(closeErr, secondErr) {
			t.Fatalf("Close error = %v, want both disk failures", closeErr)
		}
		if err := owned.Close(); !errors.Is(err, firstErr) || !errors.Is(err, secondErr) {
			t.Fatalf("second Close error = %v, want retained failures", err)
		}
		if got := firstCalls.Load(); got != 1 {
			t.Fatalf("shared disk close calls = %d, want 1", got)
		}
		if got := secondCalls.Load(); got != 1 {
			t.Fatalf("archive disk close calls = %d, want 1", got)
		}
	})

	t.Run("observer context wrappers share close lifecycle", func(t *testing.T) {
		inner, err := storage.Build(localstorage.Config{Root: t.TempDir()})
		if err != nil {
			t.Fatalf("build observed disk: %v", err)
		}
		var closeCalls atomic.Int32
		tracked := &trackedStorage{Storage: inner, closeCalls: &closeCalls}
		wrapped := wrapObservedStorage(tracked, "default", "local", ObserverFunc(func(context.Context, StorageOpEvent) {}))
		derived := wrapped.WithContext(t.Context())
		if err := derived.(interface{ Close() error }).Close(); err != nil {
			t.Fatalf("close derived observer: %v", err)
		}
		if err := wrapped.(interface{ Close() error }).Close(); err != nil {
			t.Fatalf("close root observer: %v", err)
		}
		if got := closeCalls.Load(); got != 1 {
			t.Fatalf("observed disk close calls = %d, want 1", got)
		}
	})

	t.Run("later initialization failure closes prior disks", func(t *testing.T) {
		inner, err := storage.Build(localstorage.Config{Root: t.TempDir()})
		if err != nil {
			t.Fatalf("build cleanup disk: %v", err)
		}
		initializationErr := errors.New("later disk failed")
		cleanupErr := errors.New("cleanup failed")
		var closeCalls atomic.Int32
		opened := &trackedStorage{Storage: inner, closeCalls: &closeCalls, closeErr: cleanupErr}
		buildCalls := 0
		_, err = newManagerFromEnvWithBuilder(func(storage.DriverConfig) (storage.Storage, error) {
			buildCalls++
			if buildCalls == 1 {
				return opened, nil
			}
			return nil, initializationErr
		})
		if !errors.Is(err, initializationErr) || !errors.Is(err, cleanupErr) {
			t.Fatalf("initialization error = %v, want construction and cleanup failures", err)
		}
		if got := closeCalls.Load(); got != 1 {
			t.Fatalf("previously opened disk close calls = %d, want 1", got)
		}
	})
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
	cmd.Env = append(os.Environ(), "GOCACHE=/tmp/gocache", "GOMODCACHE=/tmp/gomodcache")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated storage package test failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}

func TestGenerateStorageFilesUsesSupportedDriverImports(t *testing.T) {
	t.Setenv("STORAGE_DRIVER", "s3")
	t.Setenv("STORAGE_SUPPORTED_DRIVERS", "s3")

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
	if !strings.Contains(source, `"github.com/goforj/storage/driver/localstorage"`) {
		t.Fatal("expected generated storage manager to keep localstorage as the no-env fallback")
	}
}

func TestGenerateStorageFilesChainsMultipleObservers(t *testing.T) {
	t.Setenv("STORAGE_DRIVER", "local")
	t.Setenv("STORAGE_ROOT", "storage/app/private")
	t.Setenv("STORAGE_PUBLIC_DRIVER", "local")
	t.Setenv("STORAGE_PUBLIC_ROOT", "storage/app/public")

	repoRoot := repoRoot(t)
	root, err := os.MkdirTemp(repoRoot, ".tmp-storage-observer-chain-*")
	if err != nil {
		t.Fatalf("mkdir temp generation root: %v", err)
	}
	defer os.RemoveAll(root)
	if err := os.MkdirAll(filepath.Join(root, "internal", "storages"), 0o755); err != nil {
		t.Fatalf("mkdir storage package: %v", err)
	}

	if _, err := GenerateStorageFiles(root); err != nil {
		t.Fatalf("GenerateStorageFiles returned error: %v", err)
	}

	testSource := `package storages

import (
	"context"
	"testing"
)

func TestObserverChain(t *testing.T) {
	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	if err := mgr.Public().Put("public.txt", []byte("hello")); err != nil {
		t.Fatalf("prime public disk: %v", err)
	}

	var metricsOps int
	var inspectOps int
	mgr = mgr.WithObserver(ObserverFunc(func(_ context.Context, event StorageOpEvent) {
		if event.Err != nil {
			t.Fatalf("metrics observer saw error: %v", event.Err)
		}
		if event.Disk == "public" && event.Operation == "get" && event.Driver == "local" {
			metricsOps++
		}
	}))
	mgr = mgr.WithObserver(ObserverFunc(func(_ context.Context, event StorageOpEvent) {
		if event.Err != nil {
			t.Fatalf("inspect observer saw error: %v", event.Err)
		}
		if event.Disk == "public" && event.Operation == "get" && event.Driver == "local" {
			inspectOps++
		}
	}))

	if _, err := mgr.Public().Get("public.txt"); err != nil {
		t.Fatalf("public Get returned error: %v", err)
	}
	if metricsOps != 1 {
		t.Fatalf("metrics observer count = %d, want 1", metricsOps)
	}
	if inspectOps != 1 {
		t.Fatalf("inspect observer count = %d, want 1", inspectOps)
	}
}
`
	if err := os.WriteFile(filepath.Join(root, "internal", "storages", "observer_chain_test.go"), []byte(testSource), 0o644); err != nil {
		t.Fatalf("write generated test: %v", err)
	}

	relRoot, err := filepath.Rel(repoRoot, root)
	if err != nil {
		t.Fatalf("relative temp path: %v", err)
	}
	pkgPath := "./" + filepath.ToSlash(filepath.Join(relRoot, "internal", "storages"))
	cmd := exec.Command("go", "test", pkgPath, "-run", "TestObserverChain", "-count=1")
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated storage package observer-chain test failed: %v\n%s", err, strings.TrimSpace(string(output)))
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
		`diskRedisBacked, warningRedisBacked, err := optionalDiskFromScope(storageScope, storage.DiskName("redis_backed"), build)`,
		`manager.warnings = append(manager.warnings, *warningRedisBacked)`,
		`func optionalDiskFromScope(storageScope env.Scope, name storage.DiskName, build func(storage.DriverConfig) (storage.Storage, error)) (storage.Storage, *OptionalDiskWarning, error) {`,
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
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated storage package test failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}

func TestGenerateStorageFilesWithPinnedDriverModules(t *testing.T) {
	t.Parallel()
	environment := map[string]string{
		"STORAGE_DRIVER":       "local",
		"STORAGE_ROOT":         "storage/app/private",
		"STORAGE_CACHE_DRIVER": "memory",
	}

	root := mustTempGeneratedModuleRoot(t, ".tmp-storage-driver-pins-*", filepath.Join("internal", "storages"))
	writeFixtureGoMod(t, root, fixtureModuleSpec(
		"example.com/storagepinnedtest",
		[]string{
			"github.com/goforj/env/v2",
			"github.com/goforj/storage",
			"github.com/goforj/str/v2",
		},
		[]string{
			"github.com/goforj/storage/driver/localstorage",
			"github.com/goforj/storage/driver/memorystorage",
		},
		nil,
	))
	written, err := generateStorageFiles(fixtureGenerationInput(root, environment))
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
}

func TestGenerateStorageFilesDriverMatrixCompiles(t *testing.T) {
	t.Parallel()
	environment := map[string]string{
		"STORAGE_DRIVER":                        "local",
		"STORAGE_ROOT":                          "storage/app/private",
		"STORAGE_MEMORY_DRIVER":                 "memory",
		"STORAGE_REDIS_DRIVER":                  "redis",
		"STORAGE_REDIS_ADDR":                    "127.0.0.1:6379",
		"STORAGE_REDIS_PASSWORD":                "secret",
		"STORAGE_REDIS_DB":                      "2",
		"STORAGE_FTP_DRIVER":                    "ftp",
		"STORAGE_FTP_HOST":                      "127.0.0.1",
		"STORAGE_FTP_PORT":                      "21",
		"STORAGE_FTP_USER":                      "test",
		"STORAGE_FTP_PASSWORD":                  "secret",
		"STORAGE_FTP_TLS":                       "true",
		"STORAGE_FTP_INSECURE_SKIP_VERIFY":      "true",
		"STORAGE_SFTP_DRIVER":                   "sftp",
		"STORAGE_SFTP_HOST":                     "127.0.0.1",
		"STORAGE_SFTP_PORT":                     "22",
		"STORAGE_SFTP_USER":                     "root",
		"STORAGE_SFTP_PASSWORD":                 "secret",
		"STORAGE_SFTP_KEY_PATH":                 "/tmp/id_rsa",
		"STORAGE_SFTP_KNOWN_HOSTS_PATH":         "/tmp/known_hosts",
		"STORAGE_SFTP_INSECURE_IGNORE_HOST_KEY": "true",
		"STORAGE_S3_DRIVER":                     "s3",
		"STORAGE_S3_BUCKET":                     "app-bucket",
		"STORAGE_S3_ENDPOINT":                   "http://127.0.0.1:9000",
		"STORAGE_S3_REGION":                     "us-east-1",
		"STORAGE_S3_ACCESS_KEY_ID":              "access",
		"STORAGE_S3_SECRET_ACCESS_KEY":          "secret",
		"STORAGE_S3_USE_PATH_STYLE":             "true",
		"STORAGE_S3_UNSIGNED_PAYLOAD":           "true",
		"STORAGE_GCS_DRIVER":                    "gcs",
		"STORAGE_GCS_BUCKET":                    "gcs-bucket",
		"STORAGE_GCS_CREDENTIALS_JSON":          `{"type":"service_account"}`,
		"STORAGE_GCS_ENDPOINT":                  "http://127.0.0.1:4443",
		"STORAGE_DROPBOX_DRIVER":                "dropbox",
		"STORAGE_DROPBOX_TOKEN":                 "token",
		"STORAGE_DROPBOX_PREFIX":                "uploads",
		"STORAGE_RCLONE_DRIVER":                 "rclone",
		"STORAGE_RCLONE_REMOTE":                 "remote:",
		"STORAGE_RCLONE_RCLONE_CONFIG_PATH":     "/tmp/rclone.conf",
		"STORAGE_RCLONE_RCLONE_CONFIG_DATA":     "[remote]",
	}

	root := mustTempGeneratedModuleRoot(t, ".tmp-storage-driver-matrix-*", filepath.Join("internal", "storages"))
	writeFixtureGoMod(t, root, fixtureModuleSpec(
		"example.com/storagedrivermatrix",
		[]string{
			"github.com/goforj/env/v2",
			"github.com/goforj/storage",
			"github.com/goforj/str/v2",
		},
		nil,
		nil,
	))
	written, err := generateStorageFiles(fixtureGenerationInput(root, environment))
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
