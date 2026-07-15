package forj

import (
	"bytes"
	"go/format"
	"strings"
	"testing"
	"text/template"

	"github.com/goforj/goforj/project"
)

// TestSharedReadinessUsesProjectCapabilitiesAndAppParticipation verifies a named App can widen the shared signature without wiring those dependencies into the default App.
func TestSharedReadinessUsesProjectCapabilitiesAndAppParticipation(t *testing.T) {
	config := mixedAppSharedTemplateConfig()
	projectComponents := project.ProjectComponents(config)

	shared := renderSharedTemplate(t, "internal/http/readiness_checks.go.tmpl", templateRenderConfig{
		Config:            config,
		Components:        config.Render.Components,
		ProjectComponents: projectComponents,
	})
	for _, want := range []string{
		`"example.com/mixed/internal/database"`,
		`"example.com/mixed/internal/queues"`,
		`queues *queues.Manager`,
		`db *database.Connections`,
	} {
		if !strings.Contains(shared, want) {
			t.Fatalf("shared readiness source missing %q:\n%s", want, shared)
		}
	}

	defaultWire := renderSharedTemplate(t, "wire/inject_http.go.tmpl", appTemplateDataForProjectionTest(config, project.DefaultApp(), config.Render.Components))
	for _, want := range []string{
		`(*queues.Manager)(nil)`,
		`(*database.Connections)(nil)`,
	} {
		if !strings.Contains(defaultWire, want) {
			t.Fatalf("default App readiness wiring missing %q:\n%s", want, defaultWire)
		}
	}
	if strings.Contains(defaultWire, "\tqueues *queues.Manager") || strings.Contains(defaultWire, "\tdb *database.Connections") {
		t.Fatalf("default App unexpectedly requested named-App readiness dependencies:\n%s", defaultWire)
	}

	worker := project.DefaultNamedApp("worker")
	workerComponents := project.NormalizeConfiguredAppComponents(config, config.Apps[worker.Name].Components)
	workerWire := renderSharedTemplate(t, "wire/inject_http.go.tmpl", appTemplateDataForProjectionTest(config, worker, workerComponents))
	for _, want := range []string{
		`queues *queues.Manager`,
		`db *database.Connections`,
		"\t\tqueues,",
		"\t\tdb,",
	} {
		if !strings.Contains(workerWire, want) {
			t.Fatalf("worker App readiness wiring missing %q:\n%s", want, workerWire)
		}
	}
	if strings.Contains(workerWire, `(*queues.Manager)(nil)`) || strings.Contains(workerWire, `(*database.Connections)(nil)`) {
		t.Fatalf("worker App unexpectedly received nil readiness dependencies:\n%s", workerWire)
	}
}

// TestSharedRunCommandUsesProjectCapabilitiesAndAppParticipation verifies named-App runtimes widen the shared constructor while each App wires only its own runtimes.
func TestSharedRunCommandUsesProjectCapabilitiesAndAppParticipation(t *testing.T) {
	config := mixedAppSharedTemplateConfig()
	projectComponents := project.ProjectComponents(config)
	shared := renderSharedTemplate(t, "internal/cmd/run_cmd.go.tmpl", templateRenderConfig{
		Config:            config,
		Components:        config.Render.Components,
		ProjectComponents: projectComponents,
	})
	assertFormattedGoTemplate(t, "internal/cmd/run_cmd.go.tmpl", shared)
	for _, want := range []string{"schedulerRuntime *schedules.Runtime", "jobsRuntime *jobs.Runtime"} {
		if !strings.Contains(shared, want) {
			t.Fatalf("shared run command missing %q:\n%s", want, shared)
		}
	}
	observer := renderSharedTemplate(t, "internal/observability/queue_observer.go.tmpl", templateRenderConfig{
		Config:            config,
		Components:        config.Render.Components,
		ProjectComponents: projectComponents,
	})
	assertFormattedGoTemplate(t, "internal/observability/queue_observer.go.tmpl", observer)
	if !strings.Contains(observer, "func QueueObserver(") {
		t.Fatalf("shared queue observer omitted named-App Jobs support:\n%s", observer)
	}
	workerSource := renderSharedTemplate(t, "internal/jobs/worker.go.tmpl", templateRenderConfig{
		Config:            config,
		Components:        config.Render.Components,
		ProjectComponents: projectComponents,
	})
	assertFormattedGoTemplate(t, "internal/jobs/worker.go.tmpl", workerSource)
	if !strings.Contains(workerSource, "queues *queues.Manager") {
		t.Fatalf("shared job worker omitted named-App queue support:\n%s", workerSource)
	}

	defaultWire := renderSharedTemplate(t, "wire/inject_cmd.go.tmpl", appTemplateDataForProjectionTest(config, project.DefaultApp(), config.Render.Components))
	for _, want := range []string{"(*schedules.Runtime)(nil)", "(*jobs.Runtime)(nil)"} {
		if !strings.Contains(defaultWire, want) {
			t.Fatalf("default App run wiring missing %q:\n%s", want, defaultWire)
		}
	}

	worker := project.DefaultNamedApp("worker")
	workerComponents := project.NormalizeConfiguredAppComponents(config, config.Apps[worker.Name].Components)
	workerWire := renderSharedTemplate(t, "wire/inject_cmd.go.tmpl", appTemplateDataForProjectionTest(config, worker, workerComponents))
	for _, want := range []string{"schedulerRuntime *schedules.Runtime", "jobsRuntime *jobs.Runtime", "\t\tschedulerRuntime,", "\t\tjobsRuntime,"} {
		if !strings.Contains(workerWire, want) {
			t.Fatalf("worker App run wiring missing %q:\n%s", want, workerWire)
		}
	}
}

