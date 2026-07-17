package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestSyncLegacyGeneratedTemplatesRestoresProjectConfigOwnership verifies an
// upgraded render replaces Lighthouse's persisted model with the project package.
func TestSyncLegacyGeneratedTemplatesRestoresProjectConfigOwnership(t *testing.T) {
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

	for _, path := range []string{
		filepath.Join("project", "config.go"),
		filepath.Join(lighthouseDir, "project_config_patch.go"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected restored config file %s: %v", path, err)
		}
	}
	if _, err := os.Stat(obsoletePath); !os.IsNotExist(err) {
		t.Fatalf("obsolete Lighthouse config remains, stat error = %v", err)
	}
	server, err := os.ReadFile(serverPath)
	if err != nil {
		t.Fatalf("read migrated Lighthouse server: %v", err)
	}
	for _, expected := range []string{
		`"example.com/testapp/project"`,
		"func loadProjectConfig() (*project.Config, error)",
	} {
		if !strings.Contains(string(server), expected) {
			t.Fatalf("migrated Lighthouse server omitted %q:\n%s", expected, server)
		}
	}
}
