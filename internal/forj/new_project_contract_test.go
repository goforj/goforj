package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/konghelp"
	"github.com/goforj/goforj/project"
)

// TestProjectCreationSchemaUsesCanonicalCatalogs keeps terminal and graphical choices aligned.
func TestProjectCreationSchemaUsesCanonicalCatalogs(t *testing.T) {
	schema := projectCreationSchema()
	if schema.SchemaVersion != 1 {
		t.Fatal(schema.SchemaVersion)
	}
	for _, field := range schema.Fields {
		if field.Key == "components" && len(field.Options) != len(project.ProjectWizardComponentDefinitions()) {
			t.Fatal("component catalogs differ")
		}
		if field.Key == "starter-kit" && len(field.Options) != len(project.StarterKitCatalog()) {
			t.Fatal("starter catalogs differ")
		}
		if field.Key == "help-format" {
			if len(field.Options) != len(project.HelpFormatCatalog()) {
				t.Fatal("help format catalogs differ")
			}
			for _, option := range field.Options {
				if option.Preview == "" || option.Preview != konghelp.Preview(option.Value) {
					t.Fatalf("help format %s does not expose its actual formatter preview", option.Value)
				}
			}
		}
	}
}

// validCreationCommand provides explicit defaults without invoking a renderer or creating files.
func validCreationCommand(t *testing.T) NewProjectCmd {
	t.Helper()
	return NewProjectCmd{Name: "Sample", Module: "example.com/sample", Path: filepath.Join(t.TempDir(), "sample"), Components: "cli,web_api,web_ui", StarterKit: "vue", ComponentLibrary: false, HelpFormat: "guided", Atlas: "skip", AtlasAgents: "@default", AtlasSurfaces: "@default"}
}

// TestNoninteractiveProjectModelValidatesBeforeWriting exercises every argument validation boundary.
func TestNoninteractiveProjectModelValidatesBeforeWriting(t *testing.T) {
	cases := []struct {
		name   string
		change func(*NewProjectCmd)
		want   string
	}{
		{"empty name", func(c *NewProjectCmd) { c.Name = "" }, "project name"},
		{"control name", func(c *NewProjectCmd) { c.Name = "bad\nname" }, "project name"},
		{"long name", func(c *NewProjectCmd) { c.Name = strings.Repeat("a", 65) }, "project name"},
		{"empty path", func(c *NewProjectCmd) { c.Path = "" }, "--path"},
		{"bad module", func(c *NewProjectCmd) { c.Module = "https://example.com/app" }, "without a URL scheme"},
		{"unknown component", func(c *NewProjectCmd) { c.Components = "unknown" }, "unknown component"},
		{"multiple databases", func(c *NewProjectCmd) { c.Components = "database_mysql,database_postgres" }, "mutually exclusive"},
		{"unknown kit", func(c *NewProjectCmd) { c.StarterKit = "unknown" }, "unknown starter-kit"},
		{"unknown format", func(c *NewProjectCmd) { c.HelpFormat = "unknown" }, "unknown help-format"},
		{"unknown agent", func(c *NewProjectCmd) { c.Atlas = "custom"; c.AtlasAgents = "unknown" }, "unknown atlas agent"},
		{"unknown integration", func(c *NewProjectCmd) { c.Atlas = "custom"; c.AtlasSurfaces = "unknown" }, "unknown atlas integration"},
		{"empty agents", func(c *NewProjectCmd) { c.Atlas = "custom"; c.AtlasAgents = "" }, "unknown atlas agent"},
		{"empty integrations", func(c *NewProjectCmd) { c.Atlas = "custom"; c.AtlasSurfaces = "" }, "unknown atlas integration"},
		{"custom arguments without preset", func(c *NewProjectCmd) { c.AtlasAgents = "codex" }, "require --atlas=custom"},
		{"unknown atlas", func(c *NewProjectCmd) { c.Atlas = "unknown" }, "unknown atlas"},
		{"kit requires UI", func(c *NewProjectCmd) { c.Components = "cli" }, "requires web_ui"},
		{"occupied target", func(c *NewProjectCmd) {
			if err := os.MkdirAll(c.Path, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(c.Path, "keep.txt"), []byte("keep"), 0600); err != nil {
				t.Fatal(err)
			}
		}, "not empty"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			c := validCreationCommand(t)
			test.change(&c)
			_, err := c.noninteractiveModel()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v, want %s", err, test.want)
			}
			if _, err := os.Stat(filepath.Join(c.Path, ".goforj.yml")); !os.IsNotExist(err) {
				t.Fatalf("validation wrote configuration: %v", err)
			}
		})
	}
}

// TestNoninteractiveProjectModelSharesWizardFinalization proves dependencies, starter options, and generated tasks use the existing path.
func TestNoninteractiveProjectModelSharesWizardFinalization(t *testing.T) {
	c := validCreationCommand(t)
	c.Components = "oauth,web_ui"
	m, err := c.noninteractiveModel()
	if err != nil {
		t.Fatal(err)
	}
	if m.stage != StageDone || !m.config.Render.Components.Auth || !m.config.Render.Components.WebAPI {
		t.Fatalf("incomplete model: %+v", m.config.Render)
	}
	if m.config.Render.StarterKit != project.StarterKitVue || m.config.Render.HelpFormat != project.HelpFormatGuided {
		t.Fatal("creation ignored selected options")
	}
	if m.config.Render.StarterKitOptions == nil {
		t.Fatal("component-library=false was lost")
	}
	c.Demo = true
	m, err = c.noninteractiveModel()
	if err != nil {
		t.Fatal(err)
	}
	if !m.config.Render.Components.DemoApp || !m.config.Render.Components.DatabaseMySQL || m.config.Render.StarterKit != project.StarterKitNone {
		t.Fatal("demo constraints diverged")
	}
}

// TestCreationCustomAtlasSharesWizardSelections verifies explicit choices reach the existing installer model.
func TestCreationCustomAtlasSharesWizardSelections(t *testing.T) {
	c := validCreationCommand(t)
	c.Atlas, c.AtlasAgents, c.AtlasSurfaces = "custom", "codex", "guidelines,skills"
	m, err := c.noninteractiveModel()
	if err != nil {
		t.Fatal(err)
	}
	if agents := m.selectedAtlasAgents(); len(agents) != 1 || agents[0] != "codex" {
		t.Fatal(agents)
	}
	surfaces := m.selectedAtlasSurfaces()
	if !surfaces.guidelines || !surfaces.skills || surfaces.mcp {
		t.Fatal(surfaces)
	}
}
