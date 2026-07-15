package atlas

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/atlas/diagnostics"
	atlasproject "github.com/goforj/atlas/project"
	"github.com/goforj/atlas/skills"
	"github.com/goforj/atlas/workflows"
	"github.com/goforj/goforj/project"
)

func TestProjectUsesGoForjConfig(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".goforj.yml"), `
project_name: demo
module_name: example.com/demo
render:
  goforj_version: 0.18.0
  starter_kit: vue
  components:
    cli: true
    web_api: true
    web_ui: true
    jobs: true
    database_sqlite: true
`)
	writeFile(t, filepath.Join(root, ".env"), "QUEUE_DRIVER=nats\n")
	writeFile(t, filepath.Join(root, "cmd", "app", "main.go"), "package main\n")
	writeFile(t, filepath.Join(root, "app", "routes.go"), "package app\n")
	writeFile(t, filepath.Join(root, "cmd", "marketplace", "main.go"), "package main\n")
	writeFile(t, filepath.Join(root, "app", "marketplace", "routes.go"), "package marketplace\n")

	project := Project(root)
	if project.Name != "demo" {
		t.Fatalf("project name = %q", project.Name)
	}
	if project.GoForjVersion != "0.18.0" {
		t.Fatalf("goforj version = %q", project.GoForjVersion)
	}
	if project.FrontendKit != "vue" {
		t.Fatalf("frontend kit = %q", project.FrontendKit)
	}
	if project.DatabaseDriver != "sqlite" {
		t.Fatalf("database driver = %q", project.DatabaseDriver)
	}
	if project.QueueDriver != "nats" {
		t.Fatalf("queue driver = %q", project.QueueDriver)
	}
	if !containsString(project.Components, "web-api") || !containsString(project.Components, "jobs") {
		t.Fatalf("components = %#v", project.Components)
	}
	if !containsApp(project.Apps, "marketplace") {
		t.Fatalf("apps = %#v", project.Apps)
	}
}

// TestProjectUsesDerivedComponentEnvelopeWithoutWideningDefaultApp keeps project metadata complete while preserving App-local runtime and database choices.
func TestProjectUsesDerivedComponentEnvelopeWithoutWideningDefaultApp(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".goforj.yml"), `
project_name: multi-app
module_name: example.com/multi-app
render:
  components:
    cli: true
    database_sqlite: true
apps:
  worker:
    components:
      cli: true
      jobs: true
      database_postgres: true
`)
	writeFile(t, filepath.Join(root, "cmd", "app", "main.go"), "package main\n")
	writeFile(t, filepath.Join(root, "cmd", "worker", "main.go"), "package main\n")
	writeFile(t, filepath.Join(root, "app", "worker", "commands.go"), "package workerapp\n")
	writeFile(t, filepath.Join(root, ".env"), "QUEUE_DRIVER=redis\n")

	discovered := Project(root)
	if !containsString(discovered.Components, "jobs") || !containsString(discovered.Components, "database-postgres") {
		t.Fatalf("project component envelope = %#v", discovered.Components)
	}
	if discovered.DatabaseDriver != "sqlite" {
		t.Fatalf("default App database driver = %q, want sqlite", discovered.DatabaseDriver)
	}
	if discovered.QueueDriver != "" {
		t.Fatalf("default App queue driver = %q, want none for its disabled Jobs component", discovered.QueueDriver)
	}

	var defaultApp atlasproject.App
	var workerApp atlasproject.App
	for _, app := range discovered.Apps {
		switch app.Name {
		case "app":
			defaultApp = app
		case "worker":
			workerApp = app
		}
	}
	if containsString(defaultApp.Runtimes, "jobs") || !containsString(defaultApp.Runtimes, "cli") {
		t.Fatalf("default App runtimes = %#v, want only its CLI runtime", defaultApp.Runtimes)
	}
	if !containsString(workerApp.Runtimes, "jobs") {
		t.Fatalf("worker App runtimes = %#v, want jobs", workerApp.Runtimes)
	}
}

// TestProjectIgnoresLegacyQueueDriver keeps deployment state environment-owned during Atlas discovery.
func TestProjectIgnoresLegacyQueueDriver(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".goforj.yml"), `
project_name: legacy
module_name: example.com/legacy
render:
  queue_driver: nats
  components:
    cli: true
    jobs: true
`)

	discovered := Project(root)
	if discovered.QueueDriver == "nats" {
		t.Fatal("Atlas treated legacy queue_driver YAML as active deployment state")
	}
}

