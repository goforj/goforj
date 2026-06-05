package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/coredeps"
	"github.com/goforj/goforj/project"
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

	renderer := &ProjectRenderer{
		config: &project.Config{
			App: project.AppConfig{
				Targets: []project.AppTarget{{Name: "wire"}},
			},
		},
	}
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

	assertFileContains(t, filepath.Join("cmd", "customer-portal", "main.go"),
		`targetapp "example.com/test/app/customer-portal"`,
		`"example.com/test/app/customer-portal/wire"`,
		`&targetapp.RootCmd{}`,
	)
	assertFileContains(t, filepath.Join("app", "customer-portal", "root_cmd.go"), "package customerportal")
	assertFileContains(t, filepath.Join("app", "customer-portal", "routes.go"), "package customerportal")
	assertFileContains(t, filepath.Join("app", "customer-portal", "wire", "inject_http.go"),
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

func assertFileContains(t *testing.T, path string, snippets ...string) {
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
