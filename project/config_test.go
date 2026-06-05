package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProjectConfigSupportsWatcherEnv(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, ".goforj.yml")
	if err := os.WriteFile(configPath, []byte(`project_name: Test
module_name: example.com/test
updated_at: 2026-03-14 00:00:00 CDT
dev:
  sound_on_watch_error: true
  watches:
    - name: Wire
      watch: -file .go
      exec: wire
      env:
        WIRE_INCREMENTAL: "1"
render:
  components:
    cli: true
  queue_driver: redis
  goforj_version: 0.9.1
  module_replaces:
    github.com/goforj/web: /Users/cmiles/code/web
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(originalWD) }()

	cfg, err := LoadProjectConfig()
	if err != nil {
		t.Fatalf("LoadProjectConfig returned error: %v", err)
	}
	if len(cfg.Dev.Watches) != 1 {
		t.Fatalf("expected 1 watch, got %d", len(cfg.Dev.Watches))
	}
	if cfg.Dev.Watches[0].Env["WIRE_INCREMENTAL"] != "1" {
		t.Fatalf("expected watcher env to be loaded, got %#v", cfg.Dev.Watches[0].Env)
	}
	if got := cfg.Render.ModuleReplaces["github.com/goforj/web"]; got != "/Users/cmiles/code/web" {
		t.Fatalf("expected module replace to be loaded, got %#v", cfg.Render.ModuleReplaces)
	}
}

func TestDefaultNamedAppTargetUsesConvention(t *testing.T) {
	target := DefaultNamedAppTarget("reporting")
	if target.Entrypoint != filepath.Join("cmd", "reporting", "main.go") ||
		target.AppDir != filepath.Join("app", "reporting") ||
		target.WireDir != filepath.Join("app", "reporting", "wire") {
		t.Fatalf("expected conventional reporting paths, got %#v", target)
	}
}

func TestIsSafeAppTargetName(t *testing.T) {
	for _, name := range []string{"app", "reporting", "customer-portal", "ops_api", "v2"} {
		if !IsSafeAppTargetName(name) {
			t.Fatalf("expected %q to be safe", name)
		}
	}
	for _, name := range []string{"", ".", "..", "../reporting", "reporting/api", "reporting api"} {
		if IsSafeAppTargetName(name) {
			t.Fatalf("expected %q to be unsafe", name)
		}
	}
}

func TestIsReservedAppTargetName(t *testing.T) {
	if !IsReservedAppTargetName("wire") {
		t.Fatal("expected wire to be reserved")
	}
	if IsReservedAppTargetName("reporting") {
		t.Fatal("expected reporting not to be reserved")
	}
}

func TestIsNativeFrameworkCommandName(t *testing.T) {
	for _, name := range []string{"build", "dev", "render", "run", "x", "help", "version"} {
		if !IsNativeFrameworkCommandName(name) {
			t.Fatalf("expected %q to be a native framework command", name)
		}
	}
	for _, name := range []string{"app", "reporting", "customer-portal"} {
		if IsNativeFrameworkCommandName(name) {
			t.Fatalf("expected %q not to be a native framework command", name)
		}
	}
}

func TestAppTargetPackageName(t *testing.T) {
	tests := map[string]string{
		"app":             "app",
		"reporting":       "reporting",
		"customer-portal": "customerportal",
		"ops_api":         "opsapi",
		"2fa":             "app2fa",
	}
	for input, want := range tests {
		if got := AppTargetPackageName(input); got != want {
			t.Fatalf("AppTargetPackageName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestComponentsNormalizedAppliesDependencies(t *testing.T) {
	components := Components{
		Grafana:    true,
		OAuth:      true,
		StressTest: true,
	}

	normalized := components.WithResolvedDependencies()

	if !normalized.OAuth || !normalized.Auth || !normalized.Mail {
		t.Fatalf("expected oauth normalization to enable auth and mail: %#v", normalized)
	}
	if !normalized.Grafana || !normalized.Observability || !normalized.Metrics || !normalized.WebAPI || !normalized.Docker {
		t.Fatalf("expected grafana normalization to enable observability, metrics, web api, and docker: %#v", normalized)
	}
	if !normalized.StressTest || !normalized.Jobs {
		t.Fatalf("expected stress test normalization to enable jobs: %#v", normalized)
	}
	if components.Auth || components.Mail || components.Jobs || components.WebAPI || components.Observability || components.Metrics || components.Docker {
		t.Fatalf("expected Normalized to leave the original value unchanged: %#v", components)
	}
}

func TestValidateRenderContractHonorsDependencies(t *testing.T) {
	components := Components{
		OAuth:            true,
		WebAPI:           true,
		DatabasePostgres: true,
	}

	if err := components.ValidateRenderContract(); err != nil {
		t.Fatalf("expected oauth config to validate through normalized auth dependency, got %v", err)
	}
}

func TestStarterKitDefaultsToNone(t *testing.T) {
	if got := DefaultStarterKit(); got != StarterKitNone {
		t.Fatalf("default starter kit = %q, want %q", got, StarterKitNone)
	}
}

func TestValidateStarterKitContractRequiresWebUI(t *testing.T) {
	err := ValidateStarterKitContract(StarterKitVue, Components{})
	if err == nil {
		t.Fatalf("expected vue starter kit without web ui to fail validation")
	}

	err = ValidateStarterKitContract(StarterKitVue, Components{WebUI: true})
	if err != nil {
		t.Fatalf("expected vue starter kit with web ui to validate, got %v", err)
	}
}

func TestLoadProjectConfigPreservesRawComponentSelections(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, ".goforj.yml")
	if err := os.WriteFile(configPath, []byte(`project_name: Test
module_name: example.com/test
updated_at: 2026-03-14 00:00:00 CDT
render:
  components:
    auth: true
    web_api: true
    database_sqlite: true
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(originalWD) }()

	cfg, err := LoadProjectConfig()
	if err != nil {
		t.Fatalf("LoadProjectConfig returned error: %v", err)
	}
	if !cfg.Render.Components.Auth {
		t.Fatalf("expected auth to be loaded")
	}
	if cfg.Render.Components.Mail {
		t.Fatalf("expected raw config load to preserve mail=false, got %#v", cfg.Render.Components)
	}
}

func TestLoadProjectConfigSupportsStarterKit(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, ".goforj.yml")
	if err := os.WriteFile(configPath, []byte(`project_name: Test
module_name: example.com/test
updated_at: 2026-03-14 00:00:00 CDT
render:
  starter_kit: vue
  components:
    web_ui: true
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(originalWD) }()

	cfg, err := LoadProjectConfig()
	if err != nil {
		t.Fatalf("LoadProjectConfig returned error: %v", err)
	}
	if cfg.Render.StarterKit != StarterKitVue {
		t.Fatalf("expected vue starter kit, got %q", cfg.Render.StarterKit)
	}
}
