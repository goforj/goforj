package forj

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/coredeps"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

func TestSyncCoreLibrariesUsesCurrentQueueVersion(t *testing.T) {
	modules := coredeps.SyncCoreLibraries()
	expected := []string{
		"github.com/goforj/queue@" + coredeps.MustVersionFor("github.com/goforj/queue"),
		"github.com/goforj/queue/driver/redisqueue@" + coredeps.MustVersionFor("github.com/goforj/queue/driver/redisqueue"),
		"github.com/goforj/events/eventscore@" + coredeps.MustVersionFor("github.com/goforj/events/eventscore"),
		"github.com/goforj/web@" + coredeps.MustVersionFor("github.com/goforj/web"),
	}
	seen := make(map[string]struct{}, len(modules))
	for _, module := range modules {
		seen[module] = struct{}{}
	}
	for _, module := range expected {
		if _, ok := seen[module]; !ok {
			t.Fatalf("expected syncCoreLibraries to include %s", module)
		}
	}
}

func TestProjectRendererSyncsLighthouseLocalAuthRoute(t *testing.T) {
	data, err := os.ReadFile("project_renderer.go")
	if err != nil {
		t.Fatalf("read project_renderer.go: %v", err)
	}
	source := string(data)
	if !strings.Contains(source, `requires: []string{`) || !strings.Contains(source, `"/auth/dev-session"`) {
		t.Fatal("expected project renderer sync to require the lighthouse dev session auth route")
	}
}

