package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateProjectFilesUsesEnvironmentExampleFallback verifies a clean checkout retains its compiled driver set without a runtime .env file.
func TestGenerateProjectFilesUsesEnvironmentExampleFallback(t *testing.T) {
	root := newGenerationCacheProject(t)
	writeGenerationEnvironmentFile(t, root, ".env.example", "CACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory,redis\n")
	unsetGenerationEnvironment(t, "CACHE_DRIVER", "CACHE_SUPPORTED_DRIVERS")

	if _, _, err := GenerateProjectFiles(root, false, true, false, false, false, false); err != nil {
		t.Fatalf("GenerateProjectFiles returned error: %v", err)
	}

	source := readGeneratedCacheManager(t, root)
	if !strings.Contains(source, `"github.com/goforj/cache/driver/rediscache"`) {
		t.Fatal("expected .env.example CACHE_SUPPORTED_DRIVERS to retain the Redis cache import")
	}
	if _, exists := os.LookupEnv("CACHE_SUPPORTED_DRIVERS"); exists {
		t.Fatal("expected .env.example fallback to be removed after generation")
	}
}

// TestGenerateProjectFilesPrefersEnvironmentFile verifies owner values override matching committed-example defaults and ambient process state.
func TestGenerateProjectFilesPrefersEnvironmentFile(t *testing.T) {
	root := newGenerationCacheProject(t)
	writeGenerationEnvironmentFile(t, root, ".env", "CACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory\n")
	writeGenerationEnvironmentFile(t, root, ".env.example", "CACHE_DRIVER=redis\nCACHE_SUPPORTED_DRIVERS=redis\n")
	t.Setenv("CACHE_DRIVER", "redis")
	t.Setenv("CACHE_SUPPORTED_DRIVERS", "redis")

	if _, _, err := GenerateProjectFiles(root, false, true, false, false, false, false); err != nil {
		t.Fatalf("GenerateProjectFiles returned error: %v", err)
	}

	source := readGeneratedCacheManager(t, root)
	if strings.Contains(source, `"github.com/goforj/cache/driver/rediscache"`) {
		t.Fatal("expected .env to win over ambient and .env.example Redis selections")
	}
}

// TestGenerateProjectFilesReloadsBeforeEnvironmentExampleFallback verifies a prior cached load cannot shape generation for a later project.
func TestGenerateProjectFilesReloadsBeforeEnvironmentExampleFallback(t *testing.T) {
	unsetGenerationEnvironment(t, "CACHE_DRIVER", "CACHE_SUPPORTED_DRIVERS")

	redisRoot := newGenerationCacheProject(t)
	writeGenerationEnvironmentFile(t, redisRoot, ".env", "CACHE_DRIVER=redis\nCACHE_SUPPORTED_DRIVERS=redis\n")
	if _, _, err := GenerateProjectFiles(redisRoot, false, true, false, false, false, false); err != nil {
		t.Fatalf("generate Redis project: %v", err)
	}
	if source := readGeneratedCacheManager(t, redisRoot); !strings.Contains(source, `"github.com/goforj/cache/driver/rediscache"`) {
		t.Fatal("expected first project's .env to select the Redis cache import")
	}

	memoryRoot := newGenerationCacheProject(t)
	writeGenerationEnvironmentFile(t, memoryRoot, ".env.example", "CACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory\n")
	if _, _, err := GenerateProjectFiles(memoryRoot, false, true, false, false, false, false); err != nil {
		t.Fatalf("generate clean-checkout project: %v", err)
	}
	if source := readGeneratedCacheManager(t, memoryRoot); strings.Contains(source, `"github.com/goforj/cache/driver/rediscache"`) {
		t.Fatal("expected Reload to discard the previous project's cached Redis selection")
	}
}

// TestGenerateProjectFilesIgnoresAmbientDriverManifest verifies a shell value cannot replace a clean checkout's committed build contract.
func TestGenerateProjectFilesIgnoresAmbientDriverManifest(t *testing.T) {
	root := newGenerationCacheProject(t)
	writeGenerationEnvironmentFile(t, root, ".env.example", "CACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory\n")
	t.Setenv("CACHE_DRIVER", "redis")
	t.Setenv("CACHE_SUPPORTED_DRIVERS", "redis")

	if _, _, err := GenerateProjectFiles(root, false, true, false, false, false, false); err != nil {
		t.Fatalf("GenerateProjectFiles returned error: %v", err)
	}

	if source := readGeneratedCacheManager(t, root); strings.Contains(source, `"github.com/goforj/cache/driver/rediscache"`) {
		t.Fatal("expected the committed environment example to isolate generation from ambient Redis values")
	}
	if value := os.Getenv("CACHE_SUPPORTED_DRIVERS"); value != "redis" {
		t.Fatalf("expected ambient value to be restored, got %q", value)
	}
}

