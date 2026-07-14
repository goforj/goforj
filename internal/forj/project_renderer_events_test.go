package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/forj/makeapp"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
)

// TestEventsAppMappingsFollowAppParticipation verifies project support cannot leak generated or owner Events files into an Events-disabled App.
func TestEventsAppMappingsFollowAppParticipation(t *testing.T) {
	config := &project.Config{
		Render: project.RenderConfig{Components: project.Components{CLI: true}},
		Apps: map[string]project.AppConfig{
			"events-worker": {Components: project.Components{CLI: true, Events: true}},
		},
	}
	renderer := &ProjectRenderer{config: config}

	defaultApp := project.DefaultApp()
	workerApp := project.DefaultNamedApp("events-worker")
	if mappingDestExists(renderer.appFrameworkMappings(defaultApp), filepath.Join(defaultApp.AppDir, "event_commands.go")) {
		t.Fatal("Events-disabled default App received generated event commands")
	}
	if mappingDestExists(renderer.appOwnedMappings(defaultApp), filepath.Join(defaultApp.WireDir, "inject_subscribers_app.go")) {
		t.Fatal("Events-disabled default App received an owner subscriber injector")
	}
	if !mappingDestExists(renderer.appFrameworkMappings(workerApp), filepath.Join(workerApp.AppDir, "event_commands.go")) {
		t.Fatal("Events-enabled named App omitted generated event commands")
	}
	if !mappingDestExists(renderer.appOwnedMappings(workerApp), filepath.Join(workerApp.WireDir, "inject_subscribers_app.go")) {
		t.Fatal("Events-enabled named App omitted its owner subscriber injector")
	}
}

// TestEventsTemplatesLiveOnlyInTheGatedRendererGroup verifies Core cannot recreate an omitted project-wide Events surface.
func TestEventsTemplatesLiveOnlyInTheGatedRendererGroup(t *testing.T) {
	source, err := os.ReadFile("project_renderer.go")
	if err != nil {
		t.Fatalf("read project renderer: %v", err)
	}
	text := string(source)
	coreStart := strings.Index(text, `title:   "Core Components Rendering"`)
	eventsStart := strings.Index(text, `title:   "Events Components Rendering"`)
	legacyStart := strings.Index(text, `title:   "Legacy File Cleanup"`)
	if coreStart < 0 || eventsStart <= coreStart || legacyStart <= eventsStart {
		t.Fatalf("renderer groups are not ordered as expected")
	}
	core := text[coreStart:eventsStart]
	events := text[eventsStart:legacyStart]
	for _, path := range []string{
		"internal/events/event.go.tmpl",
		"internal/events/topics.go.tmpl",
		"internal/makecmd/make_event_cmd.go.tmpl",
		"internal/makecmd/make_subscriber_cmd.go.tmpl",
		"internal/cmd/test_event_pipeline_cmd.go.tmpl",
		"internal/observability/event_observer.go.tmpl",
		"internal/makecmd/event.tmpl",
		"internal/makecmd/subscriber.tmpl",
	} {
		if strings.Contains(core, path) {
			t.Fatalf("Core renderer still owns Events template %s", path)
		}
		if !strings.Contains(events, path) {
			t.Fatalf("Events renderer group omits %s", path)
		}
	}
	if !strings.Contains(events, "enabled: projectComponents.Events") {
		t.Fatal("Events renderer group is not gated by the project envelope")
	}
}