// TestSharedMetricsUsesProjectCapabilitiesAndAppParticipation verifies a named App can enable Metrics without changing the default App's runtime wiring.
func TestSharedMetricsUsesProjectCapabilitiesAndAppParticipation(t *testing.T) {
	config := &project.Config{
		GoModuleName: "example.com/mixed-metrics",
		Render: project.RenderConfig{Components: project.Components{
			CLI: true, WebAPI: true, Cache: true, Storage: true,
		}},
		Apps: map[string]project.AppConfig{
			"worker": {Components: project.Components{
				CLI: true, WebAPI: true, Cache: true, Metrics: true,
			}},
		},
	}
	projectComponents := project.ProjectComponents(config)
	sharedData := templateRenderConfig{
		Config:            config,
		Components:        config.Render.Components,
		ProjectComponents: projectComponents,
	}
	sharedPaths := []string{
		"internal/http/server.go.tmpl",
		"internal/http/runtime.go.tmpl",
		"internal/http/lighthouse.go.tmpl",
		"internal/http/serve_cmd.go.tmpl",
		"internal/http/health_test.go.tmpl",
		"internal/http/swagger_test.go.tmpl",
		"internal/http/inspect_child_event_test.go.tmpl",
		"internal/http/inspects_bench_test.go.tmpl",
		"internal/http/runtime_bench_test.go.tmpl",
		"internal/lighthouse/inspects.go.tmpl",
		"internal/lighthouse/inspects_test.go.tmpl",
	}
	for _, path := range sharedPaths {
		source := renderSharedTemplate(t, path, sharedData)
		assertFormattedGoTemplate(t, path, source)
	}
	for _, appMetrics := range []bool{false, true} {
		data := sharedData
		data.Components = project.Components{CLI: true, Cache: true, Jobs: true, Scheduler: true, Metrics: appMetrics}
		for _, path := range []string{"internal/jobs/lighthouse.go.tmpl", "internal/schedules/lighthouse.go.tmpl"} {
			source := renderSharedTemplate(t, path, data)
			assertFormattedGoTemplate(t, path, source)
		}
	}

	server := renderSharedTemplate(t, "internal/http/server.go.tmpl", sharedData)
	for _, want := range []string{
		`"example.com/mixed-metrics/internal/metrics"`,
		"metrics         *metrics.Manager",
		"metricsManager *metrics.Manager",
		"groups = append(groups, s.metricsRoutes()...)",
	} {
		if !strings.Contains(server, want) {
			t.Fatalf("shared HTTP server missing project Metrics shape %q:\n%s", want, server)
		}
	}

	defaultWiring := renderSharedTemplate(t, "wire/inject_managers.go.tmpl", appTemplateDataForProjectionTest(config, project.DefaultApp(), config.Render.Components))
	assertFormattedGoTemplate(t, "wire/inject_managers.go.tmpl (default App)", defaultWiring)
	for _, want := range []string{"provideDisabledMetricsManager", "func provideCacheManager("} {
		if !strings.Contains(defaultWiring, want) {
			t.Fatalf("default App manager wiring missing %q:\n%s", want, defaultWiring)
		}
	}
	if strings.Contains(defaultWiring, "\tmetrics.NewManager,") || strings.Contains(defaultWiring, "metricsManager.CacheEnabled()") {
		t.Fatalf("default App unexpectedly enabled or dereferenced Metrics:\n%s", defaultWiring)
	}

	worker := project.DefaultNamedApp("worker")
	workerComponents := project.NormalizeConfiguredAppComponents(config, config.Apps[worker.Name].Components)
	workerWiring := renderSharedTemplate(t, "wire/inject_managers.go.tmpl", appTemplateDataForProjectionTest(config, worker, workerComponents))
	assertFormattedGoTemplate(t, "wire/inject_managers.go.tmpl (worker App)", workerWiring)
	for _, want := range []string{"metrics.NewManager", "metricsManager *metrics.Manager", "metricsManager.CacheEnabled()"} {
		if !strings.Contains(workerWiring, want) {
			t.Fatalf("Metrics-enabled worker wiring missing %q:\n%s", want, workerWiring)
		}
	}
	if strings.Contains(workerWiring, "provideDisabledMetricsManager") {
		t.Fatalf("Metrics-enabled worker unexpectedly received the disabled provider:\n%s", workerWiring)
	}
}

