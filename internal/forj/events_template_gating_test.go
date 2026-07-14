package forj

import (
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestEventsGeneratedSurfaceUsesAppParticipation verifies either App can own Events without leaking its generated wiring into its sibling.
func TestEventsGeneratedSurfaceUsesAppParticipation(t *testing.T) {
	tests := []struct {
		name          string
		defaultEvents bool
		workerEvents  bool
	}{
		{name: "named App only", workerEvents: true},
		{name: "default App only", defaultEvents: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := eventsProjectionConfig(test.defaultEvents, test.workerEvents)
			renderer := &ProjectRenderer{config: config}
			apps := []struct {
				app     project.App
				enabled bool
			}{
				{app: project.DefaultApp(), enabled: test.defaultEvents},
				{app: project.DefaultNamedApp("worker"), enabled: test.workerEvents},
			}

			for _, target := range apps {
				t.Run(target.app.Name, func(t *testing.T) {
					components := appRenderComponents(config, target.app)
					data := appTemplateDataForProjectionTest(config, target.app, components)
					data.HelpFormatterFunc = "FrameworkFormatter"
					sources := map[string]string{
						"app/root_cmd.go.tmpl":         renderSharedTemplate(t, "app/root_cmd.go.tmpl", data),
						"wire/app.go.tmpl":             renderSharedTemplate(t, "wire/app.go.tmpl", data),
						"wire/inject_cmd.go.tmpl":      renderSharedTemplate(t, "wire/inject_cmd.go.tmpl", data),
						"wire/inject_http.go.tmpl":     renderSharedTemplate(t, "wire/inject_http.go.tmpl", data),
						"wire/inject_managers.go.tmpl": renderSharedTemplate(t, "wire/inject_managers.go.tmpl", data),
						"wire/wire.go.tmpl":            renderSharedTemplate(t, "wire/wire.go.tmpl", data),
					}
					for path, source := range sources {
						assertFormattedGoTemplate(t, path, source)
					}

					assertTemplateMarker(t, "app/root_cmd.go.tmpl", sources["app/root_cmd.go.tmpl"], "GeneratedEventCommands", target.enabled)
					assertTemplateMarker(t, "wire/app.go.tmpl", sources["wire/app.go.tmpl"], "registerEventBusLifecycle", target.enabled)
					assertTemplateMarker(t, "wire/app.go.tmpl", sources["wire/app.go.tmpl"], "EventSubscribersReady", target.enabled)
					assertTemplateMarker(t, "wire/inject_cmd.go.tmpl", sources["wire/inject_cmd.go.tmpl"], ".NewGeneratedEventCommands", target.enabled)
					assertTemplateMarker(t, "wire/inject_managers.go.tmpl", sources["wire/inject_managers.go.tmpl"], "provideEventManager", target.enabled)
					assertTemplateMarker(t, "wire/wire.go.tmpl", sources["wire/wire.go.tmpl"], "appSubscriberSet", target.enabled)

					httpSource := sources["wire/inject_http.go.tmpl"]
					assertTemplateMarker(t, "wire/inject_http.go.tmpl", httpSource, `"example.com/events-projection/internal/events"`, true)
					assertTemplateMarker(t, "wire/inject_http.go.tmpl", httpSource, "events *events.Manager", target.enabled)
					assertTemplateMarker(t, "wire/inject_http.go.tmpl", httpSource, "(*events.Manager)(nil)", !target.enabled)

					frameworkOwnsEvents := templateMappingsContain(renderer.appFrameworkMappings(target.app), "app/event_commands.go.tmpl")
					if frameworkOwnsEvents != target.enabled {
						t.Fatalf("framework Events command mapping presence = %t, want %t", frameworkOwnsEvents, target.enabled)
					}
					appOwnsSubscribers := templateMappingsContain(renderer.appOwnedMappings(target.app), "wire/inject_subscribers_app.go.tmpl")
					if appOwnsSubscribers != target.enabled {
						t.Fatalf("app-owned subscriber mapping presence = %t, want %t", appOwnsSubscribers, target.enabled)
					}

					if target.enabled {
						eventCommands := renderSharedTemplate(t, "app/event_commands.go.tmpl", data)
						assertFormattedGoTemplate(t, "app/event_commands.go.tmpl", eventCommands)
						for _, marker := range []string{
							"MakeEventCmd",
							"MakeSubscriberCmd",
							"TestEventPipelineCmd",
							"*makecmd.NewEventCmd()",
							"*makecmd.NewSubscriberCmd()",
							"*cmd.NewTestEventPipelineCmd(eventManager)",
						} {
							assertTemplateMarker(t, "app/event_commands.go.tmpl", eventCommands, marker, true)
						}
					}
				})
			}
		})
	}
}

