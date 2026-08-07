package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestSyncLegacyGeneratedTemplatesUsesSharedProjectConfig verifies an upgraded render removes the duplicated model and imports the canonical package.
func TestSyncLegacyGeneratedTemplatesUsesSharedProjectConfig(t *testing.T) {
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	root := t.TempDir()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}

	lighthouseDir := filepath.Join("internal", "lighthouse")
	if err := os.MkdirAll(lighthouseDir, 0o755); err != nil {
		t.Fatalf("create Lighthouse directory: %v", err)
	}
	obsoletePath := filepath.Join(lighthouseDir, "project_config.go")
	if err := os.WriteFile(obsoletePath, []byte("package lighthouse\n\ntype Config struct{}\n"), 0o644); err != nil {
		t.Fatalf("write obsolete Lighthouse config: %v", err)
	}
	serverPath := filepath.Join(lighthouseDir, "server.go")
	legacyServer := "package lighthouse\n\nfunc loadProjectConfig() (*Config, error) { return nil, nil }\n"
	if err := os.WriteFile(serverPath, []byte(legacyServer), 0o644); err != nil {
		t.Fatalf("write legacy Lighthouse server: %v", err)
	}
	patchPath := filepath.Join(lighthouseDir, "project_config_patch.go")
	legacyPatch := "package lighthouse\n\nimport \"example.com/testapp/project\"\n"
	if err := os.WriteFile(patchPath, []byte(legacyPatch), 0o644); err != nil {
		t.Fatalf("write legacy Lighthouse config patch: %v", err)
	}
	projectDir := "project"
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("create generated project package: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "config.go"), []byte("package project\n"), 0o644); err != nil {
		t.Fatalf("write generated project config: %v", err)
	}
	generatedTestPath := filepath.Join(lighthouseDir, "project_config_test.go")
	if err := os.WriteFile(generatedTestPath, []byte("package lighthouse\n"), 0o644); err != nil {
		t.Fatalf("write generated project config tests: %v", err)
	}

	renderer := unitProjectRenderer(t)
	renderer.config = &project.Config{
		GoModuleName: "example.com/testapp",
		Render: project.RenderConfig{
			Components: project.Components{WebAPI: true},
		},
	}
	if err := renderer.syncLegacyGeneratedTemplates(); err != nil {
		t.Fatalf("sync legacy templates: %v", err)
	}

	patch, err := os.ReadFile(patchPath)
	if err != nil {
		t.Fatalf("read migrated Lighthouse config patch: %v", err)
	}
	if !strings.Contains(string(patch), `"github.com/goforj/goforj/project"`) {
		t.Fatalf("migrated Lighthouse config patch omitted shared project import:\n%s", patch)
	}
	if strings.Contains(string(patch), `"example.com/testapp/project"`) {
		t.Fatalf("migrated Lighthouse config patch retained local project import:\n%s", patch)
	}
	if _, err := os.Stat(obsoletePath); !os.IsNotExist(err) {
		t.Fatalf("obsolete Lighthouse config remains, stat error = %v", err)
	}
	for _, path := range []string{projectDir, generatedTestPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("duplicated generated config artifact remains at %s: %v", path, err)
		}
	}
	server, err := os.ReadFile(serverPath)
	if err != nil {
		t.Fatalf("read migrated Lighthouse server: %v", err)
	}
	for _, expected := range []string{
		`"github.com/goforj/goforj/project"`,
		"func loadProjectConfig() (*project.Config, error)",
	} {
		if !strings.Contains(string(server), expected) {
			t.Fatalf("migrated Lighthouse server omitted %q:\n%s", expected, server)
		}
	}
}
