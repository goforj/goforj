package forj

import (
	"testing"

	"github.com/goforj/goforj/project"
)

func TestTemplateDataForAppUsesExternalCLIHelpFormatter(t *testing.T) {
	config := &project.Config{
		Render: project.RenderConfig{
			Components: project.Components{CLI: true},
			HelpFormat: project.HelpFormatExternalCLI,
		},
	}

	data := templateDataForApp(config, project.DefaultApp())
	if data.HelpFormatterFunc != "ExternalCLIFormatter" {
		t.Fatalf("HelpFormatterFunc = %q, want ExternalCLIFormatter", data.HelpFormatterFunc)
	}
	if data.HelpCommandFunc != "PrintExternalCLICommandHelp" {
		t.Fatalf("HelpCommandFunc = %q, want PrintExternalCLICommandHelp", data.HelpCommandFunc)
	}
}

func TestTemplateDataForAppUsesGuidedHelpFormatter(t *testing.T) {
	config := &project.Config{
		Render: project.RenderConfig{
			Components: project.Components{CLI: true},
			HelpFormat: project.HelpFormatGuided,
		},
	}

	data := templateDataForApp(config, project.DefaultApp())
	if data.HelpFormatterFunc != "GuidedFormatter" {
		t.Fatalf("HelpFormatterFunc = %q, want GuidedFormatter", data.HelpFormatterFunc)
	}
	if data.HelpCommandFunc != "PrintGuidedCommandHelp" {
		t.Fatalf("HelpCommandFunc = %q, want PrintGuidedCommandHelp", data.HelpCommandFunc)
	}
}

func TestTemplateDataForAppPreservesExternalCLIHelpFormatterWithOtherComponents(t *testing.T) {
	config := &project.Config{
		Render: project.RenderConfig{
			Components: project.Components{CLI: true, WebAPI: true},
			HelpFormat: project.HelpFormatExternalCLI,
		},
	}

	data := templateDataForApp(config, project.DefaultApp())
	if data.HelpFormatterFunc != "ExternalCLIFormatter" {
		t.Fatalf("HelpFormatterFunc = %q, want ExternalCLIFormatter", data.HelpFormatterFunc)
	}
	if data.HelpCommandFunc != "PrintExternalCLICommandHelp" {
		t.Fatalf("HelpCommandFunc = %q, want PrintExternalCLICommandHelp", data.HelpCommandFunc)
	}
}
