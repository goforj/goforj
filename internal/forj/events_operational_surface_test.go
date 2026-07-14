package forj

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
)

// TestRenderedEventsFreeProjectOmitsOperationalSurface verifies an all-App opt-out removes generated runtime, dependency, documentation, and dashboard artifacts together.
func TestRenderedEventsFreeProjectOmitsOperationalSurface(t *testing.T) {
	components := project.Components{
		CLI: true, WebAPI: true, Metrics: true, Docker: true, Observability: true, Grafana: true,
	}
	root := renderEventsContractProject(t, &project.Config{
		ProjectName:  "Events Free",
		GoModuleName: "example.com/events-free",
		Render:       project.RenderConfig{Components: components},
	})

	for _, path := range []string{
		"internal/events/event.go",
		"internal/observability/event_observer.go",
		"internal/makecmd/make_event_cmd.go",
		"app/event_commands.go",
		"containers/observability/grafana/dashboards/events-overview.json",
	} {
		if _, err := os.Stat(filepath.Join(root, path)); !os.IsNotExist(err) {
			t.Fatalf("Events-free render retained %s: %v", path, err)
		}
	}

	environment := readEventsContractFile(t, root, ".env")
	for _, token := range []string{"EVENTS_DRIVER=", "EVENTS_SUPPORTED_DRIVERS=", "METRICS_EVENTS_ENABLED="} {
		if strings.Contains(environment, token) {
			t.Fatalf("Events-free environment retained %q\n%s", token, environment)
		}
	}

	goMod := readEventsContractFile(t, root, "go.mod")
	if strings.Contains(goMod, "github.com/goforj/events") {
		t.Fatalf("Events-free dependency graph retained goforj/events\n%s", goMod)
	}
	manager := readEventsContractFile(t, root, "internal/metrics/manager.go")
	for _, token := range []string{"github.com/goforj/events", "RecordEventPublish", "EventPublishMetricEvent"} {
		if strings.Contains(manager, token) {
			t.Fatalf("Events-free metrics manager retained %q\n%s", token, manager)
		}
	}
	metricsReadme := readEventsContractFile(t, root, "internal/metrics/README.md")
	observabilityReadme := readEventsContractFile(t, root, "internal/observability/README.md")
	for path, body := range map[string]string{
		"internal/metrics/README.md":       metricsReadme,
		"internal/observability/README.md": observabilityReadme,
	} {
		for _, token := range []string{"Events Overview", "events_publishes_total", "METRICS_EVENTS_ENABLED"} {
			if strings.Contains(body, token) {
				t.Fatalf("Events-free documentation %s retained %q\n%s", path, token, body)
			}
		}
	}

	seed := readEventsContractFile(t, root, "containers/observability/grafana/seed-dashboards.sh")
	if strings.Contains(seed, "goforj-events-overview") {
		t.Fatalf("Events-free Grafana seed retained Events dashboard\n%s", seed)
	}
	platform := readEventsContractFile(t, root, "containers/observability/grafana/dashboards/platform-overview.json")
	var decoded any
	if err := json.Unmarshal([]byte(platform), &decoded); err != nil {
		t.Fatalf("Events-free Platform Overview is invalid JSON: %v\n%s", err, platform)
	}
	if strings.Contains(platform, "events_") {
		t.Fatalf("Events-free Platform Overview retained Events queries\n%s", platform)
	}
}

// renderEventsContractProject renders a full fixture under /tmp so file-selection contracts are exercised without touching the repository.
func renderEventsContractProject(t *testing.T, config *project.Config) string {
	t.Helper()
	root, err := os.MkdirTemp("/tmp", "goforj-events-contract-")
	if err != nil {
		t.Fatalf("create Events contract render root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change to Events contract render root: %v", err)
	}
	defer func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	}()

	if err := WriteYAML(".goforj.yml", *config); err != nil {
		t.Fatalf("write Events contract config: %v", err)
	}
	renderer := NewProjectRenderer(logger.NewSilentLogger())
	if err := renderer.Render(ComponentRenderInput{renderAll: true}); err != nil {
		t.Fatalf("render Events contract project: %v", err)
	}
	return root
}