// TestSyncCoreLibrariesUsesNamedAppEventsEnvelope verifies a named Events App pulls shared Events modules without widening the default App.
func TestSyncCoreLibrariesUsesNamedAppEventsEnvelope(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/app\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	renderer := NewProjectRenderer(logger.NewSilentLogger())
	renderer.config = &project.Config{
		Render: project.RenderConfig{Components: project.Components{CLI: true}},
		Apps: map[string]project.AppConfig{
			"events-worker": {Components: project.Components{CLI: true, Events: true}},
		},
	}
	if err := renderer.syncCoreLibrariesInDir(root); err != nil {
		t.Fatalf("sync core libraries: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	for _, module := range []string{"github.com/goforj/events ", "github.com/goforj/events/eventscore "} {
		if !strings.Contains(string(data), module) {
			t.Fatalf("named Events App did not select shared module %q:\n%s", module, data)
		}
	}
	if renderer.config.Render.Components.Events {
		t.Fatal("Events module selection widened the default App")
	}
}

// TestEventsCompatibilityFlagsUseStructuredOwnerSource verifies comments and unrelated selectors cannot trigger legacy registrations.
func TestEventsCompatibilityFlagsUseStructuredOwnerSource(t *testing.T) {
	root := useEventsRendererRoot(t)
	app := project.DefaultApp()
	writeEventsRendererFile(t, filepath.Join(root, app.AppDir, "commands.go"), `package app

import "example.test/internal/cmd"

// cmd.TestEventPipelineCmd is mentioned for migration documentation only.
type Commands struct {
	Pipeline cmd.TestEventPipelineCmd `+"`cmd:\"\"`"+`
}
`)
	writeEventsRendererFile(t, filepath.Join(root, app.WireDir, "inject_cmd_app.go"), `package wire

import (
	"example.test/internal/cmd"
	"github.com/goforj/wire"
)

var appCommandSet = wire.NewSet(cmd.NewTestEventPipelineCmd)
`)

	if !legacyEventPipelineField(app) {
		t.Fatal("legacy Events command field was not detected")
	}
	if !legacyEventPipelineProvider(app) {
		t.Fatal("legacy Events Wire provider was not detected")
	}
}

// TestValidateEventsRenderTransitionRejectsUnsupportedRemoval verifies owner and generated files are never silently stranded or deleted.
func TestValidateEventsRenderTransitionRejectsUnsupportedRemoval(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantPath string
		contents string
	}{
		{name: "project package", path: filepath.Join("internal", "events", "event.go"), wantPath: filepath.Join("internal", "events"), contents: "package events\n"},
		{name: "owner subscribers", path: filepath.Join("app", "wire", "inject_subscribers_app.go"), contents: "package wire\n"},
		{name: "generated commands", path: filepath.Join("app", "event_commands.go"), contents: "package app\n"},
		{name: "legacy field", path: filepath.Join("app", "commands.go"), contents: "package app\ntype Commands struct { Pipeline cmd.TestEventPipelineCmd }\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.wantPath == "" {
				test.wantPath = test.path
			}
			root := useEventsRendererRoot(t)
			writeEventsRendererFile(t, filepath.Join(root, test.path), test.contents)
			renderer := &ProjectRenderer{config: &project.Config{Render: project.RenderConfig{Components: project.Components{CLI: true}}}}
			err := renderer.validateEventsRenderTransition(project.Components{CLI: true})
			if err == nil || !strings.Contains(err.Error(), filepath.Clean(test.wantPath)) {
				t.Fatalf("transition error = %v, want path %s", err, test.wantPath)
			}
			if _, statErr := os.Stat(test.path); statErr != nil {
				t.Fatalf("preflight changed owner path %s: %v", test.path, statErr)
			}
		})
	}
}

// TestValidateEventsRenderTransitionRejectsMalformedOwnerSource verifies parsing failures stop both enabled and disabled renders before writes.
func TestValidateEventsRenderTransitionRejectsMalformedOwnerSource(t *testing.T) {
	for _, eventsEnabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[eventsEnabled], func(t *testing.T) {
			root := useEventsRendererRoot(t)
			path := filepath.Join(root, "app", "commands.go")
			writeEventsRendererFile(t, path, "package app\ntype Commands struct {\n")
			components := project.Components{CLI: true, Events: eventsEnabled}
			renderer := &ProjectRenderer{config: &project.Config{Render: project.RenderConfig{Components: components}}}
			err := renderer.validateEventsRenderTransition(components)
			if err == nil || !strings.Contains(err.Error(), filepath.Join("app", "commands.go")) {
				t.Fatalf("malformed owner error = %v, want commands.go path", err)
			}
		})
	}
}

