package forj

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/logger"
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
	renderer := projectRendererForGoModTest(t, "https://github.com/example/new-project")
	err := renderer.createGoMod()
	if err == nil || !strings.Contains(err.Error(), "initialize go.mod: exit status 1") || !strings.Contains(err.Error(), "\n  go: malformed module path") {
		t.Fatalf("createGoMod() error = %v, want formatted command diagnostics", err)
	}
	if _, statErr := os.Stat(filepath.Join(renderer.workspace.path(), "go.mod")); !os.IsNotExist(statErr) {
		t.Fatalf("failed initialization left go.mod behind: %v", statErr)
	}
}

// TestGoModTidyExercisesToolchainBoundary keeps the real Go command covered and ensures hooks use the active renderer workspace.
func TestGoModTidyExercisesToolchainBoundary(t *testing.T) {
	renderer := projectRendererForGoModTest(t, "example.com/rendered")
	moduleSource := []byte("module example.com/rendered\n\ngo 1.25\n\nrequire example.com/unused v0.0.0\n")
	if err := os.WriteFile(filepath.Join(renderer.workspace.path(), "go.mod"), moduleSource, 0o644); err != nil {
		t.Fatalf("write original go.mod: %v", err)
	}

	activeRenderer := projectRendererForGoModTest(t, "example.com/rendered")
	if err := os.WriteFile(filepath.Join(activeRenderer.workspace.path(), "go.mod"), moduleSource, 0o644); err != nil {
		t.Fatalf("write active go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(activeRenderer.workspace.path(), "main.go"), []byte("package rendered\n"), 0o644); err != nil {
		t.Fatalf("write active source: %v", err)
	}

	if err := renderer.tidyModule(activeRenderer); err != nil {
		t.Fatalf("go mod tidy active renderer: %v", err)
	}
	activeModule, err := os.ReadFile(filepath.Join(activeRenderer.workspace.path(), "go.mod"))
	if err != nil {
		t.Fatalf("read active go.mod: %v", err)
	}
	if strings.Contains(string(activeModule), "example.com/unused") {
		t.Fatalf("toolchain hook did not tidy the active renderer workspace:\n%s", activeModule)
	}

	originalModule, err := os.ReadFile(filepath.Join(renderer.workspace.path(), "go.mod"))
	if err != nil {
		t.Fatalf("read original go.mod: %v", err)
	}
	if !strings.Contains(string(originalModule), "example.com/unused") {
		t.Fatalf("toolchain hook tidied the renderer that supplied the hook:\n%s", originalModule)
	}
}

// TestGoModTidyReturnsCommandDiagnostics preserves actionable output when the Go tool rejects a generated module.
func TestGoModTidyReturnsCommandDiagnostics(t *testing.T) {
	renderer := projectRendererForGoModTest(t, "example.com/rendered")
	if err := os.WriteFile(filepath.Join(renderer.workspace.path(), "go.mod"), []byte("not a module file\n"), 0o644); err != nil {
		t.Fatalf("write invalid go.mod: %v", err)
	}

	err := renderer.goModTidy()
	if err == nil || !strings.Contains(err.Error(), "go mod tidy") || !strings.Contains(err.Error(), "\n  go: errors parsing go.mod") || !strings.Contains(err.Error(), "unknown directive") {
		t.Fatalf("goModTidy() error = %v, want command diagnostics", err)
	}
}

// TestCountTidyModulesCountsOnlyGoDiagnostics keeps render summaries independent of unrelated command output.
func TestCountTidyModulesCountsOnlyGoDiagnostics(t *testing.T) {
	stdout := "go: finding module for package example.com/one\nignored output\n"
	stderr := "downloading example.com/two v1.0.0\n\n"
	if got := countTidyModules(stdout, stderr); got != 2 {
		t.Fatalf("countTidyModules() = %d, want 2", got)
	}
}

// TestRenderPropagatesToolchainStageFailures keeps subprocess failures attached to the stage users need to repair.
func TestRenderPropagatesToolchainStageFailures(t *testing.T) {
	sentinel := errors.New("simulated toolchain failure")
	tests := []struct {
		name      string
		want      string
		configure func(*ProjectRenderer)
	}{
		{
			name: "go mod tidy",
			want: "go mod tidy: simulated toolchain failure",
			configure: func(renderer *ProjectRenderer) {
				renderer.tidyModule = func(*ProjectRenderer) error { return sentinel }
			},
		},
		{
			name: "wire generate",
			want: "wire generate: simulated toolchain failure",
			configure: func(renderer *ProjectRenderer) {
				renderer.generateWire = func(*ProjectRenderer) error { return sentinel }
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			config := &project.Config{
				ProjectName:  "Toolchain Failure",
				GoModuleName: "example.com/toolchain-failure",
				Render: project.RenderConfig{
					Components: project.Components{CLI: true},
				},
			}
			if err := writeProjectConfig(filepath.Join(root, ".goforj.yml"), config); err != nil {
				t.Fatalf("write project config: %v", err)
			}
			renderer := unitProjectRenderer(t)
			test.configure(renderer)

			err := renderer.Render(ComponentRenderInput{renderAll: true, root: root})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Render() error = %v, want %q", err, test.want)
			}
		})
	}
}

// projectRendererForGoModTest binds focused module tests to an explicit temporary workspace.
func projectRendererForGoModTest(t *testing.T, module string) *ProjectRenderer {
	t.Helper()
	workspace, err := resolveProjectRenderWorkspace(t.TempDir())
	if err != nil {
		t.Fatalf("resolve project workspace: %v", err)
	}
	renderer := NewProjectRenderer(logger.NewSilentLogger())
	renderer.config = &project.Config{GoModuleName: module}
	renderer.workspace = workspace
	renderer.stats = &renderStats{}
	return renderer
}
