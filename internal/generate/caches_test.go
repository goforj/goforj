package generate

import (
	"os"
	"os/exec"
	"path/filepath"
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
	if err := os.MkdirAll(filepath.Join(root, "internal", "caches"), 0o755); err != nil {
		t.Fatalf("mkdir cache package: %v", err)
	}

	written, err := GenerateCacheFiles(root)
	if err != nil {
		t.Fatalf("GenerateCacheFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated cache files to be written")
	}

	for _, generatedPath := range []string{
		filepath.Join(root, "internal", "caches", "manager_gen.go"),
		filepath.Join(root, "internal", "caches", "accessors_gen.go"),
	} {
		if _, err := os.Stat(generatedPath); err != nil {
			t.Fatalf("expected generated file %s: %v", generatedPath, err)
		}
	}

	storesGen, err := os.ReadFile(filepath.Join(root, "internal", "caches", "accessors_gen.go"))
	if err != nil {
		t.Fatalf("read accessors_gen.go: %v", err)
	}
	for _, snippet := range []string{
		"func (m *Manager) Sessions()",
		"return m.sessions",
		"func (m *Manager) Pages()",
	} {
		if !strings.Contains(string(storesGen), snippet) {
			t.Fatalf("expected generated accessors to contain %q", snippet)
		}
	}
	managerGen, err := os.ReadFile(filepath.Join(root, "internal", "caches", "manager_gen.go"))
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}
	for _, snippet := range []string{
		`Name: "cache_sessions"`,
		`func (m *Manager) ReadinessChecks() []ReadinessCheck`,
	} {
		if !strings.Contains(string(managerGen), snippet) {
			t.Fatalf("expected generated manager to contain %q", snippet)
		}
	}

	testSource := `package caches

import (
	"context"
	"testing"
	"time"

	"github.com/goforj/cache/cachecore"
)

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
	mgr = mgr.WithObserver(ObserverFunc(func(_ context.Context, name string, op string, _ string, hit bool, err error, _ time.Duration, driver cachecore.Driver) {
		if err != nil {
			t.Fatalf("observer saw error: %v", err)
		}
		observed = append(observed, name+":"+op+":"+string(driver))
		if op == "get_string" && !hit {
			t.Fatal("expected cache hit for generated observer test")
		}
	}))

	if _, ok, err := mgr.Sessions().GetString("session"); err != nil || !ok {
		t.Fatalf("observer get string = (ok=%v, err=%v), want (true, nil)", ok, err)
	}
	if len(observed) == 0 {
		t.Fatal("expected observer to capture cache operations")
	}
}
`
	if err := os.WriteFile(filepath.Join(root, "internal", "caches", "generated_accessors_test.go"), []byte(testSource), 0o644); err != nil {
		t.Fatalf("write generated test: %v", err)
	}

	relRoot, err := filepath.Rel(repoRoot, root)
	if err != nil {
		t.Fatalf("relative temp path: %v", err)
	}
	pkgPath := "./" + filepath.ToSlash(filepath.Join(relRoot, "internal", "caches"))
	cmd := exec.Command("go", "test", pkgPath, "-run", "TestGeneratedAccessors", "-count=1")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "GOCACHE=/tmp/gocache")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated cache package test failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}