// TestSharedMailMetricsUsesProjectEnvelopeAndAppParticipation verifies shared Mail metrics retain project support while each App wires only its own Metrics manager.
func TestSharedMailMetricsUsesProjectEnvelopeAndAppParticipation(t *testing.T) {
	namedMailMetricsConfig := &project.Config{
		GoModuleName: "example.com/mixed-mail-metrics",
		Render: project.RenderConfig{Components: project.Components{
			CLI: true,
		}},
		Apps: map[string]project.AppConfig{
			"worker": {Components: project.Components{
				CLI: true, Auth: true, Metrics: true,
			}},
		},
	}
	projectComponents := project.ProjectComponents(namedMailMetricsConfig)
	if namedMailMetricsConfig.Render.Components.Mail || !projectComponents.Mail || !projectComponents.Metrics {
		t.Fatalf("named Auth and Metrics did not produce the expected project envelope: default=%+v project=%+v", namedMailMetricsConfig.Render.Components, projectComponents)
	}
	sharedData := templateRenderConfig{
		Config:            namedMailMetricsConfig,
		Components:        namedMailMetricsConfig.Render.Components,
		ProjectComponents: projectComponents,
	}

	observer := renderSharedTemplate(t, "internal/observability/mail_observer.go.tmpl", sharedData)
	assertFormattedGoTemplate(t, "internal/observability/mail_observer.go.tmpl", observer)
	for _, want := range []string{
		"metricsManager *metrics.Manager",
		"if metricsManager != nil {",
		"metricsManager.RecordMailSend(ctx, metrics.MailSendMetricEvent{",
	} {
		if !strings.Contains(observer, want) {
			t.Fatalf("shared Mail observer missing project Metrics shape %q:\n%s", want, observer)
		}
	}

	manager := renderSharedTemplate(t, "internal/metrics/manager.go.tmpl", sharedData)
	assertFormattedGoTemplate(t, "internal/metrics/manager.go.tmpl", manager)
	for _, want := range []string{
		"Mail      bool",
		`runtime.CurrentApp().Components.Mail && env.GetBool("METRICS_MAIL_ENABLED", "true")`,
		"type MailSendMetricEvent struct {",
		"func (m *Manager) RecordMailSend(ctx context.Context, event MailSendMetricEvent)",
	} {
		if !strings.Contains(manager, want) {
			t.Fatalf("shared Metrics manager missing project Mail shape %q:\n%s", want, manager)
		}
	}

	defaultMailConfig := &project.Config{
		GoModuleName: "example.com/default-mail",
		Render: project.RenderConfig{Components: project.Components{
			CLI: true, Mail: true,
		}},
		Apps: map[string]project.AppConfig{
			"worker": {Components: project.Components{
				CLI: true, WebAPI: true, Metrics: true,
			}},
		},
	}
	defaultWiring := renderSharedTemplate(t, "wire/inject_managers.go.tmpl", appTemplateDataForProjectionTest(defaultMailConfig, project.DefaultApp(), defaultMailConfig.Render.Components))
	assertFormattedGoTemplate(t, "wire/inject_managers.go.tmpl (Mail default without Metrics)", defaultWiring)
	providerStart := strings.Index(defaultWiring, "func provideMailManager(")
	if providerStart < 0 {
		t.Fatalf("default App manager wiring omitted Mail provider:\n%s", defaultWiring)
	}
	providerEnd := strings.Index(defaultWiring[providerStart:], "\n}\n")
	if providerEnd < 0 {
		t.Fatalf("default App Mail provider has no closing boundary:\n%s", defaultWiring[providerStart:])
	}
	defaultProvider := defaultWiring[providerStart : providerStart+providerEnd+3]
	if strings.Contains(defaultProvider, "metricsManager *metrics.Manager") || !strings.Contains(defaultProvider, "(*metrics.Manager)(nil)") {
		t.Fatalf("Metrics-disabled default App did not use the Mail typed-nil seam:\n%s", defaultProvider)
	}

	defaultMetricsConfig := &project.Config{
		GoModuleName: "example.com/default-mail-metrics",
		Render: project.RenderConfig{Components: project.Components{
			CLI: true, Mail: true, Metrics: true,
		}},
	}
	enabledWiring := renderSharedTemplate(t, "wire/inject_managers.go.tmpl", appTemplateDataForProjectionTest(defaultMetricsConfig, project.DefaultApp(), defaultMetricsConfig.Render.Components))
	assertFormattedGoTemplate(t, "wire/inject_managers.go.tmpl (Mail default with Metrics)", enabledWiring)
	enabledStart := strings.Index(enabledWiring, "func provideMailManager(")
	if enabledStart < 0 {
		t.Fatalf("Metrics-enabled default App manager wiring omitted Mail provider:\n%s", enabledWiring)
	}
	enabledEnd := strings.Index(enabledWiring[enabledStart:], "\n}\n")
	if enabledEnd < 0 {
		t.Fatalf("Metrics-enabled default App Mail provider has no closing boundary:\n%s", enabledWiring[enabledStart:])
	}
	enabledProvider := enabledWiring[enabledStart : enabledStart+enabledEnd+3]
	if !strings.Contains(enabledProvider, "metricsManager *metrics.Manager") || strings.Contains(enabledProvider, "(*metrics.Manager)(nil)") {
		t.Fatalf("Metrics-enabled default App did not wire the real manager into Mail:\n%s", enabledProvider)
	}

	mailer := project.DefaultNamedApp("mailer")
	mailerComponents := project.Components{CLI: true, Mail: true}
	disabledWiring := renderSharedTemplate(t, "wire/inject_managers.go.tmpl", appTemplateDataForProjectionTest(defaultMetricsConfig, mailer, mailerComponents))
	assertFormattedGoTemplate(t, "wire/inject_managers.go.tmpl (named Mail App without Metrics)", disabledWiring)
	disabledStart := strings.Index(disabledWiring, "func provideMailManager(")
	if disabledStart < 0 {
		t.Fatalf("Metrics-disabled named App manager wiring omitted Mail provider:\n%s", disabledWiring)
	}
	disabledEnd := strings.Index(disabledWiring[disabledStart:], "\n}\n")
	if disabledEnd < 0 {
		t.Fatalf("Metrics-disabled named App Mail provider has no closing boundary:\n%s", disabledWiring[disabledStart:])
	}
	disabledProvider := disabledWiring[disabledStart : disabledStart+disabledEnd+3]
	if strings.Contains(disabledProvider, "metricsManager *metrics.Manager") || !strings.Contains(disabledProvider, "(*metrics.Manager)(nil)") {
		t.Fatalf("Metrics-disabled named App did not use the Mail typed-nil seam:\n%s", disabledProvider)
	}
}

