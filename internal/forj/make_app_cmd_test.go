package forj

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/forj/makeapp"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
)

func TestMakeAppCmdCreatesNamedApp(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := writeProjectConfig(".goforj.yml", &project.Config{
		ProjectName:  "TestApp",
		GoModuleName: "example.com/testapp",
		Render: project.RenderConfig{
			Components: project.Components{
				WebAPI: true,
			},
		},
	}); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := makeapp.NewCmd(logger.NewSilentLogger(), NewProjectRenderer(logger.NewSilentLogger()))
	cmd.Name = "billing"
	cmd.SkipWire = true
	if err := cmd.Run(); err != nil {
		t.Fatalf("make app: %v", err)
	}

	for _, path := range []string{
		filepath.Join("cmd", "billing", "main.go"),
		filepath.Join("app", "billing", "root_cmd.go"),
		filepath.Join("app", "billing", "routes.go"),
		filepath.Join("app", "billing", "wire", "wire.go"),
		filepath.Join("app", "billing", "wire", "inject_cmd.go"),
		filepath.Join("app", "billing", "wire", "inject_http_controllers_app.go"),
		filepath.Join("internal", "runtime", "apps.go"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}

	mainSrc := readMakeAppTestFile(t, filepath.Join("cmd", "billing", "main.go"))
	if !strings.Contains(mainSrc, `cmd.ApplyLaunchApp("billing")`) {
		t.Fatalf("expected billing app identity in cmd/billing/main.go")
	}
	runtimeSrc := readMakeAppTestFile(t, filepath.Join("internal", "runtime", "apps.go"))
	if !strings.Contains(runtimeSrc, `Name: "billing"`) || !strings.Contains(runtimeSrc, `HTTPPort: 3001`) {
		t.Fatalf("expected billing runtime app metadata, got:\n%s", runtimeSrc)
	}
	cfg, err := project.LoadProjectConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	appConfig, ok := cfg.Apps["billing"]
	if !ok {
		t.Fatalf("expected billing app config")
	}
	if !appConfig.Components.WebAPI {
		t.Fatalf("expected billing app to persist web api component")
	}
}

func TestMakeAppCmdUsesNextAvailableEnvPortForSequentialApps(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := writeProjectConfig(".goforj.yml", &project.Config{
		ProjectName:  "TestApp",
		GoModuleName: "example.com/testapp",
		Render: project.RenderConfig{
			Components: project.Components{
				WebAPI: true,
			},
		},
	}); err != nil {
		t.Fatalf("write config: %v", err)
	}

	for _, name := range []string{"workshop", "backstage"} {
		cmd := makeapp.NewCmd(logger.NewSilentLogger(), NewProjectRenderer(logger.NewSilentLogger()))
		cmd.Name = name
		cmd.SkipWire = true
		if err := cmd.Run(); err != nil {
			t.Fatalf("make app %s: %v", name, err)
		}
	}

	envSrc := readMakeAppTestFile(t, ".env")
	for _, want := range []string{
		"WORKSHOP_APP_URL=http://localhost:3001",
		"WORKSHOP_API_HTTP_PORT=3001",
		"BACKSTAGE_APP_URL=http://localhost:3002",
		"BACKSTAGE_API_HTTP_PORT=3002",
	} {
		if !strings.Contains(envSrc, want) {
			t.Fatalf("expected env entry %q, got:\n%s", want, envSrc)
		}
	}
	if strings.Count(envSrc, "API_HTTP_PORT=3001") != 1 {
		t.Fatalf("expected only one app to use HTTP port 3001, got:\n%s", envSrc)
	}
}

// TestMakeAppCmdOmitsCLIOnlyAppFromDevByDefault verifies omission remains the default for tooling Apps.
func TestMakeAppCmdOmitsCLIOnlyAppFromDevByDefault(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := writeProjectConfig(".goforj.yml", &project.Config{
		ProjectName:  "TestApp",
		GoModuleName: "example.com/testapp",
		Render: project.RenderConfig{
			Components: project.Components{CLI: true},
		},
	}); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := makeapp.NewCmd(logger.NewSilentLogger(), NewProjectRenderer(logger.NewSilentLogger()))
	cmd.Name = "ship"
	cmd.Components = "cli"
	cmd.SkipWire = true
	if err := cmd.Run(); err != nil {
		t.Fatalf("make app: %v", err)
	}

	cfg, err := project.LoadProjectConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if _, ok := cfg.Dev.Apps["ship"]; ok {
		t.Fatalf("expected CLI-only ship to be absent from native dev apps, got %#v", cfg.Dev.Apps)
	}
	if _, ok := cfg.Apps["ship"]; !ok {
		t.Fatalf("expected ship render app config")
	}
}

// TestMakeAppCmdMigratesLegacyLifecycleBeforeOmittingCLIApp prevents filesystem discovery from silently enrolling the new App.
func TestMakeAppCmdMigratesLegacyLifecycleBeforeOmittingCLIApp(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	config := &project.Config{
		ProjectName:  "TestApp",
		GoModuleName: "example.com/testapp",
		Render:       project.RenderConfig{Components: project.Components{CLI: true, WebAPI: true}},
		Dev: project.DevConfig{
			Run: map[string]string{project.DefaultAppName: "run"},
			Watches: []project.DevWatch{
				{
					Name:  "Build App",
					Watch: "-file .go -file .env -file .env.* -xdir forj -xdir _data -xfile app/wire/wire_gen\\.go$ -postpone",
					Exec:  "forj build -o ./bin/app",
				},
				{
					Name:  "Run App",
					Watch: "-file ./bin/app -file .env -file .env.*",
					Exec:  "./bin/app run",
				},
			},
		},
	}
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	cmd := makeapp.NewCmd(logger.NewSilentLogger(), NewProjectRenderer(logger.NewSilentLogger()))
	cmd.Name = "ship"
	cmd.Components = "cli"
	cmd.SkipWire = true
	if err := cmd.Run(); err != nil {
		t.Fatalf("make app: %v", err)
	}

	got, err := project.LoadProjectConfig()
	if err != nil {
		t.Fatalf("load migrated config: %v", err)
	}
	if !got.Dev.UsesStructuredApps() {
		t.Fatal("recognized legacy lifecycle was not migrated to native App presence")
	}
	if _, exists := got.Dev.Apps[project.DefaultAppName]; !exists {
		t.Fatalf("migrated default App is absent: %#v", got.Dev.Apps)
	}
	if _, exists := got.Dev.Apps["ship"]; exists {
		t.Fatalf("CLI-only App was silently enrolled: %#v", got.Dev.Apps)
	}
	if got.Dev.Run != nil || len(got.Dev.Watches) != 0 {
		t.Fatalf("legacy lifecycle remained after migration: %#v", got.Dev)
	}
	if _, err := compileDevWatchers(got); err != nil {
		t.Fatalf("migrated make:app config does not compile: %v", err)
	}
}

// TestMakeAppCmdRejectsCustomizedLegacyLifecycleBeforeWriting avoids an invalid mixed native and discovery graph.
func TestMakeAppCmdRejectsCustomizedLegacyLifecycleBeforeWriting(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	config := &project.Config{
		ProjectName:  "TestApp",
		GoModuleName: "example.com/testapp",
		Render:       project.RenderConfig{Components: project.Components{CLI: true}},
		Dev: project.DevConfig{Watches: []project.DevWatch{
			{Name: "Build App", Watch: "-file .go -postpone", Exec: "make custom-build"},
			{Name: "Run App", Watch: "-file ./bin/app", Exec: "./bin/app custom"},
		}},
	}
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatalf("write customized legacy config: %v", err)
	}

	cmd := makeapp.NewCmd(logger.NewSilentLogger(), NewProjectRenderer(logger.NewSilentLogger()))
	cmd.Name = "ship"
	cmd.Components = "cli"
	cmd.SkipWire = true
	err = cmd.Run()
	if err == nil || !strings.Contains(err.Error(), "customized legacy Build App/Run App lifecycle") {
		t.Fatalf("make app error = %v, want migration guidance", err)
	}
	got, loadErr := project.LoadProjectConfig()
	if loadErr != nil {
		t.Fatalf("reload untouched config: %v", loadErr)
	}
	if got.Dev.UsesStructuredApps() || len(got.Apps) != 0 {
		t.Fatalf("failed make:app mutated config: %#v", got)
	}
	if _, statErr := os.Stat(filepath.Join("cmd", "ship")); !os.IsNotExist(statErr) {
		t.Fatalf("failed make:app created App files: %v", statErr)
	}
}

