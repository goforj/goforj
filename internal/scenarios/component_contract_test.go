package scenarios

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// TestWriteScenarioProjectConfigUsesSequenceSemantics verifies scenario fixtures retain intentionally absent primitives without a marker.
func TestWriteScenarioProjectConfigUsesSequenceSemantics(t *testing.T) {
	root := t.TempDir()
	spec := ScenarioSpec{
		Title: "Lean Scenario",
		App: ScenarioApp{
			ModuleName: "example.com/lean-scenario",
			Components: project.Components{CLI: true},
		},
	}

	if err := writeScenarioProjectConfig(root, spec); err != nil {
		t.Fatalf("write scenario config: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(root, ".goforj.yml"))
	if err != nil {
		t.Fatalf("read scenario config: %v", err)
	}
	if strings.Contains(string(source), "component_contract:") {
		t.Fatalf("scenario config persisted the obsolete component marker:\n%s", source)
	}
	if !strings.Contains(string(source), "components: [cli]") {
		t.Fatalf("scenario config omitted the canonical component sequence:\n%s", source)
	}

	var loaded project.Config
	if err := yaml.Unmarshal(source, &loaded); err != nil {
		t.Fatalf("reload scenario config: %v", err)
	}
	if loaded.Render.Components.Cache || loaded.Render.Components.Events || loaded.Render.Components.Storage || loaded.Render.Components.Jobs {
		t.Fatalf("scenario primitive opt-outs changed after reload: %#v", loaded.Render.Components)
	}
}
