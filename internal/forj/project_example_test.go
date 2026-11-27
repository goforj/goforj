package forj

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
	"github.com/goforj/goforj/internal/envfile"
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
	wizard, err := command.noninteractiveModel()
	if err != nil {
		t.Fatal(err)
	}
	if wizard.config.Render.StarterKit != project.StarterKitVue || !wizard.config.Render.Components.WebUI || wizard.config.Render.Components.Jobs || wizard.config.Render.Components.DemoApp {
		t.Fatalf("storefront: %+v", wizard.config.Render)
	}
	if !wizard.config.Apps["kitchen"].Components.Jobs || wizard.config.Apps["delivery"].Components.Jobs || !wizard.config.Apps["delivery"].Components.Events {
		t.Fatalf("app roles: %+v", wizard.config.Apps)
	}
	preparation, err := wizard.selectedResourcePreparation()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []project.ResourceKey{project.ResourceQueue, project.ResourceEvents} {
		selection, exists := preparation.plan.Selection(key)
		if !exists || selection.Active != "nats" {
			t.Fatalf("%s selection: %+v", key, selection)
		}
	}
	assertGopherEatsLocalNATS(t, project.ProjectComponents(&wizard.config), preparation.plan, preparation.serviceIntent)
	wizard.stage = StageProjectPath
	next, _ := wizard.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wizard = next.(model)
	if wizard.stage != StageConfirm {
		t.Fatalf("Gopher Eats did not reach confirmation: %s", wizard.errorMsg)
	}
	preparation, err = wizard.selectedResourcePreparation()
	if err != nil {
		t.Fatal(err)
	}
	assertGopherEatsLocalNATS(t, project.ProjectComponents(&wizard.config), preparation.plan, preparation.serviceIntent)
	wizard.extrasIndex = 0
	wizard.applyExtrasSelection()
	if len(wizard.config.Apps) != 0 || wizard.config.Render.Example != "" {
		t.Fatal("disabled example retained extra apps")
	}
}

// TestGopherEatsRenderSelectsLocalNATS covers direct rendering without wizard resource preparation.
func TestGopherEatsRenderSelectsLocalNATS(t *testing.T) {
	renderer := unitProjectRenderer(t)
	if err := renderer.beginRenderInvocation(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	renderer.config = &project.Config{GoModuleName: "example.com/eats", Render: project.RenderConfig{Example: "gopher_eats"}}
	input := ComponentRenderInput{}
	if err := renderer.prepareExample(&input); err != nil {
		t.Fatal(err)
	}
	assertGopherEatsLocalNATS(t, project.ProjectComponents(renderer.config), input.resourcePlan, input.localServiceIntent)
}

// TestGopherEatsPreservesServicePlacement keeps explicit external brokers and existing owner configuration intact.
func TestGopherEatsPreservesServicePlacement(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing=%t", existing), func(t *testing.T) {
			root := t.TempDir()
			renderer := unitProjectRenderer(t)
			if err := renderer.beginRenderInvocation(root); err != nil {
				t.Fatal(err)
			}
			renderer.config = &project.Config{GoModuleName: "example.com/eats", Render: project.RenderConfig{Example: "gopher_eats"}}
			source := "EVENTS_URL=nats://broker.example:4222\nCOMPOSE_PROFILES=\n"
			if existing {
				if err := os.WriteFile(filepath.Join(root, ".env"), []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
			}
			intent := project.LocalServiceIntent{}.WithMode(project.ServiceEventsNATS, project.LocalServiceModeExternal)
			input := ComponentRenderInput{localServiceIntent: intent}
			if err := renderer.prepareExample(&input); err != nil {
				t.Fatal(err)
			}
			if mode, _ := input.localServiceIntent.Mode(project.ServiceEventsNATS); mode != project.LocalServiceModeExternal {
				t.Fatal("external Events broker was replaced")
			}
			if _, selected := intent.Mode(project.ServiceQueueNATS); selected {
				t.Fatal("caller service intent was mutated")
			}
			mode, selected := input.localServiceIntent.Mode(project.ServiceQueueNATS)
			if existing && (selected || len(input.resourcePlan.Selections) != 0) {
				t.Fatal("existing owner configuration received fresh demo defaults")
			}
			if !existing && (!selected || mode != project.LocalServiceModeLocal) {
				t.Fatal("fresh demo did not default its kitchen queue to local NATS")
			}
			if existing {
				body, err := os.ReadFile(filepath.Join(root, ".env"))
				if err != nil || string(body) != source {
					t.Fatalf("owner environment changed: %v", err)
				}
			}
		})
	}
}

// assertGopherEatsLocalNATS checks service activation and both container and host connection details together.
func assertGopherEatsLocalNATS(t *testing.T, components project.Components, plan project.ResourcePlan, intent project.LocalServiceIntent) {
	t.Helper()
	services, err := project.ResolveServicePlan(plan, components, intent)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []project.ServiceKey{project.ServiceQueueNATS, project.ServiceEventsNATS} {
		requirement, exists := services.Requirement(key)
		if !exists || requirement.State != project.ServiceStateActiveLocal {
			t.Fatalf("%s service = %#v, exists=%t; want active local NATS", key, requirement, exists)
		}
	}
	environment, compose := renderResourceTemplates(t, components, plan, intent)
	profiles, _ := envfile.Lookup(strings.Split(environment, "\n"), "COMPOSE_PROFILES")
	if !exactCSVToken(profiles, "nats") || !strings.Contains(compose, "profiles: [nats]") {
		t.Fatalf("NATS was not activated in generated Compose: profiles=%q", profiles)
	}
	host := renderResourceHostEnvironment(t, components, plan, intent)
	for _, key := range []string{"QUEUE_URL", "EVENTS_URL"} {
		if !strings.Contains(environment, key+"=nats://goforj:goforj@nats:4222\n") || !strings.Contains(host, key+"=nats://goforj:goforj@localhost:4222\n") {
			t.Fatalf("%s does not match the authenticated local NATS service", key)
		}
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
