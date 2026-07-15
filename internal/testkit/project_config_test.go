package testkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// TestWriteProjectConfigPreservesRenderComponents verifies requested component selections survive serialization.
func TestWriteProjectConfigPreservesRenderComponents(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".goforj.yml")
	config := project.Config{
		ProjectName:  "RenderPreferred",
		GoModuleName: "example.com/renderpreferred",
		Render: project.RenderConfig{
			Components: project.Components{
				WebAPI:         true,
				Auth:           true,
				DatabaseSQLite: true,
			},
		},
	}

	if err := WriteProjectConfig(path, config); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	loaded := readWrittenProjectConfig(t, path)
	if !loaded.Render.Components.WebAPI || !loaded.Render.Components.Auth || !loaded.Render.Components.DatabaseSQLite {
		t.Fatalf("render components not preserved: %#v", loaded.Render.Components)
	}
}

// TestWriteProjectConfigPreservesRawComponentDependencies verifies serialization does not persist dependency expansion.
func TestWriteProjectConfigPreservesRawComponentDependencies(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".goforj.yml")
	config := project.Config{
		ProjectName:  "DependencyShape",
		GoModuleName: "example.com/dependencyshape",
		Render: project.RenderConfig{
			Components: project.Components{
				Auth:           true,
				WebAPI:         true,
				DatabaseSQLite: true,
			},
		},
	}

	if err := WriteProjectConfig(path, config); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	loaded := readWrittenProjectConfig(t, path)
	if !loaded.Render.Components.Auth {
		t.Fatal("expected auth to remain selected")
	}
	if loaded.Render.Components.Mail || loaded.Render.Components.Cache {
		t.Fatalf("expected raw yaml to preserve unresolved dependencies, got %#v", loaded.Render.Components)
	}
	effective := loaded.Render.Components.WithResolvedDependencies()
	if !effective.Mail || !effective.Cache {
		t.Fatalf("expected effective Auth dependencies to include Mail and Cache, got %#v", effective)
	}
}

// TestWriteProjectConfigAppliesDefaultsWithoutMutatingComponents verifies framework metadata does not alter requested components.
func TestWriteProjectConfigAppliesDefaultsWithoutMutatingComponents(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".goforj.yml")
	config := project.Config{
		ProjectName:  "Defaults",
		GoModuleName: "example.com/defaults",
		Render: project.RenderConfig{
			Components: project.Components{
				CLI:           true,
				WebAPI:        true,
				DatabaseMySQL: true,
			},
		},
	}

	if err := WriteProjectConfig(path, config); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	loaded := readWrittenProjectConfig(t, path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written config: %v", err)
	}
	if strings.Contains(string(data), "queue_driver:") {
		t.Fatalf("test render config persisted wizard-only queue choice:\n%s", data)
	}
	if !loaded.Render.Components.CLI || !loaded.Render.Components.WebAPI || !loaded.Render.Components.DatabaseMySQL {
		t.Fatalf("render components changed unexpectedly: %#v", loaded.Render.Components)
	}
}

// TestWriteProjectConfigPersistsCurrentComponentContractForPrimitiveOptOuts verifies omissions retain current opt-out semantics.
func TestWriteProjectConfigPersistsCurrentComponentContractForPrimitiveOptOuts(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".goforj.yml")
	config := project.Config{
		ProjectName: "Lean Render",
		Render: project.RenderConfig{
			Components: project.Components{CLI: true},
		},
	}

	if err := WriteProjectConfig(path, config); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read project config: %v", err)
	}
	if !strings.Contains(string(source), "component_contract: 1") {
		t.Fatalf("test render config omitted the component contract marker:\n%s", source)
	}

	var loaded project.Config
	if err := yaml.Unmarshal(source, &loaded); err != nil {
		t.Fatalf("reload project config: %v", err)
	}
	if loaded.Render.Components.Cache || loaded.Render.Components.Events || loaded.Render.Components.Storage {
		t.Fatalf("test render primitive opt-outs changed after reload: %#v", loaded.Render.Components)
	}
}

// readWrittenProjectConfig decodes one fixture written by WriteProjectConfig for semantic assertions.
func readWrittenProjectConfig(t *testing.T, path string) project.Config {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read project config: %v", err)
	}
	var loaded project.Config
	if err := yaml.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal project config: %v", err)
	}
	return loaded
}
