package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// TestWriteProjectConfigUsesSequenceSemanticsForPrimitiveOptOuts verifies renderer-owned saves retain omissions without a version marker.
func TestWriteProjectConfigUsesSequenceSemanticsForPrimitiveOptOuts(t *testing.T) {
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
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read project config: %v", err)
	}
	if strings.Contains(string(source), "component_contract:") {
		t.Fatalf("project config persisted the obsolete component marker:\n%s", source)
	}
	if strings.Count(string(source), "components: [cli]") != 2 {
		t.Fatalf("project config omitted canonical component sequences:\n%s", source)
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
