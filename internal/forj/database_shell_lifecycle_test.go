package forj

import (
	"go/format"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/forj/makeapp"
	"github.com/goforj/goforj/project"
)

// TestDatabaseShellTemplatesFollowAppParticipation keeps DB shell ownership on the refreshed App command surface.
func TestDatabaseShellTemplatesFollowAppParticipation(t *testing.T) {
	tests := []struct {
		name            string
		defaultDatabase bool
		workerDatabase  bool
	}{
		{name: "all Apps disabled"},
		{name: "named App only", workerDatabase: true},
		{name: "default App only", defaultDatabase: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace := currentProjectRenderWorkspace(t)
			config := &project.Config{
				GoModuleName: "example.com/database-shell-projection",
				Render: project.RenderConfig{Components: project.Components{
					CLI:            true,
					DatabaseSQLite: test.defaultDatabase,
				}},
				Apps: map[string]project.AppConfig{
					"worker": {Components: project.Components{CLI: true, DatabaseSQLite: test.workerDatabase}},
				},
			}
			apps := []struct {
				app     project.App
				enabled bool
			}{
				{app: project.DefaultApp(), enabled: test.defaultDatabase},
				{app: project.DefaultNamedApp("worker"), enabled: test.workerDatabase},
			}

			for _, target := range apps {
				t.Run(target.app.Name, func(t *testing.T) {
					data := workspace.templateDataForApp(config, target.app)
					data.Components = appRenderComponents(config, target.app)
					data.HelpFormatterFunc = "FrameworkFormatter"
					sources := map[string]string{
						"app/commands.go.tmpl":    renderSharedTemplate(t, "app/commands.go.tmpl", data),
						"app/root_cmd.go.tmpl":    renderSharedTemplate(t, "app/root_cmd.go.tmpl", data),
						"wire/inject_cmd.go.tmpl": renderSharedTemplate(t, "wire/inject_cmd.go.tmpl", data),
					}
					for path, source := range sources {
						assertFormattedGoTemplate(t, path, source)
					}

					assertTemplateMarker(t, "app/root_cmd.go.tmpl", sources["app/root_cmd.go.tmpl"], "DBShellCmd", target.enabled)
					assertTemplateMarker(t, "wire/inject_cmd.go.tmpl", sources["wire/inject_cmd.go.tmpl"], "cmd.NewDBShellCmd", target.enabled)
					if strings.Contains(sources["app/commands.go.tmpl"], "DBShellCmd") {
						t.Fatalf("App-owned commands retained DB shell wiring:\n%s", sources["app/commands.go.tmpl"])
					}
				})
			}
		})
	}
}

// TestDatabaseShellExistingAppTransitions verifies refreshed command wiring follows Database changes without rewriting the App owner.
func TestDatabaseShellExistingAppTransitions(t *testing.T) {
	t.Run("enable Database", testEnableDatabaseShellForExistingApp)
	t.Run("disable Database and migrate legacy owner", testDisableDatabaseShellForExistingApp)
}

// testEnableDatabaseShellForExistingApp verifies additive Database rendering changes only refreshed command surfaces.
func testEnableDatabaseShellForExistingApp(t *testing.T) {
	usePrimitiveRendererRoot(t)
	app := project.DefaultNamedApp("worker")
	components := project.Components{CLI: true}
	config := databaseShellLifecycleConfig(app, components)
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatalf("write Database enablement config: %v", err)
	}
	writePrimitiveRendererFile(t, ".env", "OWNER_SENTINEL=keep\nDB_DRIVER=sqlite\nDB_SUPPORTED_DRIVERS=sqlite\nDB_DATABASE=app.db\n")

	initialRenderer := unitProjectRenderer(t)
	initialRenderer.config = config
	initialRenderer.resources.plan = defaultResourcePlanForTest(t, project.ProjectComponents(config))
	if err := initialRenderer.renderApp(app); err != nil {
		t.Fatalf("render Database-disabled App fixture: %v", err)
	}
	commandsPath := filepath.Join(app.AppDir, "commands.go")
	commandsBefore := readPrimitiveRendererFile(t, commandsPath) + "\n// OwnerSentinel proves Database enablement preserves this file.\n"
	writePrimitiveRendererFile(t, commandsPath, commandsBefore)

	enabled := project.Components{CLI: true, DatabaseSQLite: true}
	renderer := unitProjectRenderer(t)
	renderer.resources.plan = defaultResourcePlanForTest(t, enabled)
	if err := renderer.RenderAppOnly(app, makeapp.RenderOptions{Components: enabled, SkipWire: true}); err != nil {
		t.Fatalf("enable Database for existing App: %v", err)
	}
	if got := readPrimitiveRendererFile(t, commandsPath); got != commandsBefore {
		t.Fatal("Database enablement rewrote the App-owned commands file")
	}
	assertDatabaseShellRenderedSurface(t, app, true)

	loaded, err := project.LoadProjectConfig()
	if err != nil {
		t.Fatalf("reload Database-enabled config: %v", err)
	}
	if !loaded.Apps[app.Name].Components.HasDatabase() {
		t.Fatal("Database enablement was not persisted for the existing App")
	}
}

