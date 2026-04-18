package forj

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

func TestWriteYAMLPreservesRenderComponents(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".goforj.yml")
	cfg := project.Config{
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

	if err := WriteYAML(path, cfg); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	loaded := readWrittenConfig(t, path)
	if !loaded.Render.Components.WebAPI || !loaded.Render.Components.Auth || !loaded.Render.Components.DatabaseSQLite {
		t.Fatalf("render components not preserved: %#v", loaded.Render.Components)
	}
}

func TestWriteYAMLAppliesDefaultsWithoutMutatingComponents(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".goforj.yml")
	cfg := project.Config{
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

	if err := WriteYAML(path, cfg); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	loaded := readWrittenConfig(t, path)
	if loaded.Render.QueueDriver != "redis" {
		t.Fatalf("queue driver = %q, want %q", loaded.Render.QueueDriver, "redis")
	}
	if !loaded.Render.Components.CLI || !loaded.Render.Components.WebAPI || !loaded.Render.Components.DatabaseMySQL {
		t.Fatalf("render components changed unexpectedly: %#v", loaded.Render.Components)
	}
}

func TestBuildRenderCombosSkipsInvalidAuthSelections(t *testing.T) {
	for _, combo := range buildRenderCombos(false) {
		if combo.components.Auth && !combo.components.WebAPI {
			t.Fatalf("curated combo includes invalid auth selection: %#v", combo.components)
		}
		if err := combo.components.ValidateRenderContract(); err != nil {
			t.Fatalf("curated combo %q violates render contract: %v", combo.id, err)
		}
	}

	for _, combo := range buildRenderCombos(true) {
		if combo.components.Auth && !combo.components.WebAPI {
			t.Fatalf("full combo includes invalid auth selection: %#v", combo.components)
		}
		if err := combo.components.ValidateRenderContract(); err != nil {
			t.Fatalf("full combo %q violates render contract: %v", combo.id, err)
		}
	}
}

func TestRenderedGoTestPackages(t *testing.T) {
	root := t.TempDir()

	for _, rel := range []string{
		"internal/http/health_test.go",
		"internal/http/swagger_test.go",
		"internal/database/connections_test.go",
		"internal/database/connections_integration_test.go",
		"migrations/migrations_test.go",
		"frontend/src/components/__tests__/monitoring-chart.test.ts",
		"bin/ignored_test.go",
		"node_modules/pkg/ignored_test.go",
	} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte("package test\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	got, err := renderedGoTestPackages(root)
	if err != nil {
		t.Fatalf("renderedGoTestPackages: %v", err)
	}

	want := []string{
		"./internal/database",
		"./internal/http",
		"./migrations",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("packages = %#v, want %#v", got, want)
	}
}

func readWrittenConfig(t *testing.T, path string) project.Config {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read yaml: %v", err)
	}
	var loaded project.Config
	if err := yaml.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal yaml: %v", err)
	}
	return loaded
}