func TestNamedAppRenderTargetsUseConventionsWithoutConfig(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(originalWD) }()

	for _, path := range []string{
		filepath.Join("cmd", "reporting", "main.go"),
		filepath.Join("app", "customer-portal", "wire", "wire.go"),
		filepath.Join("app", "wire", "wire.go"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	renderer := &ProjectRenderer{config: &project.Config{}}
	targets, err := renderer.namedAppRenderTargets()
	if err != nil {
		t.Fatalf("namedAppRenderTargets returned error: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected two named targets, got %#v", targets)
	}
	if targets[0].Name != "customer-portal" || targets[1].Name != "reporting" {
		t.Fatalf("expected sorted conventional targets, got %#v", targets)
	}
}

func TestRuntimeTargetMetadataUsesCompiledTargetOrder(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(originalWD) }()

	writeProjectRendererTestFile(t, filepath.Join("cmd", "billing", "main.go"), "package main\n")
	writeProjectRendererTestFile(t, filepath.Join("cmd", "customer-portal", "main.go"), "package main\n")

	got := runtimeTargetMetadataForRender()
	want := []runtimeTargetMetadata{
		{Name: "app", Index: 0, EnvPrefix: "", HTTPPort: 3000, RuntimeBase: 10000},
		{Name: "billing", Index: 1, EnvPrefix: "BILLING", HTTPPort: 3001, RuntimeBase: 10010},
		{Name: "customer-portal", Index: 2, EnvPrefix: "CUSTOMER_PORTAL", HTTPPort: 3002, RuntimeBase: 10020},
	}
	if len(got) != len(want) {
		t.Fatalf("metadata length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("metadata[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestMigrateGeneratedEnvDefaultOnlyUpdatesOldDefault(t *testing.T) {
	lines := []string{
		"APP_NAME=test",
		"GRAFANA_PORT=3001",
		"GRAFANA_ADMIN_USER=admin",
	}

	got, changed := migrateGeneratedEnvDefault(lines, "GRAFANA_PORT", "3001", "13001")
	if !changed {
		t.Fatal("expected old generated default to be migrated")
	}
	if got[1] != "GRAFANA_PORT=13001" {
		t.Fatalf("migrated line = %q", got[1])
	}
	if lines[1] != "GRAFANA_PORT=3001" {
		t.Fatalf("migrateGeneratedEnvDefault mutated input slice: %#v", lines)
	}

	custom, changed := migrateGeneratedEnvDefault([]string{"GRAFANA_PORT=3100"}, "GRAFANA_PORT", "3001", "13001")
	if changed {
		t.Fatalf("custom value should not be migrated: %#v", custom)
	}

	commented, changed := migrateGeneratedEnvDefault([]string{"# GRAFANA_PORT=3001"}, "GRAFANA_PORT", "3001", "13001")
	if changed {
		t.Fatalf("commented value should not be migrated: %#v", commented)
	}
}

func TestExpandDefaultMigrationsForNamedTargets(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(originalWD) }()

	writeProjectRendererTestFile(t, filepath.Join("migrations", "2026_01_01_000001_create_users.up.sql"), "-- up\n")
	writeProjectRendererTestFile(t, filepath.Join("migrations", "2026_01_01_000001_create_users.down.sql"), "-- down\n")
	writeProjectRendererTestFile(t, filepath.Join("migrations", "reporting", "2026_01_02_000001_create_reports.up.sql"), "-- up\n")
	writeProjectRendererTestFile(t, filepath.Join("migrations", "reporting", "2026_01_02_000001_create_reports.down.sql"), "-- down\n")
	writeProjectRendererTestFile(t, filepath.Join("migrations", "migrations.go"), "package migrations\n")

	renderer := &ProjectRenderer{}
	if err := renderer.expandDefaultMigrationsForNamedTargets(); err != nil {
		t.Fatalf("expand migrations: %v", err)
	}

	assertProjectRendererTestFile(t, filepath.Join("migrations", "app", "default", "2026_01_01_000001_create_users.up.sql"))
	assertProjectRendererTestFile(t, filepath.Join("migrations", "app", "default", "2026_01_01_000001_create_users.down.sql"))
	assertProjectRendererTestFile(t, filepath.Join("migrations", "app", "reporting", "2026_01_02_000001_create_reports.up.sql"))
	assertProjectRendererTestFile(t, filepath.Join("migrations", "app", "reporting", "2026_01_02_000001_create_reports.down.sql"))
	assertProjectRendererTestFile(t, filepath.Join("migrations", "migrations.go"))
	assertProjectRendererTestFileMissing(t, filepath.Join("migrations", "2026_01_01_000001_create_users.up.sql"))
	assertProjectRendererTestFileMissing(t, filepath.Join("migrations", "reporting"))
}

func TestRenderExpandsDefaultMigrationsWhenNamedTargetExists(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(originalWD) }()

	cfg := &project.Config{
		ProjectName:  "MigrationFanout",
		GoModuleName: "example.com/migrationfanout",
		Render: project.RenderConfig{
			Components: project.Components{
				CLI:            true,
				DatabaseSQLite: true,
			},
		},
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(".goforj.yml", data, 0o644); err != nil {
		t.Fatalf("write .goforj.yml: %v", err)
	}

	writeProjectRendererTestFile(t, filepath.Join("migrations", "2026_01_01_000001_create_users.up.sql"), "-- up\n")
	writeProjectRendererTestFile(t, filepath.Join("migrations", "2026_01_01_000001_create_users.down.sql"), "-- down\n")
	writeProjectRendererTestFile(t, filepath.Join("migrations", "analytics", "2026_01_02_000001_create_reports.up.sql"), "-- up\n")
	writeProjectRendererTestFile(t, filepath.Join("migrations", "analytics", "2026_01_02_000001_create_reports.down.sql"), "-- down\n")
	writeProjectRendererTestFile(t, filepath.Join("cmd", "billing", "main.go"), "package main\n")

	renderer := NewProjectRenderer(logger.NewSilentLogger())
	if err := renderer.Render(ComponentRenderInput{renderAll: true}); err != nil {
		t.Fatalf("render: %v", err)
	}

	assertProjectRendererTestFile(t, filepath.Join("migrations", "app", "default", "2026_01_01_000001_create_users.up.sql"))
	assertProjectRendererTestFile(t, filepath.Join("migrations", "app", "default", "2026_01_01_000001_create_users.down.sql"))
	assertProjectRendererTestFile(t, filepath.Join("migrations", "app", "analytics", "2026_01_02_000001_create_reports.up.sql"))
	assertProjectRendererTestFile(t, filepath.Join("migrations", "app", "analytics", "2026_01_02_000001_create_reports.down.sql"))
	assertProjectRendererTestFileMissing(t, filepath.Join("migrations", "2026_01_01_000001_create_users.up.sql"))
	assertProjectRendererTestFileMissing(t, filepath.Join("migrations", "analytics"))
}

func TestRenderAppTargetWritesNamedTargetPackagesAndImports(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(originalWD) }()

	renderer := &ProjectRenderer{
		config: &project.Config{
			GoModuleName: "example.com/test",
			Render: project.RenderConfig{
				Components: project.Components{WebAPI: true},
			},
		},
		stats: &renderStats{},
	}

	target := project.DefaultNamedAppTarget("customer-portal")
	if err := renderer.renderAppTarget(target); err != nil {
		t.Fatalf("renderAppTarget returned error: %v", err)
	}

	assertProjectRendererFileContains(t, filepath.Join("cmd", "customer-portal", "main.go"),
		`targetapp "example.com/test/app/customer-portal"`,
		`"example.com/test/app/customer-portal/wire"`,
		`&targetapp.RootCmd{}`,
	)
	assertProjectRendererFileContains(t, filepath.Join("app", "customer-portal", "root_cmd.go"), "package customerportal")
	assertProjectRendererFileContains(t, filepath.Join("app", "customer-portal", "routes.go"), "package customerportal")
	assertProjectRendererFileContains(t, filepath.Join("app", "customer-portal", "wire", "inject_http.go"),
		`targetapp "example.com/test/app/customer-portal"`,
		"targetapp.ProvideRoutes",
	)

	commandsPath := filepath.Join("app", "customer-portal", "commands.go")
	customCommands := "package customerportal\n\n// custom\n"
	if err := os.WriteFile(commandsPath, []byte(customCommands), 0o644); err != nil {
		t.Fatalf("write custom commands: %v", err)
	}
	if err := renderer.renderAppTarget(target); err != nil {
		t.Fatalf("rerender target returned error: %v", err)
	}
	content, err := os.ReadFile(commandsPath)
	if err != nil {
		t.Fatalf("read commands after rerender: %v", err)
	}
	if string(content) != customCommands {
		t.Fatalf("expected app-owned commands to be preserved, got:\n%s", content)
	}
}

func TestRemoveLegacyInitialBuildTask(t *testing.T) {
	tasks := []project.DevTask{
		{Name: "Initial build", Cmd: "forj build -o ./bin/app"},
		{Name: "Run Docker Compose", Cmd: "docker-compose up -d"},
		{Name: "Initial build", Cmd: "make build"},
	}

	if !removeLegacyInitialBuildTask(&tasks) {
		t.Fatal("expected legacy initial build task to be removed")
	}
	want := []project.DevTask{
		{Name: "Run Docker Compose", Cmd: "docker-compose up -d"},
		{Name: "Initial build", Cmd: "make build"},
	}
	if !reflect.DeepEqual(tasks, want) {
		t.Fatalf("tasks = %#v, want %#v", tasks, want)
	}
}

func TestRenderAppTargetUsesPersistedTargetComponents(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(originalWD) }()

	renderer := &ProjectRenderer{
		config: &project.Config{
			ProjectName:  "Test",
			GoModuleName: "example.com/test",
			Render: project.RenderConfig{
				Components: project.Components{WebAPI: true, WebUI: true},
				StarterKit: project.StarterKitVue,
			},
			AppTargets: map[string]project.AppTargetConfig{
				"billing": {
					Components: project.Components{CLI: true, WebAPI: true},
					StarterKit: project.StarterKitNone,
				},
			},
		},
		stats: &renderStats{},
	}

	target := project.DefaultNamedAppTarget("billing")
	if err := renderer.renderAppTarget(target); err != nil {
		t.Fatalf("renderAppTarget returned error: %v", err)
	}

	mainSrc := readMakeAppTestFile(t, filepath.Join("cmd", "billing", "main.go"))
	if strings.Contains(mainSrc, `"embed"`) || strings.Contains(mainSrc, "RegisterSpa") {
		t.Fatalf("expected target components to omit frontend embedding:\n%s", mainSrc)
	}
	if _, err := os.Stat(filepath.Join("cmd", "billing", "frontend", "dist", "index.html")); !os.IsNotExist(err) {
		t.Fatalf("expected target components to omit frontend placeholder, stat err = %v", err)
	}
}

func TestRenderAppTargetWritesTargetAwareFrontendPlaceholder(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(originalWD) }()

	renderer := &ProjectRenderer{
		config: &project.Config{
			ProjectName:  "Test",
			GoModuleName: "example.com/test",
			Render: project.RenderConfig{
				Components: project.Components{WebUI: true},
			},
		},
		stats: &renderStats{},
	}

	target := project.DefaultNamedAppTarget("billing")
	if err := renderer.renderAppTarget(target); err != nil {
		t.Fatalf("renderAppTarget returned error: %v", err)
	}

	assertProjectRendererFileContains(t, filepath.Join("cmd", "billing", "frontend", "dist", "index.html"),
		"<title>Test / billing</title>",
		"<span>GoForj</span>",
		"<h1>Test / billing</h1>",
		"This app target is running, but no frontend build has been deployed yet.",
	)
}

