package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestPrimitiveRendererMigrations verifies compatibility migrations preserve owner code and reject collisions.
func TestPrimitiveRendererMigrations(t *testing.T) {
	testPrimitiveLegacyOwners(t)
}

// testPrimitiveLegacyOwners retains byte-for-byte migrations, collision preflights, and surgical Events compatibility repairs.
func testPrimitiveLegacyOwners(t *testing.T) {
	t.Run("preserved migrations", testPrimitivePreservedOwnerMigrations)
	t.Run("collision preflight", testPrimitiveOwnerMigrationCollisions)
	t.Run("Events subscriber set rename", testEventSubscriberSetRename)
	t.Run("Events structured compatibility", testEventStructuredCompatibility)
	t.Run("Jobs framework cleanup", testJobsLegacyFrameworkCleanup)
}

// testPrimitivePreservedOwnerMigrations verifies historical owner paths move without content changes.
func testPrimitivePreservedOwnerMigrations(t *testing.T) {
	tests := []struct {
		name   string
		config *project.Config
		source string
		target string
	}{
		{
			name:   "Events default App",
			config: primitiveRendererConfig(primitiveComponentsWith(project.ComponentEvents)),
			source: filepath.Join("wire", "inject_event_subscribers.go"),
			target: eventSubscriberOwnerPath(project.DefaultApp()),
		},
		{
			name: "Events named App",
			config: &project.Config{
				Render: project.RenderConfig{Components: primitiveRendererBaseComponents()},
				Apps:   map[string]project.AppConfig{"worker": {Components: primitiveComponentsWith(project.ComponentEvents)}},
			},
			source: filepath.Join(project.DefaultNamedApp("worker").WireDir, "inject_event_subscribers.go"),
			target: eventSubscriberOwnerPath(project.DefaultNamedApp("worker")),
		},
		{
			name:   "Jobs default App",
			config: primitiveRendererConfig(primitiveComponentsWith(project.ComponentJobs)),
			source: filepath.Join("wire", "inject_jobs_app.go"),
			target: filepath.Join(project.DefaultApp().WireDir, "inject_jobs_app.go"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			usePrimitiveRendererRoot(t)
			contents := "package wire\n\n// OwnerSentinel must survive migration.\nvar ownerSentinel = true\n"
			writePrimitiveRendererFile(t, test.source, contents)
			renderer := &ProjectRenderer{
				config:    test.config,
				workspace: currentProjectRenderWorkspace(t),
			}
			if err := renderer.migrateAppOwnedWireFilenames(); err != nil {
				t.Fatalf("migrate %s owner: %v", test.name, err)
			}
			if _, err := os.Stat(test.source); !os.IsNotExist(err) {
				t.Fatalf("legacy owner %s remains: %v", test.source, err)
			}
			if got := readPrimitiveRendererFile(t, test.target); got != contents {
				t.Fatalf("migrated owner %s changed", test.target)
			}
		})
	}
}

// testPrimitiveOwnerMigrationCollisions verifies competing owner files are discovered before either file moves.
func testPrimitiveOwnerMigrationCollisions(t *testing.T) {
	tests := []struct {
		name            string
		config          *project.Config
		source          string
		target          string
		unrelatedSource string
		unrelatedTarget string
	}{
		{
			name:   "Events",
			config: primitiveRendererConfig(primitiveComponentsWith(project.ComponentEvents)),
			source: filepath.Join("wire", "inject_event_subscribers.go"),
			target: eventSubscriberOwnerPath(project.DefaultApp()),
		},
		{
			name:            "Jobs",
			config:          primitiveRendererConfig(primitiveComponentsWith(project.ComponentEvents, project.ComponentJobs)),
			source:          filepath.Join("wire", "inject_jobs_app.go"),
			target:          filepath.Join(project.DefaultApp().WireDir, "inject_jobs_app.go"),
			unrelatedSource: filepath.Join(project.DefaultApp().WireDir, "inject_event_subscribers.go"),
			unrelatedTarget: eventSubscriberOwnerPath(project.DefaultApp()),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			usePrimitiveRendererRoot(t)
			sourceContents := "package wire\n\nvar legacyOwner = true\n"
			targetContents := "package wire\n\nvar currentOwner = true\n"
			unrelatedContents := "package wire\n\nvar unrelatedOwner = true\n"
			writePrimitiveRendererFile(t, test.source, sourceContents)
			writePrimitiveRendererFile(t, test.target, targetContents)
			if test.unrelatedSource != "" {
				writePrimitiveRendererFile(t, test.unrelatedSource, unrelatedContents)
			}
			renderer := &ProjectRenderer{
				config:    test.config,
				workspace: currentProjectRenderWorkspace(t),
			}
			err := renderer.migrateAppOwnedWireFilenames()
			if err == nil || !strings.Contains(err.Error(), test.source) || !strings.Contains(err.Error(), test.target) {
				t.Fatalf("%s collision error = %v", test.name, err)
			}
			if got := readPrimitiveRendererFile(t, test.source); got != sourceContents {
				t.Fatalf("%s collision changed legacy owner", test.name)
			}
			if got := readPrimitiveRendererFile(t, test.target); got != targetContents {
				t.Fatalf("%s collision changed current owner", test.name)
			}
			if test.unrelatedSource != "" {
				if got := readPrimitiveRendererFile(t, test.unrelatedSource); got != unrelatedContents {
					t.Fatalf("%s collision changed unrelated Events owner", test.name)
				}
				if _, err := os.Stat(test.unrelatedTarget); !os.IsNotExist(err) {
					t.Fatalf("%s collision created unrelated Events owner: %v", test.name, err)
				}
			}
		})
	}
}