func TestInventoryDiscoversSafeProjectResources(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".goforj.yml"), `
project_name: demo
module_name: example.com/demo
render:
  starter_kit: vue
  components:
    cli: true
    web_api: true
    web_ui: true
    jobs: true
    events: true
    scheduler: true
    metrics: true
    database_sqlite: true
`)
	writeFile(t, filepath.Join(root, ".env"), `
APP_URL=http://localhost:9000
API_HTTP_PORT=9000
METRICS_API_PORT=19000
QUEUE_REPORTS_DRIVER=redis
QUEUE_REPORTS_NAME=reports
CACHE_SESSIONS_DRIVER=file
STORAGE_PUBLIC_DRIVER=local
EVENTS_AUDIT_DRIVER=null
LIGHTHOUSE_URL=ws://127.0.0.1:7777/lighthouse/ws/agent
`)
	writeFile(t, filepath.Join(root, "cmd", "app", "main.go"), "package main\n")
	writeFile(t, filepath.Join(root, "app", "routes.go"), `
package app

func ProvideRoutes() {
	_ = web.NewRouteGroup("/api/v1", nil)
}
`)
	writeFile(t, filepath.Join(root, "app", "commands.go"), "package app\n\ntype Commands struct {\n\tHealthCmd cmd.HealthCmd `cmd:\"\"`\n}\n")
	writeFile(t, filepath.Join(root, "app", "schedules.go"), "package app\n\nfunc (r *ScheduleRegistry) Register() {\n\ts.Daily().Name(\"reports:daily\")\n}\n")

	inventory := Inventory(root)
	if !containsString(inventory.Queues, "reports") || !containsString(inventory.Caches, "sessions") ||
		!containsString(inventory.Disks, "public") || !containsString(inventory.EventBuses, "audit") {
		t.Fatalf("inventory resources = queues %#v caches %#v disks %#v events %#v", inventory.Queues, inventory.Caches, inventory.Disks, inventory.EventBuses)
	}
	if !containsString(inventory.Routes["app"], "group /api/v1") {
		t.Fatalf("routes = %#v", inventory.Routes)
	}
	if !containsString(inventory.Commands["app"], "HealthCmd") {
		t.Fatalf("commands = %#v", inventory.Commands)
	}
	if !containsString(inventory.Schedules["app"], "reports:daily") {
		t.Fatalf("schedules = %#v", inventory.Schedules)
	}
	appResource, ok := resourceLinkByID(inventory.Resources, "app")
	if !ok || appResource.URL != "http://localhost:9000" || appResource.Source != "config" {
		t.Fatalf("app resource = %#v ok=%v resources=%#v", appResource, ok, inventory.Resources)
	}
	lighthouseResource, ok := resourceLinkByID(inventory.Resources, "lighthouse")
	if !ok || lighthouseResource.URL != "http://127.0.0.1:7777/lighthouse" ||
		lighthouseResource.Category != "operator" || lighthouseResource.Source != "env" || lighthouseResource.Runtime != "operator" {
		t.Fatalf("resources = %#v", inventory.Resources)
	}
}

// TestInventoryIgnoresStaleDisabledEventBuses verifies Atlas follows project capability selection instead of owner env residue.
func TestInventoryIgnoresStaleDisabledEventBuses(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".goforj.yml"), `
project_name: demo
module_name: example.com/demo
render:
  components: [cli]
`)
	writeFile(t, filepath.Join(root, ".env"), "CACHE_SESSIONS_DRIVER=redis\nEVENTS_AUDIT_DRIVER=redis\n")

	inventory := Inventory(root)
	if len(inventory.Caches) != 0 {
		t.Fatalf("stale Cache env resurrected Atlas caches: %#v", inventory.Caches)
	}
	if len(inventory.EventBuses) != 0 {
		t.Fatalf("stale Events env resurrected Atlas event buses: %#v", inventory.EventBuses)
	}
}

// TestInventoryUsesNamedAppEventsEnvelope verifies Atlas exposes shared buses when only a named App participates in Events.
func TestInventoryUsesNamedAppEventsEnvelope(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".goforj.yml"), `
project_name: demo
module_name: example.com/demo
render:
  components: [cli]
apps:
  events-worker:
    components: [cli, cache, events]
`)
	writeFile(t, filepath.Join(root, ".env"), "CACHE_REPORTS_DRIVER=memory\nEVENTS_AUDIT_DRIVER=inproc\n")

	inventory := Inventory(root)
	if !containsString(inventory.Caches, "reports") {
		t.Fatalf("named Cache App did not expose Atlas caches: %#v", inventory.Caches)
	}
	if !containsString(inventory.EventBuses, "audit") {
		t.Fatalf("named Events App did not expose Atlas event buses: %#v", inventory.EventBuses)
	}
}