// TestLoadGenerationEnvironmentIgnoresToolchainKeys prevents a project dotenv file from shaping Go and VCS subprocess execution.
func TestLoadGenerationEnvironmentIgnoresToolchainKeys(t *testing.T) {
	root := t.TempDir()
	writeGenerationEnvironmentFile(t, root, ".env", "PATH=/project/bin\nGOFLAGS=-mod=vendor\nGOPROXY=https://example.invalid\nCACHE_DRIVER=memory\nRUNTIME_MODE=standalone\n")
	t.Setenv("PATH", "/ambient/bin")
	t.Setenv("GOFLAGS", "-mod=readonly")
	t.Setenv("GOPROXY", "https://proxy.golang.org")
	t.Setenv("RUNTIME_MODE", "distributed")

	restore, err := loadGenerationEnvironment(root)
	if err != nil {
		t.Fatalf("loadGenerationEnvironment returned error: %v", err)
	}
	defer restore()
	if got := os.Getenv("PATH"); got != "/ambient/bin" {
		t.Fatalf("project PATH escaped into generation: %q", got)
	}
	if got := os.Getenv("GOFLAGS"); got != "-mod=readonly" {
		t.Fatalf("project GOFLAGS escaped into generation: %q", got)
	}
	if got := os.Getenv("GOPROXY"); got != "https://proxy.golang.org" {
		t.Fatalf("project GOPROXY escaped into generation: %q", got)
	}
	if got := os.Getenv("CACHE_DRIVER"); got != "memory" {
		t.Fatalf("resource snapshot omitted CACHE_DRIVER: %q", got)
	}
	if got := os.Getenv("RUNTIME_MODE"); got != "distributed" {
		t.Fatalf("obsolete project RUNTIME_MODE should not become a generator input: %q", got)
	}
}

// TestLoadGenerationEnvironmentIncludesAppResourceOverlays keeps named-App planning and generation on one owner snapshot.
func TestLoadGenerationEnvironmentIncludesAppResourceOverlays(t *testing.T) {
	root := t.TempDir()
	writeGenerationEnvironmentFile(t, root, ".env", "BILLING_CACHE_DRIVER=redis\nBILLING_CACHE_ADDR=billing.redis.internal:6379\n")
	t.Setenv("BILLING_CACHE_DRIVER", "memory")
	unsetGenerationEnvironment(t, "BILLING_CACHE_ADDR")

	restore, err := loadGenerationEnvironment(root)
	if err != nil {
		t.Fatalf("loadGenerationEnvironment returned error: %v", err)
	}
	if got := os.Getenv("BILLING_CACHE_DRIVER"); got != "redis" {
		restore()
		t.Fatalf("App resource snapshot omitted owner driver: %q", got)
	}
	if got := os.Getenv("BILLING_CACHE_ADDR"); got != "billing.redis.internal:6379" {
		restore()
		t.Fatalf("App resource snapshot omitted owner endpoint: %q", got)
	}
	restore()

	if got := os.Getenv("BILLING_CACHE_DRIVER"); got != "memory" {
		t.Fatalf("App resource snapshot did not restore ambient driver: %q", got)
	}
	if _, exists := os.LookupEnv("BILLING_CACHE_ADDR"); exists {
		t.Fatal("App resource snapshot did not clear its temporary endpoint")
	}
}

// TestGenerationSubprocessEnvironmentRemovesGeneratorInputs keeps temporary credentials out of Go and VCS children.
func TestGenerationSubprocessEnvironmentRemovesGeneratorInputs(t *testing.T) {
	t.Setenv("PATH", "/ambient/bin")
	t.Setenv("CACHE_PASSWORD", "owner-secret")
	t.Setenv("BILLING_CACHE_PASSWORD", "named-app-secret")
	t.Setenv("APP_NAME", "Owner App")
	environment := generationSubprocessEnvironment()
	joined := "\n" + strings.Join(environment, "\n") + "\n"
	if !strings.Contains(joined, "\nPATH=/ambient/bin\n") {
		t.Fatalf("subprocess environment omitted PATH: %q", environment)
	}
	for _, forbidden := range []string{"CACHE_PASSWORD=", "BILLING_CACHE_PASSWORD=", "APP_NAME="} {
		if strings.Contains(joined, "\n"+forbidden) {
			t.Fatalf("subprocess environment exposed %s", forbidden)
		}
	}
}

// newGenerationCacheProject creates only the package needed to exercise cache generation without unrelated resource inputs.
func newGenerationCacheProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "caches"), 0o755); err != nil {
		t.Fatalf("mkdir cache package: %v", err)
	}
	return root
}

// writeGenerationEnvironmentFile keeps environment fixtures at the project root used by the production loader.
func writeGenerationEnvironmentFile(t *testing.T, root string, name string, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// readGeneratedCacheManager reads the artifact whose imports encode the compiled driver manifest.
func readGeneratedCacheManager(t *testing.T, root string) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(root, "internal", "caches", "manager_gen.go"))
	if err != nil {
		t.Fatalf("read generated cache manager: %v", err)
	}
	return string(contents)
}

// unsetGenerationEnvironment isolates fallback tests while restoring any developer-owned ambient values afterward.
func unsetGenerationEnvironment(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		value, exists := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
		t.Cleanup(func() {
			if exists {
				_ = os.Setenv(key, value)
				return
			}
			_ = os.Unsetenv(key)
		})
	}
}
