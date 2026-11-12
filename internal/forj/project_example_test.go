package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
)

// TestGopherEatsRerenderPreservesExampleSources keeps the living example editable after its initial scaffold.
func TestGopherEatsRerenderPreservesExampleSources(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "gopher-eats-preserve-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	renderer := NewProjectRenderer(logger.NewSilentLogger())
	if err := renderer.beginRenderInvocation(root); err != nil {
		t.Fatal(err)
	}
	renderer.config = &project.Config{GoModuleName: "example.com/eats"}
	configureGopherEats(renderer.config)
	renderer.resources.plan, err = project.DefaultResourcePlan(project.ProjectComponents(renderer.config))
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []func() error{renderer.scaffoldGopherEats, renderer.scaffoldDefaultStarterKit, renderer.scaffoldGopherEatsFrontend} {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"internal/eats/service.go", "cmd/app/frontend/src/App.vue", "cmd/app/frontend/src/eats/model.ts"} {
		if err := os.WriteFile(filepath.Join(root, path), []byte("owner change"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, step := range []func() error{renderer.scaffoldGopherEats, renderer.scaffoldDefaultStarterKit, renderer.scaffoldGopherEatsFrontend} {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"internal/eats/service.go", "cmd/app/frontend/src/App.vue", "cmd/app/frontend/src/eats/model.ts"} {
		body, err := os.ReadFile(filepath.Join(root, path))
		if err != nil || string(body) != "owner change" {
			t.Fatalf("%s was overwritten: %v", path, err)
		}
	}
}

// TestGopherEatsRenderRejectsInvalidProfiles covers render-only configuration validation before file publication.
func TestGopherEatsRenderRejectsInvalidProfiles(t *testing.T) {
	for _, variant := range []string{"unknown", "conflict", "missing capability", "wrong kit", "storefront api", "kitchen api", "delivery api"} {
		config := &project.Config{}
		configureGopherEats(config)
		switch variant {
		case "unknown":
			config.Render.Example = "unknown"
		case "conflict":
			config.Render.Components.DemoApp = true
		case "missing capability":
			config.Render.Components.WebUI = false
		case "wrong kit":
			config.Render.StarterKit = project.StarterKitNone
		case "storefront api":
			config.Render.Components.WebAPI = false
		case "kitchen api":
			app := config.Apps["kitchen"]
			app.Components.WebAPI = false
			config.Apps["kitchen"] = app
		case "delivery api":
			app := config.Apps["delivery"]
			app.Components.WebAPI = false
			config.Apps["delivery"] = app
		}
		renderer := NewProjectRenderer(logger.NewSilentLogger())
		renderer.config = config
		if err := renderer.prepareExample(&ComponentRenderInput{}); err == nil {
			t.Fatalf("accepted %s", variant)
		}
	}
}

// TestGopherEatsKeepsCustomTeardown preserves owner lifecycle commands while adding missing demo startup defaults.
func TestGopherEatsKeepsCustomTeardown(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "gopher-eats-lifecycle-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	renderer := NewProjectRenderer(logger.NewSilentLogger())
	if err := renderer.beginRenderInvocation(root); err != nil {
		t.Fatal(err)
	}
	renderer.config = &project.Config{}
	configureGopherEats(renderer.config)
	renderer.config.Dev.Down = []project.DevTask{{Name: "owner teardown", Cmd: "echo done"}}
	if err := renderer.prepareExample(&ComponentRenderInput{}); err != nil {
		t.Fatal(err)
	}
	if len(renderer.config.Dev.Down) != 1 || renderer.config.Dev.Down[0].Name != "owner teardown" {
		t.Fatal("owner teardown replaced")
	}
}

// TestGopherEatsCreationKeepsVueAndSeparateAppRoles covers the selectable example's actual authoring contract.
func TestGopherEatsCreationKeepsVueAndSeparateAppRoles(t *testing.T) {
	command := validCreationCommand(t)
	command.Example = "gopher_eats"
	model, err := command.noninteractiveModel()
	if err != nil {
		t.Fatal(err)
	}
	if model.config.Render.StarterKit != project.StarterKitVue || !model.config.Render.Components.WebUI || model.config.Render.Components.Jobs || model.config.Render.Components.DemoApp {
		t.Fatalf("storefront: %+v", model.config.Render)
	}
	if !model.config.Apps["kitchen"].Components.Jobs || model.config.Apps["delivery"].Components.Jobs || !model.config.Apps["delivery"].Components.Events {
		t.Fatalf("app roles: %+v", model.config.Apps)
	}
	preparation, err := model.selectedResourcePreparation()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []project.ResourceKey{project.ResourceQueue, project.ResourceEvents} {
		selection, exists := preparation.plan.Selection(key)
		if !exists || selection.Active != "nats" {
			t.Fatalf("%s selection: %+v", key, selection)
		}
	}
	model.extrasIndex = 0
	model.applyExtrasSelection()
	if len(model.config.Apps) != 0 || model.config.Render.Example != "" {
		t.Fatal("disabled example retained extra apps")
	}
}

// TestGopherEatsRejectsConflictingAndUnknownExamples protects both creation validation branches.
func TestGopherEatsRejectsConflictingAndUnknownExamples(t *testing.T) {
	for _, example := range []string{"unknown", "gopher_eats"} {
		command := validCreationCommand(t)
		command.Example = example
		command.Demo = example == "gopher_eats"
		if _, err := command.noninteractiveModel(); err == nil || !strings.Contains(err.Error(), "example") {
			t.Fatalf("example=%q: %v", example, err)
		}
	}
}

// TestGopherEatsWizardShowsOneSelectedDemo checks the actual rendered radio choices and review summary.
func TestGopherEatsWizardShowsOneSelectedDemo(t *testing.T) {
	for index, label := range []string{"Off", "Uptime Gopher", "Gopher Eats"} {
		m := initialModel()
		m.stage = StageExtras
		m.termWidth = wizardWidth
		m.extrasIndex = index
		m.applyExtrasSelection()
		if count := strings.Count(m.View().Content, "●"); count != 1 {
			t.Fatalf("choice %d shows %d selected markers", index, count)
		}
		if !strings.Contains(m.selectedDemoSummary(), label) {
			t.Fatalf("choice %d summary=%s", index, m.selectedDemoSummary())
		}
	}
}