// TestInventoryIgnoresStaleDisabledStorageDisks verifies Atlas follows project capability selection instead of owner env residue.
func TestInventoryIgnoresStaleDisabledStorageDisks(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".goforj.yml"), `
project_name: demo
module_name: example.com/demo
render:
  components: [cli]
`)
	writeFile(t, filepath.Join(root, ".env"), "STORAGE_PUBLIC_DRIVER=s3\n")

	inventory := Inventory(root)
	if len(inventory.Disks) != 0 {
		t.Fatalf("stale Storage env resurrected Atlas disks: %#v", inventory.Disks)
	}
	if _, ok := resourceLinkByID(inventory.Resources, "storage-public"); ok {
		t.Fatalf("stale Storage env resurrected an Atlas resource link: %#v", inventory.Resources)
	}
}

// TestInventoryUsesNamedAppStorageEnvelope verifies Atlas exposes shared disks when only a named App participates in Storage.
func TestInventoryUsesNamedAppStorageEnvelope(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".goforj.yml"), `
project_name: demo
module_name: example.com/demo
render:
  components: [cli]
apps:
  files:
    components: [cli, storage]
`)
	writeFile(t, filepath.Join(root, ".env"), "STORAGE_PUBLIC_DRIVER=local\n")

	inventory := Inventory(root)
	if !containsString(inventory.Disks, "public") {
		t.Fatalf("named Storage App did not expose Atlas disks: %#v", inventory.Disks)
	}
	if _, ok := resourceLinkByID(inventory.Resources, "storage-public"); !ok {
		t.Fatalf("named Storage App did not expose Atlas resource links: %#v", inventory.Resources)
	}
}

// TestInventoryIgnoresStaleDisabledQueues verifies Atlas does not turn Queue env residue into Background Jobs capability.
func TestInventoryIgnoresStaleDisabledQueues(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".goforj.yml"), `
project_name: demo
module_name: example.com/demo
render:
  components: [cli]
`)
	writeFile(t, filepath.Join(root, ".env"), `
QUEUE_DRIVER=redis
QUEUE_REPORTS_DRIVER=redis
WORKER_QUEUE_EMAILS_DRIVER=redis
`)

	inventory := Inventory(root)
	if len(inventory.Queues) != 0 {
		t.Fatalf("stale Queue env resurrected Atlas queues: %#v", inventory.Queues)
	}
	for _, resource := range inventory.Resources {
		if resource.Category == "queue" {
			t.Fatalf("stale Queue env resurrected an Atlas resource link: %#v", inventory.Resources)
		}
	}
}

// TestInventoryKeepsNamedAppQueuesLocal verifies Atlas projects Queue resources only through participating Jobs Apps.
func TestInventoryKeepsNamedAppQueuesLocal(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".goforj.yml"), `
project_name: demo
module_name: example.com/demo
render:
  components: [cli]
apps:
  api:
    components: [cli, web_api]
  worker:
    components: [cli, jobs]
`)
	writeFile(t, filepath.Join(root, ".env"), `
QUEUE_DRIVER=workerpool
QUEUE_REPORTS_DRIVER=workerpool
API_QUEUE_EXPORTS_DRIVER=redis
WORKER_QUEUE_EMAILS_DRIVER=redis
`)

	inventory := Inventory(root)
	if len(inventory.Queues) != 3 {
		t.Fatalf("named Jobs App queues = %#v, want exactly default, emails, and reports", inventory.Queues)
	}
	for _, name := range []string{"default", "emails", "reports"} {
		if !containsString(inventory.Queues, name) {
			t.Fatalf("named Jobs App queue %q missing from Atlas inventory: %#v", name, inventory.Queues)
		}
	}
	if containsString(inventory.Queues, "exports") {
		t.Fatalf("disabled API App Queue overlay leaked into Atlas inventory: %#v", inventory.Queues)
	}
}

