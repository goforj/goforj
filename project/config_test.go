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
}