func TestRenderAppTargetMigratesOldFrontendPlaceholder(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(originalWD) }()

	renderer := &ProjectRenderer{
		config: &project.Config{
			ProjectName:  "Test",
			GoModuleName: "example.com/test",
			Render: project.RenderConfig{
				Components: project.Components{WebUI: true},
			},
		},
		stats: &renderStats{},
	}

	target := project.DefaultNamedAppTarget("billing")
	indexPath := filepath.Join("cmd", "billing", "frontend", "dist", "index.html")
	writeProjectRendererTestFile(t, indexPath, oldFrontendDistPlaceholder("Test")+"\n")

	if err := renderer.renderAppTarget(target); err != nil {
		t.Fatalf("renderAppTarget returned error: %v", err)
	}

	assertProjectRendererFileContains(t, indexPath,
		"<title>Test / billing</title>",
		"<span>GoForj</span>",
		"<h1>Test / billing</h1>",
		"This app target is running, but no frontend build has been deployed yet.",
	)
}

func TestRenderAppTargetPreservesCustomFrontendPlaceholder(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(originalWD) }()

	renderer := &ProjectRenderer{
		config: &project.Config{
			ProjectName:  "Test",
			GoModuleName: "example.com/test",
			Render: project.RenderConfig{
				Components: project.Components{WebUI: true},
			},
		},
		stats: &renderStats{},
	}

	target := project.DefaultNamedAppTarget("billing")
	indexPath := filepath.Join("cmd", "billing", "frontend", "dist", "index.html")
	custom := "<!doctype html><html><body>custom</body></html>\n"
	writeProjectRendererTestFile(t, indexPath, custom)

	if err := renderer.renderAppTarget(target); err != nil {
		t.Fatalf("renderAppTarget returned error: %v", err)
	}

	content, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read %s: %v", indexPath, err)
	}
	if string(content) != custom {
		t.Fatalf("expected custom frontend placeholder to be preserved, got:\n%s", content)
	}
}

