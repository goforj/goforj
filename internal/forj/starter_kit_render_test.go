package forj

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goforj/goforj/internal/logger"
)

func TestScaffoldVueStarterKitOverwritesFrontend(t *testing.T) {
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	root := t.TempDir()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join("frontend", "dist"), 0o755); err != nil {
		t.Fatalf("mkdir frontend: %v", err)
	}
	if err := os.WriteFile(filepath.Join("frontend", "custom.txt"), []byte("user file"), 0o644); err != nil {
		t.Fatalf("write custom file: %v", err)
	}

	renderer := NewProjectRenderer(logger.NewAppLogger())
	if err := renderer.scaffoldVueStarterKit(); err != nil {
		t.Fatalf("scaffold vue starter kit: %v", err)
	}

	if _, err := os.Stat(filepath.Join("frontend", "custom.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected existing frontend to be overwritten, stat err = %v", err)
	}
	for _, path := range []string{
		filepath.Join("frontend", "package.json"),
		filepath.Join("frontend", "components.json"),
		filepath.Join("frontend", "src", "App.vue"),
		filepath.Join("frontend", "dist", "index.html"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join("frontend", "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("expected node_modules to be excluded, stat err = %v", err)
	}
}

func TestScaffoldDemoFrontendExcludesNodeModulesAndKeepsDist(t *testing.T) {
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	root := t.TempDir()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	renderer := NewProjectRenderer(logger.NewAppLogger())
	if err := renderer.scaffoldDemoFrontend(); err != nil {
		t.Fatalf("scaffold demo frontend: %v", err)
	}

	for _, path := range []string{
		filepath.Join("frontend", "package.json"),
		filepath.Join("frontend", "src", "App.vue"),
		filepath.Join("frontend", "dist", "index.html"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join("frontend", "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("expected node_modules to be excluded, stat err = %v", err)
	}
}
