package forj

import (
	"bytes"
	"encoding/json"
	"go/format"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"github.com/goforj/goforj/project"
)

// primitiveTemplateMarker describes one token whose source shape follows either App or project participation.
type primitiveTemplateMarker struct {
	path  string
	token string
}

// primitiveTemplateContract captures the minimal generated surface that distinguishes one optional primitive.
type primitiveTemplateContract struct {
	name             string
	appMarkers       []primitiveTemplateMarker
	projectMarkers   []primitiveTemplateMarker
	disabledBridge   primitiveTemplateMarker
	rootEnvironment  string
	namedEnvironment string
	dashboardMarker  string
}

// TestPrimitiveTemplateProjection covers all-off and both mixed-App directions through one shared projection matrix.
func TestPrimitiveTemplateProjection(t *testing.T) {
	scenarios := []struct {
		name           string
		defaultEnabled bool
		workerEnabled  bool
	}{
		{name: "all Apps disabled"},
		{name: "default App only", defaultEnabled: true},
		{name: "named App only", workerEnabled: true},
	}

	for _, contract := range primitiveTemplateContracts() {
		contract := contract
		t.Run(contract.name, func(t *testing.T) {
			for _, scenario := range scenarios {
				scenario := scenario
				t.Run(scenario.name, func(t *testing.T) {
					config := primitiveProjectionConfig(contract.name, scenario.defaultEnabled, scenario.workerEnabled)
					projectEnabled := scenario.defaultEnabled || scenario.workerEnabled
					sharedData := templateDataForApp(config, project.DefaultApp())
					sharedData.Resources = primitiveProjectionResources()

					for _, marker := range contract.projectMarkers {
						assertProjectedTemplateMarker(t, marker, sharedData, projectEnabled)
					}
					environment := renderSharedTemplate(t, ".env.tmpl", sharedData)
					assertTemplateMarker(t, ".env.tmpl", environment, contract.rootEnvironment, projectEnabled)
					assertTemplateMarker(t, ".env.tmpl", environment, contract.namedEnvironment, scenario.workerEnabled)

					renderer := &ProjectRenderer{config: config}
					apps := []struct {
						app     project.App
						enabled bool
					}{
						{app: project.DefaultApp(), enabled: scenario.defaultEnabled},
						{app: project.DefaultNamedApp("worker"), enabled: scenario.workerEnabled},
					}
					for _, target := range apps {
						target := target
						t.Run(target.app.Name, func(t *testing.T) {
							data := templateDataForApp(config, target.app)
							data.Resources = primitiveProjectionResources()
							for _, marker := range contract.appMarkers {
								assertProjectedTemplateMarker(t, marker, data, target.enabled)
							}
							if contract.disabledBridge.path != "" {
								bridgeEnabled := projectEnabled && !target.enabled
								assertProjectedTemplateMarker(t, contract.disabledBridge, data, bridgeEnabled)
							}
							assertPrimitiveAppMappings(t, renderer, contract.name, target.app, target.enabled)
						})
					}
				})
			}
		})
	}

	t.Run("Events owner compatibility", testEventCommandProjection)
	t.Run("Events shared examples", testEventSharedExamples)
	t.Run("dashboard conditionals", testPrimitiveDashboardProjection)
}

