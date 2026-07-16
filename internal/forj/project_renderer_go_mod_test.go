package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestCreateGoModCreatesAndThenSkips verifies only a pre-existing module is treated as an ordinary skip.
func TestCreateGoModCreatesAndThenSkips(t *testing.T) {
	renderer := projectRendererForGoModTest(t, "example.com/rendered")
	if err := renderer.createGoMod(); err != nil {
		t.Fatalf("create go.mod: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(renderer.workspace.path(), "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	if !strings.Contains(string(body), "module example.com/rendered") {
		t.Fatalf("go.mod omitted module name:\n%s", body)
	}

	if err := renderer.createGoMod(); err != nil {
		t.Fatalf("skip existing go.mod: %v", err)
	}
	counts := renderer.stats.counts()
	if counts.created != 1 || counts.skipped != 1 {
		t.Fatalf("go.mod counts = %#v, want one create and one skip", counts)
	}
}

// TestCreateGoModReturnsCommandFailure prevents invalid module configuration from masquerading as an existing file.
func TestCreateGoModReturnsCommandFailure(t *testing.T) {
	renderer := projectRendererForGoModTest(t, "invalid module")
	err := renderer.createGoMod()
	if err == nil || !strings.Contains(err.Error(), "initialize go.mod") {
		t.Fatalf("createGoMod() error = %v, want command failure", err)
	}
	if _, statErr := os.Stat(filepath.Join(renderer.workspace.path(), "go.mod")); !os.IsNotExist(statErr) {
		t.Fatalf("failed initialization left go.mod behind: %v", statErr)
	}
}

// projectRendererForGoModTest binds focused module tests to an explicit temporary workspace.
func projectRendererForGoModTest(t *testing.T, module string) *ProjectRenderer {
	t.Helper()
	workspace, err := resolveProjectRenderWorkspace(t.TempDir())
	if err != nil {
		t.Fatalf("resolve project workspace: %v", err)
	}
	return &ProjectRenderer{
		config:    &project.Config{GoModuleName: module},
		workspace: workspace,
		stats:     &renderStats{},
	}
}
