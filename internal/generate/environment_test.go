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

	if _, _, err := GenerateProjectFiles(root, GenerationSelection{Cache: true}); err != nil {
		t.Fatalf("GenerateProjectFiles returned error: %v", err)
	}

	source := readGeneratedCacheManager(t, root)
	if !strings.Contains(source, `"github.com/goforj/cache/driver/rediscache"`) {
		t.Fatal("expected .env.example CACHE_SUPPORTED_DRIVERS to retain the Redis cache import")
	}
	if _, exists := os.LookupEnv("CACHE_SUPPORTED_DRIVERS"); exists {
		t.Fatal("expected generation to leave the unset ambient value unchanged")
	}
}

// TestGenerateProjectFilesPrefersEnvironmentFile verifies owner values override matching committed-example defaults and ambient process state.
func TestGenerateProjectFilesPrefersEnvironmentFile(t *testing.T) {
	root := newGenerationCacheProject(t)
	writeGenerationEnvironmentFile(t, root, ".env", "CACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=\n")
	writeGenerationEnvironmentFile(t, root, ".env.example", "CACHE_DRIVER=redis\nCACHE_SUPPORTED_DRIVERS=redis\n")
	t.Setenv("CACHE_DRIVER", "redis")
	t.Setenv("CACHE_SUPPORTED_DRIVERS", "redis")
	input, err := loadProjectGenerationInput(root)
	if err != nil {
		t.Fatalf("load project input: %v", err)
	}
	if value, exists := input.environment.Lookup("CACHE_SUPPORTED_DRIVERS"); !exists || value != "" {
		t.Fatalf("owner blank CACHE_SUPPORTED_DRIVERS = %q, %t; want blank, true", value, exists)
	}

	if _, _, err := GenerateProjectFiles(root, GenerationSelection{Cache: true}); err != nil {
		t.Fatalf("GenerateProjectFiles returned error: %v", err)
	}

	source := readGeneratedCacheManager(t, root)
	if strings.Contains(source, `"github.com/goforj/cache/driver/rediscache"`) {
		t.Fatal("expected .env to win over ambient and .env.example Redis selections")
	}
}

// TestGenerateProjectFilesIsolatesConcurrentProjectEnvironments verifies simultaneous projects cannot exchange driver manifests.
func TestGenerateProjectFilesIsolatesConcurrentProjectEnvironments(t *testing.T) {
	unsetGenerationEnvironment(t, "CACHE_DRIVER", "CACHE_SUPPORTED_DRIVERS")

	redisRoot := newGenerationCacheProject(t)
	writeGenerationEnvironmentFile(t, redisRoot, ".env", "CACHE_DRIVER=redis\nCACHE_SUPPORTED_DRIVERS=redis\n")
	memoryRoot := newGenerationCacheProject(t)
	writeGenerationEnvironmentFile(t, memoryRoot, ".env.example", "CACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory\n")

	start := make(chan struct{})
	errors := make(chan error, 2)
	for _, root := range []string{redisRoot, memoryRoot} {
		go func() {
			<-start
			_, _, err := GenerateProjectFiles(root, GenerationSelection{Cache: true})
			errors <- err
		}()
	}
	close(start)
	for range 2 {
		if err := <-errors; err != nil {
			t.Fatalf("generate project: %v", err)
		}
	}

	if source := readGeneratedCacheManager(t, redisRoot); !strings.Contains(source, `"github.com/goforj/cache/driver/rediscache"`) {
		t.Fatal("expected Redis project's .env to select the Redis cache import")
	}
	if source := readGeneratedCacheManager(t, memoryRoot); strings.Contains(source, `"github.com/goforj/cache/driver/rediscache"`) {
		t.Fatal("expected memory project's .env.example to exclude the Redis cache import")
	}
}

// TestGenerateProjectFilesUsesAppOverlayManifestFromProjectEnvironment verifies production orchestration compiles drivers selected only by an App overlay.
func TestGenerateProjectFilesUsesAppOverlayManifestFromProjectEnvironment(t *testing.T) {
	root := newGenerationCacheProject(t)
	writeGenerationEnvironmentFile(t, root, ".env", "CACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory,redis\nBILLING_CACHE_DRIVER=redis\n")

	if _, _, err := GenerateProjectFiles(root, GenerationSelection{Cache: true}); err != nil {
		t.Fatalf("GenerateProjectFiles returned error: %v", err)
	}
	if source := readGeneratedCacheManager(t, root); !strings.Contains(source, `"github.com/goforj/cache/driver/rediscache"`) {
		t.Fatal("expected the App overlay to retain the Redis cache import")
	}
}