// primitiveTemplateContracts returns the high-signal markers that are not already exercised by real render sentinels.
func primitiveTemplateContracts() []primitiveTemplateContract {
	return []primitiveTemplateContract{
		{
			name: "Events",
			appMarkers: []primitiveTemplateMarker{
				{path: "wire/app.go.tmpl", token: "func (a *App) Events() *events.Manager"},
				{path: "app/root_cmd.go.tmpl", token: "GeneratedEventCommands"},
				{path: "wire/inject_managers.go.tmpl", token: "func provideEventManager("},
				{path: "wire/inject_http.go.tmpl", token: "for _, check := range events.ReadinessChecks()"},
			},
			projectMarkers: []primitiveTemplateMarker{
				{path: "internal/runtime/about.go.tmpl", token: "report.Events = aboutEventReports()"},
				{path: "internal/runtime/discovery.go.tmpl", token: "func DiscoverEventInstances("},
				{path: "internal/metrics/manager.go.tmpl", token: "func (m *Manager) RecordEventPublish"},
				{path: "internal/metrics/manager.go.tmpl", token: `runtime.CurrentApp().Components.Events && env.GetBool("METRICS_EVENTS_ENABLED", "true")`},
				{path: "internal/metrics/manager_test.go.tmpl", token: "NewManagerWithConfig(Config{Events: true})"},
				{path: "containers/observability/grafana/seed-dashboards.sh.tmpl", token: "goforj-events-overview"},
				{path: "internal/makecmd/README.md.tmpl", token: "make:event"},
			},
			rootEnvironment:  "\nEVENTS_DRIVER=inproc",
			namedEnvironment: "\nWORKER_EVENTS_DRIVER=inproc",
			dashboardMarker:  "events_",
		},
		{
			name: "Storage",
			appMarkers: []primitiveTemplateMarker{
				{path: "wire/app.go.tmpl", token: "func (a *App) Storage() *storages.Manager"},
				{path: "wire/inject_managers.go.tmpl", token: "func provideStorageManager("},
				{path: "wire/inject_http.go.tmpl", token: "for _, check := range storage.ReadinessChecks()"},
			},
			projectMarkers: []primitiveTemplateMarker{
				{path: "internal/runtime/about.go.tmpl", token: "report.Storages = aboutStorageReports()"},
				{path: "internal/runtime/discovery.go.tmpl", token: "func DiscoverStorageInstances("},
				{path: "internal/metrics/manager.go.tmpl", token: "func (m *Manager) RecordStorageOperation"},
				{path: "internal/metrics/manager_test.go.tmpl", token: "NewManagerWithConfig(Config{Storage: true})"},
				{path: "containers/observability/grafana/seed-dashboards.sh.tmpl", token: "goforj-storage-overview"},
				{path: "internal/observability/README.md.tmpl", token: "Storage Overview"},
			},
			disabledBridge:   primitiveTemplateMarker{path: "wire/inject_managers.go.tmpl", token: "func provideDisabledStorageManager("},
			rootEnvironment:  "\nSTORAGE_DRIVER=local",
			namedEnvironment: "\nWORKER_STORAGE_DRIVER=local",
			dashboardMarker:  "storage_",
		},
		{
			name: "Jobs",
			appMarkers: []primitiveTemplateMarker{
				{path: "wire/app.go.tmpl", token: "func (a *App) Queues() *queues.Manager"},
				{path: "wire/inject_cmd.go.tmpl", token: "jobsRuntime *jobs.Runtime"},
				{path: "wire/inject_managers.go.tmpl", token: "func provideQueueManager("},
				{path: "wire/inject_http.go.tmpl", token: "for _, check := range queues.ReadinessChecks()"},
			},
			projectMarkers: []primitiveTemplateMarker{
				{path: "internal/cmd/run_cmd.go.tmpl", token: "jobsRuntime *jobs.Runtime"},
				{path: "internal/runtime/about.go.tmpl", token: "report.Queues = aboutQueueReports()"},
				{path: "internal/metrics/manager.go.tmpl", token: "func (m *Manager) RecordQueueEvent"},
				{path: "containers/observability/grafana/seed-dashboards.sh.tmpl", token: "goforj-queue-overview"},
				{path: "internal/makecmd/README.md.tmpl", token: "make:job"},
			},
			disabledBridge:   primitiveTemplateMarker{path: "wire/inject_cmd.go.tmpl", token: "(*jobs.Runtime)(nil)"},
			rootEnvironment:  "\nQUEUE_DRIVER=workerpool",
			namedEnvironment: "\nWORKER_QUEUE_DRIVER=workerpool",
			dashboardMarker:  "queue_jobs_",
		},
	}
}