// TestMakeAppCmdPersistsDevRunCommand verifies an explicit CLI App command becomes a scalar native override.
func TestMakeAppCmdPersistsDevRunCommand(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := writeProjectConfig(".goforj.yml", &project.Config{
		ProjectName:  "TestApp",
		GoModuleName: "example.com/testapp",
		Render: project.RenderConfig{
			Components: project.Components{CLI: true},
		},
	}); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := makeapp.NewCmd(logger.NewSilentLogger(), NewProjectRenderer(logger.NewSilentLogger()))
	cmd.Name = "ship"
	cmd.Components = "cli"
	cmd.DevRun = "sync --once"
	cmd.SkipWire = true
	if err := cmd.Run(); err != nil {
		t.Fatalf("make app: %v", err)
	}

	cfg, err := project.LoadProjectConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	devApp, ok := cfg.Dev.Apps["ship"]
	if !ok || devApp.Run == nil || devApp.Run.Exec != "sync --once" || !devApp.Run.Shorthand {
		t.Fatalf("expected custom ship native dev run command, got %#v", devApp)
	}
}

// TestMakeAppCmdPreservesExplicitRunForCLIOnlyApp distinguishes a user command from capability-derived build-only behavior.
func TestMakeAppCmdPreservesExplicitRunForCLIOnlyApp(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := writeProjectConfig(".goforj.yml", &project.Config{
		ProjectName:  "TestApp",
		GoModuleName: "example.com/testapp",
		Render:       project.RenderConfig{Components: project.Components{CLI: true}},
		Dev:          project.DevConfig{Apps: map[string]project.DevApp{}},
	}); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := makeapp.NewCmd(logger.NewSilentLogger(), NewProjectRenderer(logger.NewSilentLogger()))
	cmd.Name = "ship"
	cmd.Components = "cli"
	cmd.DevRun = "run"
	cmd.SkipWire = true
	if err := cmd.Run(); err != nil {
		t.Fatalf("make app: %v", err)
	}

	config, err := project.LoadProjectConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	devApp := config.Dev.Apps["ship"]
	if devApp.Run == nil || !devApp.Run.Shorthand || devApp.Run.Exec != "run" {
		t.Fatalf("explicit CLI run choice became conventional build-only config: %#v", devApp)
	}
	watchers, err := compileDevWatchers(config)
	if err != nil {
		t.Fatalf("compile explicit CLI run choice: %v", err)
	}
	if got, want := compiledDevWatcherNames(watchers), []string{"Build ship", "Run ship"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("compiled watcher names = %#v, want %#v", got, want)
	}
}