// testDisableDatabaseShellForExistingApp verifies removal refreshes command gating after safely migrating historical generated ownership.
func testDisableDatabaseShellForExistingApp(t *testing.T) {
	usePrimitiveRendererRoot(t)
	app := project.DefaultNamedApp("worker")
	enabled := project.Components{CLI: true, DatabaseSQLite: true}
	config := databaseShellLifecycleConfig(app, enabled)
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatalf("write Database disablement config: %v", err)
	}
	writePrimitiveRendererFile(t, ".env", "OWNER_SENTINEL=keep\nDB_DRIVER=sqlite\nDB_SUPPORTED_DRIVERS=sqlite\nDB_DATABASE=app.db\nWORKER_DB_DRIVER=sqlite\n")

	initialRenderer := unitProjectRenderer(t)
	initialRenderer.config = config
	initialRenderer.resources.plan = defaultResourcePlanForTest(t, project.ProjectComponents(config))
	if err := initialRenderer.renderApp(app); err != nil {
		t.Fatalf("render Database-enabled App fixture: %v", err)
	}
	assertDatabaseShellRenderedSurface(t, app, true)

	commandsPath := filepath.Join(app.AppDir, "commands.go")
	legacyOwner := legacyDBShellCommandOwnerSource(project.AppPackageName(app.Name))
	writePrimitiveRendererFile(t, commandsPath, legacyOwner)
	wantOwner, changed, err := removeLegacyDBShellCommandSource(commandsPath, []byte(legacyOwner))
	if err != nil || !changed {
		t.Fatalf("prepare expected legacy DB shell owner migration: changed=%t err=%v", changed, err)
	}

	disabled := project.Components{CLI: true}
	renderer := unitProjectRenderer(t)
	renderer.resources.plan = defaultResourcePlanForTest(t, project.ProjectComponents(config))
	if err := renderer.RenderAppOnly(app, makeapp.RenderOptions{Components: disabled, SkipWire: true}); err != nil {
		t.Fatalf("disable Database for existing App: %v", err)
	}
	if got := readPrimitiveRendererFile(t, commandsPath); got != string(wantOwner) {
		t.Fatalf("Database disablement did not preserve the migrated App owner:\n%s", got)
	}
	assertDatabaseShellRenderedSurface(t, app, false)

	loaded, err := project.LoadProjectConfig()
	if err != nil {
		t.Fatalf("reload Database-disabled config: %v", err)
	}
	if loaded.Apps[app.Name].Components.HasDatabase() {
		t.Fatal("Database disablement was not persisted for the existing App")
	}
}

// databaseShellLifecycleConfig creates an existing named-App fixture with an independently gated Database selection.
func databaseShellLifecycleConfig(app project.App, components project.Components) *project.Config {
	return &project.Config{
		ProjectName:  "Database Shell Lifecycle",
		GoModuleName: "example.test/database-shell-lifecycle",
		Render: project.RenderConfig{
			Components: project.Components{CLI: true, DatabaseSQLite: true},
		},
		Apps: map[string]project.AppConfig{
			app.Name: {Components: components},
		},
	}
}

// assertDatabaseShellRenderedSurface verifies DB shell registration on both refreshed command boundaries.
func assertDatabaseShellRenderedSurface(t *testing.T, app project.App, enabled bool) {
	t.Helper()
	rootPath := filepath.Join(app.AppDir, "root_cmd.go")
	injectPath := filepath.Join(app.WireDir, "inject_cmd.go")
	assertTemplateMarker(t, rootPath, readPrimitiveRendererFile(t, rootPath), "DBShellCmd", enabled)
	assertTemplateMarker(t, injectPath, readPrimitiveRendererFile(t, injectPath), "cmd.NewDBShellCmd", enabled)
}

