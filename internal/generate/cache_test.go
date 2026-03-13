package generate

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGenerateCacheFilesSupportsDefaultAndNamedAccessors(t *testing.T) {
	t.Setenv("CACHE_DRIVER", "memory")
	t.Setenv("CACHE_SESSIONS_DRIVER", "file")
	t.Setenv("CACHE_SESSIONS_FILE_DIR", filepath.Join(t.TempDir(), "sessions"))
	t.Setenv("CACHE_PAGES_DRIVER", "null")

	repoRoot := repoRoot(t)
	root, err := os.MkdirTemp(repoRoot, ".tmp-cache-generation-*")
	if err != nil {
		t.Fatalf("mkdir temp generation root: %v", err)
	}
	defer os.RemoveAll(root)
	if err := os.MkdirAll(filepath.Join(root, "internal", "cache"), 0o755); err != nil {
		t.Fatalf("mkdir cache package: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "internal", "cache", "manager.go"), loadCacheManagerFixture(t), 0o644); err != nil {
		t.Fatalf("write manager.go: %v", err)
	}

	written, err := GenerateCacheFiles(root)
	if err != nil {
		t.Fatalf("GenerateCacheFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated cache files to be written")
	}

	for _, generatedPath := range []string{
		filepath.Join(root, "internal", "cache", "stores_gen.go"),
		filepath.Join(root, "internal", "cache", "config_gen.go"),
	} {
		if _, err := os.Stat(generatedPath); err != nil {
			t.Fatalf("expected generated file %s: %v", generatedPath, err)
		}
	}

	storesGen, err := os.ReadFile(filepath.Join(root, "internal", "cache", "stores_gen.go"))
	if err != nil {
		t.Fatalf("read stores_gen.go: %v", err)
	}
	for _, snippet := range []string{
		"func (m *Manager) Sessions()",
		"func (m *Manager) Pages()",
	} {
		if !strings.Contains(string(storesGen), snippet) {
			t.Fatalf("expected generated accessors to contain %q", snippet)
		}
	}

	testSource := `package cache

import "testing"

func TestGeneratedAccessors(t *testing.T) {
	sessionsDir := t.TempDir()

	t.Setenv("CACHE_DRIVER", "memory")
	t.Setenv("CACHE_SESSIONS_DRIVER", "file")
	t.Setenv("CACHE_SESSIONS_FILE_DIR", sessionsDir)
	t.Setenv("CACHE_PAGES_DRIVER", "null")

	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	if err := mgr.Default().SetString("default", "alpha", 0); err != nil {
		t.Fatalf("default SetString returned error: %v", err)
	}
	if err := mgr.Sessions().SetString("session", "bravo", 0); err != nil {
		t.Fatalf("sessions SetString returned error: %v", err)
	}
	if err := mgr.Pages().SetString("page", "charlie", 0); err != nil {
		t.Fatalf("pages SetString returned error: %v", err)
	}

	if got, ok, err := mgr.Default().GetString("default"); err != nil || !ok || got != "alpha" {
		t.Fatalf("default GetString = (%q, %v, %v), want (%q, true, nil)", got, ok, err, "alpha")
	}
	if got, ok, err := mgr.Sessions().GetString("session"); err != nil || !ok || got != "bravo" {
		t.Fatalf("sessions GetString = (%q, %v, %v), want (%q, true, nil)", got, ok, err, "bravo")
	}
	if _, ok, err := mgr.Pages().GetString("page"); err != nil || ok {
		t.Fatalf("pages GetString = (ok=%v, err=%v), want (false, nil)", ok, err)
	}
}
`
	if err := os.WriteFile(filepath.Join(root, "internal", "cache", "generated_accessors_test.go"), []byte(testSource), 0o644); err != nil {
		t.Fatalf("write generated test: %v", err)
	}

	relRoot, err := filepath.Rel(repoRoot, root)
	if err != nil {
		t.Fatalf("relative temp path: %v", err)
	}
	pkgPath := "./" + filepath.ToSlash(filepath.Join(relRoot, "internal", "cache"))
	cmd := exec.Command("go", "test", pkgPath, "-run", "TestGeneratedAccessors", "-count=1")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "GOCACHE=/tmp/gocache")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated cache package test failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}