// TestEventsSharedSurfaceUsesProjectEnvelopeAndCurrentApp verifies shared source compiles Events once while runtime reports stay App-local.
func TestEventsSharedSurfaceUsesProjectEnvelopeAndCurrentApp(t *testing.T) {
	config := eventsProjectionConfig(false, true)
	workerComponents := appRenderComponents(config, project.DefaultNamedApp("worker"))
	data := templateRenderConfig{
		Config:            config,
		Components:        config.Render.Components,
		ProjectComponents: project.ProjectComponents(config),
		RuntimeApps: []runtimeAppMetadata{
			{Name: project.DefaultAppName, Index: 0, HTTPPort: 3000, RuntimeBase: 10000, Components: config.Render.Components},
			{Name: "worker", Index: 1, EnvPrefix: "WORKER", HTTPPort: 3001, RuntimeBase: 10010, Components: workerComponents},
		},
	}

	sources := renderEventsSharedSources(t, data)
	for path, source := range sources {
		assertFormattedGoTemplate(t, path, source)
	}
	for path, markers := range map[string][]string{
		"internal/http/readiness_checks.go.tmpl": {`"example.com/events-projection/internal/events"`, "events *events.Manager"},
		"internal/http/health.go.tmpl":           {`case "events":`, "func eventDriver("},
		"internal/runtime/about.go.tmpl":         {`"github.com/goforj/str"`, "Events      []AboutEvent", "if appComponents.Events {", "report.Events = aboutEventReports()"},
		"internal/runtime/apps.go.tmpl":          {"Events           bool", "Events:           false", "Events:           true"},
		"internal/runtime/apps_test.go.tmpl":     {"Events:           false", "Events:           true"},
		"internal/runtime/discovery.go.tmpl":     {"func DiscoverEventInstances(", "if !CurrentApp().Components.Events {", "func NormalizeEventDriver("},
		"internal/cmd/about_cmd_test.go.tmpl":    {"cache, storage, events, lighthouse"},
	} {
		for _, marker := range markers {
			assertTemplateMarker(t, path, sources[path], marker, true)
		}
	}

	offConfig := eventsProjectionConfig(false, false)
	offData := templateRenderConfig{
		Config:            offConfig,
		Components:        offConfig.Render.Components,
		ProjectComponents: project.ProjectComponents(offConfig),
		RuntimeApps: []runtimeAppMetadata{
			{Name: project.DefaultAppName, Index: 0, HTTPPort: 3000, RuntimeBase: 10000, Components: offConfig.Render.Components},
			{Name: "worker", Index: 1, EnvPrefix: "WORKER", HTTPPort: 3001, RuntimeBase: 10010, Components: appRenderComponents(offConfig, project.DefaultNamedApp("worker"))},
		},
	}
	offSources := renderEventsSharedSources(t, offData)
	for path, source := range offSources {
		assertFormattedGoTemplate(t, path, source)
	}
	for path, markers := range map[string][]string{
		"internal/http/readiness_checks.go.tmpl": {`internal/events`, "events *events.Manager"},
		"internal/http/health.go.tmpl":           {`case "events":`, "func eventDriver("},
		"internal/runtime/about.go.tmpl":         {`"github.com/goforj/str"`, "AboutEvent", "appComponents.Events", "aboutEventReports"},
		"internal/runtime/apps.go.tmpl":          {"Events           bool", "Events:"},
		"internal/runtime/apps_test.go.tmpl":     {"Events:"},
		"internal/runtime/discovery.go.tmpl":     {"DiscoverEventInstances", "NormalizeEventDriver"},
		"internal/cmd/about_cmd_test.go.tmpl":    {"cache, storage, events, lighthouse"},
	} {
		for _, marker := range markers {
			assertTemplateMarker(t, path, offSources[path], marker, false)
		}
	}
}