// TestRuntimeAboutUsesCompiledAppComponents verifies shared About support is project-shaped while report inclusion is selected at runtime.
func TestRuntimeAboutUsesCompiledAppComponents(t *testing.T) {
	config := mixedAppSharedTemplateConfig()
	projectComponents := project.ProjectComponents(config)
	workerComponents := project.NormalizeConfiguredAppComponents(config, config.Apps["worker"].Components)
	data := templateRenderConfig{
		Config:            config,
		Components:        config.Render.Components,
		ProjectComponents: projectComponents,
		RuntimeApps: []runtimeAppMetadata{
			{Name: project.DefaultAppName, Index: 0, HTTPPort: 3000, RuntimeBase: 10000, Components: config.Render.Components},
			{Name: "worker", Index: 1, EnvPrefix: "WORKER", HTTPPort: 3001, RuntimeBase: 10010, Components: workerComponents},
		},
	}

	appsSource := renderSharedTemplate(t, "internal/runtime/apps.go.tmpl", data)
	assertFormattedGoTemplate(t, "internal/runtime/apps.go.tmpl", appsSource)
	for _, want := range []string{
		`Name: "worker"`,
		`Components: AppComponents{`,
		`DatabasePostgres: true`,
		`Jobs:             true`,
	} {
		if !strings.Contains(appsSource, want) {
			t.Fatalf("runtime App metadata missing %q:\n%s", want, appsSource)
		}
	}
	appsTestSource := renderSharedTemplate(t, "internal/runtime/apps_test.go.tmpl", data)
	assertFormattedGoTemplate(t, "internal/runtime/apps_test.go.tmpl", appsTestSource)
	if !strings.Contains(appsTestSource, `App("worker").Components`) {
		t.Fatalf("runtime App metadata test does not verify the worker component projection:\n%s", appsTestSource)
	}

	aboutSource := renderSharedTemplate(t, "internal/runtime/about.go.tmpl", data)
	assertFormattedGoTemplate(t, "internal/runtime/about.go.tmpl", aboutSource)
	for _, want := range []string{
		`appComponents := CurrentApp().Components`,
		`if appComponents.HasDatabase() {`,
		`report.Databases = aboutDatabaseReports()`,
		`if appComponents.Jobs {`,
		`report.Queues = aboutQueueReports()`,
	} {
		if !strings.Contains(aboutSource, want) {
			t.Fatalf("runtime About source missing %q:\n%s", want, aboutSource)
		}
	}

	defaultOnly := data
	defaultOnly.ProjectComponents = config.Render.Components
	defaultAboutSource := renderSharedTemplate(t, "internal/runtime/about.go.tmpl", defaultOnly)
	assertFormattedGoTemplate(t, "internal/runtime/about.go.tmpl (default only)", defaultAboutSource)
	if strings.Contains(defaultAboutSource, `report.Databases = aboutDatabaseReports()`) || strings.Contains(defaultAboutSource, `report.Queues = aboutQueueReports()`) {
		t.Fatalf("shared About source included capabilities outside the project envelope:\n%s", defaultAboutSource)
	}
	lean := data
	lean.Components = project.Components{CLI: true}
	lean.ProjectComponents = lean.Components
	leanAboutSource := renderSharedTemplate(t, "internal/runtime/about.go.tmpl", lean)
	assertFormattedGoTemplate(t, "internal/runtime/about.go.tmpl (lean CLI)", leanAboutSource)
}