func TestRenderAppTargetWritesDefaultTargetShape(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(originalWD) }()

	renderer := &ProjectRenderer{
		config: &project.Config{
			GoModuleName: "example.com/test",
			Render: project.RenderConfig{
				Components: project.Components{
					WebAPI:         true,
					DatabaseSQLite: true,
					Scheduler:      true,
					Jobs:           true,
				},
			},
		},
		stats: &renderStats{},
	}
	if err := renderer.renderAppTarget(project.DefaultAppTarget()); err != nil {
		t.Fatalf("renderAppTarget returned error: %v", err)
	}

	for _, path := range []string{
		filepath.Join("cmd", "app", "main.go"),
		filepath.Join("app", "commands.go"),
		filepath.Join("app", "lifecycle.go"),
		filepath.Join("app", "root_cmd.go"),
		filepath.Join("app", "routes.go"),
		filepath.Join("app", "schedules.go"),
		filepath.Join("app", "wire", "wire.go"),
		filepath.Join("app", "wire", "inject_cmd.go"),
		filepath.Join("app", "wire", "inject_cmd_app.go"),
		filepath.Join("app", "wire", "inject_db.go"),
		filepath.Join("app", "wire", "inject_http.go"),
		filepath.Join("app", "wire", "inject_jobs.go"),
		filepath.Join("app", "wire", "inject_jobs_app.go"),
		filepath.Join("app", "wire", "inject_repositories_app.go"),
		filepath.Join("app", "wire", "inject_schedules_app.go"),
		filepath.Join("app", "wire", "inject_scheduler.go"),
	} {
		assertProjectRendererTestFile(t, path)
	}
}

func writeProjectRendererTestFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertProjectRendererTestFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s: %v", path, err)
	}
}

func assertProjectRendererTestFileMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected %s to be missing", path)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat %s: %v", path, err)
	}
}

func assertProjectRendererFileContains(t *testing.T, path string, snippets ...string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	source := string(content)
	for _, snippet := range snippets {
		if !strings.Contains(source, snippet) {
			t.Fatalf("expected %s to contain %q:\n%s", path, snippet, source)
		}
	}
}