// TestMetricsMetadataKeepsJobsProjectionAppLocal verifies Jobs counters are advertised only for participating Apps.
func TestMetricsMetadataKeepsJobsProjectionAppLocal(t *testing.T) {
	config := &project.Config{
		Render: project.RenderConfig{Components: project.Components{
			CLI: true, WebAPI: true, Metrics: true, Cache: true,
		}},
		Apps: map[string]project.AppConfig{
			"worker": {Components: project.Components{CLI: true, Jobs: true}},
		},
	}
	metadata := metricsMetadata([]atlasproject.App{
		{Name: project.DefaultAppName, Default: true},
		{Name: "worker"},
	}, config, nil)
	if counters := metadata["app/http"].Counters; containsString(counters, "queue_jobs_total") {
		t.Fatalf("default App advertised Jobs counters without Jobs: %#v", counters)
	}
	if counters := metadata["app/http"].Counters; !containsString(counters, "cache_operations_total") {
		t.Fatalf("default App omitted its Cache counters: %#v", counters)
	}
	if counters := metadata["worker/jobs"].Counters; !containsString(counters, "queue_jobs_total") {
		t.Fatalf("worker App omitted its Jobs counters: %#v", counters)
	}
	if counters := metadata["worker/jobs"].Counters; containsString(counters, "cache_operations_total") {
		t.Fatalf("worker App advertised Cache counters without Cache: %#v", counters)
	}
}

func TestDiagnosticsDiscoversSafeAtlasMetadata(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".goforj.yml"), `
project_name: demo
module_name: example.com/demo
render:
  components:
    cli: true
    web_api: true
    metrics: true
    database_postgres: true
`)
	writeFile(t, filepath.Join(root, ".env"), `
APP_URL=http://localhost:9100
DB_DRIVER=postgres # options: sqlite, mysql, postgres
DB_DATABASE=demo_app
METRICS_API_PORT=19100
`)
	writeFile(t, filepath.Join(root, "cmd", "app", "main.go"), "package main\n")
	writeFile(t, filepath.Join(root, "app", "routes.go"), "package app\n")

	provider := Diagnostics(root)
	connections, err := provider.DatabaseConnections(t.Context())
	if err != nil {
		t.Fatalf("database connections: %v", err)
	}
	if len(connections) != 1 || connections[0].Driver != "postgres" || connections[0].Database != "demo_app" {
		t.Fatalf("connections = %#v", connections)
	}
	url, err := provider.AbsoluteURL(t.Context(), diagnostics.URLRequest{App: "app", Path: "/health"})
	if err != nil {
		t.Fatalf("absolute url: %v", err)
	}
	if url != "http://localhost:9100/health" {
		t.Fatalf("url = %q", url)
	}
	metrics, err := provider.MetricsMetadata(t.Context(), diagnostics.MetricsMetadataRequest{App: "app", Runtime: "http"})
	if err != nil {
		t.Fatalf("metrics metadata: %v", err)
	}
	if len(metrics.Targets) != 1 || metrics.Targets[0] != "http://localhost:19100/metrics" {
		t.Fatalf("metrics = %#v", metrics)
	}
}

func TestInstallCmdWritesDefaultCodexFiles(t *testing.T) {
	root := t.TempDir()
	withWorkingDir(t, root)
	writeFile(t, ".goforj.yml", `
project_name: demo
module_name: example.com/demo
render:
  goforj_version: 0.18.0
  components:
    cli: true
`)

	cmd := &InstallCmd{Agent: []string{"codex"}, NoInteraction: true}
	if err := cmd.Run(); err != nil {
		t.Fatalf("run install: %v", err)
	}

	assertFileContains(t, "AGENTS.md", "GoForj Atlas")
	assertFileContains(t, filepath.Join(".codex", "config.toml"), "atlas:mcp")
	assertFileContains(t, filepath.Join(".codex", "config.toml"), `cwd = "."`)
	assertFileContains(t, filepath.Join(".agents", "skills", "goforj-make-commands", "SKILL.md"), "forj <app> make:*")
	assertFileContains(t, filepath.Join(".goforj", "atlas.json"), `"codex"`)
}

func TestInstallCmdDryRunDoesNotWriteFiles(t *testing.T) {
	root := t.TempDir()
	withWorkingDir(t, root)
	writeFile(t, ".goforj.yml", `
project_name: demo
module_name: example.com/demo
render:
  components:
    cli: true
`)

	cmd := &InstallCmd{Agent: []string{"codex"}, NoInteraction: true, DryRun: true}
	if err := cmd.Run(); err != nil {
		t.Fatalf("run dry install: %v", err)
	}
	if _, err := os.Stat("AGENTS.md"); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote AGENTS.md: %v", err)
	}
}

