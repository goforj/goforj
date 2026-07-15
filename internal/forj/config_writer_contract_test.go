package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// TestWriteProjectConfigPersistsCurrentComponentContractForPrimitiveOptOuts verifies renderer-owned saves cannot reinterpret omissions as legacy defaults.
func TestWriteProjectConfigPersistsCurrentComponentContractForPrimitiveOptOuts(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".goforj.yml")
	config := &project.Config{
		ProjectName: "Explicit Opt Outs",
		Extra:       map[string]any{"future_project": map[string]any{"enabled": true}},
		Render: project.RenderConfig{
			Components: project.Components{CLI: true},
			Extra:      map[string]any{"future_runtime": "canary"},
		},
		Apps: map[string]project.AppConfig{
			"worker": {
				Components: project.Components{CLI: true},
				Extra:      map[string]any{"future_routes": []string{"audit"}},
			},
		},
	}

	if err := writeProjectConfig(path, config); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	if config.Render.ComponentContractVersion != project.CurrentComponentContractVersion {
		t.Fatalf("written component contract = %d, want %d", config.Render.ComponentContractVersion, project.CurrentComponentContractVersion)
	}

	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read project config: %v", err)
	}
	if !strings.Contains(string(source), "component_contract: 1") {
		t.Fatalf("project config omitted the component contract marker:\n%s", source)
	}
	for _, expected := range []string{"future_project:", "future_runtime: canary", "future_routes:"} {
		if !strings.Contains(string(source), expected) {
			t.Fatalf("project config omitted extension %q:\n%s", expected, source)
		}
	}

	var loaded project.Config
	if err := yaml.Unmarshal(source, &loaded); err != nil {
		t.Fatalf("reload project config: %v", err)
	}
	if loaded.Render.Components.Cache || loaded.Render.Components.Events || loaded.Render.Components.Storage {
		t.Fatalf("default App primitive opt-outs changed after reload: %#v", loaded.Render.Components)
	}
	worker := loaded.Apps["worker"].Components
	if worker.Cache || worker.Events || worker.Storage {
		t.Fatalf("named App primitive opt-outs changed after reload: %#v", worker)
	}
}

// TestWriteYAMLPersistsCurrentComponentContractForPrimitiveOptOuts verifies generated test projects exercise the requested component shape.
func TestWriteYAMLPersistsCurrentComponentContractForPrimitiveOptOuts(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".goforj.yml")
	config := project.Config{
		ProjectName: "Lean Render",
		Render: project.RenderConfig{
			Components: project.Components{CLI: true},
		},
	}

	if err := WriteYAML(path, config); err != nil {
		t.Fatalf("write test render config: %v", err)
	}
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read test render config: %v", err)
	}
	if !strings.Contains(string(source), "component_contract: 1") {
		t.Fatalf("test render config omitted the component contract marker:\n%s", source)
	}

	var loaded project.Config
	if err := yaml.Unmarshal(source, &loaded); err != nil {
		t.Fatalf("reload test render config: %v", err)
	}
	if loaded.Render.Components.Cache || loaded.Render.Components.Events || loaded.Render.Components.Storage {
		t.Fatalf("test render primitive opt-outs changed after reload: %#v", loaded.Render.Components)
	}
}
