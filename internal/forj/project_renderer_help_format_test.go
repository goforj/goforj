package forj

import (
	"testing"

	"github.com/goforj/goforj/project"
)

func TestTemplateDataForAppUsesExternalCLIHelpFormatter(t *testing.T) {
	workspace := currentProjectRenderWorkspace(t)
	config := &project.Config{
		Render: project.RenderConfig{
			Components: project.Components{CLI: true},
			HelpFormat: project.HelpFormatExternalCLI,
		},
	}

	data := workspace.templateDataForApp(config, project.DefaultApp())
	if data.HelpFormatterFunc != "ExternalCLIFormatter" {
		t.Fatalf("HelpFormatterFunc = %q, want ExternalCLIFormatter", data.HelpFormatterFunc)
	}
	if data.HelpCommandFunc != "PrintExternalCLICommandHelp" {
		t.Fatalf("HelpCommandFunc = %q, want PrintExternalCLICommandHelp", data.HelpCommandFunc)
	}
}

func TestTemplateDataForAppUsesGuidedHelpFormatter(t *testing.T) {
	workspace := currentProjectRenderWorkspace(t)
	config := &project.Config{
		Render: project.RenderConfig{
			Components: project.Components{CLI: true},
			HelpFormat: project.HelpFormatGuided,
		},
	}

	data := workspace.templateDataForApp(config, project.DefaultApp())
	if data.HelpFormatterFunc != "GuidedFormatter" {
		t.Fatalf("HelpFormatterFunc = %q, want GuidedFormatter", data.HelpFormatterFunc)
	}
	if data.HelpCommandFunc != "PrintGuidedCommandHelp" {
		t.Fatalf("HelpCommandFunc = %q, want PrintGuidedCommandHelp", data.HelpCommandFunc)
	}
}

func TestTemplateDataForAppPreservesExternalCLIHelpFormatterWithOtherComponents(t *testing.T) {
	workspace := currentProjectRenderWorkspace(t)
	config := &project.Config{
		Render: project.RenderConfig{
			Components: project.Components{CLI: true, WebAPI: true},
			HelpFormat: project.HelpFormatExternalCLI,
		},
	}

	data := workspace.templateDataForApp(config, project.DefaultApp())
	if data.HelpFormatterFunc != "ExternalCLIFormatter" {
		t.Fatalf("HelpFormatterFunc = %q, want ExternalCLIFormatter", data.HelpFormatterFunc)
	}
	if data.HelpCommandFunc != "PrintExternalCLICommandHelp" {
		t.Fatalf("HelpCommandFunc = %q, want PrintExternalCLICommandHelp", data.HelpCommandFunc)
	}
}

func TestTemplateDataForAppSeparatesAppSelectionFromProjectEnvelope(t *testing.T) {
	workspace := currentProjectRenderWorkspace(t)
	config := &project.Config{
		Render: project.RenderConfig{Components: project.Components{CLI: true, WebAPI: true, DatabaseMySQL: true}},
		Apps: map[string]project.AppConfig{
			"worker": {Components: project.Components{CLI: true, Jobs: true}},
		},
	}

	defaultData := workspace.templateDataForApp(config, project.DefaultApp())
	if defaultData.Components.Jobs {
		t.Fatalf("default App inherited named-App jobs: %#v", defaultData.Components)
	}
	if !defaultData.ProjectComponents.Jobs || !defaultData.ProjectComponents.WebAPI {
		t.Fatalf("project envelope omitted App capabilities: %#v", defaultData.ProjectComponents)
	}

	workerData := workspace.templateDataForApp(config, project.DefaultNamedApp("worker"))
	if !workerData.Components.Jobs || workerData.Components.WebAPI || workerData.Components.HasDatabase() {
		t.Fatalf("worker App components leaked from the default App: %#v", workerData.Components)
	}
	if workerData.ProjectComponents != defaultData.ProjectComponents {
		t.Fatalf("App templates received different project envelopes: default=%#v worker=%#v", defaultData.ProjectComponents, workerData.ProjectComponents)
	}
}

