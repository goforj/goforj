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
	writeAPIIndexFixture(t, root)

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

func TestAPIIndexRunnerSkipsRuntimeDataDir(t *testing.T) {
	root := t.TempDir()
	writeAPIIndexFixture(t, root)
	dataDir := filepath.Join(root, "_data", "mariadb", "billing")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir data dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "ignored.go"), []byte("package billing\n"), 0o644); err != nil {
		t.Fatalf("write ignored runtime data: %v", err)
	}
	if err := os.Chmod(dataDir, 0); err != nil {
		t.Fatalf("chmod data dir: %v", err)
	}
	defer func() { _ = os.Chmod(dataDir, 0o755) }()

	runner := NewAPIIndexRunner(logger.NewSilentLogger())
	out := filepath.Join(root, "build", "api_index.json")
	diagnostics := filepath.Join(root, "build", "api_index.diagnostics.json")
	openAPI := filepath.Join(root, "build", "openapi.json")

	if err := runner.Run(root, out, diagnostics, openAPI, false); err != nil {
		t.Fatalf("run failed: %v", err)
	}
}

func TestAPIIndexRunnerDefaultStatusIncludesActiveApp(t *testing.T) {
	root := t.TempDir()
	writeAPIIndexFixture(t, root)
	writeAPIIndexRouteComposition(t, root, "customer-portal")
	t.Setenv("FORJ_APP", "customer-portal")

	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()

	runner := NewAPIIndexRunner(logger.NewSilentLogger())

	status, err := runner.RunDefaultWithStatus()
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if status != "app customer-portal" {
		t.Fatalf("unexpected status: %q", status)
	}

	status, err = runner.RunDefaultWithStatus()
	if err != nil {
		t.Fatalf("rerun failed: %v", err)
	}
	if status != "app customer-portal, no changes" {
		t.Fatalf("unexpected no-change status: %q", status)
	}
}

func TestDefaultAPIIndexPathsUsesDefaultAppArtifacts(t *testing.T) {
	paths := defaultAPIIndexPaths(project.DefaultApp())

	if paths.appName != "app" {
		t.Fatalf("unexpected app: %s", paths.appName)
	}
	if paths.out != filepath.Join("build", "api_index.json") {
		t.Fatalf("unexpected api index path: %s", paths.out)
	}
	if paths.diagnostics != filepath.Join("build", "api_index.diagnostics.json") {
		t.Fatalf("unexpected diagnostics path: %s", paths.diagnostics)
	}
	if paths.openAPI != filepath.Join("build", "openapi.json") {
		t.Fatalf("unexpected openapi path: %s", paths.openAPI)
	}
	if paths.routeComposition != filepath.Join("app", "routes.go") {
		t.Fatalf("unexpected route composition path: %s", paths.routeComposition)
	}
}

// writeAPIIndexFixture keeps these tests focused on runner behavior instead of route discovery setup.
func writeAPIIndexFixture(t *testing.T, root string) {
	t.Helper()
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
}

// writeAPIIndexRouteComposition gives named-app default runs the composition marker they require.
func writeAPIIndexRouteComposition(t *testing.T, root string, appName string) {
	t.Helper()
	path := filepath.Join(root, "app", appName, "routes.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir app routes dir: %v", err)
	}
	source := `package customerportal

import "github.com/goforj/web"

func ProvideRoutes() []web.RouteGroup {
	return nil
}
`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("write app route composition: %v", err)
	}
}

func TestDefaultAPIIndexPathsUsesNamedAppArtifacts(t *testing.T) {
	paths := defaultAPIIndexPaths(project.DefaultNamedApp("customer-portal"))

	if paths.appName != "customer-portal" {
		t.Fatalf("unexpected app: %s", paths.appName)
	}
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

	got, err := existingRouteCompositionPath(project.DefaultApp(), path)
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
	got, err = existingRouteCompositionPath(project.DefaultApp(), path)
	if err != nil {
		t.Fatalf("expected existing default route composition, got %v", err)
	}
	if got != path {
		t.Fatalf("expected existing route composition, got %q", got)
	}
}

func TestExistingRouteCompositionPathSkipsMissingNamedAppFile(t *testing.T) {
	root := t.TempDir()
	app := project.DefaultNamedApp("customer-portal")
	app.AppDir = filepath.Join(root, "app", "customer-portal")
	path := filepath.Join(app.AppDir, "routes.go")

	got, err := existingRouteCompositionPath(app, path)
	if err != nil {
		t.Fatalf("expected missing named app route composition to skip without error, got %v", err)
	}
	if got != "" {
		t.Fatalf("expected missing named app route composition to be ignored, got %q", got)
	}
}