// testEventSubscriberSetRename verifies identifier repair changes code references without rewriting comments or strings.
func testEventSubscriberSetRename(t *testing.T) {
	usePrimitiveRendererRoot(t)
	path := eventSubscriberOwnerPath(project.DefaultApp())
	contents := `package wire

// eventSubscriberSet remains here as migration documentation.
var eventSubscriberSet = wire.NewSet(NewCustomSubscriber)
var historicalName = "eventSubscriberSet"

func currentSubscriberSet() any { return eventSubscriberSet }
`
	writePrimitiveRendererFile(t, path, contents)
	renderer := &ProjectRenderer{
		config:    primitiveRendererConfig(primitiveComponentsWith(project.ComponentEvents)),
		workspace: currentProjectRenderWorkspace(t),
	}
	if err := renderer.migrateAppOwnedWireFilenames(); err != nil {
		t.Fatalf("repair Events subscriber set: %v", err)
	}
	got := readPrimitiveRendererFile(t, path)
	for _, want := range []string{"var appSubscriberSet =", "return appSubscriberSet", "// eventSubscriberSet", `"eventSubscriberSet"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("repaired Events owner omitted %q:\n%s", want, got)
		}
	}
}

// testEventStructuredCompatibility verifies real declarations are detected while ambiguous App-owned providers remain untouched.
func testEventStructuredCompatibility(t *testing.T) {
	t.Run("detect declarations", func(t *testing.T) {
		usePrimitiveRendererRoot(t)
		app := project.DefaultApp()
		commandsPath := filepath.Join(app.AppDir, "commands.go")
		wiringPath := filepath.Join(app.WireDir, "inject_cmd_app.go")
		writePrimitiveRendererFile(t, commandsPath, `package app

import "example.test/internal/cmd"

// cmd.TestEventPipelineCmd is documentation only.
type Commands struct { Pipeline cmd.TestEventPipelineCmd }
`)
		writePrimitiveRendererFile(t, wiringPath, `package wire

var appCommandSet = wire.NewSet(cmd.NewTestEventPipelineCmd)
`)
		workspace := currentProjectRenderWorkspace(t)
		if !workspace.legacyEventPipelineField(app) || !workspace.legacyEventPipelineProvider(app) {
			t.Fatal("structured Events compatibility declarations were not detected")
		}
	})

	t.Run("preserve ambiguous providers", func(t *testing.T) {
		usePrimitiveRendererRoot(t)
		app := project.DefaultApp()
		servicePath := filepath.Join(app.WireDir, "inject_services_app.go")
		serviceContents := `package wire

func CustomProvider() any { return nil }
func provideSharedRedisClient() any { return nil }
`
		writePrimitiveRendererFile(t, servicePath, serviceContents)
		config := &project.Config{
			Render: project.RenderConfig{Components: primitiveRendererBaseComponents()},
			Apps:   map[string]project.AppConfig{"worker": {Components: primitiveComponentsWith(project.ComponentEvents)}},
		}
		renderer := &ProjectRenderer{
			config:    config,
			workspace: currentProjectRenderWorkspace(t),
		}
		err := renderer.validateEventsRenderTransition(project.ProjectComponents(config))
		if err == nil || !strings.Contains(err.Error(), servicePath) {
			t.Fatalf("ambiguous Events provider error = %v", err)
		}
		if got := readPrimitiveRendererFile(t, servicePath); got != serviceContents {
			t.Fatal("Events compatibility preflight changed App-owned providers")
		}
	})
}

// testJobsLegacyFrameworkCleanup verifies obsolete generated files remain cleanup-owned while custom Jobs source survives.
func testJobsLegacyFrameworkCleanup(t *testing.T) {
	usePrimitiveRendererRoot(t)
	for _, path := range legacyJobsFrameworkPaths() {
		writePrimitiveRendererFile(t, path, "package jobs\n")
	}
	customPath := filepath.Join("internal", "jobs", "reports", "custom_job.go")
	customContents := "package reports\n\nvar customJobSentinel = true\n"
	writePrimitiveRendererFile(t, customPath, customContents)
	writePrimitiveRendererFile(t, filepath.Join("project", "config.go"), "package project\n")
	writePrimitiveRendererFile(t, filepath.Join("internal", "lighthouse", "project_config_patch.go"), "package lighthouse\n")
	components := primitiveComponentsWith(project.ComponentJobs)
	renderer := &ProjectRenderer{
		config:    primitiveRendererConfig(components),
		workspace: currentProjectRenderWorkspace(t),
	}
	if err := renderer.validateJobsRenderTransition(components); err != nil {
		t.Fatalf("legacy Jobs framework files blocked enabled rerender: %v", err)
	}
	if err := renderer.cleanupLegacyGeneratedFiles(); err != nil {
		t.Fatalf("clean legacy Jobs framework files: %v", err)
	}
	for _, path := range legacyJobsFrameworkPaths() {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("legacy Jobs framework file %s remains: %v", path, err)
		}
	}
	if got := readPrimitiveRendererFile(t, customPath); got != customContents {
		t.Fatalf("custom Jobs source changed: %q", got)
	}
}
