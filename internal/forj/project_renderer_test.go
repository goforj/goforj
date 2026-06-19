package forj

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestCoreModulesNeedingSyncSkipsPinnedAndReplacedModules(t *testing.T) {
	root := t.TempDir()
	goModPath := filepath.Join(root, "go.mod")
	body := `module example.test/app

go 1.25

require (
	github.com/goforj/web v0.5.2
	github.com/goforj/queue v0.1.0
	github.com/goforj/cache v0.1.0
)

replace github.com/goforj/cache => ../cache
`
	if err := os.WriteFile(goModPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	pending, skipped, err := coreModulesNeedingSync(goModPath, []string{
		"github.com/goforj/web@v0.5.2",
		"github.com/goforj/queue@v0.2.1",
		"github.com/goforj/cache@v0.3.0",
		"github.com/goforj/storage@v0.4.6",
	})
	if err != nil {
		t.Fatalf("coreModulesNeedingSync returned error: %v", err)
	}

	want := []string{
		"github.com/goforj/queue@v0.2.1",
		"github.com/goforj/storage@v0.4.6",
	}
	if !reflect.DeepEqual(pending, want) {
		t.Fatalf("pending modules = %#v, want %#v", pending, want)
	}
	if skipped != 2 {
		t.Fatalf("skipped modules = %d, want 2", skipped)
	}
}

func TestSyncCoreLibrariesUsesGoModEditWithoutResolvingGraph(t *testing.T) {
	root := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/app\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	renderer := NewProjectRenderer(logger.NewSilentLogger())
	if err := renderer.syncCoreLibrariesInDir(root); err != nil {
		t.Fatalf("syncCoreLibraries returned error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"github.com/goforj/web v0.5.2",
		"github.com/goforj/queue v0.2.1",
		"github.com/goforj/cache v0.3.0",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected go.mod to contain %q:\n%s", want, text)
		}
	}
	if len(renderer.lines) != 1 || !strings.Contains(renderer.lines[0], "sync core libs") {
		t.Fatalf("expected sync core libs render line, got %#v", renderer.lines)
	}
}

func TestSyncCoreLibrariesAddsTemplDependencyForTemplStarter(t *testing.T) {
	root := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/app\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	renderer := NewProjectRenderer(logger.NewSilentLogger())
	renderer.config = &project.Config{}
	renderer.config.Render.StarterKit = project.StarterKitTemplHTMX
	if err := renderer.syncCoreLibrariesInDir(root); err != nil {
		t.Fatalf("syncCoreLibraries returned error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	want := "github.com/a-h/templ " + coredeps.MustVersionFor("github.com/a-h/templ")
	if !strings.Contains(string(data), want) {
		t.Fatalf("expected go.mod to contain %q:\n%s", want, string(data))
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

func TestGrafanaSeedComposeStopsQuickly(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "templates", "docker-compose.yml.tmpl"))
	if err != nil {
		t.Fatalf("read docker compose template: %v", err)
	}
	template := string(data)
	for _, token := range []string{
		"grafana-data-init:",
		"chown -R",
		"chmod -R a+rwX /var/lib/grafana",
		"service_completed_successfully",
		"./_data/grafana:/var/lib/grafana",
		"./containers/observability/vmagent:/etc/vmagent:ro",
		"./containers/observability/grafana/provisioning:/etc/grafana/provisioning:ro",
		"./containers/observability/grafana/dashboards:/etc/grafana/dashboards:ro",
		"./containers/observability/grafana/seed-dashboards.sh:/seed-dashboards.sh:ro",
		`command: ["sh", "/seed-dashboards.sh"]`,
	} {
		if strings.Contains(template, token) {
			t.Fatalf("expected docker compose template not to contain %q\n%s", token, template)
		}
	}
	for _, token := range []string{
		"grafana:\n    driver: local",
		"grafana:/var/lib/grafana",
		"source: ./containers/observability/vmagent/prometheus.yml",
		"target: /etc/vmagent/prometheus.yml",
		"source: ./containers/observability/vmagent/metrics-targets.json",
		"target: /etc/vmagent/metrics-targets.json",
		"source: ./containers/observability/grafana/provisioning",
		"target: /etc/grafana/provisioning",
		"source: ./containers/observability/grafana/dashboards",
		"target: /etc/grafana/dashboards",
		"source: ./containers/observability/grafana/seed-dashboards.sh",
		"target: /seed-dashboards.sh",
		"target: /dashboards",
		"create_host_path: false",
		`entrypoint: ["sh"]`,
		`command: ["/seed-dashboards.sh"]`,
	} {
		if !strings.Contains(template, token) {
			t.Fatalf("expected docker compose template to contain %q\n%s", token, template)
		}
	}
	seedIndex := strings.Index(template, "  grafana-seed:")
	if seedIndex < 0 {
		t.Fatal("expected docker compose template to include grafana-seed")
	}
	seedBlock := template[seedIndex:]
	if endIndex := strings.Index(seedBlock, "\n{{- end }}"); endIndex >= 0 {
		seedBlock = seedBlock[:endIndex]
	}
	if !strings.Contains(seedBlock, "stop_grace_period: 1s") {
		t.Fatalf("expected grafana-seed to stop quickly during dev shutdown:\n%s", seedBlock)
	}
}

func TestGrafanaSeedScriptUsesIdempotentAPIImports(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "templates", "containers", "observability", "grafana", "seed-dashboards.sh.tmpl"))
	if err != nil {
		t.Fatalf("read grafana seed script template: %v", err)
	}
	template := string(data)
	for _, token := range []string{
		"/api/datasources/uid/goforj-victoriametrics",
		"-X PUT",
		"-X POST",
		"/api/datasources",
		"/api/dashboards/db",
		`"overwrite":true`,
		"for file in /dashboards/*.json",
		`/api/dashboards/uid/${uid}" >/dev/null 2>/dev/null`,
	} {
		if !strings.Contains(template, token) {
			t.Fatalf("expected grafana seed script template to contain %q\n%s", token, template)
		}
	}
	for _, token := range []string{
		"curl -fsS -u \"${auth}\" \"${grafana_url}/api/dashboards/uid/${uid}\" >/dev/null; do",
		"curl -fsS -u \"${auth}\" \"${grafana_url}/api/dashboards/uid/${uid}\" >/dev/null do",
	} {
		if strings.Contains(template, token) {
			t.Fatalf("expected grafana seed script template not to contain noisy wait loop %q\n%s", token, template)
		}
	}
}

func TestGrafanaSeedDevTask(t *testing.T) {
	task := grafanaSeedDevTask()
	want := project.DevTask{
		Name: "Seed Grafana Dashboards",
		Cmd:  "docker-compose run --rm --no-deps grafana-seed",
	}
	if !reflect.DeepEqual(task, want) {
		t.Fatalf("grafanaSeedDevTask() = %#v, want %#v", task, want)
	}
}

func TestNormalizeGrafanaDevTasks(t *testing.T) {
	tasks := []project.DevTask{
		{Name: "Run Docker Compose", Cmd: "docker-compose up -d"},
		{Name: "Seed Grafana Dashboards", Cmd: "docker-compose up -d --force-recreate grafana-seed"},
	}

	components := project.Components{Grafana: true}
	if !normalizeDockerComposeUpTask(&tasks, components) {
		t.Fatal("expected docker compose up task to be normalized")
	}
	if !normalizeGrafanaSeedTask(&tasks) {
		t.Fatal("expected grafana seed task to be normalized")
	}

	want := []project.DevTask{
		{Name: "Run Docker Compose", Cmd: "docker-compose up -d --scale grafana-seed=0"},
		{Name: "Seed Grafana Dashboards", Cmd: "docker-compose run --rm --no-deps grafana-seed"},
	}
	if !reflect.DeepEqual(tasks, want) {
		t.Fatalf("tasks = %#v, want %#v", tasks, want)
	}
}

func TestDatabaseComposeDataUsesNamedVolumes(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "templates", "docker-compose.yml.tmpl"))
	if err != nil {
		t.Fatalf("read docker compose template: %v", err)
	}
	template := string(data)
	for _, token := range []string{
		"mariadb:\n    driver: local",
		"postgres:\n    driver: local",
		"victoriametrics:\n    driver: local",
		"grafana:\n    driver: local",
		"mariadb:/var/lib/mysql",
		"postgres:/var/lib/postgresql/data",
		"victoriametrics:/victoria-metrics-data",
		"grafana:/var/lib/grafana",
	} {
		if !strings.Contains(template, token) {
			t.Fatalf("expected docker compose template to contain %q\n%s", token, template)
		}
	}
	for _, token := range []string{
		"./_data/mariadb:/var/lib/mysql",
		"./_data/postgres:/var/lib/postgresql/data",
		"./_data/victoriametrics:/victoria-metrics-data",
		"./_data/grafana:/var/lib/grafana",
	} {
		if strings.Contains(template, token) {
			t.Fatalf("expected docker compose template not to contain %q\n%s", token, template)
		}
	}
}