// TestGrafanaSeedUsesProjectCapabilities verifies named-App capabilities are included in the shared dashboard seed contract.
func TestGrafanaSeedUsesProjectCapabilities(t *testing.T) {
	config := mixedAppSharedTemplateConfig()
	data := templateRenderConfig{
		Config:            config,
		Components:        config.Render.Components,
		ProjectComponents: project.ProjectComponents(config),
	}

	seed := renderSharedTemplate(t, "containers/observability/grafana/seed-dashboards.sh.tmpl", data)
	for _, want := range []string{"goforj-database-overview", "goforj-queue-overview", "goforj-scheduler-overview"} {
		if !strings.Contains(seed, want) {
			t.Fatalf("Grafana seed missing named-App dashboard %q:\n%s", want, seed)
		}
	}

	data.ProjectComponents = config.Render.Components
	seed = renderSharedTemplate(t, "containers/observability/grafana/seed-dashboards.sh.tmpl", data)
	for _, unwanted := range []string{"goforj-database-overview", "goforj-queue-overview", "goforj-scheduler-overview"} {
		if strings.Contains(seed, unwanted) {
			t.Fatalf("Grafana seed included out-of-envelope dashboard %q:\n%s", unwanted, seed)
		}
	}
}

// mixedAppSharedTemplateConfig returns a default HTTP App plus a database-backed worker App for projection tests.
func mixedAppSharedTemplateConfig() *project.Config {
	return &project.Config{
		GoModuleName: "example.com/mixed",
		Render: project.RenderConfig{
			Components: project.Components{
				CLI:           true,
				WebAPI:        true,
				Metrics:       true,
				Observability: true,
				Grafana:       true,
				Cache:         true,
				Events:        true,
				Storage:       true,
			},
		},
		Apps: map[string]project.AppConfig{
			"worker": {
				Components: project.Components{
					CLI:              true,
					WebAPI:           true,
					DatabasePostgres: true,
					Scheduler:        true,
					Cache:            true,
					Events:           true,
					Storage:          true,
					Jobs:             true,
				},
			},
		},
	}
}

// appTemplateDataForProjectionTest builds the established per-App render projection without invoking a project render.
func appTemplateDataForProjectionTest(config *project.Config, app project.App, components project.Components) templateRenderConfig {
	return templateRenderConfig{
		Config:            config,
		Components:        components,
		ProjectComponents: project.ProjectComponents(config),
		App:               app,
		AppPackageName:    project.AppPackageName(app.Name),
		AppImportPath:     app.AppDir,
		WireImportPath:    app.WireDir,
	}
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