func TestRemoveLegacyDBShellCommandSourceMigratesOnlyGeneratedShape(t *testing.T) {
	generated := legacyDBShellCommandOwnerSource("app")
	formattedGenerated, err := format.Source([]byte(generated))
	if err != nil {
		t.Fatalf("format generated DB shell command owner: %v", err)
	}
	tests := []struct {
		name        string
		source      string
		wantChanged bool
		wantError   bool
	}{
		{name: "generated owner", source: generated, wantChanged: true},
		{name: "gofmt-aligned generated owner", source: string(formattedGenerated), wantChanged: true},
		{name: "already neutral", source: "package app\n\ntype Commands struct{}\n"},
		{name: "customized field", source: "package app\n\ntype Commands struct {\n\tLegacyDB cmd.DBShellCmd `cmd:\"\"`\n}\n", wantError: true},
		{name: "owner comment", source: strings.Replace(generated, "DBShellCmd cmd.DBShellCmd", "DBShellCmd cmd.DBShellCmd // keep this owner note", 1), wantError: true},
		{name: "incomplete constructor", source: strings.Replace(generated, "\tdbShellCmd *cmd.DBShellCmd,\n", "", 1), wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			updated, changed, err := removeLegacyDBShellCommandSource("commands.go", []byte(test.source))
			if got := err != nil; got != test.wantError {
				t.Fatalf("migration error presence = %t, want %t: %v", got, test.wantError, err)
			}
			if test.wantError {
				if !strings.Contains(err.Error(), "customized or is incomplete") || !strings.Contains(err.Error(), "commands.go") {
					t.Fatalf("migration error lacks actionable owner context: %v", err)
				}
				return
			}
			if changed != test.wantChanged {
				t.Fatalf("migration changed = %t, want %t", changed, test.wantChanged)
			}
			if test.wantChanged && (strings.Contains(string(updated), "DBShellCmd") || !strings.Contains(string(updated), "OwnerSentinel")) {
				t.Fatalf("migration changed more than the generated DB shell lines:\n%s", updated)
			}
		})
	}
}

// TestMigrateLegacyDBShellCommandOwnersPreflightsEveryApp prevents an ambiguous owner from leaving earlier Apps partially migrated.
func TestMigrateLegacyDBShellCommandOwnersPreflightsEveryApp(t *testing.T) {
	t.Chdir(t.TempDir())
	apps := []project.App{project.DefaultApp(), project.DefaultNamedApp("worker")}
	sources := map[string]string{}
	for _, app := range apps {
		path := filepath.Join(app.AppDir, "commands.go")
		source := legacyDBShellCommandOwnerSource(project.AppPackageName(app.Name))
		if app.Name == "worker" {
			source = strings.Replace(source, "DBShellCmd cmd.DBShellCmd", "DBShellCmd cmd.DBShellCmd // owner customization", 1)
		}
		sources[path] = source
		writePrimitiveRendererFile(t, path, source)
	}

	err := currentProjectRenderWorkspace(t).migrateLegacyDBShellCommandOwners(apps)
	workerPath := filepath.Join(apps[1].AppDir, "commands.go")
	if err == nil || !strings.Contains(err.Error(), workerPath) || !strings.Contains(err.Error(), "customized or is incomplete") {
		t.Fatalf("DB shell owner migration error = %v, want customized worker owner", err)
	}
	for path, source := range sources {
		if got := readPrimitiveRendererFile(t, path); got != source {
			t.Fatalf("failed preflight changed %s", path)
		}
	}
}

// legacyDBShellCommandOwnerSource returns the exact historical generated wiring plus an owner sentinel.
func legacyDBShellCommandOwnerSource(packageName string) string {
	return `package ` + packageName + `

import "example.test/project/internal/cmd"

// OwnerSentinel proves the migration preserves unrelated App code.
var OwnerSentinel = true

type Commands struct {
	AboutCmd cmd.AboutCmd ` + "`cmd:\"\"`" + `
	DBShellCmd cmd.DBShellCmd ` + "`cmd:\"\"`" + `
}

func NewCommands(
	aboutCmd *cmd.AboutCmd,
	dbShellCmd *cmd.DBShellCmd,
) *Commands {
	return &Commands{
		AboutCmd: *aboutCmd,
		DBShellCmd: *dbShellCmd,
	}
}
`
}
