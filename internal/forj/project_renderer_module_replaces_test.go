package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

func TestApplyModuleReplacesAddsAndRemovesManagedEntries(t *testing.T) {
	root := t.TempDir()
	goMod := "module example.com/test\n\ngo 1.26.1\n\nreplace github.com/example/manual => ../manual\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(wd) }()

	renderer := &ProjectRenderer{
		workspace: currentProjectRenderWorkspace(t),
		config: &project.Config{Render: project.RenderConfig{
			ModuleReplaces: map[string]string{
				"github.com/goforj/web":   "../web",
				"github.com/goforj/cache": "../cache",
			},
		}},
	}

	if err := renderer.applyModuleReplaces(); err != nil {
		t.Fatalf("apply replaces: %v", err)
	}

	content, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "replace github.com/goforj/web => ../web") {
		t.Fatalf("expected web replace, got:\n%s", text)
	}
	if !strings.Contains(text, "replace github.com/goforj/cache => ../cache") {
		t.Fatalf("expected cache replace, got:\n%s", text)
	}
	if !strings.Contains(text, "replace github.com/example/manual => ../manual") {
		t.Fatalf("expected manual replace to stay, got:\n%s", text)
	}

	renderer.config.Render.ModuleReplaces = map[string]string{
		"github.com/goforj/cache": "../cache-v2",
	}
	if err := renderer.applyModuleReplaces(); err != nil {
		t.Fatalf("reapply replaces: %v", err)
	}

	content, err = os.ReadFile("go.mod")
	if err != nil {
		t.Fatalf("read go.mod after reapply: %v", err)
	}
	text = string(content)
	if strings.Contains(text, "replace github.com/goforj/web => ../web") {
		t.Fatalf("expected managed web replace to be removed, got:\n%s", text)
	}
	if !strings.Contains(text, "replace github.com/goforj/cache => ../cache-v2") {
		t.Fatalf("expected cache replace to update, got:\n%s", text)
	}
	if !strings.Contains(text, "replace github.com/example/manual => ../manual") {
		t.Fatalf("expected manual replace to remain, got:\n%s", text)
	}
}

func TestApplyModuleReplacesRemovesStateFileWhenEmpty(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n\ngo 1.26.1\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(wd) }()

	renderer := &ProjectRenderer{
		workspace: currentProjectRenderWorkspace(t),
		config: &project.Config{Render: project.RenderConfig{
			ModuleReplaces: map[string]string{
				"github.com/goforj/web": "../web",
			},
		}},
	}
	if err := renderer.applyModuleReplaces(); err != nil {
		t.Fatalf("apply replaces: %v", err)
	}
	if _, err := os.Stat(moduleReplacesStateFile); err != nil {
		t.Fatalf("expected state file: %v", err)
	}

	renderer.config.Render.ModuleReplaces = nil
	if err := renderer.applyModuleReplaces(); err != nil {
		t.Fatalf("remove replaces: %v", err)
	}
	if _, err := os.Stat(moduleReplacesStateFile); !os.IsNotExist(err) {
		t.Fatalf("expected state file removed, got err=%v", err)
	}
}
