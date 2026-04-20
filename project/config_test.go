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
  goforj_version: 0.1.0
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

func TestComponentsNormalizedAppliesDependencies(t *testing.T) {
	components := Components{
		OAuth:      true,
		StressTest: true,
	}

	normalized := components.WithResolvedDependencies()

	if !normalized.OAuth || !normalized.Auth || !normalized.Mail {
		t.Fatalf("expected oauth normalization to enable auth and mail: %#v", normalized)
	}
	if !normalized.StressTest || !normalized.Jobs {
		t.Fatalf("expected stress test normalization to enable jobs: %#v", normalized)
	}
	if components.Auth || components.Mail || components.Jobs {
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