func TestGenerateCacheFilesChainsMultipleObservers(t *testing.T) {
	t.Setenv("CACHE_DRIVER", "memory")
	t.Setenv("CACHE_SESSIONS_DRIVER", "memory")

	repoRoot := repoRoot(t)
	root, err := os.MkdirTemp(repoRoot, ".tmp-cache-observer-chain-*")
	if err != nil {
		t.Fatalf("mkdir temp generation root: %v", err)
	}
	defer os.RemoveAll(root)
	if err := os.MkdirAll(filepath.Join(root, "internal", "caches"), 0o755); err != nil {
		t.Fatalf("mkdir cache package: %v", err)
	}

	if _, err := GenerateCacheFiles(root); err != nil {
		t.Fatalf("GenerateCacheFiles returned error: %v", err)
	}

	testSource := `package caches

import (
	"context"
	"testing"
	"time"

	"github.com/goforj/cache/cachecore"
)

func TestObserverChain(t *testing.T) {
	t.Setenv("CACHE_DRIVER", "memory")
	t.Setenv("CACHE_SESSIONS_DRIVER", "memory")

	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	if err := mgr.Sessions().SetString("session", "alpha", 0); err != nil {
		t.Fatalf("prime session cache: %v", err)
	}

	var metricsOps int
	var inspectOps int
	mgr = mgr.WithObserver(ObserverFunc(func(_ context.Context, name string, op string, _ string, _ bool, err error, _ time.Duration, _ cachecore.Driver) {
		if err != nil {
			t.Fatalf("metrics observer saw error: %v", err)
		}
		if name == "sessions" && op == "get_string" {
			metricsOps++
		}
	}))
	mgr = mgr.WithObserver(ObserverFunc(func(_ context.Context, name string, op string, _ string, _ bool, err error, _ time.Duration, _ cachecore.Driver) {
		if err != nil {
			t.Fatalf("inspect observer saw error: %v", err)
		}
		if name == "sessions" && op == "get_string" {
			inspectOps++
		}
	}))

	if _, ok, err := mgr.Sessions().GetString("session"); err != nil || !ok {
		t.Fatalf("sessions GetString = (ok=%v, err=%v), want (true, nil)", ok, err)
	}
	if metricsOps != 1 {
		t.Fatalf("metrics observer count = %d, want 1", metricsOps)
	}
	if inspectOps != 1 {
		t.Fatalf("inspect observer count = %d, want 1", inspectOps)
	}
}
`
	if err := os.WriteFile(filepath.Join(root, "internal", "caches", "observer_chain_test.go"), []byte(testSource), 0o644); err != nil {
		t.Fatalf("write generated test: %v", err)
	}

	relRoot, err := filepath.Rel(repoRoot, root)
	if err != nil {
		t.Fatalf("relative temp path: %v", err)
	}
	pkgPath := "./" + filepath.ToSlash(filepath.Join(relRoot, "internal", "caches"))
	cmd := exec.Command("go", "test", pkgPath, "-run", "TestObserverChain", "-count=1")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "GOCACHE=/tmp/gocache")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated cache package observer-chain test failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}

func TestGenerateCacheFilesUsesSupportedDriverImports(t *testing.T) {
	t.Setenv("CACHE_DRIVER", "memory")
	t.Setenv("CACHE_SUPPORTED_DRIVERS", "memory,redis")

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "caches"), 0o755); err != nil {
		t.Fatalf("mkdir cache package: %v", err)
	}

	if _, err := GenerateCacheFiles(root); err != nil {
		t.Fatalf("GenerateCacheFiles returned error: %v", err)
	}

	managerGen, err := os.ReadFile(filepath.Join(root, "internal", "caches", "manager_gen.go"))
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}
	source := string(managerGen)
	if !strings.Contains(source, `"github.com/goforj/cache/driver/rediscache"`) {
		t.Fatal("expected generated cache manager to import rediscache from CACHE_SUPPORTED_DRIVERS")
	}
}

func TestGenerateCacheFilesAlwaysExposesSessionsAccessor(t *testing.T) {
	t.Setenv("CACHE_DRIVER", "memory")

	root := mustTempGeneratedModuleRoot(t, ".tmp-cache-sessions-accessor-*", filepath.Join("internal", "caches"))
	writeFixtureGoMod(t, root, fixtureModuleSpec(
		"example.com/cachesessionsaccessor",
		[]string{
			"github.com/goforj/cache",
			"github.com/goforj/cache/cachecore",
			"github.com/goforj/cache/cachetest",
			"github.com/goforj/env/v2",
			"github.com/goforj/str",
		},
		nil,
		cacheLocalReplaces(t),
	))

	written, err := GenerateCacheFiles(root)
	if err != nil {
		t.Fatalf("GenerateCacheFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated cache files to be written")
	}

	accessorsGen, err := os.ReadFile(filepath.Join(root, "internal", "caches", "accessors_gen.go"))
	if err != nil {
		t.Fatalf("read accessors_gen.go: %v", err)
	}
	if !strings.Contains(string(accessorsGen), "func (m *Manager) Sessions() *cache.Cache") {
		t.Fatal("expected generated cache accessors to always expose Sessions accessor")
	}

	testSource := `package caches

import "testing"

func TestSessionsAccessorExistsWithoutNamedStore(t *testing.T) {
	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	if mgr.Sessions() != nil {
		t.Fatal("expected Sessions accessor to return nil when sessions cache is not configured")
	}
}
`
	if err := os.WriteFile(filepath.Join(root, "internal", "caches", "sessions_accessor_test.go"), []byte(testSource), 0o644); err != nil {
		t.Fatalf("write generated test: %v", err)
	}

	runFixtureGoModTidy(t, root, map[string]string{"GOPROXY": "direct"})
	runFixtureGoTest(t, root, "./internal/caches", "TestSessionsAccessorExistsWithoutNamedStore", nil)
}

