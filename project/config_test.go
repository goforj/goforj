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
  goforj_version: 0.18.0
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

func TestDefaultNamedAppUsesConvention(t *testing.T) {
	app := DefaultNamedApp("reporting")
	if app.Entrypoint != filepath.Join("cmd", "reporting", "main.go") ||
		app.AppDir != filepath.Join("app", "reporting") ||
		app.WireDir != filepath.Join("app", "reporting", "wire") {
		t.Fatalf("expected conventional reporting paths, got %#v", app)
	}
}

func TestDefaultSelectedComponentsIncludeMetricsStack(t *testing.T) {
	components := DefaultSelectedComponents()
	if !components.Metrics || !components.Observability || !components.Grafana {
		t.Fatalf("expected metrics, observability, and grafana to be selected by default: %#v", components)
	}
}

func TestComponentCatalogDefinitionsHaveDescriptions(t *testing.T) {
	for _, definition := range ComponentCatalog() {
		if definition.Description == "" {
			t.Fatalf("expected component %q to have a wizard description", definition.Key)
		}
	}
}

func TestIsSafeAppName(t *testing.T) {
	for _, name := range []string{"app", "reporting", "customer-portal", "v2"} {
		if !IsSafeAppName(name) {
			t.Fatalf("expected %q to be safe", name)
		}
	}
	for _, name := range []string{"", ".", "..", "../reporting", "reporting/api", "reporting api", "ops_api", "CustomerPortal", "2fa", "-admin", "admin-", "admin--api"} {
		if IsSafeAppName(name) {
			t.Fatalf("expected %q to be unsafe", name)
		}
	}
}

func TestIsReservedAppName(t *testing.T) {
	if !IsReservedAppName("wire") {
		t.Fatal("expected wire to be reserved")
	}
	if IsReservedAppName("reporting") {
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

func TestAppPackageName(t *testing.T) {
	tests := map[string]string{
		"app":             "app",
		"reporting":       "reportingapp",
		"customer-portal": "customerportalapp",
		"2fa":             "app2faapp",
	}
	for input, want := range tests {
		if got := AppPackageName(input); got != want {
			t.Fatalf("AppPackageName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestComponentsNormalizedAppliesDependencies(t *testing.T) {
	components := Components{
		Grafana: true,
		OAuth:   true,
	}

	normalized := components.WithResolvedDependencies()

	if !normalized.OAuth || !normalized.Auth || !normalized.Mail {
		t.Fatalf("expected oauth normalization to enable auth and mail: %#v", normalized)
	}
	if !normalized.Grafana || !normalized.Observability || !normalized.Metrics || !normalized.WebAPI || !normalized.Docker {
		t.Fatalf("expected grafana normalization to enable observability, metrics, web api, and docker: %#v", normalized)
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

func TestAppComponentsFromKeysUsesProjectCapabilities(t *testing.T) {
	available := Components{
		WebAPI:        true,
		WebUI:         true,
		Metrics:       true,
		DatabaseMySQL: true,
		Auth:          true,
		Mail:          true,
		Jobs:          true,
	}

	components, err := AppComponentsFromKeys(available, []ComponentKey{ComponentAuth, ComponentJobs})
	if err != nil {
		t.Fatalf("AppComponentsFromKeys returned error: %v", err)
	}
	if !components.Auth || !components.WebAPI || !components.DatabaseMySQL || !components.Mail || !components.Jobs || !components.Metrics {
		t.Fatalf("app components missing expected dependencies: %#v", components)
	}
	if components.WebUI || components.Docker || components.Observability || components.Grafana || components.DemoApp {
		t.Fatalf("app components included non-selected project-level components: %#v", components)
	}
}

func TestAppComponentsAllowNewDatabaseDriver(t *testing.T) {
	available := Components{
		WebAPI:        true,
		DatabaseMySQL: true,
	}

	components, err := AppComponentsFromKeys(available, []ComponentKey{ComponentWebAPI, ComponentDatabasePostgres})
	if err != nil {
		t.Fatalf("AppComponentsFromKeys returned error: %v", err)
	}
	if !components.WebAPI || !components.DatabasePostgres {
		t.Fatalf("app components missing selected postgres driver: %#v", components)
	}
	if components.DatabaseMySQL || components.DatabaseSQLite {
		t.Fatalf("expected app database selection to be exclusive: %#v", components)
	}
}

func TestAppComponentsKeepLastDatabaseDriver(t *testing.T) {
	components, err := AppComponentsFromKeys(Components{}, []ComponentKey{ComponentDatabaseMySQL, ComponentDatabasePostgres})
	if err != nil {
		t.Fatalf("AppComponentsFromKeys returned error: %v", err)
	}
	if !components.DatabasePostgres || components.DatabaseMySQL || components.DatabaseSQLite {
		t.Fatalf("expected last database driver to win: %#v", components)
	}
}

func TestPromoteAppComponentsAddsProjectCapabilities(t *testing.T) {
	available := Components{
		WebAPI:        true,
		DatabaseMySQL: true,
		Docker:        true,
	}
	selected := Components{
		WebAPI:           true,
		Auth:             true,
		DatabasePostgres: true,
		Jobs:             true,
	}

	promoted := PromoteAppComponents(available, selected)
	if !promoted.WebAPI || !promoted.Auth || !promoted.Mail || !promoted.DatabaseMySQL || !promoted.DatabasePostgres || !promoted.Jobs {
		t.Fatalf("promoted components missing expected capabilities: %#v", promoted)
	}
	if !promoted.Docker {
		t.Fatalf("expected project-level docker capability to be preserved: %#v", promoted)
	}
}

func TestAppComponentsRejectProjectOnlyComponent(t *testing.T) {
	_, err := AppComponentsFromKeys(Components{Docker: true}, []ComponentKey{ComponentDocker})
	if err == nil {
		t.Fatal("expected project-only component to be rejected")
	}
}

func TestParseComponentKeyAcceptsCliSpelling(t *testing.T) {
	key, err := ParseComponentKey("web-api")
	if err != nil {
		t.Fatalf("ParseComponentKey returned error: %v", err)
	}
	if key != ComponentWebAPI {
		t.Fatalf("key = %q, want %q", key, ComponentWebAPI)
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
