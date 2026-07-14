package testkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// TestWriteProjectConfigFilePersistsCurrentComponentContractForPrimitiveOptOuts verifies integration renders retain their requested surface.
func TestWriteProjectConfigFilePersistsCurrentComponentContractForPrimitiveOptOuts(t *testing.T) {
	root := t.TempDir()
	config := project.Config{
		Render: project.RenderConfig{
			Components: project.Components{CLI: true},
		},
	}

	WriteProjectConfigFile(t, root, config)
	source, err := os.ReadFile(filepath.Join(root, ".goforj.yml"))
	if err != nil {
		t.Fatalf("read integration render config: %v", err)
	}
	if !strings.Contains(string(source), "component_contract: 1") {
		t.Fatalf("integration render config omitted the component contract marker:\n%s", source)
	}

	var loaded project.Config
	if err := yaml.Unmarshal(source, &loaded); err != nil {
		t.Fatalf("reload integration render config: %v", err)
	}
	if loaded.Render.Components.Cache || loaded.Render.Components.Events || loaded.Render.Components.Storage {
		t.Fatalf("integration render primitive opt-outs changed after reload: %#v", loaded.Render.Components)
	}
}
