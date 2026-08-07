package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

func TestProjectRendererMigratesComponentsWithoutPersistingResolvedDependencies(t *testing.T) {
	root := useProjectRendererComponentMigrationRoot(t)
	configPath := filepath.Join(root, ".goforj.yml")
	source := `project_name: Component Migration
module_name: example.com/component-migration
updated_at: "2026-07-14 00:00:00 UTC"
dev:
  pre: []
  down: []
  auto_migrate: false
  down_on_exit: false
  sound_on_watch_error: false
  wire_paths: [app/wire]
render:
  components:
    cli: true
    auth: true
    mail: false
    web_api: true
    database_sqlite: true
  starter_kit: none
  goforj_version: test
`
	if err := os.WriteFile(configPath, []byte(source), 0o644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	renderer := unitProjectRenderer(t)
	if err := renderer.Render(ComponentRenderInput{renderAll: true}); err != nil {
		t.Fatalf("render legacy config: %v", err)
	}

	migrated, err := project.LoadProjectConfig()
	if err != nil {
		t.Fatalf("load migrated config: %v", err)
	}
	want := project.Components{CLI: true, Auth: true, WebAPI: true, DatabaseSQLite: true, Cache: true, Events: true, Storage: true}
	if migrated.Render.Components != want {
		t.Fatalf("persisted components = %#v, want raw selection %#v", migrated.Render.Components, want)
	}
	if migrated.NeedsComponentMigration() {
		t.Fatal("public render left the migrated component mapping in legacy form")
	}
	if renderer.config.Render.Components != want.WithResolvedDependencies() {
		t.Fatalf("effective render components were not restored: %#v", renderer.config.Render.Components)
	}
	if _, err := os.Stat(filepath.Join(root, "internal", "mail", "manager_gen.go")); err != nil {
		t.Fatalf("resolved mail dependency was not rendered: %v", err)
	}
}

// TestSyncProjectConfigForRenderCanonicalizesComponentMappings verifies that full-render sync preserves the semantics selected by the render shape.
func TestSyncProjectConfigForRenderCanonicalizesComponentMappings(t *testing.T) {
	tests := []struct {
		name           string
		source         string
		wantRenderLine string
		wantAppLine    string
	}{
		{
			name:           "render components",
			wantRenderLine: "components: [cli, auth, web_api, database_sqlite, cache, events, storage]",
			wantAppLine:    "components: [cli, cache, events, storage, jobs]",
			source: `project_name: Component Migration
module_name: example.com/component-migration
updated_at: "2026-07-14 00:00:00 UTC"
dev:
  pre: []
  down: []
  auto_migrate: false
  down_on_exit: false
  sound_on_watch_error: false
  wire_paths: [app/wire]
render:
  components:
    cli: true
    auth: true
    web_api: true
    database_sqlite: true
    mail: false
    jobs: false
  starter_kit: none
  goforj_version: test
apps:
  billing:
    components: [cli, jobs]
    starter_kit: none
`,
		},
		{
			name:           "App components",
			wantRenderLine: "components: [cli, web_api]",
			wantAppLine:    "components: [cli, jobs]",
			source: `project_name: Component Migration
module_name: example.com/component-migration
updated_at: "2026-07-14 00:00:00 UTC"
dev:
  pre: []
  down: []
  auto_migrate: false
  down_on_exit: false
  sound_on_watch_error: false
  wire_paths: [app/wire]
render:
  components: [cli, web_api]
  starter_kit: none
  goforj_version: test
apps:
  billing:
    components:
      cli: true
      web_ui: false
      jobs: true
    starter_kit: none
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := useProjectRendererComponentMigrationRoot(t)
			configPath := filepath.Join(root, ".goforj.yml")
			if err := os.WriteFile(configPath, []byte(test.source), 0o644); err != nil {
				t.Fatalf("write legacy config: %v", err)
			}

			config, err := project.LoadProjectConfig()
			if err != nil {
				t.Fatalf("load legacy config: %v", err)
			}
			if !config.NeedsComponentMigration() {
				t.Fatal("legacy component mapping did not request config migration")
			}
			wantRender := config.Render.Components
			wantApps := make(map[string]project.Components, len(config.Apps))
			for name, app := range config.Apps {
				wantApps[name] = app.Components
			}

			config.Render.Components.ResolveDependencies()
			effectiveRender := config.Render.Components
			renderer := projectRendererForTest(t, config)
			if err := renderer.syncProjectConfigForRender(wantRender); err != nil {
				t.Fatalf("sync project config: %v", err)
			}
			if config.Render.Components != effectiveRender {
				t.Fatalf("effective render components changed from %#v to %#v", effectiveRender, config.Render.Components)
			}

			migrated, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("read migrated config: %v", err)
			}
			text := string(migrated)
			for _, want := range []string{
				test.wantRenderLine,
				test.wantAppLine,
			} {
				if !strings.Contains(text, want) {
					t.Fatalf("migrated config missing %q:\n%s", want, text)
				}
			}
			if strings.Contains(text, "component_contract:") {
				t.Fatalf("migrated config retained the obsolete component marker:\n%s", text)
			}
			if strings.Count(text, "components: [") != 2 {
				t.Fatalf("component selections were not normalized to one-line arrays:\n%s", text)
			}

			reloaded, err := project.LoadProjectConfig()
			if err != nil {
				t.Fatalf("reload migrated config: %v", err)
			}
			if reloaded.NeedsComponentMigration() {
				t.Fatal("canonical component arrays still requested migration")
			}
			if reloaded.Render.Components != wantRender {
				t.Fatalf("render component selection changed from %#v to %#v", wantRender, reloaded.Render.Components)
			}
			for name, want := range wantApps {
				if got := reloaded.Apps[name].Components; got != want {
					t.Fatalf("App %s component selection changed from %#v to %#v", name, want, got)
				}
			}
		})
	}
}

// useProjectRendererComponentMigrationRoot isolates config rewrites from the repository checkout.
func useProjectRendererComponentMigrationRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	return root
}
