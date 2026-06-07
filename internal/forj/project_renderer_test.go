package forj

import (
	"os"
	"path/filepath"
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
		"You've not deployed anything for Test / billing yet.",
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
		"You've not deployed anything for Test / billing yet.",
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