func TestNextStepsIncludeVueAuthLoginHint(t *testing.T) {
	renderer := &ProjectRenderer{config: &project.Config{
		Render: project.RenderConfig{
			StarterKit: project.StarterKitVue,
			Components: project.Components{
				WebUI:          true,
				Auth:           true,
				DatabaseSQLite: true,
			},
		},
	}}

	steps := strings.Join(renderer.nextSteps(), "\n")
	for _, want := range []string{
		"Sign into the Vue app locally",
		"Create another auth user",
		"admin",
		"auth:create-user",
	} {
		if !strings.Contains(steps, want) {
			t.Fatalf("expected next steps to contain %q:\n%s", want, steps)
		}
	}

	renderer.config.Render.Components.Auth = false
	steps = strings.Join(renderer.nextSteps(), "\n")
	if strings.Contains(steps, "auth:create-user") {
		t.Fatalf("expected auth login hint to be omitted without auth:\n%s", steps)
	}
}

func TestNamedAppRenderAppsUseConventionsWithoutConfig(t *testing.T) {
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
	apps, err := renderer.namedAppRenderApps()
	if err != nil {
		t.Fatalf("namedAppRenderApps returned error: %v", err)
	}
	if len(apps) != 2 {
		t.Fatalf("expected two named apps, got %#v", apps)
	}
	if apps[0].Name != "customer-portal" || apps[1].Name != "reporting" {
		t.Fatalf("expected sorted conventional apps, got %#v", apps)
	}
}