func TestMakeAppCmdCreatesAPIOnlyAppInWebUIProject(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := writeProjectConfig(".goforj.yml", &project.Config{
		ProjectName:  "TestApp",
		GoModuleName: "example.com/testapp",
		Render: project.RenderConfig{
			Components: project.Components{
				WebAPI: true,
				WebUI:  true,
			},
			StarterKit: project.StarterKitVue,
		},
	}); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := makeapp.NewCmd(logger.NewSilentLogger(), NewProjectRenderer(logger.NewSilentLogger()))
	cmd.Name = "billing"
	cmd.Components = "web-api"
	cmd.SkipWire = true
	if err := cmd.Run(); err != nil {
		t.Fatalf("make app: %v", err)
	}

	if _, err := os.Stat(filepath.Join("cmd", "billing", "frontend", "dist", "index.html")); !os.IsNotExist(err) {
		t.Fatalf("expected api-only app not to render frontend placeholder, stat err = %v", err)
	}
	mainSrc := readMakeAppTestFile(t, filepath.Join("cmd", "billing", "main.go"))
	if strings.Contains(mainSrc, `"embed"`) || strings.Contains(mainSrc, "RegisterSpa") {
		t.Fatalf("expected api-only app main.go not to embed frontend:\n%s", mainSrc)
	}

	cfg, err := project.LoadProjectConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	appConfig := cfg.Apps["billing"]
	if !appConfig.Components.WebAPI || appConfig.Components.WebUI {
		t.Fatalf("unexpected app components: %#v", appConfig.Components)
	}
	if appConfig.StarterKit != project.StarterKitNone {
		t.Fatalf("api-only app starter kit = %q, want none", appConfig.StarterKit)
	}
}