func TestGenerateCacheFilesDerivesAccessorNamesFromCacheNames(t *testing.T) {
	t.Setenv("CACHE_DRIVER", "memory")
	t.Setenv("CACHE_SESSIONS_DRIVER", "redis")
	t.Setenv("CACHE_PAGE_CACHE_DRIVER", "memcached")
	t.Setenv("CACHE_USER_SESSIONS_DRIVER", "sqlite")
	t.Setenv("CACHE_USER_SESSIONS_DSN", "file::memory:?cache=shared")

	repoRoot := repoRoot(t)
	root, err := os.MkdirTemp(repoRoot, ".tmp-cache-accessor-names-*")
	if err != nil {
		t.Fatalf("mkdir temp generation root: %v", err)
	}
	defer os.RemoveAll(root)
	if err := os.MkdirAll(filepath.Join(root, "internal", "cache"), 0o755); err != nil {
		t.Fatalf("mkdir cache package: %v", err)
	}

	goMod := `module example.com/cacheaccessornametest

go 1.24

	require (
		github.com/goforj/cache v0.1.5
		github.com/goforj/cache/cachecore v0.1.5
		github.com/goforj/cache/cachetest v0.1.5
		github.com/goforj/cache/driver/sqlcore v0.1.5
		github.com/goforj/env/v2 v2.3.1
		github.com/goforj/str v1.2.0
	)
`
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "cache", "manager.go"), loadCacheManagerFixture(t), 0o644); err != nil {
		t.Fatalf("write manager.go: %v", err)
	}

	written, err := GenerateCacheFiles(root)
	if err != nil {
		t.Fatalf("GenerateCacheFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated cache files to be written")
	}

	storesGen, err := os.ReadFile(filepath.Join(root, "internal", "cache", "stores_gen.go"))
	if err != nil {
		t.Fatalf("read stores_gen.go: %v", err)
	}
	for _, snippet := range []string{
		"func (m *Manager) Sessions()",
		"func (m *Manager) PageCache()",
		"func (m *Manager) UserSessions()",
	} {
		if !strings.Contains(string(storesGen), snippet) {
			t.Fatalf("expected generated accessor to contain %q", snippet)
		}
	}

	testSource := `package cache

import "testing"

func TestGeneratedAccessorNames(t *testing.T) {
	t.Setenv("CACHE_DRIVER", "memory")
	t.Setenv("CACHE_SESSIONS_DRIVER", "memory")
	t.Setenv("CACHE_PAGE_CACHE_DRIVER", "file")
	t.Setenv("CACHE_PAGE_CACHE_FILE_DIR", t.TempDir())
	t.Setenv("CACHE_USER_SESSIONS_DRIVER", "sqlite")
	t.Setenv("CACHE_USER_SESSIONS_DSN", "file::memory:?cache=shared")

	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	if err := mgr.Sessions().SetString("sessions", "alpha", 0); err != nil {
		t.Fatalf("Sessions SetString returned error: %v", err)
	}
	if err := mgr.PageCache().SetString("page-cache", "bravo", 0); err != nil {
		t.Fatalf("PageCache SetString returned error: %v", err)
	}
	if err := mgr.UserSessions().SetString("user-sessions", "charlie", 0); err != nil {
		t.Fatalf("UserSessions SetString returned error: %v", err)
	}

	if got, ok, err := mgr.Sessions().GetString("sessions"); err != nil || !ok || got != "alpha" {
		t.Fatalf("Sessions GetString = (%q, %v, %v), want (%q, true, nil)", got, ok, err, "alpha")
	}
	if got, ok, err := mgr.PageCache().GetString("page-cache"); err != nil || !ok || got != "bravo" {
		t.Fatalf("PageCache GetString = (%q, %v, %v), want (%q, true, nil)", got, ok, err, "bravo")
	}
	if got, ok, err := mgr.UserSessions().GetString("user-sessions"); err != nil || !ok || got != "charlie" {
		t.Fatalf("UserSessions GetString = (%q, %v, %v), want (%q, true, nil)", got, ok, err, "charlie")
	}
}
`
	if err := os.WriteFile(filepath.Join(root, "internal", "cache", "generated_accessor_names_test.go"), []byte(testSource), 0o644); err != nil {
		t.Fatalf("write generated test: %v", err)
	}

	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = root
	tidy.Env = append(os.Environ(),
		"GOCACHE=/tmp/gocache",
		"GOMODCACHE=/tmp/gomodcache",
	)
	output, err := tidy.CombinedOutput()
	if err != nil {
		t.Fatalf("go mod tidy failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}

	goTest := exec.Command("go", "test", "./internal/cache", "-run", "TestGeneratedAccessorNames", "-count=1")
	goTest.Dir = root
	goTest.Env = append(os.Environ(),
		"GOCACHE=/tmp/gocache",
		"GOMODCACHE=/tmp/gomodcache",
	)
	output, err = goTest.CombinedOutput()
	if err != nil {
		t.Fatalf("generated cache accessor names test failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}

func TestGenerateCacheFilesAddsDriverImportsToGoMod(t *testing.T) {
	t.Setenv("CACHE_DRIVER", "memory")
	t.Setenv("CACHE_SESSIONS_DRIVER", "sqlite")

	repoRoot := repoRoot(t)
	root, err := os.MkdirTemp(repoRoot, ".tmp-cache-driver-imports-*")
	if err != nil {
		t.Fatalf("mkdir temp module root: %v", err)
	}
	defer os.RemoveAll(root)
	if err := os.MkdirAll(filepath.Join(root, "internal", "cache"), 0o755); err != nil {
		t.Fatalf("mkdir cache package: %v", err)
	}

	goMod := `module example.com/cacheimporttest

go 1.24

	require (
		github.com/goforj/cache v0.1.5
		github.com/goforj/cache/cachecore v0.1.5
		github.com/goforj/cache/cachetest v0.1.5
		github.com/goforj/cache/driver/sqlcore v0.1.5
		github.com/goforj/env/v2 v2.3.1
		github.com/goforj/str v1.2.0
	)
`
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "cache", "manager.go"), loadCacheManagerFixture(t), 0o644); err != nil {
		t.Fatalf("write manager.go: %v", err)
	}

	written, err := GenerateCacheFiles(root)
	if err != nil {
		t.Fatalf("GenerateCacheFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated cache files to be written")
	}

	configGenPath := filepath.Join(root, "internal", "cache", "config_gen.go")
	configGen, err := os.ReadFile(configGenPath)
	if err != nil {
		t.Fatalf("read config_gen.go: %v", err)
	}
	if !strings.Contains(string(configGen), `"github.com/goforj/cache/driver/sqlitecache"`) {
		t.Fatal("expected config_gen.go to import github.com/goforj/cache/driver/sqlitecache")
	}

	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = root
	tidy.Env = append(os.Environ(),
		"GOCACHE=/tmp/gocache",
		"GOMODCACHE=/tmp/gomodcache",
	)
	output, err := tidy.CombinedOutput()
	if err != nil {
		t.Fatalf("go mod tidy failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}

	goModAfter, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod after tidy: %v", err)
	}
	if !strings.Contains(string(goModAfter), "github.com/goforj/cache/driver/sqlitecache") {
		t.Fatal("expected go.mod to contain github.com/goforj/cache/driver/sqlitecache after tidy")
	}

	goTest := exec.Command("go", "test", "./internal/cache", "-run", "TestDoesNotExist", "-count=1")
	goTest.Dir = root
	goTest.Env = append(os.Environ(),
		"GOCACHE=/tmp/gocache",
		"GOMODCACHE=/tmp/gomodcache",
	)
	output, err = goTest.CombinedOutput()
	if err != nil {
		t.Fatalf("generated cache package compile failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}

func TestGenerateCacheFilesWithPinnedDriverModules(t *testing.T) {
	t.Setenv("CACHE_DRIVER", "memory")
	t.Setenv("CACHE_SESSIONS_DRIVER", "sqlite")

	repoRoot := repoRoot(t)
	root, err := os.MkdirTemp(repoRoot, ".tmp-cache-driver-pins-*")
	if err != nil {
		t.Fatalf("mkdir temp module root: %v", err)
	}
	defer os.RemoveAll(root)
	if err := os.MkdirAll(filepath.Join(root, "internal", "cache"), 0o755); err != nil {
		t.Fatalf("mkdir cache package: %v", err)
	}

	goMod := `module example.com/cachepinnedtest

go 1.24

	require (
		github.com/goforj/cache v0.1.5
		github.com/goforj/cache/cachecore v0.1.5
		github.com/goforj/cache/cachetest v0.1.5
		github.com/goforj/cache/driver/sqlcore v0.1.5
		github.com/goforj/cache/driver/sqlitecache v0.1.5
		github.com/goforj/env/v2 v2.3.1
		github.com/goforj/str v1.2.0
	)
`
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "cache", "manager.go"), loadCacheManagerFixture(t), 0o644); err != nil {
		t.Fatalf("write manager.go: %v", err)
	}

	written, err := GenerateCacheFiles(root)
	if err != nil {
		t.Fatalf("GenerateCacheFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated cache files to be written")
	}

	configGenPath := filepath.Join(root, "internal", "cache", "config_gen.go")
	configGen, err := os.ReadFile(configGenPath)
	if err != nil {
		t.Fatalf("read config_gen.go: %v", err)
	}
	if !strings.Contains(string(configGen), `"github.com/goforj/cache/driver/sqlitecache"`) {
		t.Fatal("expected config_gen.go to import github.com/goforj/cache/driver/sqlitecache")
	}

	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = root
	tidy.Env = append(os.Environ(),
		"GOCACHE=/tmp/gocache",
		"GOMODCACHE=/tmp/gomodcache",
	)
	output, err := tidy.CombinedOutput()
	if err != nil {
		t.Fatalf("go mod tidy failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}

	goModAfter, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod after tidy: %v", err)
	}
	if !strings.Contains(string(goModAfter), "github.com/goforj/cache/driver/sqlitecache") {
		t.Fatal("expected pinned go.mod to retain github.com/goforj/cache/driver/sqlitecache after tidy")
	}

	goTest := exec.Command("go", "test", "./internal/cache", "-run", "TestDoesNotExist", "-count=1")
	goTest.Dir = root
	goTest.Env = append(os.Environ(),
		"GOCACHE=/tmp/gocache",
		"GOMODCACHE=/tmp/gomodcache",
	)
	output, err = goTest.CombinedOutput()
	if err != nil {
		t.Fatalf("generated cache package compile failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}

func TestGenerateCacheFilesDriverMatrixCompiles(t *testing.T) {
	t.Setenv("CACHE_DRIVER", "memory")
	t.Setenv("CACHE_REDIS_DRIVER", "redis")
	t.Setenv("CACHE_REDIS_ADDR", "127.0.0.1:6379")
	t.Setenv("CACHE_REDIS_TLS", "true")
	t.Setenv("CACHE_REDIS_INSECURE_SKIP_VERIFY", "true")
	t.Setenv("CACHE_MEMCACHED_DRIVER", "memcached")
	t.Setenv("CACHE_MEMCACHED_ADDRESSES", "127.0.0.1:11211,127.0.0.1:11212")
	t.Setenv("CACHE_DYNAMO_DRIVER", "dynamodb")
	t.Setenv("CACHE_DYNAMO_REGION", "us-east-1")
	t.Setenv("CACHE_DYNAMO_ENDPOINT", "http://127.0.0.1:8000")
	t.Setenv("CACHE_DYNAMO_TABLE", "cache_entries")
	t.Setenv("CACHE_SQLITE_DRIVER", "sqlite")
	t.Setenv("CACHE_SQLITE_DSN", "file::memory:?cache=shared")
	t.Setenv("CACHE_SQLITE_TABLE", "cache_entries")
	t.Setenv("CACHE_POSTGRES_DRIVER", "postgres")
	t.Setenv("CACHE_POSTGRES_DSN", "postgres://user:pass@127.0.0.1:5432/app?sslmode=disable")
	t.Setenv("CACHE_POSTGRES_TABLE", "cache_entries")
	t.Setenv("CACHE_MYSQL_DRIVER", "mysql")
	t.Setenv("CACHE_MYSQL_DSN", "user:pass@tcp(127.0.0.1:3306)/app?parseTime=true")
	t.Setenv("CACHE_MYSQL_TABLE", "cache_entries")
	t.Setenv("CACHE_NATS_DRIVER", "nats")
	t.Setenv("CACHE_NATS_URL", "nats://127.0.0.1:4222")
	t.Setenv("CACHE_NATS_BUCKET", "CACHE_TEST")
	t.Setenv("CACHE_NATS_BUCKET_TTL", "true")
	t.Setenv("CACHE_NATS_BUCKET_TTL_SECONDS", "60")
	t.Setenv("CACHE_NATS_DESCRIPTION", "cache test bucket")
	t.Setenv("CACHE_NATS_HISTORY", "5")
	t.Setenv("CACHE_NATS_MAX_BYTES", "4096")
	t.Setenv("CACHE_NATS_MAX_VALUE_SIZE", "1024")
	t.Setenv("CACHE_NATS_REPLICAS", "1")
	t.Setenv("CACHE_NATS_STORAGE", "file")
	t.Setenv("CACHE_NATS_COMPRESSED", "true")

	repoRoot := repoRoot(t)
	root, err := os.MkdirTemp(repoRoot, ".tmp-cache-driver-matrix-*")
	if err != nil {
		t.Fatalf("mkdir temp module root: %v", err)
	}
	defer os.RemoveAll(root)
	if err := os.MkdirAll(filepath.Join(root, "internal", "cache"), 0o755); err != nil {
		t.Fatalf("mkdir cache package: %v", err)
	}

	goMod := `module example.com/cachedrivermatrix

go 1.24

	require (
		github.com/goforj/cache v0.1.5
		github.com/goforj/cache/cachecore v0.1.5
		github.com/goforj/cache/cachetest v0.1.5
		github.com/goforj/cache/driver/sqlcore v0.1.5
		github.com/goforj/env/v2 v2.3.1
		github.com/goforj/str v1.2.0
	)
`
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "cache", "manager.go"), loadCacheManagerFixture(t), 0o644); err != nil {
		t.Fatalf("write manager.go: %v", err)
	}

	written, err := GenerateCacheFiles(root)
	if err != nil {
		t.Fatalf("GenerateCacheFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated cache files to be written")
	}

	configGenPath := filepath.Join(root, "internal", "cache", "config_gen.go")
	configGen, err := os.ReadFile(configGenPath)
	if err != nil {
		t.Fatalf("read config_gen.go: %v", err)
	}
	for _, importPath := range []string{
		`"github.com/goforj/cache/driver/rediscache"`,
		`"github.com/goforj/cache/driver/memcachedcache"`,
		`"github.com/goforj/cache/driver/dynamocache"`,
		`"github.com/goforj/cache/driver/sqlitecache"`,
		`"github.com/goforj/cache/driver/postgrescache"`,
		`"github.com/goforj/cache/driver/mysqlcache"`,
		`"github.com/goforj/cache/driver/natscache"`,
		`"github.com/nats-io/nats.go"`,
	} {
		if !strings.Contains(string(configGen), importPath) {
			t.Fatalf("expected config_gen.go to import %s", importPath)
		}
	}

	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = root
	tidy.Env = append(os.Environ(),
		"GOCACHE=/tmp/gocache",
		"GOMODCACHE=/tmp/gomodcache",
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
		"github.com/goforj/cache/driver/rediscache",
		"github.com/goforj/cache/driver/memcachedcache",
		"github.com/goforj/cache/driver/dynamocache",
		"github.com/goforj/cache/driver/sqlitecache",
		"github.com/goforj/cache/driver/postgrescache",
		"github.com/goforj/cache/driver/mysqlcache",
		"github.com/goforj/cache/driver/natscache",
	} {
		if !strings.Contains(string(goModAfter), module) {
			t.Fatalf("expected go.mod to contain %s after tidy", module)
		}
	}

	goTest := exec.Command("go", "test", "./internal/cache", "-run", "TestDoesNotExist", "-count=1")
	goTest.Dir = root
	goTest.Env = append(os.Environ(),
		"GOCACHE=/tmp/gocache",
		"GOMODCACHE=/tmp/gomodcache",
	)
	output, err = goTest.CombinedOutput()
	if err != nil {
		t.Fatalf("generated cache package compile failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}

func loadCacheManagerFixture(t *testing.T) []byte {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve test file path")
	}
	fixturePath := filepath.Join(filepath.Dir(currentFile), "..", "forj", "internal", "cache", "manager.go")
	content, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read cache manager fixture: %v", err)
	}
	return content
}