// readEventsContractFile reads one generated artifact from an isolated Events render.
func readEventsContractFile(t *testing.T, root string, path string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatalf("read rendered %s: %v", path, err)
	}
	return string(content)
}

// TestEventsEnvironmentProjectsResourceContractAndAppOverrides keeps the shared driver contract while limiting overrides to participating Apps.
func TestEventsEnvironmentProjectsResourceContractAndAppOverrides(t *testing.T) {
	tests := []struct {
		name                 string
		config               *project.Config
		wantRootContract     bool
		wantSupportedDrivers bool
		wantNamedDriver      bool
	}{
		{
			name: "all Apps disable Events",
			config: &project.Config{Render: project.RenderConfig{Components: project.Components{
				CLI: true, WebAPI: true, Metrics: true,
			}}},
		},
		{
			name: "default App enables Events",
			config: &project.Config{Render: project.RenderConfig{Components: project.Components{
				CLI: true, WebAPI: true, Metrics: true, Events: true,
			}}},
			wantRootContract:     true,
			wantSupportedDrivers: true,
		},
		{
			name: "named App alone enables Events",
			config: &project.Config{
				Render: project.RenderConfig{Components: project.Components{CLI: true, WebAPI: true, Metrics: true}},
				Apps: map[string]project.AppConfig{
					"api":           {Components: project.Components{CLI: true, WebAPI: true}},
					"events-worker": {Components: project.Components{CLI: true, Events: true}},
				},
			},
			wantRootContract:     true,
			wantSupportedDrivers: true,
			wantNamedDriver:      true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := templateDataForApp(test.config, project.DefaultApp())
			data.Resources = resourceRenderValues{EventsDriver: "inproc", EventsSupportedDrivers: "inproc,redis"}
			environment := renderSharedTemplate(t, ".env.tmpl", data)

			if got := strings.Contains(environment, "\nEVENTS_DRIVER=inproc"); got != test.wantRootContract {
				t.Fatalf("root Events driver presence = %t, want %t\n%s", got, test.wantRootContract, environment)
			}
			if got := strings.Contains(environment, "\nEVENTS_SUPPORTED_DRIVERS=inproc,redis"); got != test.wantSupportedDrivers {
				t.Fatalf("project Events build contract presence = %t, want %t\n%s", got, test.wantSupportedDrivers, environment)
			}
			if got := strings.Contains(environment, "\nEVENTS_WORKER_EVENTS_DRIVER=inproc"); got != test.wantNamedDriver {
				t.Fatalf("named Events driver presence = %t, want %t\n%s", got, test.wantNamedDriver, environment)
			}
			if strings.Contains(environment, "\nAPI_EVENTS_DRIVER=") {
				t.Fatalf("Events-disabled named App received an Events driver\n%s", environment)
			}
			if got := strings.Contains(environment, "EVENTS_INPROC_WORKERS="); got != test.wantRootContract {
				t.Fatalf("root Events tuning presence = %t, want %t\n%s", got, test.wantRootContract, environment)
			}
		})
	}
}

// TestMetricsEnvironmentUsesTheProjectEnvelope keeps shared defaults available when only a named App enables Metrics and Events.
func TestMetricsEnvironmentUsesTheProjectEnvelope(t *testing.T) {
	tests := []struct {
		name             string
		workerComponents project.Components
		wantMetrics      bool
		wantEvents       bool
	}{
		{name: "all Apps disable Metrics and Events"},
		{name: "named App enables Metrics", workerComponents: project.Components{CLI: true, Metrics: true}, wantMetrics: true},
		{name: "named App enables Metrics and Events", workerComponents: project.Components{CLI: true, Metrics: true, Events: true}, wantMetrics: true, wantEvents: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := &project.Config{
				Render: project.RenderConfig{Components: project.Components{CLI: true}},
				Apps: map[string]project.AppConfig{
					"worker": {Components: test.workerComponents},
				},
			}
			environment := renderSharedTemplate(t, ".env.tmpl", templateDataForApp(config, project.DefaultApp()))

			if got := strings.Contains(environment, "\nMETRICS_PORT=10000"); got != test.wantMetrics {
				t.Fatalf("project Metrics defaults presence = %t, want %t\n%s", got, test.wantMetrics, environment)
			}
			if got := strings.Contains(environment, "\nMETRICS_EVENTS_ENABLED=true"); got != test.wantEvents {
				t.Fatalf("project Events metrics flag presence = %t, want %t\n%s", got, test.wantEvents, environment)
			}
		})
	}
}