func TestSyncLegacyAppServiceInjectorMovesLifecycleRegistryToTargetApp(t *testing.T) {
	legacy := `package wire

import (
	"github.com/goforj/wire"

	"example.com/testapp/internal/app"
	"example.com/testapp/internal/makecmd"
)

var appSet = wire.NewSet(
	app.NewLifecycleRegistry,
	app.NewTimeouts,
	makecmd.NewEventCmd,
)
`

	updated := syncLegacyAppServiceInjector(legacy, "example.com/testapp")
	for _, want := range []string{
		`targetapp "example.com/testapp/app"`,
		`"example.com/testapp/internal/runtime"`,
		"targetapp.NewLifecycleRegistry",
		"runtime.NewTimeouts",
	} {
		if !strings.Contains(updated, want) {
			t.Fatalf("expected migrated service injector to contain %q:\n%s", want, updated)
		}
	}
	for _, unexpected := range []string{
		"\t\"example.com/testapp/internal/app\"",
		"\tapp.NewLifecycleRegistry",
		"\tapp.NewTimeouts",
		"targettargetapp",
		"runtimeruntime",
		"runtimeapp",
	} {
		if strings.Contains(updated, unexpected) {
			t.Fatalf("expected migrated service injector not to contain %q:\n%s", unexpected, updated)
		}
	}

	idempotent := syncLegacyAppServiceInjector(updated, "example.com/testapp")
	if idempotent != updated {
		t.Fatalf("expected migration to be idempotent:\n%s", idempotent)
	}
}

func TestSyncLegacyAppLifecycleRegistryRenamesRuntimePackage(t *testing.T) {
	legacy := `package app

import (
	"context"

	runtimeapp "example.com/testapp/internal/app"
)

type LifecycleRegistry struct{}

func (r *LifecycleRegistry) Register(lifecycle *runtimeapp.Lifecycle) {
	lifecycle.On(runtimeapp.BeforeStartup, r.BeforeStartup)
	lifecycle.On(runtimeapp.Startup, r.Startup)
	lifecycle.On(runtimeapp.AfterStartup, r.AfterStartup)
	lifecycle.On(runtimeapp.BeforeShutdown, r.BeforeShutdown)
	lifecycle.On(runtimeapp.Shutdown, r.Shutdown)
	lifecycle.On(runtimeapp.AfterShutdown, r.AfterShutdown)
}

func (r *LifecycleRegistry) BeforeStartup(context.Context) error { return nil }
func (r *LifecycleRegistry) Startup(context.Context) error { return nil }
func (r *LifecycleRegistry) AfterStartup(context.Context) error { return nil }
func (r *LifecycleRegistry) BeforeShutdown(context.Context) error { return nil }
func (r *LifecycleRegistry) Shutdown(context.Context) error { return nil }
func (r *LifecycleRegistry) AfterShutdown(context.Context) error { return nil }
`

	updated := syncLegacyAppLifecycleRegistry(legacy, "example.com/testapp")
	for _, want := range []string{
		`"example.com/testapp/internal/runtime"`,
		"func (r *LifecycleRegistry) Register(lifecycle *runtime.Lifecycle)",
		"lifecycle.On(runtime.BeforeStartup, r.BeforeStartup)",
		"lifecycle.On(runtime.AfterShutdown, r.AfterShutdown)",
	} {
		if !strings.Contains(updated, want) {
			t.Fatalf("expected migrated lifecycle registry to contain %q:\n%s", want, updated)
		}
	}
	for _, unexpected := range []string{
		"internal/app",
		"runtimeapp",
	} {
		if strings.Contains(updated, unexpected) {
			t.Fatalf("expected migrated lifecycle registry not to contain %q:\n%s", unexpected, updated)
		}
	}

	idempotent := syncLegacyAppLifecycleRegistry(updated, "example.com/testapp")
	if idempotent != updated {
		t.Fatalf("expected migration to be idempotent:\n%s", idempotent)
	}
}

func TestSyncLegacyScheduleInjectorAddsTargetRegistryProvider(t *testing.T) {
	legacy := `package wire

import (
	"github.com/goforj/wire"
	"example.com/testapp/internal/reports"
	"example.com/testapp/internal/schedules"
)

var appScheduleSet = wire.NewSet(
	ProvideAppSchedules,
	reports.NewDailySchedule,
)

func ProvideAppSchedules(
	dailySchedule *reports.DailySchedule,
) *schedules.AppSchedules {
	return schedules.NewAppSchedules(
		dailySchedule,
	)
}
`

	updated := syncLegacyScheduleInjector(legacy, "example.com/testapp")
	for _, want := range []string{
		`targetapp "example.com/testapp/app"`,
		`"example.com/testapp/internal/schedules"`,
		"targetapp.NewScheduleRegistry",
		"wire.Bind(new(schedules.ScheduleRegistry), new(*targetapp.ScheduleRegistry))",
		"ProvideAppSchedules",
		"reports.NewDailySchedule",
		"dailySchedule *reports.DailySchedule",
	} {
		if !strings.Contains(updated, want) {
			t.Fatalf("expected migrated schedule injector to contain %q:\n%s", want, updated)
		}
	}

	idempotent := syncLegacyScheduleInjector(updated, "example.com/testapp")
	if idempotent != updated {
		t.Fatalf("expected schedule migration to be idempotent:\n%s", idempotent)
	}
}