func TestGenerateCacheFilesDerivesAccessorNamesFromCacheNames(t *testing.T) {
	t.Setenv("CACHE_DRIVER", "memory")
	t.Setenv("CACHE_SESSIONS_DRIVER", "redis")
	t.Setenv("CACHE_PAGE_CACHE_DRIVER", "memcached")
	t.Setenv("CACHE_USER_SESSIONS_DRIVER", "sqlite")
	t.Setenv("CACHE_USER_SESSIONS_DSN", "file::memory:?cache=shared")

	root := mustTempGeneratedModuleRoot(t, ".tmp-cache-accessor-names-*", filepath.Join("internal", "caches"))
	writeFixtureGoMod(t, root, fixtureModuleSpec(
		"example.com/cacheaccessornametest",
		[]string{
			"github.com/goforj/cache",
			"github.com/goforj/cache/cachecore",
			"github.com/goforj/cache/cachetest",
			"github.com/goforj/cache/driver/dynamocache",
			"github.com/goforj/cache/driver/memcachedcache",
			"github.com/goforj/cache/driver/mysqlcache",
			"github.com/goforj/cache/driver/natscache",
			"github.com/goforj/cache/driver/postgrescache",
			"github.com/goforj/cache/driver/rediscache",
			"github.com/goforj/cache/driver/sqlcore",
			"github.com/goforj/cache/driver/sqlitecache",
			"github.com/goforj/env/v2",
			"github.com/nats-io/nats.go",
			"github.com/goforj/str",
		},
		nil,
		cacheLocalReplaces(t),
	))
	written, err := GenerateCacheFiles(root)
	if err != nil {
		t.Fatalf("GenerateCacheFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated cache files to be written")
	}

	storesGen, err := os.ReadFile(filepath.Join(root, "internal", "caches", "accessors_gen.go"))
	if err != nil {
		t.Fatalf("read accessors_gen.go: %v", err)
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

	testSource := `package caches

import "testing"

func TestGeneratedAccessorNames(t *testing.T) {
	_ = (*Manager).Sessions
	_ = (*Manager).PageCache
	_ = (*Manager).UserSessions
}
`
	if err := os.WriteFile(filepath.Join(root, "internal", "caches", "generated_accessor_names_test.go"), []byte(testSource), 0o644); err != nil {
		t.Fatalf("write generated test: %v", err)
	}

	runFixtureGoModTidy(t, root, map[string]string{"GOPROXY": "direct"})
	runFixtureGoTest(t, root, "./internal/caches", "TestGeneratedAccessorNames", nil)
}

func TestGenerateCacheFilesRejectsUnknownEnvVars(t *testing.T) {
	t.Setenv("CACHE_DRIVER", "memory")
	t.Setenv("CACHE_SESSIONS_DRVIER", "redis")

	_, err := GenerateCacheFiles(t.TempDir())
	if err == nil {
		t.Fatal("expected GenerateCacheFiles to reject unknown cache env vars")
	}
	if !strings.Contains(err.Error(), "CACHE_SESSIONS_DRVIER") {
		t.Fatalf("expected error to mention unknown env var, got: %v", err)
	}
}

func TestGenerateCacheFilesRejectsWrongDriverEnvVars(t *testing.T) {
	t.Setenv("CACHE_DRIVER", "memory")
	t.Setenv("CACHE_SESSIONS_DRIVER", "redis")
	t.Setenv("CACHE_SESSIONS_FILE_DIR", t.TempDir())

	_, err := GenerateCacheFiles(t.TempDir())
	if err == nil {
		t.Fatal("expected GenerateCacheFiles to reject wrong-driver cache env vars")
	}
	if !strings.Contains(err.Error(), "CACHE_SESSIONS_FILE_DIR") {
		t.Fatalf("expected error to mention wrong-driver env var, got: %v", err)
	}
	if !strings.Contains(err.Error(), `driver "redis"`) {
		t.Fatalf("expected error to mention redis driver, got: %v", err)
	}
}

func TestGenerateCacheFilesAllowsInactiveRootDriverEnvVars(t *testing.T) {
	t.Setenv("CACHE_DRIVER", "memory")
	t.Setenv("CACHE_ADDR", "127.0.0.1:6379")
	t.Setenv("CACHE_DB", "0")

	if _, err := GenerateCacheFiles(t.TempDir()); err != nil {
		t.Fatalf("expected GenerateCacheFiles to allow documented inactive root cache env vars, got %v", err)
	}
}

func TestGenerateCacheFilesAddsDriverImportsToGoMod(t *testing.T) {
	t.Setenv("CACHE_DRIVER", "memory")
	t.Setenv("CACHE_SESSIONS_DRIVER", "sqlite")

	root := mustTempGeneratedModuleRoot(t, ".tmp-cache-driver-imports-*", filepath.Join("internal", "caches"))
	writeFixtureGoMod(t, root, fixtureModuleSpec(
		"example.com/cacheimporttest",
		[]string{
			"github.com/goforj/cache",
			"github.com/goforj/cache/cachecore",
			"github.com/goforj/cache/cachetest",
			"github.com/goforj/cache/driver/sqlcore",
			"github.com/goforj/env/v2",
			"github.com/goforj/str",
		},
		nil,
		cacheLocalReplaces(t),
	))
	written, err := GenerateCacheFiles(root)
	if err != nil {
		t.Fatalf("GenerateCacheFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated cache files to be written")
	}

	configGenPath := filepath.Join(root, "internal", "caches", "manager_gen.go")
	configGen, err := os.ReadFile(configGenPath)
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}
	if !strings.Contains(string(configGen), `"github.com/goforj/cache/driver/sqlitecache"`) {
		t.Fatal("expected manager_gen.go to import github.com/goforj/cache/driver/sqlitecache")
	}

	runFixtureGoModTidy(t, root, map[string]string{"GOPROXY": "direct"})
	assertFixtureGoModContains(t, root, "github.com/goforj/cache/driver/sqlitecache")
	runFixtureGoTest(t, root, "./internal/caches", "TestDoesNotExist", nil)
}

