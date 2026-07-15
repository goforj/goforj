package testkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// TestWriteProjectConfigFileUsesSequenceSemanticsForPrimitiveOptOuts verifies integration renders retain their requested surface without a marker.
func TestWriteProjectConfigFileUsesSequenceSemanticsForPrimitiveOptOuts(t *testing.T) {
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
	if strings.Contains(string(source), "component_contract:") {
		t.Fatalf("integration render config persisted the obsolete component marker:\n%s", source)
	}
	if !strings.Contains(string(source), "components: [cli]") {
		t.Fatalf("integration render config omitted the canonical component sequence:\n%s", source)
	}

	var loaded project.Config
	if err := yaml.Unmarshal(source, &loaded); err != nil {
		t.Fatalf("reload integration render config: %v", err)
	}
	if loaded.Render.Components.Cache || loaded.Render.Components.Events || loaded.Render.Components.Storage {
		t.Fatalf("integration render primitive opt-outs changed after reload: %#v", loaded.Render.Components)
	}
}