func TestMakeAppCmdWiresMetricsRunCommandDependency(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := writeProjectConfig(".goforj.yml", &project.Config{
		ProjectName:  "TestApp",
		GoModuleName: "example.com/testapp",
		Render: project.RenderConfig{
			Components: project.Components{
				WebAPI:  true,
				Metrics: true,
			},
		},
	}); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := makeapp.NewCmd(logger.NewSilentLogger(), NewProjectRenderer(logger.NewSilentLogger()))
	cmd.Name = "billing"
	cmd.SkipWire = true
	if err := cmd.Run(); err != nil {
		t.Fatalf("make app: %v", err)
	}

	injectSrc := readMakeAppTestFile(t, filepath.Join("app", "billing", "wire", "inject_cmd.go"))
	for _, want := range []string{
		`"example.com/testapp/internal/metrics"`,
		"metricsManager *metrics.Manager",
		"metricsManager,",
	} {
		if !strings.Contains(injectSrc, want) {
			t.Fatalf("expected metrics run command wiring %q in inject_cmd.go:\n%s", want, injectSrc)
		}
	}
}

// TestMakeAppCmdProjectsDisabledMetricsIntoCLIOnlyApp verifies shared Metrics constructors receive a typed nil without enabling App-local observers.
func TestMakeAppCmdProjectsDisabledMetricsIntoCLIOnlyApp(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := writeProjectConfig(".goforj.yml", &project.Config{
		ProjectName:  "TestApp",
		GoModuleName: "example.com/testapp",
		Render: project.RenderConfig{
			Components: project.Components{
				CLI:     true,
				WebAPI:  true,
				Metrics: true,
			},
		},
	}); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := makeapp.NewCmd(logger.NewSilentLogger(), NewProjectRenderer(logger.NewSilentLogger()))
	cmd.Name = "ship"
	cmd.Components = "cli"
	cmd.SkipWire = true
	if err := cmd.Run(); err != nil {
		t.Fatalf("make app: %v", err)
	}

	managersSrc := readMakeAppTestFile(t, filepath.Join("app", "ship", "wire", "inject_managers.go"))
	for _, want := range []string{
		`"example.com/testapp/internal/metrics"`,
		"provideDisabledMetricsManager",
	} {
		if !strings.Contains(managersSrc, want) {
			t.Fatalf("expected project Metrics boundary %q in inject_managers.go:\n%s", want, managersSrc)
		}
	}
	for _, unwanted := range []string{"metrics.NewManager", "metricsManager.CacheEnabled()", "observability.EventObserver(", "observability.StorageObserver(", `/internal/storages`} {
		if strings.Contains(managersSrc, unwanted) {
			t.Fatalf("expected CLI-only App not to enable Metrics or primitive observer %q:\n%s", unwanted, managersSrc)
		}
	}

	appSetSrc := readMakeAppTestFile(t, filepath.Join("app", "ship", "wire", "inject_services_app.go"))
	if strings.Contains(appSetSrc, "metrics.NewManager") || strings.Contains(appSetSrc, `/internal/metrics`) {
		t.Fatalf("expected app-owned service injector not to provide framework metrics manager:\n%s", appSetSrc)
	}
}

func TestMakeAppCmdDoesNotCreateDemoJobProvidersForNamedApp(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := writeProjectConfig(".goforj.yml", &project.Config{
		ProjectName:  "TestApp",
		GoModuleName: "example.com/testapp",
		Render: project.RenderConfig{
			Components: project.Components{
				DemoApp:       true,
				WebAPI:        true,
				Jobs:          true,
				DatabaseMySQL: true,
			},
		},
	}); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := makeapp.NewCmd(logger.NewSilentLogger(), NewProjectRenderer(logger.NewSilentLogger()))
	cmd.Name = "billing"
	cmd.SkipWire = true
	if err := cmd.Run(); err != nil {
		t.Fatalf("make app: %v", err)
	}

	jobInjectSrc := readMakeAppTestFile(t, filepath.Join("app", "billing", "wire", "inject_jobs_app.go"))
	for _, unwanted := range []string{
		`"example.com/testapp/internal/alerts"`,
		`"example.com/testapp/internal/monitoring"`,
		"alerts.NewDispatchJob",
		"monitoring.NewMonitorCheckJob",
	} {
		if strings.Contains(jobInjectSrc, unwanted) {
			t.Fatalf("did not expect demo job provider %q in named app inject_jobs_app.go:\n%s", unwanted, jobInjectSrc)
		}
	}

	cfg, err := project.LoadProjectConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	appConfig := cfg.Apps["billing"]
	if appConfig.Components.DemoApp {
		t.Fatalf("did not expect Demo App to persist as an app-selectable component")
	}
}