// TestSetAppConfigDoesNotPromoteNamedCapabilitiesIntoDefaultApp verifies make:app persists only App-local participation.
func TestSetAppConfigDoesNotPromoteNamedCapabilitiesIntoDefaultApp(t *testing.T) {
	defaultComponents := project.Components{CLI: true, WebAPI: true, DatabaseMySQL: true, Docker: true}
	renderer := &ProjectRenderer{workspace: currentProjectRenderWorkspace(t), config: &project.Config{
		Render: project.RenderConfig{Components: defaultComponents},
	}}

	changed, err := renderer.setAppConfig(
		"reporting",
		project.Components{CLI: true, WebAPI: true, DatabasePostgres: true, Jobs: true},
		project.StarterKitNone,
		project.HelpFormatFramework,
	)
	if err != nil {
		t.Fatalf("setAppConfig returned error: %v", err)
	}
	if !changed {
		t.Fatal("named App did not report a widened project envelope")
	}
	if renderer.config.Render.Components != defaultComponents {
		t.Fatalf("named App changed the default App selection: %#v", renderer.config.Render.Components)
	}
	envelope := project.ProjectComponents(renderer.config)
	if !envelope.DatabaseMySQL || !envelope.DatabasePostgres || !envelope.Jobs {
		t.Fatalf("derived project envelope omitted named-App support: %#v", envelope)
	}
}

// TestSetAppConfigNormalizesImplicitDatabaseAgainstDefaultApp verifies an existing sibling cannot choose a new App's database.
func TestSetAppConfigNormalizesImplicitDatabaseAgainstDefaultApp(t *testing.T) {
	renderer := &ProjectRenderer{workspace: currentProjectRenderWorkspace(t), config: &project.Config{
		Render: project.RenderConfig{Components: project.Components{CLI: true}},
		Apps: map[string]project.AppConfig{
			"reporting": {Components: project.Components{CLI: true, DatabasePostgres: true}},
		},
	}}

	changed, err := renderer.setAppConfig(
		"accounts",
		project.Components{CLI: true, Auth: true},
		project.StarterKitNone,
		project.HelpFormatFramework,
	)
	if err != nil {
		t.Fatalf("setAppConfig returned error: %v", err)
	}
	if !changed {
		t.Fatal("accounts App did not widen the shared database support")
	}
	accounts := renderer.config.Apps["accounts"].Components
	if !accounts.DatabaseMySQL || accounts.DatabasePostgres {
		t.Fatalf("persisted accounts database leaked from reporting App: %#v", accounts)
	}
	rendered := appRenderComponents(renderer.config, project.DefaultNamedApp("accounts"))
	if rendered.DatabaseMySQL != accounts.DatabaseMySQL || rendered.DatabasePostgres != accounts.DatabasePostgres {
		t.Fatalf("rendered accounts components = %#v, want persisted selection %#v", rendered, accounts)
	}
	envelope := project.ProjectComponents(renderer.config)
	if !envelope.DatabaseMySQL || !envelope.DatabasePostgres {
		t.Fatalf("shared database envelope = %#v, want MySQL and Postgres", envelope)
	}
}

// TestAppRenderComponentsResolvesDefaultAppDependencies keeps direct App projections aligned with full renders.
func TestAppRenderComponentsResolvesDefaultAppDependencies(t *testing.T) {
	config := &project.Config{
		Render: project.RenderConfig{
			Components: project.Components{Auth: true},
		},
	}

	components := appRenderComponents(config, project.DefaultApp())
	if !components.Cache {
		t.Fatalf("default App dependencies omitted Cache: %#v", components)
	}
	if config.Render.Components.Cache {
		t.Fatalf("default App projection mutated persisted components: %#v", config.Render.Components)
	}
}