// TestValidateEventsRenderTransitionRejectsPartialProjectResidue verifies every known project-owned Events artifact blocks disabling the component.
func TestValidateEventsRenderTransitionRejectsPartialProjectResidue(t *testing.T) {
	for _, path := range projectEventsResiduePaths() {
		path := path
		t.Run(strings.ReplaceAll(path, string(filepath.Separator), "_"), func(t *testing.T) {
			root := useEventsRendererRoot(t)
			fixturePath := path
			if path == filepath.Join("internal", "events") {
				fixturePath = filepath.Join(path, "event.go")
			}
			contents := "fixture\n"
			if filepath.Ext(fixturePath) == ".go" {
				contents = "package fixture\n"
			}
			writeEventsRendererFile(t, filepath.Join(root, fixturePath), contents)

			renderer := &ProjectRenderer{config: &project.Config{Render: project.RenderConfig{Components: project.Components{CLI: true}}}}
			err := renderer.validateEventsRenderTransition(project.Components{CLI: true})
			if err == nil || !strings.Contains(err.Error(), path) {
				t.Fatalf("transition error = %v, want residue path %s", err, path)
			}
			if _, statErr := os.Stat(path); statErr != nil {
				t.Fatalf("preflight changed Events residue %s: %v", path, statErr)
			}
		})
	}
}

// TestValidateEventsRenderTransitionRejectsLegacySubscriberOwners verifies historical App-owned paths cannot be stranded when a sibling keeps Events available.
func TestValidateEventsRenderTransitionRejectsLegacySubscriberOwners(t *testing.T) {
	for _, path := range legacyEventSubscriberOwnerPaths(project.DefaultApp()) {
		path := path
		t.Run(strings.ReplaceAll(path, string(filepath.Separator), "_"), func(t *testing.T) {
			root := useEventsRendererRoot(t)
			writeEventsRendererFile(t, filepath.Join(root, path), "package wire\n")
			config := mixedEventsRendererConfig()
			renderer := &ProjectRenderer{config: config}

			err := renderer.validateEventsRenderTransition(project.ProjectComponents(config))
			if err == nil || !strings.Contains(err.Error(), path) {
				t.Fatalf("transition error = %v, want legacy owner path %s", err, path)
			}
			if _, statErr := os.Stat(path); statErr != nil {
				t.Fatalf("preflight changed legacy owner %s: %v", path, statErr)
			}
		})
	}
}