func TestMakeAppCmdCleansLegacyWorkerJobConstructorDeps(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := writeProjectConfig(".goforj.yml", &project.Config{
		ProjectName:  "TestApp",
		GoModuleName: "example.com/testapp",
		Render: project.RenderConfig{
			Components: project.Components{
				DemoApp:       true,
				WebAPI:        true,
				Jobs:          true,
				DatabaseMySQL: true,
			},
		},
	}); err != nil {
		t.Fatalf("write config: %v", err)
	}

	staleWorker := filepath.Join("internal", "jobs", "worker.go")
	if err := os.MkdirAll(filepath.Dir(staleWorker), 0o755); err != nil {
		t.Fatalf("mkdir worker dir: %v", err)
	}
	if err := os.WriteFile(staleWorker, []byte(`package jobs

import (
	"example.com/testapp/internal/alerts"
	"example.com/testapp/internal/monitoring"
)

type Worker struct {
	hello *ExampleHelloJob
	monitorCheck *monitoring.MonitorCheckJob
	alertDispatch *alerts.DispatchJob
}

func NewWorker(
	hello *ExampleHelloJob,
	monitorCheck *monitoring.MonitorCheckJob,
	alertDispatch *alerts.DispatchJob,
) *Worker {
	return &Worker{
		hello: hello,
		monitorCheck: monitorCheck,
		alertDispatch: alertDispatch,
	}
}
`), 0o644); err != nil {
		t.Fatalf("write stale worker: %v", err)
	}

	cmd := makeapp.NewCmd(logger.NewSilentLogger(), NewProjectRenderer(logger.NewSilentLogger()))
	cmd.Name = "billing"
	cmd.SkipWire = true
	if err := cmd.Run(); err != nil {
		t.Fatalf("make app: %v", err)
	}

	workerSrc := readMakeAppTestFile(t, staleWorker)
	for _, unwanted := range []string{
		"internal/alerts",
		"internal/monitoring",
		"hello *ExampleHelloJob,",
		"monitorCheck *monitoring.MonitorCheckJob,",
		"alertDispatch *alerts.DispatchJob,",
	} {
		if strings.Contains(workerSrc, unwanted) {
			t.Fatalf("expected stale worker dependency %q to be removed:\n%s", unwanted, workerSrc)
		}
	}
}