// TestEventsOperationalTemplatesFollowTheProjectEnvelope verifies shared code survives a named-App-only Events selection without leaking into an Events-free project.
func TestEventsOperationalTemplatesFollowTheProjectEnvelope(t *testing.T) {
	tests := []struct {
		name       string
		config     *project.Config
		wantEvents bool
	}{
		{
			name: "all Apps disable Events",
			config: &project.Config{
				GoModuleName: "example.com/events-off",
				Render: project.RenderConfig{Components: project.Components{
					CLI: true, WebAPI: true, Metrics: true,
				}},
			},
		},
		{
			name: "named App enables Events",
			config: &project.Config{
				GoModuleName: "example.com/named-events",
				Render: project.RenderConfig{Components: project.Components{
					CLI: true, WebAPI: true, Metrics: true,
				}},
				Apps: map[string]project.AppConfig{
					"events-worker": {Components: project.Components{CLI: true, Events: true}},
				},
			},
			wantEvents: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := templateDataForApp(test.config, project.DefaultApp())
			manager := renderSharedTemplate(t, "internal/metrics/manager.go.tmpl", data)
			managerTest := renderSharedTemplate(t, "internal/metrics/manager_test.go.tmpl", data)
			assertFormattedGoTemplate(t, "internal/metrics/manager.go.tmpl", manager)
			assertFormattedGoTemplate(t, "internal/metrics/manager_test.go.tmpl", managerTest)

			for _, token := range []string{
				`github.com/goforj/events`,
				`func (m *Manager) EventsEnabled() bool`,
				`func (m *Manager) RecordEventPublish`,
				`METRICS_EVENTS_ENABLED`,
			} {
				if got := strings.Contains(manager, token); got != test.wantEvents {
					t.Fatalf("manager token %q presence = %t, want %t\n%s", token, got, test.wantEvents, manager)
				}
			}
			if got := strings.Contains(managerTest, `github.com/goforj/events`); got != test.wantEvents {
				t.Fatalf("manager test Events import presence = %t, want %t\n%s", got, test.wantEvents, managerTest)
			}
			if test.wantEvents {
				observer := renderSharedTemplate(t, "internal/observability/event_observer.go.tmpl", data)
				assertFormattedGoTemplate(t, "internal/observability/event_observer.go.tmpl", observer)
				if !strings.Contains(observer, `internal/metrics`) || !strings.Contains(observer, `metrics *metrics.Manager`) {
					t.Fatalf("project Metrics did not reach the shared Events observer\n%s", observer)
				}
				cacheObserver := renderSharedTemplate(t, "internal/observability/cache_observer.go.tmpl", data)
				storageObserver := renderSharedTemplate(t, "internal/observability/storage_observer.go.tmpl", data)
				assertFormattedGoTemplate(t, "internal/observability/cache_observer.go.tmpl", cacheObserver)
				assertFormattedGoTemplate(t, "internal/observability/storage_observer.go.tmpl", storageObserver)
				if !strings.Contains(cacheObserver, "func CacheMetricsObserver(") {
					t.Fatalf("project Metrics did not reach the shared Cache observer\n%s", cacheObserver)
				}
				if !strings.Contains(storageObserver, "metricsManager *metrics.Manager") {
					t.Fatalf("project Metrics did not reach the shared Storage observer\n%s", storageObserver)
				}
			}
		})
	}
}