func TestGenerateCacheFilesWithPinnedDriverModules(t *testing.T) {
	t.Setenv("CACHE_DRIVER", "memory")
	t.Setenv("CACHE_SESSIONS_DRIVER", "sqlite")

	root := mustTempGeneratedModuleRoot(t, ".tmp-cache-driver-pins-*", filepath.Join("internal", "caches"))
	writeFixtureGoMod(t, root, fixtureModuleSpec(
		"example.com/cachepinnedtest",
		[]string{
			"github.com/goforj/cache",
			"github.com/goforj/cache/cachecore",
			"github.com/goforj/cache/cachetest",
			"github.com/goforj/cache/driver/sqlcore",
			"github.com/goforj/env/v2",
			"github.com/goforj/str",
		},
		[]string{"github.com/goforj/cache/driver/sqlitecache"},
		cacheLocalReplaces(t),
	))
	written, err := GenerateCacheFiles(root)
	if err != nil {
		t.Fatalf("GenerateCacheFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated cache files to be written")
	}

	configGenPath := filepath.Join(root, "internal", "caches", "manager_gen.go")
	configGen, err := os.ReadFile(configGenPath)
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}
	if !strings.Contains(string(configGen), `"github.com/goforj/cache/driver/sqlitecache"`) {
		t.Fatal("expected manager_gen.go to import github.com/goforj/cache/driver/sqlitecache")
	}

	runFixtureGoModTidy(t, root, map[string]string{"GOPROXY": "direct"})
	assertFixtureGoModContains(t, root, "github.com/goforj/cache/driver/sqlitecache")
	runFixtureGoTest(t, root, "./internal/caches", "TestDoesNotExist", nil)
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

	root := mustTempGeneratedModuleRoot(t, ".tmp-cache-driver-matrix-*", filepath.Join("internal", "caches"))
	writeFixtureGoMod(t, root, fixtureModuleSpec(
		"example.com/cachedrivermatrix",
		[]string{
			"github.com/goforj/cache",
			"github.com/goforj/cache/cachecore",
			"github.com/goforj/cache/cachetest",
			"github.com/goforj/cache/driver/sqlcore",
			"github.com/goforj/env/v2",
			"github.com/goforj/str",
		},
		nil,
		cacheLocalReplaces(t),
	))
	written, err := GenerateCacheFiles(root)
	if err != nil {
		t.Fatalf("GenerateCacheFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated cache files to be written")
	}

	configGenPath := filepath.Join(root, "internal", "caches", "manager_gen.go")
	configGen, err := os.ReadFile(configGenPath)
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
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
			t.Fatalf("expected manager_gen.go to import %s", importPath)
		}
	}

	runFixtureGoModTidy(t, root, map[string]string{"GOPROXY": "direct"})
	assertFixtureGoModContains(t, root,
		"github.com/goforj/cache/driver/rediscache",
		"github.com/goforj/cache/driver/memcachedcache",
		"github.com/goforj/cache/driver/dynamocache",
		"github.com/goforj/cache/driver/sqlitecache",
		"github.com/goforj/cache/driver/postgrescache",
		"github.com/goforj/cache/driver/mysqlcache",
		"github.com/goforj/cache/driver/natscache",
	)
	runFixtureGoTest(t, root, "./internal/caches", "TestDoesNotExist", nil)
}
