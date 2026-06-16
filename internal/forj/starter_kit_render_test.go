package forj

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
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
	frontendDir := defaultFrontendDir()
	if err := os.MkdirAll(filepath.Join(frontendDir, "dist"), 0o755); err != nil {
		t.Fatalf("mkdir frontend: %v", err)
	}
	if err := os.WriteFile(filepath.Join(frontendDir, "custom.txt"), []byte("user file"), 0o644); err != nil {
		t.Fatalf("write custom file: %v", err)
	}

	renderer := NewProjectRenderer(logger.NewAppLogger())
	if err := renderer.scaffoldVueStarterKit(); err != nil {
		t.Fatalf("scaffold vue starter kit: %v", err)
	}

	if _, err := os.Stat(filepath.Join(frontendDir, "custom.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected existing frontend to be overwritten, stat err = %v", err)
	}
	for _, path := range []string{
		filepath.Join(frontendDir, "package.json"),
		filepath.Join(frontendDir, "components.json"),
		filepath.Join(frontendDir, "src", "App.vue"),
		filepath.Join(frontendDir, "dist", "index.html"),
		filepath.Join(frontendDir, "dist", "goforj-logo.png"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(frontendDir, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("expected node_modules to be excluded, stat err = %v", err)
	}
}

func TestFrontendDistPlaceholderUsesNamedApps(t *testing.T) {
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	root := t.TempDir()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join("cmd", "customer-portal"), 0o755); err != nil {
		t.Fatalf("mkdir named app entrypoint: %v", err)
	}
	if err := os.WriteFile(filepath.Join("cmd", "customer-portal", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write named app entrypoint: %v", err)
	}

	renderer := &ProjectRenderer{
		config: &project.Config{
			Render: project.RenderConfig{
				Components: project.Components{WebUI: true},
			},
		},
		stats: &renderStats{},
	}
	if err := renderer.ensureFrontendDistPlaceholder(); err != nil {
		t.Fatalf("ensure frontend dist placeholder: %v", err)
	}

	for _, path := range []string{
		filepath.Join("cmd", "app", "frontend", "dist", "index.html"),
		filepath.Join("cmd", "app", "frontend", "dist", "goforj-logo.png"),
		filepath.Join("cmd", "customer-portal", "frontend", "dist", "index.html"),
		filepath.Join("cmd", "customer-portal", "frontend", "dist", "goforj-logo.png"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}
}