// TestGenerateProjectFilesRejectsAppResourceTypos verifies recognized Apps retain malformed keys long enough to report them.
func TestGenerateProjectFilesRejectsAppResourceTypos(t *testing.T) {
	tests := []struct {
		name        string
		projectFile string
		environment string
	}{
		{
			name:        "configured App",
			projectFile: generationCacheProjectConfig,
			environment: "CACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory,redis\nBILLING_CACHE_ADRR=invalid\n",
		},
		{
			name:        "inferred App",
			environment: "CACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory,redis\nBILLING_CACHE_DRIVER=redis\nBILLING_CACHE_ADRR=invalid\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := newGenerationCacheProject(t)
			if test.projectFile != "" {
				writeGenerationEnvironmentFile(t, root, ".goforj.yml", test.projectFile)
			}
			writeGenerationEnvironmentFile(t, root, ".env", test.environment)

			_, _, err := GenerateProjectFiles(root, GenerationSelection{Cache: true})
			if err == nil || !strings.Contains(err.Error(), "BILLING_CACHE_ADRR") {
				t.Fatalf("GenerateProjectFiles error = %v, want BILLING_CACHE_ADRR validation", err)
			}
		})
	}
}

// TestGenerateProjectFilesIgnoresAmbientDriverManifest verifies a shell value cannot replace a clean checkout's committed build contract.
func TestGenerateProjectFilesIgnoresAmbientDriverManifest(t *testing.T) {
	root := newGenerationCacheProject(t)
	writeGenerationEnvironmentFile(t, root, ".env.example", "CACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory\n")
	t.Setenv("CACHE_DRIVER", "redis")
	t.Setenv("CACHE_SUPPORTED_DRIVERS", "redis")

	if _, _, err := GenerateProjectFiles(root, GenerationSelection{Cache: true}); err != nil {
		t.Fatalf("GenerateProjectFiles returned error: %v", err)
	}

	if source := readGeneratedCacheManager(t, root); strings.Contains(source, `"github.com/goforj/cache/driver/rediscache"`) {
		t.Fatal("expected the committed environment example to isolate generation from ambient Redis values")
	}
	if value := os.Getenv("CACHE_SUPPORTED_DRIVERS"); value != "redis" {
		t.Fatalf("expected ambient value to be restored, got %q", value)
	}
}

// TestLoadProjectGenerationInputIgnoresToolchainKeys prevents a project dotenv file from shaping Go and VCS subprocess execution.
func TestLoadProjectGenerationInputIgnoresToolchainKeys(t *testing.T) {
	root := t.TempDir()
	writeGenerationEnvironmentFile(t, root, ".env", "PATH=/project/bin\nGOFLAGS=-mod=vendor\nGOPROXY=https://example.invalid\nCACHE_DRIVER=memory\nRUNTIME_MODE=standalone\n")
	t.Setenv("PATH", "/ambient/bin")
	t.Setenv("GOFLAGS", "-mod=readonly")
	t.Setenv("GOPROXY", "https://proxy.golang.org")
	t.Setenv("CACHE_DRIVER", "redis")
	t.Setenv("RUNTIME_MODE", "distributed")

	input, err := loadProjectGenerationInput(root)
	if err != nil {
		t.Fatalf("loadProjectGenerationInput returned error: %v", err)
	}
	if got := os.Getenv("PATH"); got != "/ambient/bin" {
		t.Fatalf("project PATH escaped into generation: %q", got)
	}
	if got := os.Getenv("GOFLAGS"); got != "-mod=readonly" {
		t.Fatalf("project GOFLAGS escaped into generation: %q", got)
	}
	if got := os.Getenv("GOPROXY"); got != "https://proxy.golang.org" {
		t.Fatalf("project GOPROXY escaped into generation: %q", got)
	}
	if got := input.environment.Get("CACHE_DRIVER", ""); got != "memory" {
		t.Fatalf("resource snapshot omitted CACHE_DRIVER: %q", got)
	}
	if got := os.Getenv("CACHE_DRIVER"); got != "redis" {
		t.Fatalf("resource snapshot changed ambient CACHE_DRIVER: %q", got)
	}
	if got := os.Getenv("RUNTIME_MODE"); got != "distributed" {
		t.Fatalf("obsolete project RUNTIME_MODE should not become a generator input: %q", got)
	}
	if _, exists := input.environment.Lookup("RUNTIME_MODE"); exists {
		t.Fatal("obsolete project RUNTIME_MODE became a generator input")
	}
}