func TestSyncLegacyScheduleInjectorReplacesVariadicScheduleProvider(t *testing.T) {
	legacy := `package wire

import (
	"github.com/goforj/wire"

	targetapp "example.com/testapp/app"
	"example.com/testapp/internal/schedules"
)

var appScheduleSet = wire.NewSet(
	schedules.NewAppSchedules,
	targetapp.NewScheduleRegistry,
	wire.Bind(new(schedules.ScheduleRegistry), new(*targetapp.ScheduleRegistry)),
)
`

	updated := syncLegacyScheduleInjector(legacy, "example.com/testapp")
	for _, want := range []string{
		"ProvideAppSchedules,",
		"func ProvideAppSchedules() *schedules.AppSchedules",
		"return schedules.NewAppSchedules()",
	} {
		if !strings.Contains(updated, want) {
			t.Fatalf("expected migrated schedule injector to contain %q:\n%s", want, updated)
		}
	}
	if strings.Contains(updated, "\tschedules.NewAppSchedules,") {
		t.Fatalf("expected direct variadic provider to be removed:\n%s", updated)
	}
}

func TestSyncDemoAppJobInjectorAddsMissingProviders(t *testing.T) {
	legacy := `package wire

import (
	"github.com/goforj/wire"
	"example.com/testapp/internal/jobs"
)

var appJobSet = wire.NewSet(
	jobs.NewExampleHelloJob,
	jobs.NewExampleHelloJobCmd,
)
`

	updated := syncDemoAppJobInjector(legacy, "example.com/testapp", project.Components{
		DemoApp: true,
		Jobs:    true,
	})
	for _, want := range []string{
		`"example.com/testapp/internal/alerts"`,
		`"example.com/testapp/internal/monitoring"`,
		"alerts.NewDispatchJob",
		"monitoring.NewCheckService",
		"monitoring.NewMonitorCheckJob",
		"jobs.NewExampleHelloJob",
	} {
		if !strings.Contains(updated, want) {
			t.Fatalf("expected demo job injector migration to contain %q:\n%s", want, updated)
		}
	}

	idempotent := syncDemoAppJobInjector(updated, "example.com/testapp", project.Components{
		DemoApp: true,
		Jobs:    true,
	})
	if idempotent != updated {
		t.Fatalf("expected demo job injector migration to be idempotent:\n%s", idempotent)
	}
}

func TestSyncDemoAppRepositoryInjectorAddsMissingProviders(t *testing.T) {
	legacy := `package wire

import (
	"github.com/goforj/wire"
)

var repositorySet = wire.NewSet(
	wire.Value(repositorySetPlaceholder{}),
)

type repositorySetPlaceholder struct{}
`

	updated := syncDemoAppRepositoryInjector(legacy, "example.com/testapp", project.Components{
		DemoApp:       true,
		DatabaseMySQL: true,
	})
	for _, want := range []string{
		`"example.com/testapp/internal/appsettings"`,
		`"example.com/testapp/internal/models"`,
		`"example.com/testapp/internal/notification"`,
		"appsettings.NewAppSettingRepo",
		"models.NewMonitorRepo",
		"notification.NewChannelRepo",
	} {
		if !strings.Contains(updated, want) {
			t.Fatalf("expected demo repository injector migration to contain %q:\n%s", want, updated)
		}
	}
	if strings.Contains(updated, "wire.Value(repositorySetPlaceholder{})") {
		t.Fatalf("expected repository placeholder to be removed:\n%s", updated)
	}

	idempotent := syncDemoAppRepositoryInjector(updated, "example.com/testapp", project.Components{
		DemoApp:       true,
		DatabaseMySQL: true,
	})
	if idempotent != updated {
		t.Fatalf("expected demo repository injector migration to be idempotent:\n%s", idempotent)
	}
}

