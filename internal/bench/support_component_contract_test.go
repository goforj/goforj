package bench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// TestWriteYAMLPersistsCurrentComponentContractForPrimitiveOptOuts verifies benchmark fixtures retain their deliberately narrow surface.
func TestWriteYAMLPersistsCurrentComponentContractForPrimitiveOptOuts(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".goforj.yml")
	config := project.Config{
		Render: project.RenderConfig{
			Components: project.Components{CLI: true},
		},
	}

	if err := writeYAML(path, config); err != nil {
		t.Fatalf("write benchmark config: %v", err)
	}
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read benchmark config: %v", err)
	}
	if !strings.Contains(string(source), "component_contract: 1") {
		t.Fatalf("benchmark config omitted the component contract marker:\n%s", source)
	}

	var loaded project.Config
	if err := yaml.Unmarshal(source, &loaded); err != nil {
		t.Fatalf("reload benchmark config: %v", err)
	}
	if loaded.Render.Components.Cache || loaded.Render.Components.Events || loaded.Render.Components.Storage {
		t.Fatalf("benchmark primitive opt-outs changed after reload: %#v", loaded.Render.Components)
	}
}