func TestInstallCmdWritesGeminiFiles(t *testing.T) {
	root := t.TempDir()
	withWorkingDir(t, root)
	writeFile(t, ".goforj.yml", `
project_name: demo
module_name: example.com/demo
render:
  goforj_version: 0.18.0
  components:
    cli: true
`)

	cmd := &InstallCmd{Agent: []string{"gemini"}, NoInteraction: true}
	if err := cmd.Run(); err != nil {
		t.Fatalf("run install: %v", err)
	}

	assertFileContains(t, "GEMINI.md", "GoForj Atlas")
	assertFileContains(t, filepath.Join(".gemini", "settings.json"), "atlas:mcp")
	assertFileContains(t, filepath.Join(".gemini", "skills", "goforj-make-commands", "GEMINI.md"), "forj <app> make:*")
	assertFileContains(t, filepath.Join(".goforj", "atlas.json"), `"gemini"`)
}

func TestDoctorCmdReportsAtlasStatus(t *testing.T) {
	root := t.TempDir()
	withWorkingDir(t, root)
	writeFile(t, ".goforj.yml", `
project_name: demo
module_name: example.com/demo
render:
  goforj_version: 0.18.0
  components:
    cli: true
`)
	if err := (&InstallCmd{Agent: []string{"codex"}, NoInteraction: true}).Run(); err != nil {
		t.Fatalf("run install: %v", err)
	}

	output := captureStdout(t, func() {
		if err := (&DoctorCmd{}).Run(); err != nil {
			t.Fatalf("run doctor: %v", err)
		}
	})
	if !strings.Contains(output, "Atlas installed: true") || !strings.Contains(output, "Agent codex: configured=true") || !strings.Contains(output, "Skills:") {
		t.Fatalf("unexpected doctor output:\n%s", output)
	}
}

func TestListSkillsIncludesMakeCommands(t *testing.T) {
	if _, ok := skills.ByName("goforj-make-commands"); !ok {
		t.Fatal("expected built-in make command skill")
	}
}

func TestMakeSkillCmdCreatesProjectSkill(t *testing.T) {
	root := t.TempDir()
	withWorkingDir(t, root)

	cmd := &MakeSkillCmd{Name: "checkout-rules"}
	if err := cmd.Run(); err != nil {
		t.Fatalf("run make skill: %v", err)
	}

	assertFileContains(t, filepath.Join(".ai", "skills", "checkout-rules", "SKILL.md"), "Checkout Rules")
}

func TestListSkillsCmdIncludesProjectSkills(t *testing.T) {
	root := t.TempDir()
	withWorkingDir(t, root)
	writeFile(t, filepath.Join(".ai", "skills", "checkout-rules", "SKILL.md"), "# Checkout Rules\n")

	got := captureStdout(t, func() {
		cmd := &ListSkillsCmd{}
		if err := cmd.Run(); err != nil {
			t.Fatalf("run list skills: %v", err)
		}
	})
	if !strings.Contains(got, "Built-in skills:") || !strings.Contains(got, "Project skills:") || !strings.Contains(got, "checkout-rules") {
		t.Fatalf("unexpected list skills output:\n%s", got)
	}
}

func captureStdout(t *testing.T, run func()) string {
	t.Helper()
	previous := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = write
	t.Cleanup(func() {
		os.Stdout = previous
	})

	run()
	if err := write.Close(); err != nil {
		t.Fatalf("close stdout pipe: %v", err)
	}
	content, err := io.ReadAll(read)
	if err != nil {
		t.Fatalf("read stdout pipe: %v", err)
	}
	return string(content)
}

func withWorkingDir(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}
	if err := os.WriteFile(path, []byte(strings.TrimLeft(content, "\n")), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertFileContains(t *testing.T, path string, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(content), want) {
		t.Fatalf("%s missing %q:\n%s", path, want, string(content))
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsApp(values []atlasproject.App, want string) bool {
	for _, value := range values {
		if value.Name == want {
			return true
		}
	}
	return false
}

func resourceLinkByID(resources []workflows.ResourceLink, id string) (workflows.ResourceLink, bool) {
	for _, resource := range resources {
		if resource.ID == id {
			return resource, true
		}
	}
	return workflows.ResourceLink{}, false
}