func TestSyncDemoAppServiceInjectorAddsMissingProviders(t *testing.T) {
	legacy := `package wire

import (
	"github.com/goforj/wire"
	targetapp "example.com/testapp/app"
	"example.com/testapp/internal/runtime"
	"example.com/testapp/internal/makecmd"
)

var appSet = wire.NewSet(
	targetapp.NewLifecycleRegistry,
	runtime.NewTimeouts,
	makecmd.NewEventCmd,
	makecmd.NewSubscriberCmd,
)
`

	updated := syncDemoAppServiceInjector(legacy, "example.com/testapp", project.Components{
		DemoApp:       true,
		DatabaseMySQL: true,
	})
	for _, want := range []string{
		`"example.com/testapp/internal/appsettings"`,
		`"example.com/testapp/internal/logger"`,
		`"example.com/testapp/internal/monitoring"`,
		`"example.com/testapp/internal/notification"`,
		"monitoring.NewIncidentTransitionService",
		"monitoring.NewRetentionService",
		"notification.NewManager",
		"preseedDemoDefaults",
		"type demoPreseedReady struct{}",
	} {
		if !strings.Contains(updated, want) {
			t.Fatalf("expected demo service injector migration to contain %q:\n%s", want, updated)
		}
	}

	idempotent := syncDemoAppServiceInjector(updated, "example.com/testapp", project.Components{
		DemoApp:       true,
		DatabaseMySQL: true,
	})
	if idempotent != updated {
		t.Fatalf("expected demo service injector migration to be idempotent:\n%s", idempotent)
	}
}

func TestUpsertEnvDefaultsAddsTargetDatabaseDriver(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".env")
	if err := os.WriteFile(path, []byte("DB_DRIVER=mysql\nDB_SUPPORTED_DRIVERS=mysql\n"), 0o644); err != nil {
		t.Fatalf("write env: %v", err)
	}

	err := upsertEnvDefaults(path, targetDatabaseEnvDefaults("REPORTING", "postgres", "mysql", false))
	if err != nil {
		t.Fatalf("upsert env defaults: %v", err)
	}

	text := readMakeAppTestFile(t, path)
	for _, want := range []string{
		"DB_DRIVER=mysql",
		"DB_SUPPORTED_DRIVERS=mysql,postgres",
		"REPORTING_DB_DRIVER=postgres",
		"REPORTING_DB_DATABASE=reporting",
		"REPORTING_DB_HOST=postgres",
		"REPORTING_DB_PORT=5432",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected env to contain %q:\n%s", want, text)
		}
	}
}

func TestUpsertTargetEnvDefaultsGroupsAndOrdersTargetKeys(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".env")
	initial := strings.Join([]string{
		"APP_NAME=Test",
		"BILLING_DB_DATABASE=old",
		"# Billing",
		"BILLING_APP_URL=http://localhost:2999",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatalf("write env: %v", err)
	}

	defaults := map[string]string{
		"BILLING_DB_DATABASE":        "billing",
		"BILLING_APP_URL":            "http://localhost:3001",
		"BILLING_API_HTTP_PORT":      "3001",
		"BILLING_DB_SQLITE_DATABASE": "./_data/sqlite/billing.db",
	}
	err := upsertTargetEnvDefaults(path, "billing", "BILLING", defaults)
	if err != nil {
		t.Fatalf("upsert target env defaults: %v", err)
	}

	text := readMakeAppTestFile(t, path)
	want := strings.Join([]string{
		"APP_NAME=Test",
		"",
		"# Billing",
		"BILLING_APP_URL=http://localhost:3001",
		"BILLING_API_HTTP_PORT=3001",
		"BILLING_DB_DATABASE=billing",
		"BILLING_DB_SQLITE_DATABASE=./_data/sqlite/billing.db",
		"",
	}, "\n")
	if text != want {
		t.Fatalf("unexpected target env section:\nwant:\n%s\ngot:\n%s", want, text)
	}
	if strings.Count(text, "BILLING_DB_DATABASE=") != 1 {
		t.Fatalf("expected exactly one target database override:\n%s", text)
	}
}

func TestTargetDatabaseHostDefaultsUseLocalhostForHostEnv(t *testing.T) {
	defaults := targetDatabaseEnvDefaults("REPORTING", "postgres", "mysql", true)
	if got := defaults["REPORTING_DB_HOST"]; got != "localhost" {
		t.Fatalf("REPORTING_DB_HOST = %q, want localhost", got)
	}
}