// TestGeneratedEventCommandsPreserveLegacyOwnerShapes verifies existing app-owned pipeline fields remain the only Kong owner after refresh.
func TestGeneratedEventCommandsPreserveLegacyOwnerShapes(t *testing.T) {
	config := eventsProjectionConfig(true, false)
	base := appTemplateDataForProjectionTest(config, project.DefaultApp(), config.Render.Components)
	base.HelpFormatterFunc = "FrameworkFormatter"

	tests := []struct {
		name                  string
		legacyField           bool
		legacyProvider        bool
		wantGeneratedPipeline bool
		wantBackfillProvider  bool
	}{
		{name: "new App", wantGeneratedPipeline: true},
		{name: "legacy field and provider", legacyField: true, legacyProvider: true},
		{name: "legacy field missing provider", legacyField: true, wantBackfillProvider: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := base
			data.LegacyEventPipelineField = test.legacyField
			data.LegacyEventPipelineProvider = test.legacyProvider
			eventCommands := renderSharedTemplate(t, "app/event_commands.go.tmpl", data)
			injectCmd := renderSharedTemplate(t, "wire/inject_cmd.go.tmpl", data)
			assertFormattedGoTemplate(t, "app/event_commands.go.tmpl", eventCommands)
			assertFormattedGoTemplate(t, "wire/inject_cmd.go.tmpl", injectCmd)

			assertTemplateMarker(t, "app/event_commands.go.tmpl", eventCommands, "TestEventPipelineCmd", test.wantGeneratedPipeline)
			assertTemplateMarker(t, "app/event_commands.go.tmpl", eventCommands, `"example.com/events-projection/internal/events"`, test.wantGeneratedPipeline)
			assertTemplateMarker(t, "wire/inject_cmd.go.tmpl", injectCmd, "cmd.NewTestEventPipelineCmd,", test.wantBackfillProvider)
			for _, marker := range []string{"MakeEventCmd", "MakeSubscriberCmd", "*makecmd.NewEventCmd()", "*makecmd.NewSubscriberCmd()"} {
				assertTemplateMarker(t, "app/event_commands.go.tmpl", eventCommands, marker, true)
			}
		})
	}
}

// TestFutureAppOwnedCommandTemplatesAreEventNeutral protects customized App files from later Events toggles.
func TestFutureAppOwnedCommandTemplatesAreEventNeutral(t *testing.T) {
	for _, path := range []string{
		"app/commands.go.tmpl",
		"wire/inject_cmd_app.go.tmpl",
		"wire/inject_services_app.go.tmpl",
	} {
		content, err := templatesFS.ReadFile(path)
		if err != nil {
			t.Fatalf("read template %s: %v", path, err)
		}
		source := string(content)
		for _, marker := range []string{"MakeEventCmd", "MakeSubscriberCmd", "TestEventPipeline", "NewEventCmd", "NewSubscriberCmd"} {
			assertTemplateMarker(t, path, source, marker, false)
		}
	}
}

// TestMakeCommandDocumentationFollowsEventsEnvelope keeps generated command discovery honest for Events-free projects.
func TestMakeCommandDocumentationFollowsEventsEnvelope(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		config := eventsProjectionConfig(enabled, false)
		data := templateRenderConfig{Config: config, ProjectComponents: project.ProjectComponents(config)}
		readme := renderSharedTemplate(t, "internal/makecmd/README.md.tmpl", data)
		for _, marker := range []string{"make:event", "make:subscriber"} {
			assertTemplateMarker(t, "internal/makecmd/README.md.tmpl", readme, marker, enabled)
		}
	}
}

// eventsProjectionConfig creates two HTTP Apps with independently selected Events participation.
func eventsProjectionConfig(defaultEvents bool, workerEvents bool) *project.Config {
	return &project.Config{
		GoModuleName: "example.com/events-projection",
		Render: project.RenderConfig{Components: project.Components{
			CLI: true, WebAPI: true, Cache: true, Events: defaultEvents, Storage: true,
		}},
		Apps: map[string]project.AppConfig{
			"worker": {Components: project.Components{
				CLI: true, WebAPI: true, Cache: true, Events: workerEvents, Storage: true,
			}},
		},
	}
}

// renderEventsSharedSources renders the shared templates whose shape depends on the project Events envelope.
func renderEventsSharedSources(t *testing.T, data templateRenderConfig) map[string]string {
	t.Helper()
	paths := []string{
		"internal/cmd/about_cmd_test.go.tmpl",
		"internal/http/health.go.tmpl",
		"internal/http/readiness_checks.go.tmpl",
		"internal/runtime/about.go.tmpl",
		"internal/runtime/apps.go.tmpl",
		"internal/runtime/apps_test.go.tmpl",
		"internal/runtime/discovery.go.tmpl",
	}
	sources := make(map[string]string, len(paths))
	for _, path := range paths {
		sources[path] = renderSharedTemplate(t, path, data)
	}
	return sources
}

// templateMappingsContain reports whether a renderer mapping list contains one source template.
func templateMappingsContain(mappings []templateMapping, path string) bool {
	for _, mapping := range mappings {
		if mapping.tmpl == path {
			return true
		}
	}
	return false
}

// assertTemplateMarker verifies one rendered ownership marker is present exactly when expected.
func assertTemplateMarker(t *testing.T, path string, source string, marker string, want bool) {
	t.Helper()
	if got := strings.Contains(source, marker); got != want {
		t.Fatalf("template %s marker %q presence = %t, want %t\n%s", path, marker, got, want, source)
	}
}