// primitiveProjectionConfig creates two Apps whose primitive participation can vary independently.
func primitiveProjectionConfig(name string, defaultEnabled bool, workerEnabled bool) *project.Config {
	defaultComponents := primitiveProjectionBaseComponents()
	workerComponents := primitiveProjectionBaseComponents()
	setPrimitiveComponent(&defaultComponents, name, defaultEnabled)
	setPrimitiveComponent(&workerComponents, name, workerEnabled)
	return &project.Config{
		GoModuleName: "example.com/primitive-projection",
		Render: project.RenderConfig{
			Components:               defaultComponents,
			ComponentContractVersion: project.CurrentComponentContractVersion,
		},
		Apps: map[string]project.AppConfig{
			"worker": {Components: workerComponents},
		},
	}
}

// primitiveProjectionBaseComponents keeps unrelated template dependencies stable while a primitive varies.
func primitiveProjectionBaseComponents() project.Components {
	return project.Components{
		CLI: true, WebAPI: true, Cache: true, Metrics: true, Observability: true, Grafana: true,
	}
}

// primitiveProjectionResources supplies deterministic environment values without invoking resource reconciliation.
func primitiveProjectionResources() resourceRenderValues {
	return resourceRenderValues{
		CacheDriver:             "memory",
		CacheSupportedDrivers:   "memory",
		EventsDriver:            "inproc",
		EventsSupportedDrivers:  "inproc,redis",
		StorageDriver:           "local",
		StorageSupportedDrivers: "local,memory",
		StoragePublicDriver:     "local",
		QueueDriver:             "workerpool",
		QueueSupportedDrivers:   "workerpool,redis",
	}
}

// setPrimitiveComponent changes one optional primitive without widening unrelated component state.
func setPrimitiveComponent(components *project.Components, name string, enabled bool) {
	switch name {
	case "Events":
		components.Events = enabled
	case "Storage":
		components.Storage = enabled
	case "Jobs":
		components.Jobs = enabled
	}
}

// primitiveComponentEnabled reports one optional primitive from a component projection.
func primitiveComponentEnabled(components project.Components, name string) bool {
	switch name {
	case "Events":
		return components.Events
	case "Storage":
		return components.Storage
	case "Jobs":
		return components.Jobs
	default:
		return false
	}
}

// assertProjectedTemplateMarker renders one template, checks Go syntax when applicable, and verifies its projection token.
func assertProjectedTemplateMarker(t *testing.T, marker primitiveTemplateMarker, data templateRenderConfig, want bool) {
	t.Helper()
	if marker.path == "" {
		return
	}
	source := renderSharedTemplate(t, marker.path, data)
	if strings.HasSuffix(marker.path, ".go.tmpl") {
		assertFormattedGoTemplate(t, marker.path, source)
	}
	assertTemplateMarker(t, marker.path, source, marker.token, want)
}

// assertPrimitiveAppMappings verifies component-specific framework and owner files stay App-local.
func assertPrimitiveAppMappings(t *testing.T, renderer *ProjectRenderer, name string, app project.App, want bool) {
	t.Helper()
	var frameworkPath string
	var ownerPath string
	switch name {
	case "Events":
		frameworkPath = filepath.Join(app.AppDir, "event_commands.go")
		ownerPath = filepath.Join(app.WireDir, "inject_subscribers_app.go")
	case "Jobs":
		frameworkPath = filepath.Join(app.WireDir, "inject_jobs.go")
		ownerPath = filepath.Join(app.WireDir, "inject_jobs_app.go")
	default:
		return
	}
	if got := templateMappingDestExists(renderer.appFrameworkMappings(app), frameworkPath); got != want {
		t.Fatalf("%s framework mapping %s presence = %t, want %t", name, frameworkPath, got, want)
	}
	if got := templateMappingDestExists(renderer.appOwnedMappings(app), ownerPath); got != want {
		t.Fatalf("%s owner mapping %s presence = %t, want %t", name, ownerPath, got, want)
	}
}