// TestSharedPrimitiveObserversUseTheProjectMetricsEnvelope protects generated-once observers from default-App projection leaks.
func TestSharedPrimitiveObserversUseTheProjectMetricsEnvelope(t *testing.T) {
	data := templateRenderConfig{
		Config:            &project.Config{GoModuleName: "example.com/shared-observers"},
		Components:        project.Components{},
		ProjectComponents: project.Components{Metrics: true, Events: true},
	}
	tests := []struct {
		path   string
		marker string
	}{
		{path: "internal/observability/cache_observer.go.tmpl", marker: "func CacheMetricsObserver("},
		{path: "internal/observability/event_observer.go.tmpl", marker: "metrics *metrics.Manager"},
		{path: "internal/observability/storage_observer.go.tmpl", marker: "metricsManager *metrics.Manager"},
	}
	for _, test := range tests {
		source := renderSharedTemplate(t, test.path, data)
		assertFormattedGoTemplate(t, test.path, source)
		if !strings.Contains(source, test.marker) {
			t.Fatalf("shared observer %s omitted project Metrics marker %q\n%s", test.path, test.marker, source)
		}
	}
}

// TestEventManagerMetricsParticipationIsAppLocal verifies the shared observer remains usable when one Events App opts out of project Metrics.
func TestEventManagerMetricsParticipationIsAppLocal(t *testing.T) {
	for _, metricsEnabled := range []bool{false, true} {
		data := templateRenderConfig{
			Config:            &project.Config{GoModuleName: "example.com/event-metrics"},
			Components:        project.Components{Events: true, Metrics: metricsEnabled},
			ProjectComponents: project.Components{Events: true, Metrics: true},
		}
		wiring := renderSharedTemplate(t, "wire/inject_managers.go.tmpl", data)
		assertFormattedGoTemplate(t, "wire/inject_managers.go.tmpl", wiring)
		start := strings.Index(wiring, "func provideEventManager(")
		end := strings.Index(wiring, "// provideStorageManager")
		if start < 0 || end <= start {
			t.Fatalf("rendered wiring omitted the Events manager provider\n%s", wiring)
		}
		provider := wiring[start:end]
		if got := strings.Contains(provider, "metricsManager *metrics.Manager"); got != metricsEnabled {
			t.Fatalf("metricsEnabled=%t Events provider parameter presence=%t\n%s", metricsEnabled, got, provider)
		}
		if got := strings.Contains(provider, "(*metrics.Manager)(nil)"); got == metricsEnabled {
			t.Fatalf("metricsEnabled=%t typed nil presence=%t\n%s", metricsEnabled, got, provider)
		}
	}

	data := templateRenderConfig{
		Config:            &project.Config{GoModuleName: "example.com/event-metrics"},
		Components:        project.Components{Events: true},
		ProjectComponents: project.Components{Events: true, Metrics: true},
	}
	observer := renderSharedTemplate(t, "internal/observability/event_observer.go.tmpl", data)
	assertFormattedGoTemplate(t, "internal/observability/event_observer.go.tmpl", observer)
	if got := strings.Count(observer, "if o.metrics != nil {"); got != 5 {
		t.Fatalf("optional Events metric guard count = %d, want 5\n%s", got, observer)
	}
}

// TestEventsDocumentationFollowsTheProjectEnvelope keeps generated guidance aligned with the shared capability surface.
func TestEventsDocumentationFollowsTheProjectEnvelope(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		components := project.Components{CLI: true, WebAPI: true, Metrics: true, Observability: true, Grafana: true, Events: enabled}
		config := &project.Config{Render: project.RenderConfig{Components: components}}
		data := templateDataForApp(config, project.DefaultApp())
		metricsReadme := renderSharedTemplate(t, "internal/metrics/README.md.tmpl", data)
		observabilityReadme := renderSharedTemplate(t, "internal/observability/README.md.tmpl", data)
		inspectsReadme := renderSharedTemplate(t, "internal/inspects/README.md.tmpl", data)

		for _, token := range []string{"METRICS_EVENTS_ENABLED", "events_publishes_total", "### Events"} {
			if got := strings.Contains(metricsReadme, token); got != enabled {
				t.Fatalf("enabled=%t metrics README token %q presence=%t\n%s", enabled, token, got, metricsReadme)
			}
		}
		for _, token := range []string{"Events Overview", "`topic`, `handler`"} {
			if got := strings.Contains(observabilityReadme, token); got != enabled {
				t.Fatalf("enabled=%t observability README token %q presence=%t\n%s", enabled, token, got, observabilityReadme)
			}
		}
		for _, token := range []string{"- `event`", "- `RecordEventBusEvent`"} {
			if got := strings.Contains(inspectsReadme, token); got != enabled {
				t.Fatalf("enabled=%t Inspects README token %q presence=%t\n%s", enabled, token, got, inspectsReadme)
			}
		}
	}
}