func TestMakeAppCmdCreatesAppVueStarterKit(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := writeProjectConfig(".goforj.yml", &project.Config{
		ProjectName:  "TestApp",
		GoModuleName: "example.com/testapp",
		Render: project.RenderConfig{
			Components: project.Components{
				WebAPI: true,
				WebUI:  true,
			},
			StarterKit: project.StarterKitNone,
		},
	}); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := makeapp.NewCmd(logger.NewSilentLogger(), NewProjectRenderer(logger.NewSilentLogger()))
	cmd.Name = "portal"
	cmd.Components = "web-api,web-ui"
	cmd.StarterKit = "vue"
	cmd.DevRun = "run"
	cmd.SkipWire = true
	if err := cmd.Run(); err != nil {
		t.Fatalf("make app: %v", err)
	}

	for _, path := range []string{
		filepath.Join("cmd", "portal", "frontend", "package.json"),
		filepath.Join("cmd", "portal", "frontend", "components.json"),
		filepath.Join("cmd", "portal", "frontend", "src", "App.vue"),
		filepath.Join("cmd", "portal", "frontend", "dist", "index.html"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
	cfg, err := project.LoadProjectConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	appConfig := cfg.Apps["portal"]
	if !appConfig.Components.WebUI || appConfig.StarterKit != project.StarterKitVue {
		t.Fatalf("unexpected portal app config: %#v", appConfig)
	}
	devApp, ok := cfg.Dev.Apps["portal"]
	portal := project.DefaultNamedApp("portal")
	wantBuild := conventionalDevAppBuildCommand(cfg, portal)
	wantRun := conventionalDevAppRuntimeCommand(portal)
	wantSPA := conventionalDevSPAConfig("./cmd/portal/frontend")
	if !ok || devApp.Build == nil || !reflect.DeepEqual(*devApp.Build, wantBuild) ||
		devApp.Run == nil || !reflect.DeepEqual(*devApp.Run, wantRun) ||
		!reflect.DeepEqual(devApp.SPAs[generatedFrontendSPAName], wantSPA) {
		t.Fatalf("expected portal frontend in the native dev graph, got %#v", devApp)
	}
	wantTask := generatedDevFrontendInstallTask(project.DefaultNamedApp("portal"))
	if !hasDevTask(cfg.Dev.Pre, wantTask) {
		t.Fatalf("expected named frontend dependency task %#v, got %#v", wantTask, cfg.Dev.Pre)
	}
}

func TestMakeAppCmdTreatsExistingAppFilesAsNoOp(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := os.MkdirAll(filepath.Join("cmd", "billing"), 0o755); err != nil {
		t.Fatalf("mkdir app: %v", err)
	}
	writeMakeAppTestFile(t, filepath.Join("cmd", "billing", "main.go"), "package main\n")
	cmd := makeapp.NewCmd(logger.NewSilentLogger(), NewProjectRenderer(logger.NewSilentLogger()))
	cmd.Name = "billing"
	cmd.SkipWire = true
	if err := cmd.Run(); err != nil {
		t.Fatalf("expected existing app to return nil, got %v", err)
	}
}

func TestMakeAppCmdAllowsEmptyConventionalAppDirs(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := writeProjectConfig(".goforj.yml", &project.Config{
		ProjectName:  "TestApp",
		GoModuleName: "example.com/testapp",
		Render: project.RenderConfig{
			Components: project.Components{
				CLI:    true,
				WebAPI: true,
			},
		},
	}); err != nil {
		t.Fatalf("write config: %v", err)
	}
	for _, path := range []string{
		filepath.Join("cmd", "billing"),
		filepath.Join("app", "billing", "wire"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}

	cmd := makeapp.NewCmd(logger.NewSilentLogger(), NewProjectRenderer(logger.NewSilentLogger()))
	cmd.Name = "billing"
	cmd.Components = "cli"
	cmd.SkipWire = true
	if err := cmd.Run(); err != nil {
		t.Fatalf("make app: %v", err)
	}

	for _, path := range []string{
		filepath.Join("cmd", "billing", "main.go"),
		filepath.Join("app", "billing", "root_cmd.go"),
		filepath.Join("app", "billing", "wire", "inject_managers.go"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s after recreating app from empty dirs: %v", path, err)
		}
	}
}

func TestMakeAppCmdRemovesNamedApp(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := writeProjectConfig(".goforj.yml", &project.Config{
		ProjectName:  "TestApp",
		GoModuleName: "example.com/testapp",
		Render: project.RenderConfig{
			Components: project.Components{
				WebAPI: true,
			},
		},
		Dev: project.DevConfig{
			Pre: []project.DevTask{legacyGeneratedDevFrontendInstallTask(project.DefaultNamedApp("billing"))},
			Run: map[string]string{
				"billing": "run",
			},
			Apps: map[string]project.DevApp{
				"billing": {},
			},
		},
		Apps: map[string]project.AppConfig{
			"billing": {
				Components: project.Components{WebAPI: true},
			},
		},
	}); err != nil {
		t.Fatalf("write config: %v", err)
	}
	writeMakeAppTestFile(t, filepath.Join("cmd", "billing", "main.go"), "package main\n")
	writeMakeAppTestFile(t, filepath.Join("cmd", "billing", "custom.go"), "package main\n")
	writeMakeAppTestFile(t, filepath.Join("cmd", "billing", "frontend", "dist", "index.html"), "<html></html>\n")
	writeMakeAppTestFile(t, filepath.Join("app", "billing", "root_cmd.go"), "package billingapp\n")
	writeMakeAppTestFile(t, filepath.Join("bin", "billing"), "binary\n")
	writeMakeAppTestFile(t, filepath.Join("migrations", "billing", "default", "001_create_widgets.up.sql"), "-- user migration\n")
	writeMakeAppTestFile(t, ".env", strings.Join([]string{
		"APP_NAME=TestApp",
		"",
		"# billing",
		"BILLING_APP_URL=http://localhost:3001",
		"BILLING_API_HTTP_PORT=3001",
		"BILLING_CUSTOM_OVERRIDE=true",
		"",
		"# Reporting",
		"REPORTING_APP_URL=http://localhost:3002",
		"",
	}, "\n"))
	writeMakeAppTestFile(t, ".env.host", strings.Join([]string{
		"DB_HOST=localhost",
		"",
		"# Billing",
		"BILLING_DB_HOST=localhost",
		"",
	}, "\n"))

	cmd := makeapp.NewCmd(logger.NewSilentLogger(), NewProjectRenderer(logger.NewSilentLogger()))
	cmd.Name = "billing"
	cmd.Remove = true
	if err := cmd.Run(); err != nil {
		t.Fatalf("remove app: %v", err)
	}

	for _, path := range []string{
		filepath.Join("cmd", "billing", "main.go"),
		filepath.Join("cmd", "billing", "frontend"),
		filepath.Join("app", "billing"),
		filepath.Join("bin", "billing"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, stat err = %v", path, err)
		}
	}
	for _, path := range []string{
		filepath.Join("cmd", "billing", "custom.go"),
		filepath.Join("migrations", "billing", "default", "001_create_widgets.up.sql"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to remain: %v", path, err)
		}
	}

	cfg, err := project.LoadProjectConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if _, ok := cfg.Apps["billing"]; ok {
		t.Fatalf("expected billing app config to be removed")
	}
	if _, ok := cfg.Dev.Run["billing"]; ok {
		t.Fatalf("expected billing dev run config to be removed")
	}
	if cfg.Dev.Run == nil {
		t.Fatal("removing the final legacy allowlist entry changed explicit exclusion into pre-allowlist discovery")
	}
	if configSource := readMakeAppTestFile(t, ".goforj.yml"); !strings.Contains(configSource, "run: {}") {
		t.Fatalf("saved config omitted the empty legacy allowlist:\n%s", configSource)
	}
	if _, ok := cfg.Dev.Apps["billing"]; ok {
		t.Fatalf("expected billing native dev app config to be removed")
	}
	if hasDevTask(cfg.Dev.Pre, generatedDevFrontendInstallTask(project.DefaultNamedApp("billing"))) ||
		hasDevTask(cfg.Dev.Pre, legacyGeneratedDevFrontendInstallTask(project.DefaultNamedApp("billing"))) {
		t.Fatalf("expected billing frontend dependency task to be removed")
	}
	runtimeSrc := readMakeAppTestFile(t, filepath.Join("internal", "runtime", "apps.go"))
	if strings.Contains(runtimeSrc, `Name: "billing"`) {
		t.Fatalf("expected billing runtime metadata to be removed, got:\n%s", runtimeSrc)
	}
	envSrc := readMakeAppTestFile(t, ".env")
	for _, unwanted := range []string{"# billing", "# Billing", "BILLING_"} {
		if strings.Contains(envSrc, unwanted) {
			t.Fatalf("expected billing env entries to be removed, found %q:\n%s", unwanted, envSrc)
		}
	}
	for _, want := range []string{"APP_NAME=TestApp", "# Reporting", "REPORTING_APP_URL=http://localhost:3002"} {
		if !strings.Contains(envSrc, want) {
			t.Fatalf("expected non-billing env entry %q to remain:\n%s", want, envSrc)
		}
	}
	hostEnvSrc := readMakeAppTestFile(t, ".env.host")
	for _, unwanted := range []string{"# billing", "# Billing", "BILLING_"} {
		if strings.Contains(hostEnvSrc, unwanted) {
			t.Fatalf("expected billing host env entries to be removed, found %q:\n%s", unwanted, hostEnvSrc)
		}
	}
	if !strings.Contains(hostEnvSrc, "DB_HOST=localhost") {
		t.Fatalf("expected host env defaults to remain:\n%s", hostEnvSrc)
	}
}

func TestMakeAppCmdRejectsNativeCommandName(t *testing.T) {
	cmd := makeapp.NewCmd(logger.NewSilentLogger(), NewProjectRenderer(logger.NewSilentLogger()))
	cmd.Name = "render"
	if err := cmd.Run(); err == nil {
		t.Fatal("expected native command app name error")
	}
}

// hasDevTask reports whether generated task configuration contains the expected normalized entry.
func hasDevTask(tasks []project.DevTask, want project.DevTask) bool {
	for _, task := range tasks {
		if strings.TrimSpace(task.Name) == want.Name && strings.TrimSpace(task.Cmd) == want.Cmd {
			return true
		}
	}
	return false
}

func readMakeAppTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func writeMakeAppTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