// templateMappingDestExists reports whether a mapping set contains one normalized destination.
func templateMappingDestExists(mappings []templateMapping, want string) bool {
	want = filepath.Clean(want)
	for _, mapping := range mappings {
		if filepath.Clean(mapping.dest) == want {
			return true
		}
	}
	return false
}

// testEventCommandProjection preserves legacy command owners while keeping future owner templates Events-neutral.
func testEventCommandProjection(t *testing.T) {
	config := primitiveProjectionConfig("Events", true, false)
	base := templateDataForApp(config, project.DefaultApp())
	tests := []struct {
		name                  string
		legacyField           bool
		legacyProvider        bool
		wantGeneratedPipeline bool
		wantBackfillProvider  bool
	}{
		{name: "new App", wantGeneratedPipeline: true},
		{name: "legacy field and provider", legacyField: true, legacyProvider: true},
		{name: "legacy field without provider", legacyField: true, wantBackfillProvider: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := base
			data.LegacyEventPipelineField = test.legacyField
			data.LegacyEventPipelineProvider = test.legacyProvider
			commands := renderSharedTemplate(t, "app/event_commands.go.tmpl", data)
			wiring := renderSharedTemplate(t, "wire/inject_cmd.go.tmpl", data)
			assertFormattedGoTemplate(t, "app/event_commands.go.tmpl", commands)
			assertFormattedGoTemplate(t, "wire/inject_cmd.go.tmpl", wiring)
			assertTemplateMarker(t, "app/event_commands.go.tmpl", commands, "TestEventPipelineCmd", test.wantGeneratedPipeline)
			assertTemplateMarker(t, "wire/inject_cmd.go.tmpl", wiring, "cmd.NewTestEventPipelineCmd,", test.wantBackfillProvider)
		})
	}

	for _, path := range []string{"app/commands.go.tmpl", "wire/inject_cmd_app.go.tmpl", "wire/inject_services_app.go.tmpl"} {
		content, err := templatesFS.ReadFile(path)
		if err != nil {
			t.Fatalf("read owner template %s: %v", path, err)
		}
		for _, token := range []string{"MakeEventCmd", "MakeSubscriberCmd", "TestEventPipeline", "NewEventCmd", "NewSubscriberCmd"} {
			assertTemplateMarker(t, path, string(content), token, false)
		}
	}
}

// testEventSharedExamples keeps demo declarations out of the generic Events transport and examples.
func testEventSharedExamples(t *testing.T) {
	for _, demoEnabled := range []bool{false, true} {
		data := templateRenderConfig{
			Components:        project.Components{Events: true},
			ProjectComponents: project.Components{Events: true, DemoApp: demoEnabled},
		}
		topics := renderSharedTemplate(t, "internal/events/topics.go.tmpl", data)
		assertFormattedGoTemplate(t, "internal/events/topics.go.tmpl", topics)
		assertTemplateMarker(t, "internal/events/topics.go.tmpl", topics, "type MonitorDown struct", demoEnabled)
	}

	data := templateRenderConfig{
		Config:            &project.Config{GoModuleName: "example.com/events-only"},
		Components:        project.Components{Events: true},
		ProjectComponents: project.Components{Events: true},
	}
	integration := renderSharedTemplate(t, "internal/events/bus_integration_test.go.tmpl", data)
	assertFormattedGoTemplate(t, "internal/events/bus_integration_test.go.tmpl", integration)
	assertTemplateMarker(t, "internal/events/bus_integration_test.go.tmpl", integration, "type inprocIntegrationEvent struct", true)
	assertTemplateMarker(t, "internal/events/bus_integration_test.go.tmpl", integration, "MonitorDown", false)
}

