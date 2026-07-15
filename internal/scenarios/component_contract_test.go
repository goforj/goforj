package scenarios

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// TestWriteScenarioProjectConfigPersistsCurrentComponentContract verifies scenario fixtures retain intentionally absent primitives.
func TestWriteScenarioProjectConfigPersistsCurrentComponentContract(t *testing.T) {
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
	if !strings.Contains(string(source), "component_contract: 1") {
		t.Fatalf("scenario config omitted the component contract marker:\n%s", source)
	}

	var loaded project.Config
	if err := yaml.Unmarshal(source, &loaded); err != nil {
		t.Fatalf("reload scenario config: %v", err)
	}
	if loaded.Render.Components.Cache || loaded.Render.Components.Events || loaded.Render.Components.Storage {
		t.Fatalf("scenario primitive opt-outs changed after reload: %#v", loaded.Render.Components)
	}
}
