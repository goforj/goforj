package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/logger"
)

func TestEnsureFrontendDistPlaceholderCreatesIndex(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	r := NewProjectRenderer(logger.NewAppLogger())
	r.stats = &renderStats{}
	if err := r.ensureFrontendDistPlaceholder(); err != nil {
		t.Fatalf("ensureFrontendDistPlaceholder: %v", err)
	}

	index := filepath.Join("frontend", "dist", "index.html")
	if _, err := os.Stat(index); err != nil {
		t.Fatalf("expected placeholder index: %v", err)
	}
}

func TestScaffoldDemoFrontendCopiesTemplateProject(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	r := NewProjectRenderer(logger.NewAppLogger())
	r.stats = &renderStats{}
	if err := r.scaffoldDemoFrontend(); err != nil {
		t.Fatalf("scaffoldDemoFrontend: %v", err)
	}

	checks := []string{
		filepath.Join("frontend", "package.json"),
		filepath.Join("frontend", "src", "App.vue"),
		filepath.Join("frontend", "src", "router", "index.ts"),
		filepath.Join("frontend", "components.json"),
	}
	for _, path := range checks {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}

	routerContent, err := os.ReadFile(filepath.Join("frontend", "src", "router", "index.ts"))
	if err != nil {
		t.Fatalf("read router: %v", err)
	}
	if !strings.Contains(string(routerContent), "/status") {
		t.Fatalf("expected public status route in router")
	}
}