// testPrimitiveDashboardProjection verifies optional dashboard fragments remain valid JSON in both component states.
func testPrimitiveDashboardProjection(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		components := primitiveProjectionBaseComponents()
		components.Cache = enabled
		config := &project.Config{Render: project.RenderConfig{Components: components}}
		body := renderSharedTemplate(t, "containers/observability/grafana/dashboards/platform-overview.json.tmpl", templateDataForApp(config, project.DefaultApp()))
		var decoded any
		if err := json.Unmarshal([]byte(body), &decoded); err != nil {
			t.Fatalf("Cache enabled=%t Platform Overview is invalid JSON: %v\n%s", enabled, err, body)
		}
		assertTemplateMarker(t, "platform-overview.json.tmpl", body, "cache_operations_total", enabled)
		assertTemplateMarker(t, "platform-overview.json.tmpl", body, "cache read p95", enabled)

		seed := renderSharedTemplate(t, "containers/observability/grafana/seed-dashboards.sh.tmpl", templateDataForApp(config, project.DefaultApp()))
		assertTemplateMarker(t, "seed-dashboards.sh.tmpl", seed, "goforj-cache-overview", enabled)

		readme := renderSharedTemplate(t, "internal/observability/README.md.tmpl", templateDataForApp(config, project.DefaultApp()))
		assertTemplateMarker(t, "internal/observability/README.md.tmpl", readme, "Cache Overview", enabled)
		assertTemplateMarker(t, "internal/observability/README.md.tmpl", readme, "named cache", enabled)
		assertTemplateMarker(t, "internal/observability/README.md.tmpl", readme, "`cache`: which named cache handled the work", enabled)
	}

	for _, contract := range primitiveTemplateContracts() {
		for _, enabled := range []bool{false, true} {
			components := primitiveProjectionBaseComponents()
			setPrimitiveComponent(&components, contract.name, enabled)
			config := &project.Config{Render: project.RenderConfig{Components: components}}
			body := renderSharedTemplate(t, "containers/observability/grafana/dashboards/platform-overview.json.tmpl", templateDataForApp(config, project.DefaultApp()))
			var decoded any
			if err := json.Unmarshal([]byte(body), &decoded); err != nil {
				t.Fatalf("%s enabled=%t Platform Overview is invalid JSON: %v\n%s", contract.name, enabled, err, body)
			}
			if contract.dashboardMarker != "" {
				assertTemplateMarker(t, "platform-overview.json.tmpl", body, contract.dashboardMarker, enabled)
			}
		}
	}
}

// appTemplateDataForProjectionTest builds the established per-App render projection without invoking a project render.
func appTemplateDataForProjectionTest(config *project.Config, app project.App, components project.Components) templateRenderConfig {
	data := templateDataForApp(config, app)
	data.Components = components
	return data
}

// renderSharedTemplate executes one embedded template against explicit projection data without writing a rendered project.
func renderSharedTemplate(t *testing.T, path string, data templateRenderConfig) string {
	t.Helper()
	content, err := templatesFS.ReadFile(path)
	if err != nil {
		t.Fatalf("read template %s: %v", path, err)
	}
	tmpl, err := template.New(path).Parse(string(content))
	if err != nil {
		t.Fatalf("parse template %s: %v", path, err)
	}
	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, data); err != nil {
		t.Fatalf("execute template %s: %v", path, err)
	}
	return rendered.String()
}

// assertFormattedGoTemplate verifies a rendered Go template is syntactically valid and gofmt-compatible.
func assertFormattedGoTemplate(t *testing.T, path string, source string) {
	t.Helper()
	if _, err := format.Source([]byte(source)); err != nil {
		t.Fatalf("format rendered template %s: %v\n%s", path, err, source)
	}
}

// assertTemplateMarker verifies one rendered marker is present exactly when expected.
func assertTemplateMarker(t *testing.T, path string, source string, marker string, want bool) {
	t.Helper()
	if got := strings.Contains(source, marker); got != want {
		t.Fatalf("template %s marker %q presence = %t, want %t\n%s", path, marker, got, want, source)
	}
}
