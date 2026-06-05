package build

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
)

func TestAPIIndexRunnerRunWritesArtifacts(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/test\n\ngo 1.24\n",
		"internal/hello/controller.go": `package hello
import (
	"net/http"

	"github.com/goforj/web"
)
type Controller struct{}
func (c *Controller) Routes() []any {
	return []any{
		web.NewRoute(http.MethodGet, "/hello", c.Hello),
	}
}
func (c *Controller) Hello(ctx any) error { return nil }`,
	}
	for rel, contents := range files {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	runner := NewAPIIndexRunner(logger.NewSilentLogger())
	out := filepath.Join(root, "build", "api_index.json")
	diagnostics := filepath.Join(root, "build", "api_index.diagnostics.json")
	openAPI := filepath.Join(root, "build", "openapi.json")

	if err := runner.Run(root, out, diagnostics, openAPI, false); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	for _, p := range []string{out, diagnostics, openAPI} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected artifact %s: %v", p, err)
		}
	}
}

func TestDefaultAPIIndexPathsUsesNamedTargetArtifacts(t *testing.T) {
	paths := defaultAPIIndexPaths(project.DefaultNamedAppTarget("customer-portal"))

	if paths.out != filepath.Join("build", "customer-portal", "api_index.json") {
		t.Fatalf("unexpected api index path: %s", paths.out)
	}
	if paths.diagnostics != filepath.Join("build", "customer-portal", "api_index.diagnostics.json") {
		t.Fatalf("unexpected diagnostics path: %s", paths.diagnostics)
	}
	if paths.openAPI != filepath.Join("build", "customer-portal", "openapi.json") {
		t.Fatalf("unexpected openapi path: %s", paths.openAPI)
	}
	if paths.routeComposition != filepath.Join("app", "customer-portal", "routes.go") {
		t.Fatalf("unexpected route composition path: %s", paths.routeComposition)
	}
}

func TestExistingRouteCompositionPathKeepsDefaultFallback(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app", "routes.go")

	got, err := existingRouteCompositionPath(project.DefaultAppTarget(), path)
	if err != nil {
		t.Fatalf("expected missing default route composition to fall back without error, got %v", err)
	}
	if got != "" {
		t.Fatalf("expected missing route composition to be ignored, got %q", got)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("package app\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err = existingRouteCompositionPath(project.DefaultAppTarget(), path)
	if err != nil {
		t.Fatalf("expected existing default route composition, got %v", err)
	}
	if got != path {
		t.Fatalf("expected existing route composition, got %q", got)
	}
}

func TestExistingRouteCompositionPathRequiresNamedTargetFile(t *testing.T) {
	root := t.TempDir()
	target := project.DefaultNamedAppTarget("customer-portal")
	target.AppDir = filepath.Join(root, "app", "customer-portal")
	path := filepath.Join(target.AppDir, "routes.go")

	if _, err := existingRouteCompositionPath(target, path); err == nil {
		t.Fatal("expected missing named target route composition to fail")
	}
}
