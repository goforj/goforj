package rendercheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// TestWriteLegacyRenderComboConfigUsesHistoricalMappingsAtEveryScope keeps the real migration fixture faithful and marker-free.
func TestWriteLegacyRenderComboConfigUsesHistoricalMappingsAtEveryScope(t *testing.T) {
	tests := []struct {
		name   string
		config project.Config
		scopes map[string]project.Components
	}{
		{
			name: "default App",
			config: project.Config{Render: project.RenderConfig{Components: project.Components{
				CLI: true, WebAPI: true, Cache: true, Events: true, Storage: true,
			}}},
			scopes: map[string]project.Components{
				"default App": {CLI: true, WebAPI: true, Cache: true, Events: true, Storage: true},
			},
		},
		{
			name: "default and named Apps",
			config: project.Config{
				Render: project.RenderConfig{Components: project.Components{
					CLI: true, Docker: true, Cache: true, Events: true, Storage: true,
				}},
				Apps: map[string]project.AppConfig{
					"worker": {Components: project.Components{
						CLI: true, Cache: true, Events: true, Storage: true, Jobs: true,
					}},
				},
			},
			scopes: map[string]project.Components{
				"default App": {CLI: true, Docker: true, Cache: true, Events: true, Storage: true},
				"worker":      {CLI: true, Cache: true, Events: true, Storage: true, Jobs: true},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".goforj.yml")
			if err := writeLegacyRenderComboConfig(path, test.config); err != nil {
				t.Fatalf("writeLegacyRenderComboConfig() error: %v", err)
			}
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read legacy config: %v", err)
			}
			if strings.Contains(string(source), "component_contract:") {
				t.Fatalf("legacy fixture invented retired metadata:\n%s", source)
			}

			var document yaml.Node
			if err := yaml.Unmarshal(source, &document); err != nil {
				t.Fatalf("decode legacy config node: %v", err)
			}
			root, err := renderConfigDocumentRoot(&document)
			if err != nil {
				t.Fatalf("renderConfigDocumentRoot() error: %v", err)
			}
			for scope, components := range test.scopes {
				componentNode := legacyComponentScopeNode(root, scope)
				assertLegacyComponentMapping(t, scope, componentNode, components)
			}

			var decoded project.Config
			if err := yaml.Unmarshal(source, &decoded); err != nil {
				t.Fatalf("decode legacy config: %v", err)
			}
			if !decoded.NeedsComponentMigration() {
				t.Fatal("historical mappings did not request canonical migration")
			}
			for scope := range test.scopes {
				components := decoded.Render.Components
				if scope != "default App" {
					components = decoded.Apps[scope].Components
				}
				if !components.Cache || !components.Events || !components.Storage {
					t.Fatalf("%s omission defaults were not restored: %#v", scope, components)
				}
			}
		})
	}
}

// legacyComponentScopeNode locates one raw component selection without normalizing its mapping shape.
func legacyComponentScopeNode(root *yaml.Node, scope string) *yaml.Node {
	if scope == "default App" {
		return renderConfigMappingValue(renderConfigMappingValue(root, "render"), "components")
	}
	apps := renderConfigMappingValue(root, "apps")
	return renderConfigMappingValue(renderConfigMappingValue(apps, scope), "components")
}

// assertLegacyComponentMapping checks every old toggle while ensuring the later primitives remain genuine omissions.
func assertLegacyComponentMapping(t *testing.T, scope string, mapping *yaml.Node, components project.Components) {
	t.Helper()
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		t.Fatalf("%s components kind = %v, want historical mapping", scope, mapping)
	}
	if got, want := len(mapping.Content)/2, len(legacyComponentMappingKeys); got != want {
		t.Fatalf("%s mapping entries = %d, want %d", scope, got, want)
	}
	for _, key := range []project.ComponentKey{project.ComponentCache, project.ComponentEvents, project.ComponentStorage} {
		if renderConfigMappingValue(mapping, string(key)) != nil {
			t.Fatalf("%s historical mapping unexpectedly defines %q", scope, key)
		}
	}
	for _, key := range legacyComponentMappingKeys {
		value := renderConfigMappingValue(mapping, string(key))
		if value == nil {
			t.Fatalf("%s historical mapping omits %q", scope, key)
		}
		var enabled bool
		if err := value.Decode(&enabled); err != nil {
			t.Fatalf("decode %s %q: %v", scope, key, err)
		}
		if enabled != components.Enabled(key) {
			t.Fatalf("%s %q = %t, want %t", scope, key, enabled, components.Enabled(key))
		}
	}
}