// TestDemoMonitoringEventTopicsFollowTheProjectEnvelope keeps shared Events types free of demo-only declarations unless some App enables Demo.
func TestDemoMonitoringEventTopicsFollowTheProjectEnvelope(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		data := templateRenderConfig{
			Components:        project.Components{Events: true},
			ProjectComponents: project.Components{Events: true, DemoApp: enabled},
		}
		source := renderSharedTemplate(t, "internal/events/topics.go.tmpl", data)
		assertFormattedGoTemplate(t, "internal/events/topics.go.tmpl", source)

		for _, token := range []string{"MonitorPollingTopic", "type MonitorStatusChanged struct", "type MonitorDown struct", "type MonitorRecovered struct"} {
			if got := strings.Contains(source, token); got != enabled {
				t.Fatalf("enabled=%t demo monitoring Events token %q presence=%t\n%s", enabled, token, got, source)
			}
		}
		if enabled && strings.Count(source, "// Topic returns the event bus topic") != 4 {
			t.Fatalf("demo monitoring Topic doc comment count = %d, want 4\n%s", strings.Count(source, "// Topic returns the event bus topic"), source)
		}
	}
}

// TestGenericEventsExamplesDoNotDependOnDemo keeps the core transport test and documentation usable in non-Demo Events projects.
func TestGenericEventsExamplesDoNotDependOnDemo(t *testing.T) {
	data := templateRenderConfig{
		Config:            &project.Config{GoModuleName: "example.com/events-only"},
		Components:        project.Components{Events: true},
		ProjectComponents: project.Components{Events: true},
	}
	integrationTest := renderSharedTemplate(t, "internal/events/bus_integration_test.go.tmpl", data)
	assertFormattedGoTemplate(t, "internal/events/bus_integration_test.go.tmpl", integrationTest)
	readme := renderSharedTemplate(t, "internal/events/README.md.tmpl", data)

	for _, source := range []struct {
		path string
		body string
	}{
		{path: "internal/events/bus_integration_test.go.tmpl", body: integrationTest},
		{path: "internal/events/README.md.tmpl", body: readme},
	} {
		for _, token := range []string{"MonitorDown", "monitor.down", "MonitorID"} {
			if strings.Contains(source.body, token) {
				t.Fatalf("non-Demo Events source %s retained monitoring token %q\n%s", source.path, token, source.body)
			}
		}
	}
	for _, token := range []string{"type inprocIntegrationEvent struct", "func (inprocIntegrationEvent) Topic() string"} {
		if !strings.Contains(integrationTest, token) {
			t.Fatalf("Events integration test missing %q\n%s", token, integrationTest)
		}
	}
	for _, token := range []string{"type UserRegistered struct", "func (UserRegistered) Topic() string"} {
		if !strings.Contains(readme, token) {
			t.Fatalf("Events README missing %q\n%s", token, readme)
		}
	}
}