func TestTargetDatabaseEnvDefaultsInheritSharedConnection(t *testing.T) {
	defaults := targetDatabaseEnvDefaults("BILLING", "mysql", "mysql", false)
	for _, want := range []string{
		"DB_SUPPORTED_DRIVERS",
		"BILLING_DB_DATABASE",
		"BILLING_DB_SQLITE_DATABASE",
	} {
		if _, ok := defaults[want]; !ok {
			t.Fatalf("expected %s in target database defaults: %#v", want, defaults)
		}
	}
	for _, unwanted := range []string{
		"BILLING_DB_DRIVER",
		"BILLING_DB_HOST",
		"BILLING_DB_USERNAME",
		"BILLING_DB_PASSWORD",
		"BILLING_DB_PORT",
	} {
		if _, ok := defaults[unwanted]; ok {
			t.Fatalf("did not expect inherited connection key %s in target database defaults: %#v", unwanted, defaults)
		}
	}
	if defaults["BILLING_DB_DATABASE"] != "billing" {
		t.Fatalf("BILLING_DB_DATABASE = %q, want billing", defaults["BILLING_DB_DATABASE"])
	}
	if defaults["BILLING_DB_SQLITE_DATABASE"] != "./_data/sqlite/billing.db" {
		t.Fatalf("BILLING_DB_SQLITE_DATABASE = %q, want sqlite fallback", defaults["BILLING_DB_SQLITE_DATABASE"])
	}
}

func TestTargetDatabaseEnvDefaultsDoNotDuplicateActiveSQLiteDatabase(t *testing.T) {
	defaults := targetDatabaseEnvDefaults("BILLING", "sqlite", "sqlite", false)
	if defaults["BILLING_DB_DATABASE"] != "./_data/sqlite/billing.db" {
		t.Fatalf("BILLING_DB_DATABASE = %q, want sqlite path", defaults["BILLING_DB_DATABASE"])
	}
	if _, ok := defaults["BILLING_DB_SQLITE_DATABASE"]; ok {
		t.Fatalf("did not expect duplicate sqlite fallback for active sqlite driver: %#v", defaults)
	}
}

func TestWriteTargetEnvDefaultsKeepsSupportedDriversInBaseEnv(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := os.WriteFile(".env", []byte("DB_SUPPORTED_DRIVERS=mysql\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if err := os.WriteFile(".env.host", []byte("DB_HOST=localhost\n"), 0o644); err != nil {
		t.Fatalf("write .env.host: %v", err)
	}

	renderer := NewProjectRenderer(logger.NewSilentLogger())
	renderer.config = &project.Config{
		Render: project.RenderConfig{
			Components: project.Components{DatabaseMySQL: true},
		},
	}
	err = renderer.writeTargetEnvDefaults(project.DefaultNamedAppTarget("reporting"), project.Components{WebAPI: true, Metrics: true, DatabasePostgres: true})
	if err != nil {
		t.Fatalf("write target env defaults: %v", err)
	}

	envText := readMakeAppTestFile(t, ".env")
	for _, want := range []string{
		"DB_SUPPORTED_DRIVERS=mysql,postgres",
		"# Reporting",
		"REPORTING_APP_URL=http://localhost:3001",
		"REPORTING_API_HTTP_PORT=3001",
		"REPORTING_DB_DRIVER=postgres",
		"REPORTING_DB_DATABASE=reporting",
		"REPORTING_DB_SQLITE_DATABASE=./_data/sqlite/reporting.db",
	} {
		if !strings.Contains(envText, want) {
			t.Fatalf(".env did not include %q:\n%s", want, envText)
		}
	}
	for _, unwanted := range []string{
		"REPORTING_METRICS_API_PORT",
		"REPORTING_METRICS_SCHEDULER_PORT",
		"REPORTING_METRICS_JOBS_PORT",
	} {
		if strings.Contains(envText, unwanted) {
			t.Fatalf(".env should not include generated metrics override %q:\n%s", unwanted, envText)
		}
	}
	hostText := readMakeAppTestFile(t, ".env.host")
	if strings.Contains(hostText, "DB_SUPPORTED_DRIVERS") {
		t.Fatalf(".env.host should not override supported drivers:\n%s", hostText)
	}
	if !strings.Contains(hostText, "# Reporting") {
		t.Fatalf(".env.host missing target section heading:\n%s", hostText)
	}
	if !strings.Contains(hostText, "REPORTING_DB_HOST=localhost") {
		t.Fatalf(".env.host missing target localhost override:\n%s", hostText)
	}
}
