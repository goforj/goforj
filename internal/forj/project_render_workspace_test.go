package forj

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestProjectRenderWorkspaceLogicalErrorPreservesWrappedContext verifies path normalization does not discard useful boundary context or causes.
func TestProjectRenderWorkspaceLogicalErrorPreservesWrappedContext(t *testing.T) {
	root := t.TempDir()
	workspace, err := resolveProjectRenderWorkspace(root)
	if err != nil {
		t.Fatalf("resolve workspace: %v", err)
	}
	pathError := &os.PathError{
		Op:   "open",
		Path: filepath.Join(root, ".goforj.yml"),
		Err:  fs.ErrNotExist,
	}

	err = workspace.logicalError(fmt.Errorf("load project config: %w", pathError))
	if !strings.Contains(err.Error(), "load project config") || !strings.Contains(err.Error(), ".goforj.yml") {
		t.Fatalf("logical error lost boundary context: %v", err)
	}
	if strings.Contains(err.Error(), root) {
		t.Fatalf("logical error exposed physical root: %v", err)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("logical error lost underlying cause: %v", err)
	}
	var normalized *os.PathError
	if !errors.As(err, &normalized) || normalized.Path != ".goforj.yml" {
		t.Fatalf("logical error path = %#v, want project-relative .goforj.yml", normalized)
	}
}

// currentProjectRenderWorkspace resolves the caller's test directory explicitly so zero-value workspace use cannot hide global cwd coupling.
func currentProjectRenderWorkspace(t *testing.T) projectRenderWorkspace {
	t.Helper()
	workspace, err := resolveProjectRenderWorkspace(".")
	if err != nil {
		t.Fatalf("resolve current project render workspace: %v", err)
	}
	return workspace
}

// projectRendererForTest constructs focused renderer fixtures with the same required workspace invariant as production entry points.
func projectRendererForTest(t *testing.T, config *project.Config) *ProjectRenderer {
	t.Helper()
	return &ProjectRenderer{
		config:    config,
		workspace: currentProjectRenderWorkspace(t),
	}
}