// TestPlatformDashboardEventsConditionalsRemainValidJSON covers Events-off output with Mail both enabled and disabled.
func TestPlatformDashboardEventsConditionalsRemainValidJSON(t *testing.T) {
	tests := []struct {
		name         string
		events       bool
		mail         bool
		wantHotspots string
	}{
		{name: "both off"},
		{name: "Events only", events: true, wantHotspots: "Event Hotspots"},
		{name: "Mail only", mail: true, wantHotspots: "Mail Hotspots"},
		{name: "Events and Mail", events: true, mail: true, wantHotspots: "Event And Mail Hotspots"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			components := project.Components{Events: test.events, Mail: test.mail}
			config := &project.Config{Render: project.RenderConfig{Components: components}}
			body := renderSharedTemplate(t, "containers/observability/grafana/dashboards/platform-overview.json.tmpl", templateDataForApp(config, project.DefaultApp()))
			var decoded any
			if err := json.Unmarshal([]byte(body), &decoded); err != nil {
				t.Fatalf("rendered Platform Overview is invalid JSON: %v\n%s", err, body)
			}
			if got := strings.Contains(body, "events_"); got != test.events {
				t.Fatalf("Events metrics presence = %t, want %t\n%s", got, test.events, body)
			}
			if test.wantHotspots == "" {
				if strings.Contains(body, "Event And Mail Hotspots") || strings.Contains(body, "Event Hotspots") || strings.Contains(body, "Mail Hotspots") {
					t.Fatalf("primitive hotspot panel survived both components being disabled\n%s", body)
				}
				return
			}
			if !strings.Contains(body, test.wantHotspots) {
				t.Fatalf("dashboard missing %q\n%s", test.wantHotspots, body)
			}
		})
	}
}

// TestEventsDashboardQueriesMatchGeneratedMetricNames keeps Prometheus queries aligned with metric descriptor types and names.
func TestEventsDashboardQueriesMatchGeneratedMetricNames(t *testing.T) {
	components := project.Components{Events: true, Metrics: true, Observability: true, Grafana: true}
	config := &project.Config{GoModuleName: "example.com/events-dashboard", Render: project.RenderConfig{Components: components}}
	data := templateDataForApp(config, project.DefaultApp())
	manager := renderSharedTemplate(t, "internal/metrics/manager.go.tmpl", data)
	platform := renderSharedTemplate(t, "containers/observability/grafana/dashboards/platform-overview.json.tmpl", data)
	events := renderSharedTemplate(t, "containers/observability/grafana/dashboards/events-overview.json.tmpl", data)

	for path, body := range map[string]string{
		"containers/observability/grafana/dashboards/platform-overview.json.tmpl": platform,
		"containers/observability/grafana/dashboards/events-overview.json.tmpl":   events,
	} {
		var decoded any
		if err := json.Unmarshal([]byte(body), &decoded); err != nil {
			t.Fatalf("rendered dashboard %s is invalid JSON: %v\n%s", path, err, body)
		}
	}

	contracts := []struct {
		name             string
		descriptorMarker string
		queryMetric      string
		dashboard        string
	}{
		{name: "delivery inflight gauge", descriptorMarker: `Name: "events.deliveries.inflight"`, queryMetric: "events_deliveries_inflight", dashboard: platform},
		{name: "subscriptions counter", descriptorMarker: `Name: "events.subscriptions"`, queryMetric: "events_subscriptions_total", dashboard: events},
	}
	for _, contract := range contracts {
		if !strings.Contains(manager, contract.descriptorMarker) {
			t.Fatalf("metrics manager missing %s descriptor %q\n%s", contract.name, contract.descriptorMarker, manager)
		}
		if !strings.Contains(contract.dashboard, contract.queryMetric) {
			t.Fatalf("%s dashboard query missing generated metric %q\n%s", contract.name, contract.queryMetric, contract.dashboard)
		}
	}
	for _, stale := range []struct {
		metric    string
		dashboard string
	}{
		{metric: "events_delivery_inflight{", dashboard: platform},
		{metric: "events_subscriptions{", dashboard: events},
	} {
		if strings.Contains(stale.dashboard, stale.metric) {
			t.Fatalf("dashboard retained stale Events metric %q\n%s", stale.metric, stale.dashboard)
		}
	}
}

// TestGrafanaSeedIncludesEventsDashboardOnlyForAnEventsProject keeps the seeder in lockstep with rendered dashboards.
func TestGrafanaSeedIncludesEventsDashboardOnlyForAnEventsProject(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		components := project.Components{Events: enabled}
		config := &project.Config{Render: project.RenderConfig{Components: components}}
		seed := renderSharedTemplate(t, "containers/observability/grafana/seed-dashboards.sh.tmpl", templateDataForApp(config, project.DefaultApp()))
		if got := strings.Contains(seed, "goforj-events-overview"); got != enabled {
			t.Fatalf("enabled=%t Events dashboard seed presence=%t\n%s", enabled, got, seed)
		}
	}
}