func TestRunWireGenerateRunsAppDirsInParallel(t *testing.T) {
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
		filepath.Join("app", "wire"),
		filepath.Join("app", "billing", "wire"),
		filepath.Join("cmd", "billing"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}
	if err := os.WriteFile(filepath.Join("cmd", "billing", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write billing main: %v", err)
	}

	toolsDir := t.TempDir()
	wirePath := filepath.Join(toolsDir, "wire")
	wireScript := "#!/bin/sh\nsleep 0.2\nexit 0\n"
	if err := os.WriteFile(wirePath, []byte(wireScript), 0o755); err != nil {
		t.Fatalf("write fake wire: %v", err)
	}
	t.Setenv("PATH", toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	wireInstallOnce = sync.Once{}
	wireInstallErr = nil
	wireBinaryPath = ""
	defer func() {
		wireInstallOnce = sync.Once{}
		wireInstallErr = nil
		wireBinaryPath = ""
	}()

	renderer := NewProjectRenderer(logger.NewSilentLogger())
	start := time.Now()
	if err := renderer.runWireGenerate(); err != nil {
		t.Fatalf("runWireGenerate returned error: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 350*time.Millisecond {
		t.Fatalf("expected wire dirs to run in parallel, took %s", elapsed)
	}
}

func TestRuntimeAppMetadataUsesCompiledAppOrder(t *testing.T) {
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

	got := runtimeAppMetadataForRender()
	want := []runtimeAppMetadata{
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

func TestExpandDefaultMigrationsForNamedApps(t *testing.T) {
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
	if err := renderer.expandDefaultMigrationsForNamedApps(); err != nil {
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

func TestRenderExpandsDefaultMigrationsWhenNamedAppExists(t *testing.T) {
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

func TestRenderAppWritesNamedAppPackagesAndImports(t *testing.T) {
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

	app := project.DefaultNamedApp("customer-portal")
	if err := renderer.renderApp(app); err != nil {
		t.Fatalf("renderApp returned error: %v", err)
	}

	assertProjectRendererFileContains(t, filepath.Join("cmd", "customer-portal", "main.go"),
		`"example.com/test/app/customer-portal"`,
		`"example.com/test/app/customer-portal/wire"`,
		`&customerportalapp.RootCmd{}`,
	)
	assertProjectRendererFileContains(t, filepath.Join("app", "customer-portal", "root_cmd.go"), "package customerportalapp")
	assertProjectRendererFileContains(t, filepath.Join("app", "customer-portal", "routes.go"), "package customerportalapp")
	assertProjectRendererFileContains(t, filepath.Join("app", "customer-portal", "wire", "inject_http.go"),
		`"example.com/test/app/customer-portal"`,
		"customerportalapp.ProvideRoutes",
	)

	commandsPath := filepath.Join("app", "customer-portal", "commands.go")
	customCommands := "package customerportalapp\n\n// custom\n"
	if err := os.WriteFile(commandsPath, []byte(customCommands), 0o644); err != nil {
		t.Fatalf("write custom commands: %v", err)
	}
	if err := renderer.renderApp(app); err != nil {
		t.Fatalf("rerender app returned error: %v", err)
	}
	content, err := os.ReadFile(commandsPath)
	if err != nil {
		t.Fatalf("read commands after rerender: %v", err)
	}
	if string(content) != customCommands {
		t.Fatalf("expected app-owned commands to be preserved, got:\n%s", content)
	}
}

func TestRenderAppTemplAuthUsesStarterUIInsteadOfAuthAPIController(t *testing.T) {
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
					WebUI:          true,
					Auth:           true,
					DatabaseSQLite: true,
				},
				StarterKit: project.StarterKitTemplHTMX,
			},
		},
		stats: &renderStats{},
	}
	if err := renderer.renderApp(project.DefaultApp()); err != nil {
		t.Fatalf("renderApp returned error: %v", err)
	}

	routesPath := filepath.Join("app", "routes.go")
	assertProjectRendererFileContains(t, routesPath,
		"starterUIController.Routes()",
		"authService.RequireAuth",
	)
	assertProjectRendererFileNotContains(t, routesPath,
		"authController *auth.Controller",
		"authController.Routes()",
	)

	injectPath := filepath.Join("app", "wire", "inject_http_controllers_app.go")
	assertProjectRendererFileContains(t, injectPath, "starterui.NewController")
	assertProjectRendererFileNotContains(t, injectPath,
		`"example.com/test/internal/auth"`,
		"auth.NewController",
	)
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

func TestNormalizeDevWatchWireGenExclusionIsIdempotent(t *testing.T) {
	tests := map[string]string{
		"-file .go -xfile wire/wire_gen\\.go$ -postpone":                         "-file .go -xfile app/wire/wire_gen\\.go$ -postpone",
		"-file .go -xfile app/wire/wire_gen\\.go$ -postpone":                     "-file .go -xfile app/wire/wire_gen\\.go$ -postpone",
		"-file .go -xfile app/app/app/wire/wire_gen\\.go$ -postpone":             "-file .go -xfile app/wire/wire_gen\\.go$ -postpone",
		"-file .go -xfile app/customer-portal/wire/wire_gen\\.go$ -postpone":     "-file .go -xfile app/customer-portal/wire/wire_gen\\.go$ -postpone",
		"-file .go -xfile app/app/customer-portal/wire/wire_gen\\.go$ -postpone": "-file .go -xfile app/customer-portal/wire/wire_gen\\.go$ -postpone",
	}

	for input, want := range tests {
		if got := normalizeDevWatchWireGenExclusion(input); got != want {
			t.Fatalf("normalizeDevWatchWireGenExclusion(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeFrontendNPMWatchExclusions(t *testing.T) {
	input := "-cd ./cmd/app/frontend -xdir _data -xdir ."
	got := normalizeFrontendNPMWatchExclusions(input)
	for _, expected := range []string{input, "-xdir node_modules", "-xdir dist"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("expected normalized NPM watch to contain %q, got %q", expected, got)
		}
	}
	if gotAgain := normalizeFrontendNPMWatchExclusions(got); gotAgain != got {
		t.Fatalf("expected NPM watch normalization to be idempotent, got %q then %q", got, gotAgain)
	}
}

func TestNormalizeTemplBuildWatchExclusions(t *testing.T) {
	input := "-file .go -file .templ -xfile app/wire/wire_gen\\.go$ -postpone"
	got := normalizeTemplBuildWatchExclusions(input)
	if !strings.Contains(got, ".*_templ\\.go$") {
		t.Fatalf("expected templ build watch to exclude generated templ go files, got %q", got)
	}
	if gotAgain := normalizeTemplBuildWatchExclusions(got); gotAgain != got {
		t.Fatalf("expected templ build watch normalization to be idempotent, got %q then %q", got, gotAgain)
	}
}

func TestRenderAppUsesPersistedAppComponents(t *testing.T) {
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
			Apps: map[string]project.AppConfig{
				"billing": {
					Components: project.Components{CLI: true, WebAPI: true},
					StarterKit: project.StarterKitNone,
				},
			},
		},
		stats: &renderStats{},
	}

	app := project.DefaultNamedApp("billing")
	if err := renderer.renderApp(app); err != nil {
		t.Fatalf("renderApp returned error: %v", err)
	}

	mainSrc := readMakeAppTestFile(t, filepath.Join("cmd", "billing", "main.go"))
	if strings.Contains(mainSrc, `"embed"`) || strings.Contains(mainSrc, "RegisterSpa") {
		t.Fatalf("expected app components to omit frontend embedding:\n%s", mainSrc)
	}
	if _, err := os.Stat(filepath.Join("cmd", "billing", "frontend", "dist", "index.html")); !os.IsNotExist(err) {
		t.Fatalf("expected app components to omit frontend placeholder, stat err = %v", err)
	}
}

func TestRenderAppWritesAppAwareFrontendPlaceholder(t *testing.T) {
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

	app := project.DefaultNamedApp("billing")
	if err := renderer.renderApp(app); err != nil {
		t.Fatalf("renderApp returned error: %v", err)
	}

	assertProjectRendererFileContains(t, filepath.Join("cmd", "billing", "frontend", "dist", "index.html"),
		"<title>Test / billing</title>",
		`<link rel="icon" href="./goforj-logo.png" type="image/png">`,
		`<link rel="apple-touch-icon" href="./goforj-logo.png">`,
		`<img class="mark" src="./goforj-logo.png" alt="GoForj logo">`,
		`<span class="brand-tagline">Composable apps for Go</span>`,
		`<div class="status"><span class="status-dot"></span>Running</div>`,
		"<h1>billing</h1>",
		`<div class="app-meta">`,
		`<span>billing</span>`,
		`<span class="app-meta-divider"></span>`,
		"Read the docs",
		`<section class="visual" aria-hidden="true">`,
		`<div class="core">`,
		`<div class="cube">`,
		`<img src="./goforj-logo.png" alt="">`,
	)
	assertProjectRendererLogoCopied(t, filepath.Join("cmd", "billing", "frontend", "dist", "goforj-logo.png"))
}

func TestRenderAppMigratesOldFrontendPlaceholder(t *testing.T) {
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

	app := project.DefaultNamedApp("billing")
	indexPath := filepath.Join("cmd", "billing", "frontend", "dist", "index.html")
	writeProjectRendererTestFile(t, indexPath, oldFrontendDistPlaceholder("Test")+"\n")

	if err := renderer.renderApp(app); err != nil {
		t.Fatalf("renderApp returned error: %v", err)
	}

	assertProjectRendererFileContains(t, indexPath,
		"<title>Test / billing</title>",
		`<link rel="icon" href="./goforj-logo.png" type="image/png">`,
		`<link rel="apple-touch-icon" href="./goforj-logo.png">`,
		`<img class="mark" src="./goforj-logo.png" alt="GoForj logo">`,
		`<span class="brand-tagline">Composable apps for Go</span>`,
		`<div class="status"><span class="status-dot"></span>Running</div>`,
		"<h1>billing</h1>",
		`<div class="app-meta">`,
		`<span>billing</span>`,
		`<span class="app-meta-divider"></span>`,
		"Read the docs",
		`<section class="visual" aria-hidden="true">`,
		`<div class="core">`,
		`<div class="cube">`,
		`<img src="./goforj-logo.png" alt="">`,
	)
	assertProjectRendererLogoCopied(t, filepath.Join("cmd", "billing", "frontend", "dist", "goforj-logo.png"))
}

func TestRenderAppMigratesStyledFrontendPlaceholderWithoutLogo(t *testing.T) {
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

	app := project.DefaultNamedApp("billing")
	indexPath := filepath.Join("cmd", "billing", "frontend", "dist", "index.html")
	writeProjectRendererTestFile(t, indexPath, styledFrontendPlaceholderWithoutLogo("Test / billing"))

	if err := renderer.renderApp(app); err != nil {
		t.Fatalf("renderApp returned error: %v", err)
	}

	assertProjectRendererFileContains(t, indexPath,
		"<title>Test / billing</title>",
		`<link rel="icon" href="./goforj-logo.png" type="image/png">`,
		`<link rel="apple-touch-icon" href="./goforj-logo.png">`,
		`<img class="mark" src="./goforj-logo.png" alt="GoForj logo">`,
		`<span class="brand-tagline">Composable apps for Go</span>`,
		`<div class="status"><span class="status-dot"></span>Running</div>`,
		"<h1>billing</h1>",
		`<div class="app-meta">`,
		`<span>billing</span>`,
		`<span class="app-meta-divider"></span>`,
		"Read the docs",
		`<section class="visual" aria-hidden="true">`,
		`<div class="core">`,
		`<div class="cube">`,
		`<img src="./goforj-logo.png" alt="">`,
	)
	assertProjectRendererLogoCopied(t, filepath.Join("cmd", "billing", "frontend", "dist", "goforj-logo.png"))
}

func TestRenderAppMigratesStyledFrontendPlaceholderWithLegacyLogoName(t *testing.T) {
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

	app := project.DefaultNamedApp("billing")
	indexPath := filepath.Join("cmd", "billing", "frontend", "dist", "index.html")
	legacyLogoName := "goforj-" + "v7.png"
	writeProjectRendererTestFile(t, indexPath, styledFrontendPlaceholderWithLogo("Test / billing", legacyLogoName))

	if err := renderer.renderApp(app); err != nil {
		t.Fatalf("renderApp returned error: %v", err)
	}

	assertProjectRendererFileContains(t, indexPath,
		`<link rel="icon" href="./goforj-logo.png" type="image/png">`,
		`<link rel="apple-touch-icon" href="./goforj-logo.png">`,
		`<img class="mark" src="./goforj-logo.png" alt="GoForj logo">`,
		`<span class="brand-tagline">Composable apps for Go</span>`,
		`<div class="status"><span class="status-dot"></span>Running</div>`,
		`<div class="app-meta">`,
		`<span class="app-meta-divider"></span>`,
		"Read the docs",
		`<section class="visual" aria-hidden="true">`,
		`<div class="core">`,
		`<div class="cube">`,
		`<img src="./goforj-logo.png" alt="">`,
	)
	assertProjectRendererLogoCopied(t, filepath.Join("cmd", "billing", "frontend", "dist", "goforj-logo.png"))
}

func TestRenderAppPreservesCustomFrontendPlaceholder(t *testing.T) {
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

	app := project.DefaultNamedApp("billing")
	indexPath := filepath.Join("cmd", "billing", "frontend", "dist", "index.html")
	custom := "<!doctype html><html><body>custom</body></html>\n"
	writeProjectRendererTestFile(t, indexPath, custom)

	if err := renderer.renderApp(app); err != nil {
		t.Fatalf("renderApp returned error: %v", err)
	}

	content, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read %s: %v", indexPath, err)
	}
	if string(content) != custom {
		t.Fatalf("expected custom frontend placeholder to be preserved, got:\n%s", content)
	}
	if _, err := os.Stat(filepath.Join("cmd", "billing", "frontend", "dist", "goforj-logo.png")); !os.IsNotExist(err) {
		t.Fatalf("expected custom frontend placeholder not to receive logo asset, stat err = %v", err)
	}
}

func TestRenderAppWritesDefaultAppShape(t *testing.T) {
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
	if err := renderer.renderApp(project.DefaultApp()); err != nil {
		t.Fatalf("renderApp returned error: %v", err)
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

func assertProjectRendererFileNotContains(t *testing.T, path string, snippets ...string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	source := string(content)
	for _, snippet := range snippets {
		if strings.Contains(source, snippet) {
			t.Fatalf("expected %s not to contain %q:\n%s", path, snippet, source)
		}
	}
}

func styledFrontendPlaceholderWithoutLogo(title string) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>%s</title>
</head>
<body>
    <main>
        <div class="brand">
            <span class="mark">G</span>
            <span>GoForj</span>
        </div>
        <h1>%s</h1>
        <p>%s</p>
    </main>
</body>
</html>
`, title, title, oldStyledFrontendPlaceholderCopy())
}

func styledFrontendPlaceholderWithLogo(title string, logo string) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>%s</title>
</head>
<body>
    <main>
        <div class="brand">
            <img class="mark" src="./%s" alt="GoForj logo">
            <span>GoForj</span>
        </div>
        <h1>%s</h1>
        <p>%s</p>
    </main>
</body>
</html>
`, title, logo, title, oldStyledFrontendPlaceholderCopy())
}

func oldStyledFrontendPlaceholderCopy() string {
	return "This app is running, but no frontend build has been deployed yet."
}

func assertProjectRendererLogoCopied(t *testing.T, path string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read copied logo %s: %v", path, err)
	}
	want, err := templatesFS.ReadFile(frontendPlaceholderLogoTemplate)
	if err != nil {
		t.Fatalf("read template logo: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("copied logo %s did not match template asset", path)
	}
}

func TestSyncLegacyAppServiceInjectorMovesLifecycleRegistryToCompositionApp(t *testing.T) {
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

	updated := syncLegacyAppServiceInjector(legacy, "example.com/testapp", "app")
	for _, want := range []string{
		`"example.com/testapp/app"`,
		`"example.com/testapp/internal/runtime"`,
		"app.NewLifecycleRegistry",
		"runtime.NewTimeouts",
	} {
		if !strings.Contains(updated, want) {
			t.Fatalf("expected migrated service injector to contain %q:\n%s", want, updated)
		}
	}
	for _, unexpected := range []string{
		"\t\"example.com/testapp/internal/app\"",
		"\tapp.NewTimeouts",
		"compositionapp",
		"runtimeruntime",
		"runtimeapp",
	} {
		if strings.Contains(updated, unexpected) {
			t.Fatalf("expected migrated service injector not to contain %q:\n%s", unexpected, updated)
		}
	}

	idempotent := syncLegacyAppServiceInjector(updated, "example.com/testapp", "app")
	if idempotent != updated {
		t.Fatalf("expected migration to be idempotent:\n%s", idempotent)
	}
}

func TestSyncLegacyAppServiceInjectorUsesNamedAppImport(t *testing.T) {
	legacy := `package wire

import (
	"github.com/goforj/wire"

	targetapp "example.com/testapp/app/billing"
)

var appSet = wire.NewSet(
	targetapp.NewLifecycleRegistry,
)
`

	updated := syncLegacyAppServiceInjector(legacy, "example.com/testapp", "app/billing")
	for _, want := range []string{
		`"example.com/testapp/app/billing"`,
		"billingapp.NewLifecycleRegistry",
	} {
		if !strings.Contains(updated, want) {
			t.Fatalf("expected migrated service injector to contain %q:\n%s", want, updated)
		}
	}
	if strings.Contains(updated, "targetapp") {
		t.Fatalf("expected targetapp alias to be replaced:\n%s", updated)
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

func TestSyncLegacyScheduleInjectorAddsAppRegistryProvider(t *testing.T) {
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

	updated := syncLegacyScheduleInjector(legacy, "example.com/testapp", "app")
	for _, want := range []string{
		`"example.com/testapp/app"`,
		`"example.com/testapp/internal/schedules"`,
		"app.NewScheduleRegistry",
		"wire.Bind(new(schedules.ScheduleRegistry), new(*app.ScheduleRegistry))",
		"ProvideAppSchedules",
		"reports.NewDailySchedule",
		"dailySchedule *reports.DailySchedule",
	} {
		if !strings.Contains(updated, want) {
			t.Fatalf("expected migrated schedule injector to contain %q:\n%s", want, updated)
		}
	}

	idempotent := syncLegacyScheduleInjector(updated, "example.com/testapp", "app")
	if idempotent != updated {
		t.Fatalf("expected schedule migration to be idempotent:\n%s", idempotent)
	}
}

func TestSyncLegacyScheduleInjectorAliasesExistingAppImport(t *testing.T) {
	legacy := `package wire

import (
	"github.com/goforj/wire"
	targetapp "example.com/testapp/app"
	"example.com/testapp/internal/schedules"
)

var appScheduleSet = wire.NewSet(
	ProvideAppSchedules,
	targetapp.NewScheduleRegistry,
	wire.Bind(new(schedules.ScheduleRegistry), new(*targetapp.ScheduleRegistry)),
	compositionapp.NewScheduleRegistry,
	wire.Bind(new(schedules.ScheduleRegistry), new(*compositionapp.ScheduleRegistry)),
)

func ProvideAppSchedules() *schedules.AppSchedules {
	return schedules.NewAppSchedules()
}
`

	updated := syncLegacyScheduleInjector(legacy, "example.com/testapp", "app")
	for _, want := range []string{
		`"example.com/testapp/app"`,
		"app.NewScheduleRegistry",
		"wire.Bind(new(schedules.ScheduleRegistry), new(*app.ScheduleRegistry))",
	} {
		if !strings.Contains(updated, want) {
			t.Fatalf("expected migrated schedule injector to contain %q:\n%s", want, updated)
		}
	}
	if strings.Contains(updated, "targetapp.") {
		t.Fatalf("expected targetapp references to be replaced:\n%s", updated)
	}
	if count := strings.Count(updated, "app.NewScheduleRegistry"); count != 1 {
		t.Fatalf("expected one schedule registry provider, got %d:\n%s", count, updated)
	}
	if count := strings.Count(updated, "wire.Bind(new(schedules.ScheduleRegistry), new(*app.ScheduleRegistry))"); count != 1 {
		t.Fatalf("expected one schedule registry binding, got %d:\n%s", count, updated)
	}
}

func TestSyncLegacyScheduleInjectorUsesNamedAppImport(t *testing.T) {
	legacy := `package wire

import (
	"github.com/goforj/wire"
	targetapp "example.com/testapp/app/billing"
	"example.com/testapp/internal/schedules"
)

var appScheduleSet = wire.NewSet(
	targetapp.NewScheduleRegistry,
	wire.Bind(new(schedules.ScheduleRegistry), new(*targetapp.ScheduleRegistry)),
)
`

	updated := syncLegacyScheduleInjector(legacy, "example.com/testapp", "app/billing")
	for _, want := range []string{
		`"example.com/testapp/app/billing"`,
		"billingapp.NewScheduleRegistry",
		"wire.Bind(new(schedules.ScheduleRegistry), new(*billingapp.ScheduleRegistry))",
	} {
		if !strings.Contains(updated, want) {
			t.Fatalf("expected migrated schedule injector to contain %q:\n%s", want, updated)
		}
	}
	if strings.Contains(updated, "targetapp") {
		t.Fatalf("expected targetapp alias to be replaced:\n%s", updated)
	}
}

func TestSyncLegacyScheduleInjectorReplacesVariadicScheduleProvider(t *testing.T) {
	legacy := `package wire

import (
	"github.com/goforj/wire"

	compositionapp "example.com/testapp/app"
	"example.com/testapp/internal/schedules"
)

var appScheduleSet = wire.NewSet(
	schedules.NewAppSchedules,
	compositionapp.NewScheduleRegistry,
	wire.Bind(new(schedules.ScheduleRegistry), new(*compositionapp.ScheduleRegistry)),
)
`

	updated := syncLegacyScheduleInjector(legacy, "example.com/testapp", "app")
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
	compositionapp "example.com/testapp/app"
	"example.com/testapp/internal/runtime"
	"example.com/testapp/internal/makecmd"
)

var appSet = wire.NewSet(
	compositionapp.NewLifecycleRegistry,
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

func TestUpsertEnvDefaultsAddsAppDatabaseDriver(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".env")
	if err := os.WriteFile(path, []byte("DB_DRIVER=mysql\nDB_SUPPORTED_DRIVERS=mysql\n"), 0o644); err != nil {
		t.Fatalf("write env: %v", err)
	}

	err := upsertEnvDefaults(path, appDatabaseEnvDefaults("REPORTING", "postgres", "mysql", false))
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

func TestUpsertAppEnvDefaultsGroupsAndOrdersAppKeys(t *testing.T) {
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
	err := upsertAppEnvDefaults(path, "billing", "BILLING", defaults)
	if err != nil {
		t.Fatalf("upsert app env defaults: %v", err)
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
		t.Fatalf("unexpected app env section:\nwant:\n%s\ngot:\n%s", want, text)
	}
	if strings.Count(text, "BILLING_DB_DATABASE=") != 1 {
		t.Fatalf("expected exactly one app database override:\n%s", text)
	}
}

func TestAppDatabaseHostDefaultsUseLocalhostForHostEnv(t *testing.T) {
	defaults := appDatabaseEnvDefaults("REPORTING", "postgres", "mysql", true)
	if got := defaults["REPORTING_DB_HOST"]; got != "localhost" {
		t.Fatalf("REPORTING_DB_HOST = %q, want localhost", got)
	}
}

func TestAppDatabaseEnvDefaultsInheritSharedConnection(t *testing.T) {
	defaults := appDatabaseEnvDefaults("BILLING", "mysql", "mysql", false)
	for _, want := range []string{
		"DB_SUPPORTED_DRIVERS",
		"BILLING_DB_DATABASE",
		"BILLING_DB_SQLITE_DATABASE",
	} {
		if _, ok := defaults[want]; !ok {
			t.Fatalf("expected %s in app database defaults: %#v", want, defaults)
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
			t.Fatalf("did not expect inherited connection key %s in app database defaults: %#v", unwanted, defaults)
		}
	}
	if defaults["BILLING_DB_DATABASE"] != "billing" {
		t.Fatalf("BILLING_DB_DATABASE = %q, want billing", defaults["BILLING_DB_DATABASE"])
	}
	if defaults["BILLING_DB_SQLITE_DATABASE"] != "./_data/sqlite/billing.db" {
		t.Fatalf("BILLING_DB_SQLITE_DATABASE = %q, want sqlite fallback", defaults["BILLING_DB_SQLITE_DATABASE"])
	}
}

func TestAppDatabaseEnvDefaultsDoNotDuplicateActiveSQLiteDatabase(t *testing.T) {
	defaults := appDatabaseEnvDefaults("BILLING", "sqlite", "sqlite", false)
	if defaults["BILLING_DB_DATABASE"] != "./_data/sqlite/billing.db" {
		t.Fatalf("BILLING_DB_DATABASE = %q, want sqlite path", defaults["BILLING_DB_DATABASE"])
	}
	if _, ok := defaults["BILLING_DB_SQLITE_DATABASE"]; ok {
		t.Fatalf("did not expect duplicate sqlite fallback for active sqlite driver: %#v", defaults)
	}
}

func TestWriteAppEnvDefaultsKeepsSupportedDriversInBaseEnv(t *testing.T) {
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
	err = renderer.writeAppEnvDefaults(project.DefaultNamedApp("reporting"), project.Components{WebAPI: true, Metrics: true, DatabasePostgres: true})
	if err != nil {
		t.Fatalf("write app env defaults: %v", err)
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
		t.Fatalf(".env.host missing app section heading:\n%s", hostText)
	}
	if !strings.Contains(hostText, "REPORTING_DB_HOST=localhost") {
		t.Fatalf(".env.host missing app localhost override:\n%s", hostText)
	}
}