// TestValidateEventsRenderTransitionRejectsLegacyProviders verifies disabled Apps cannot retain Events providers in shared owner sets.
func TestValidateEventsRenderTransitionRejectsLegacyProviders(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		contents string
	}{
		{
			name: "pipeline command",
			path: filepath.Join("app", "wire", "inject_cmd_app.go"),
			contents: `package wire

var appCommandSet = wire.NewSet(cmd.NewTestEventPipelineCmd)
`,
		},
		{
			name: "make commands",
			path: filepath.Join("app", "wire", "inject_services_app.go"),
			contents: `package wire

var appServiceSet = wire.NewSet(makecmd.NewEventCmd, makecmd.NewSubscriberCmd)
`,
		},
		{
			name: "shared redis provider",
			path: filepath.Join("app", "wire", "inject_services_app.go"),
			contents: `package wire

// CustomProvider must survive an ambiguous legacy migration.
func CustomProvider() any { return nil }

func provideSharedRedisClient() any { return nil }
`,
		},
		{
			name: "redis events bus",
			path: filepath.Join("app", "wire", "inject_services_app.go"),
			contents: `package wire

// CustomProvider must survive an ambiguous legacy migration.
func CustomProvider() any { return nil }

func provideEventsBus(redisClient any) any {
	return events.NewBus(context.Background(), redisClient)
}
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := useEventsRendererRoot(t)
			writeEventsRendererFile(t, filepath.Join(root, test.path), test.contents)
			config := mixedEventsRendererConfig()
			renderer := &ProjectRenderer{config: config}

			err := renderer.validateEventsRenderTransition(project.ProjectComponents(config))
			if err == nil || !strings.Contains(err.Error(), test.path) {
				t.Fatalf("transition error = %v, want legacy provider path %s", err, test.path)
			}
			assertEventsRendererFileContents(t, test.path, test.contents)
		})
	}
}

// TestSyncLegacyGeneratedTemplatesPreservesAppOwnedServiceInjector verifies legacy marker text cannot replace a customization file.
func TestSyncLegacyGeneratedTemplatesPreservesAppOwnedServiceInjector(t *testing.T) {
	root := useEventsRendererRoot(t)
	path := filepath.Join("app", "wire", "inject_services_app.go")
	contents := `package wire

// provideSharedRedisClient and events.NewBus(context.Background(), redisClient) are migration notes only.
var appSet = wire.NewSet(NewCustomService)

type CustomService struct{}

func NewCustomService() *CustomService { return &CustomService{} }
`
	writeEventsRendererFile(t, filepath.Join(root, path), contents)
	renderer := NewProjectRenderer(logger.NewSilentLogger())
	renderer.config = &project.Config{
		ProjectName:  "Events App",
		GoModuleName: "example.test/events-app",
		Render:       project.RenderConfig{Components: project.Components{CLI: true, Events: true}},
	}

	if err := renderer.syncLegacyGeneratedTemplates(); err != nil {
		t.Fatalf("sync legacy generated templates: %v", err)
	}
	assertEventsRendererFileContents(t, path, contents)
}

// TestMigrateAppOwnedWireFilenamesPreservesLegacyEventSubscriberOwners verifies every known unambiguous owner path moves byte-for-byte.
func TestMigrateAppOwnedWireFilenamesPreservesLegacyEventSubscriberOwners(t *testing.T) {
	tests := []struct {
		name   string
		app    project.App
		source string
	}{
		{name: "root old name", app: project.DefaultApp(), source: filepath.Join("wire", "inject_event_subscribers.go")},
		{name: "root current name", app: project.DefaultApp(), source: filepath.Join("wire", "inject_subscribers_app.go")},
		{name: "app old name", app: project.DefaultApp(), source: filepath.Join("app", "wire", "inject_event_subscribers.go")},
		{name: "named app old name", app: project.DefaultNamedApp("events-worker"), source: filepath.Join("app", "events-worker", "wire", "inject_event_subscribers.go")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := useEventsRendererRoot(t)
			contents := "package wire\n\n// Custom subscriber ownership must survive migration.\nvar customSubscriberOwner = \"" + test.name + "\"\n"
			writeEventsRendererFile(t, filepath.Join(root, test.source), contents)
			renderer := &ProjectRenderer{config: eventsMigrationConfig(test.app)}

			if err := renderer.migrateAppOwnedWireFilenames(); err != nil {
				t.Fatalf("migrate legacy Events subscriber owner: %v", err)
			}
			target := eventSubscriberOwnerPath(test.app)
			assertEventsRendererFileContents(t, target, contents)
			if _, err := os.Stat(test.source); !os.IsNotExist(err) {
				t.Fatalf("legacy owner %s still exists after migration: %v", test.source, err)
			}
		})
	}
}

// TestMigrateAppOwnedWireFilenamesRejectsSubscriberOwnerCollision verifies a current owner never overwrites or deletes its legacy peer.
func TestMigrateAppOwnedWireFilenamesRejectsSubscriberOwnerCollision(t *testing.T) {
	root := useEventsRendererRoot(t)
	source := filepath.Join("app", "wire", "inject_event_subscribers.go")
	target := filepath.Join("app", "wire", "inject_subscribers_app.go")
	legacyController := filepath.Join("app", "wire", "inject_controllers_app.go")
	currentController := filepath.Join("app", "wire", "inject_http_controllers_app.go")
	sourceContents := "package wire\n\nvar legacySubscriberOwner = true\n"
	targetContents := "package wire\n\nvar currentSubscriberOwner = true\n"
	writeEventsRendererFile(t, filepath.Join(root, source), sourceContents)
	writeEventsRendererFile(t, filepath.Join(root, target), targetContents)
	writeEventsRendererFile(t, filepath.Join(root, legacyController), "package wire\n")
	renderer := &ProjectRenderer{config: eventsMigrationConfig(project.DefaultApp())}

	err := renderer.migrateAppOwnedWireFilenames()
	if err == nil || !strings.Contains(err.Error(), source) || !strings.Contains(err.Error(), target) {
		t.Fatalf("migration error = %v, want source and target collision", err)
	}
	assertEventsRendererFileContents(t, source, sourceContents)
	assertEventsRendererFileContents(t, target, targetContents)
	if _, statErr := os.Stat(legacyController); statErr != nil {
		t.Fatalf("collision preflight changed unrelated owner %s: %v", legacyController, statErr)
	}
	if _, statErr := os.Stat(currentController); !os.IsNotExist(statErr) {
		t.Fatalf("collision preflight created unrelated target %s: %v", currentController, statErr)
	}
}

// TestMigrateAppOwnedWireFilenamesRejectsMultipleLegacySubscriberOwners verifies ambiguous historical owners remain untouched for manual reconciliation.
func TestMigrateAppOwnedWireFilenamesRejectsMultipleLegacySubscriberOwners(t *testing.T) {
	root := useEventsRendererRoot(t)
	sources := []string{
		filepath.Join("wire", "inject_event_subscribers.go"),
		filepath.Join("wire", "inject_subscribers_app.go"),
	}
	for index, source := range sources {
		writeEventsRendererFile(t, filepath.Join(root, source), "package wire\n\nvar legacyOwner"+string(rune('A'+index))+" = true\n")
	}
	renderer := &ProjectRenderer{config: eventsMigrationConfig(project.DefaultApp())}

	err := renderer.migrateAppOwnedWireFilenames()
	if err == nil {
		t.Fatal("multiple legacy subscriber owners migrated without reconciliation")
	}
	for _, source := range sources {
		if !strings.Contains(err.Error(), source) {
			t.Fatalf("migration error = %v, want legacy source %s", err, source)
		}
		if _, statErr := os.Stat(source); statErr != nil {
			t.Fatalf("ambiguous legacy owner %s was changed: %v", source, statErr)
		}
	}
	if target := eventSubscriberOwnerPath(project.DefaultApp()); !strings.Contains(err.Error(), target) {
		t.Fatalf("migration error = %v, want destination %s", err, target)
	} else if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("ambiguous migration created destination %s: %v", target, statErr)
	}
}

// TestMigrateAppOwnedWireFilenamesRejectsMalformedLegacySubscriberOwner verifies malformed preserved code stops all migrations before mutation.
func TestMigrateAppOwnedWireFilenamesRejectsMalformedLegacySubscriberOwner(t *testing.T) {
	root := useEventsRendererRoot(t)
	source := filepath.Join("app", "wire", "inject_event_subscribers.go")
	target := eventSubscriberOwnerPath(project.DefaultApp())
	legacyController := filepath.Join("app", "wire", "inject_controllers_app.go")
	contents := "package wire\nvar appSubscriberSet = wire.NewSet(\n"
	writeEventsRendererFile(t, filepath.Join(root, source), contents)
	writeEventsRendererFile(t, filepath.Join(root, legacyController), "package wire\n")
	renderer := &ProjectRenderer{config: eventsMigrationConfig(project.DefaultApp())}

	err := renderer.migrateAppOwnedWireFilenames()
	if err == nil || !strings.Contains(err.Error(), source) {
		t.Fatalf("migration error = %v, want malformed source %s", err, source)
	}
	assertEventsRendererFileContents(t, source, contents)
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("malformed migration created destination %s: %v", target, statErr)
	}
	if _, statErr := os.Stat(legacyController); statErr != nil {
		t.Fatalf("malformed migration changed unrelated owner %s: %v", legacyController, statErr)
	}
}

// TestMigrateAppOwnedWireFilenamesRejectsSubscriberSetNameCollision verifies a surgical repair never chooses between competing owner declarations.
func TestMigrateAppOwnedWireFilenamesRejectsSubscriberSetNameCollision(t *testing.T) {
	root := useEventsRendererRoot(t)
	path := eventSubscriberOwnerPath(project.DefaultApp())
	contents := `package wire

var eventSubscriberSet = wire.NewSet(LegacySubscriber)
var appSubscriberSet = wire.NewSet(CurrentSubscriber)
`
	writeEventsRendererFile(t, filepath.Join(root, path), contents)
	renderer := &ProjectRenderer{config: eventsMigrationConfig(project.DefaultApp())}

	err := renderer.migrateAppOwnedWireFilenames()
	if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "eventSubscriberSet") || !strings.Contains(err.Error(), "appSubscriberSet") {
		t.Fatalf("migration error = %v, want actionable set-name collision", err)
	}
	assertEventsRendererFileContents(t, path, contents)
}

// TestRenderAppOnlyRepairsLegacyEventSubscriberSetName verifies App-only renders share the ownership-safe subscriber migration.
func TestRenderAppOnlyRepairsLegacyEventSubscriberSetName(t *testing.T) {
	root := useEventsRendererRoot(t)
	components := project.Components{CLI: true, Events: true}
	app := project.DefaultNamedApp("events-worker")
	config := &project.Config{
		ProjectName:  "Events App",
		GoModuleName: "example.test/events-app",
		Render:       project.RenderConfig{Components: components},
		Apps: map[string]project.AppConfig{
			app.Name: {Components: components},
		},
	}
	if err := writeProjectConfig(filepath.Join(root, ".goforj.yml"), config); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	writeEventsRendererFile(t, filepath.Join(root, ".env"), "EVENTS_DRIVER=inproc\nEVENTS_SUPPORTED_DRIVERS=inproc,redis\n")
	source := filepath.Join(app.WireDir, "inject_event_subscribers.go")
	target := eventSubscriberOwnerPath(app)
	legacyContents := `package wire

// eventSubscriberSet remains here to document the historical owner name.
var eventSubscriberSet = wire.NewSet(NewCustomSubscriber)

var historicalSetName = "eventSubscriberSet"

func currentSubscriberSet() any {
	return eventSubscriberSet
}
`
	wantContents := `package wire

// eventSubscriberSet remains here to document the historical owner name.
var appSubscriberSet = wire.NewSet(NewCustomSubscriber)

var historicalSetName = "eventSubscriberSet"

func currentSubscriberSet() any {
	return appSubscriberSet
}
`
	writeEventsRendererFile(t, filepath.Join(root, source), legacyContents)
	renderer := NewProjectRenderer(logger.NewSilentLogger())

	if err := renderer.RenderAppOnly(app, makeapp.RenderOptions{Components: components, SkipWire: true}); err != nil {
		t.Fatalf("render App-only legacy Events owner: %v", err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("legacy App-only owner %s still exists after migration: %v", source, err)
	}
	assertEventsRendererFileContents(t, target, wantContents)
}

// TestRenderAppOnlyRejectsEventsRemovalBeforeWrites verifies prospective App configuration is checked before migrations or persistence.
func TestRenderAppOnlyRejectsEventsRemovalBeforeWrites(t *testing.T) {
	root := useEventsRendererRoot(t)
	config := &project.Config{
		ProjectName:  "Events App",
		GoModuleName: "example.test/events-app",
		Render:       project.RenderConfig{Components: project.Components{CLI: true}},
		Apps: map[string]project.AppConfig{
			"events-worker": {Components: project.Components{CLI: true, Events: true}},
		},
	}
	if err := writeProjectConfig(filepath.Join(root, ".goforj.yml"), config); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	configBefore, err := os.ReadFile(filepath.Join(root, ".goforj.yml"))
	if err != nil {
		t.Fatalf("read project config: %v", err)
	}

	app := project.DefaultNamedApp("events-worker")
	writeEventsRendererFile(t, filepath.Join(root, app.AppDir, "event_commands.go"), "package eventsworker\n")
	legacyPath := filepath.Join(root, "app", "wire", "inject_controllers_app.go")
	migratedPath := filepath.Join(root, "app", "wire", "inject_http_controllers_app.go")
	writeEventsRendererFile(t, legacyPath, "package wire\n")

	renderer := NewProjectRenderer(logger.NewSilentLogger())
	err = renderer.RenderAppOnly(app, makeapp.RenderOptions{
		Components: project.Components{CLI: true},
		SkipWire:   true,
	})
	if err == nil || !strings.Contains(err.Error(), filepath.Join(app.AppDir, "event_commands.go")) {
		t.Fatalf("Events removal error = %v, want generated App path", err)
	}
	if _, statErr := os.Stat(legacyPath); statErr != nil {
		t.Fatalf("failed preflight changed legacy owner path: %v", statErr)
	}
	if _, statErr := os.Stat(migratedPath); !os.IsNotExist(statErr) {
		t.Fatalf("failed preflight created migrated owner path: %v", statErr)
	}
	configAfter, readErr := os.ReadFile(filepath.Join(root, ".goforj.yml"))
	if readErr != nil {
		t.Fatalf("read project config after failure: %v", readErr)
	}
	if string(configAfter) != string(configBefore) {
		t.Fatalf("failed preflight rewrote project config\nbefore:\n%s\nafter:\n%s", configBefore, configAfter)
	}
}

// TestRenderAppOnlyProjectsEventsDriver verifies incremental App creation receives the active project transport without requiring a full render.
func TestRenderAppOnlyProjectsEventsDriver(t *testing.T) {
	root := useEventsRendererRoot(t)
	writeEventsRendererFile(t, filepath.Join(root, ".env"), "EVENTS_DRIVER=redis\nEVENTS_SUPPORTED_DRIVERS=inproc,redis\n")
	config := &project.Config{
		ProjectName:  "Events App",
		GoModuleName: "example.test/events-app",
		Render:       project.RenderConfig{Components: project.Components{CLI: true, Events: true}},
	}
	if err := writeProjectConfig(filepath.Join(root, ".goforj.yml"), config); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	renderer := NewProjectRenderer(logger.NewSilentLogger())
	app := project.DefaultNamedApp("events-worker")
	if err := renderer.RenderAppOnly(app, makeapp.RenderOptions{
		Components: project.Components{CLI: true, Events: true},
		SkipWire:   true,
	}); err != nil {
		t.Fatalf("render named Events App: %v", err)
	}

	environment, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatalf("read named App Events defaults: %v", err)
	}
	if !strings.Contains(string(environment), "EVENTS_WORKER_EVENTS_DRIVER=redis") {
		t.Fatalf("named App did not inherit active Events transport:\n%s", environment)
	}
}

// TestEventDriverDefaultFromEnvValidatesOwnerContract prevents incremental Apps from copying an invalid project Events selection.
func TestEventDriverDefaultFromEnvValidatesOwnerContract(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		want        string
		wantError   string
	}{
		{name: "valid portable contract", environment: "EVENTS_DRIVER=redis\nEVENTS_SUPPORTED_DRIVERS=inproc,redis\n", want: "redis"},
		{name: "unknown active driver", environment: "EVENTS_DRIVER=unknown\nEVENTS_SUPPORTED_DRIVERS=unknown\n", wantError: "unsupported driver"},
		{name: "active driver excluded", environment: "EVENTS_DRIVER=redis\nEVENTS_SUPPORTED_DRIVERS=inproc\n", wantError: "excludes active"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := useEventsRendererRoot(t)
			writeEventsRendererFile(t, filepath.Join(root, ".env"), test.environment)
			got, err := eventDriverDefaultFromEnv(".env")
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("Events driver error = %v, want %q", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolve Events driver: %v", err)
			}
			if got != test.want {
				t.Fatalf("Events driver = %q, want %q", got, test.want)
			}
		})
	}
}

// mappingDestExists reports whether a mapping set contains one normalized destination.
func mappingDestExists(mappings []templateMapping, want string) bool {
	want = filepath.Clean(want)
	for _, mapping := range mappings {
		if filepath.Clean(mapping.dest) == want {
			return true
		}
	}
	return false
}

// useEventsRendererRoot changes into an isolated render root for the duration of one test.
func useEventsRendererRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	return root
}

// writeEventsRendererFile writes one isolated owner fixture with its conventional parent directories.
func writeEventsRendererFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
}

// assertEventsRendererFileContents verifies an ownership-sensitive fixture remains byte-for-byte intact.
func assertEventsRendererFileContents(t *testing.T, path string, want string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	if string(contents) != want {
		t.Fatalf("fixture %s changed\nwant:\n%s\ngot:\n%s", path, want, contents)
	}
}

// mixedEventsRendererConfig keeps the project Events envelope active while the default App remains disabled.
func mixedEventsRendererConfig() *project.Config {
	return &project.Config{
		Render: project.RenderConfig{Components: project.Components{CLI: true}},
		Apps: map[string]project.AppConfig{
			"events-worker": {Components: project.Components{CLI: true, Events: true}},
		},
	}
}

// eventsMigrationConfig enables Events only for the App whose legacy owner is under test.
func eventsMigrationConfig(app project.App) *project.Config {
	if app.Name == "" || app.Name == project.DefaultAppName {
		return &project.Config{Render: project.RenderConfig{Components: project.Components{CLI: true, Events: true}}}
	}
	return &project.Config{
		Render: project.RenderConfig{Components: project.Components{CLI: true}},
		Apps: map[string]project.AppConfig{
			app.Name: {Components: project.Components{CLI: true, Events: true}},
		},
	}
}