// TestLoadProjectGenerationInputIncludesAppResourceOverlays keeps named-App planning and generation on one owner snapshot.
func TestLoadProjectGenerationInputIncludesAppResourceOverlays(t *testing.T) {
	root := t.TempDir()
	writeGenerationEnvironmentFile(t, root, ".env", "BILLING_CACHE_DRIVER=redis\nBILLING_CACHE_ADDR=billing.redis.internal:6379\n")
	t.Setenv("BILLING_CACHE_DRIVER", "memory")
	unsetGenerationEnvironment(t, "BILLING_CACHE_ADDR")

	input, err := loadProjectGenerationInput(root)
	if err != nil {
		t.Fatalf("loadProjectGenerationInput returned error: %v", err)
	}
	if got := input.environment.Get("BILLING_CACHE_DRIVER", ""); got != "redis" {
		t.Fatalf("App resource snapshot omitted owner driver: %q", got)
	}
	if got := input.environment.Get("BILLING_CACHE_ADDR", ""); got != "billing.redis.internal:6379" {
		t.Fatalf("App resource snapshot omitted owner endpoint: %q", got)
	}
	if got := os.Getenv("BILLING_CACHE_DRIVER"); got != "memory" {
		t.Fatalf("App resource snapshot changed ambient driver: %q", got)
	}
	if _, exists := os.LookupEnv("BILLING_CACHE_ADDR"); exists {
		t.Fatal("App resource snapshot installed its endpoint into process state")
	}
}

// TestGenerationEnvironmentKeyRejectsUnrelatedCacheNames prevents common toolchain variables from masquerading as App cache overlays.
func TestGenerationEnvironmentKeyRejectsUnrelatedCacheNames(t *testing.T) {
	for _, key := range []string{"XDG_CACHE_HOME", "GOCACHE", "GOMODCACHE", "PIP_CACHE_DIR", "UV_CACHE_DIR", "SCCACHE_CACHE_SIZE"} {
		if isGenerationEnvironmentKey(key) {
			t.Fatalf("isGenerationEnvironmentKey(%q) = true, want false", key)
		}
	}
	for _, key := range []string{"CACHE_DRIVER", "BILLING_CACHE_DRIVER", "BILLING_CACHE_SESSIONS_DRIVER"} {
		if !isGenerationEnvironmentKey(key) {
			t.Fatalf("isGenerationEnvironmentKey(%q) = false, want true", key)
		}
	}
}

// TestGenerationSubprocessEnvironmentRemovesGeneratorInputs keeps temporary credentials out of Go and VCS children.
func TestGenerationSubprocessEnvironmentRemovesGeneratorInputs(t *testing.T) {
	root := newGenerationCacheProject(t)
	writeGenerationEnvironmentFile(t, root, ".goforj.yml", generationCacheProjectConfig)
	t.Setenv("PATH", "/ambient/bin")
	t.Setenv("CACHE_PASSWORD", "owner-secret")
	t.Setenv("BILLING_CACHE_ADRR", "invalid-secret")
	t.Setenv("APP_NAME", "Owner App")
	t.Setenv("XDG_CACHE_HOME", "/tool/xdg")
	t.Setenv("PIP_CACHE_DIR", "/tool/pip")
	t.Setenv("UV_CACHE_DIR", "/tool/uv")
	t.Setenv("SCCACHE_CACHE_SIZE", "1G")
	environment := generationSubprocessEnvironment(root)
	joined := "\n" + strings.Join(environment, "\n") + "\n"
	for _, preserved := range []string{"PATH=/ambient/bin", "XDG_CACHE_HOME=/tool/xdg", "PIP_CACHE_DIR=/tool/pip", "UV_CACHE_DIR=/tool/uv", "SCCACHE_CACHE_SIZE=1G"} {
		if !strings.Contains(joined, "\n"+preserved+"\n") {
			t.Fatalf("subprocess environment omitted %s: %q", preserved, environment)
		}
	}
	for _, forbidden := range []string{"CACHE_PASSWORD=", "BILLING_CACHE_ADRR=", "APP_NAME="} {
		if strings.Contains(joined, "\n"+forbidden) {
			t.Fatalf("subprocess environment exposed %s", forbidden)
		}
	}
}

// TestGenerateProjectFilesPreservesUnmanagedObservabilityTargets verifies a blank project-owned host remains an explicit opt-out.
func TestGenerateProjectFilesPreservesUnmanagedObservabilityTargets(t *testing.T) {
	root := observabilityTestProjectDir(t, "http")
	targetsPath := filepath.Join(root, "containers", "observability", "vmagent", "metrics-targets.json")
	original := []byte("[\"custom\"]\n")
	if err := os.WriteFile(targetsPath, original, 0o644); err != nil {
		t.Fatalf("write custom targets: %v", err)
	}
	writeGenerationEnvironmentFile(t, root, ".env", "OBSERVABILITY_METRICS_TARGET_MODE=local-single\nOBSERVABILITY_METRICS_TARGET_HOST=\n")

	if _, _, err := GenerateProjectFiles(root, GenerationSelection{Observability: true}); err != nil {
		t.Fatalf("GenerateProjectFiles returned error: %v", err)
	}
	content, err := os.ReadFile(targetsPath)
	if err != nil {
		t.Fatalf("read custom targets: %v", err)
	}
	if string(content) != string(original) {
		t.Fatalf("metrics-targets.json was modified:\n%s", content)
	}
}

const generationCacheProjectConfig = `project_name: Cache overlays
module_name: example.com/cache-overlays
render:
  component_contract: 1
  components: [cli, cache]
apps:
  billing:
    components: [cli, cache]
`

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
