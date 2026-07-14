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
