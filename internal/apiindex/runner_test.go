package apiindex

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// newTestRunner mirrors the CLI's late-bound App selection without coupling this package back to build.
func newTestRunner() *Runner {
	return NewRunner(func() project.App {
		appName := strings.TrimSpace(os.Getenv("FORJ_APP"))
		if project.IsSafeAppName(appName) {
			return project.DefaultNamedApp(appName)
		}
		return project.DefaultApp()
	})
}

// runIndexAtPaths keeps low-level path coverage out of Runner's production API.
func runIndexAtPaths(t *testing.T, input paths) {
	t.Helper()
	resolvedPaths, err := resolvePaths(input)
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}
	if _, err := newTestRunner().runIndex(resolvedPaths, runOptions{}); err != nil {
		t.Fatalf("run API index: %v", err)
	}
}

// TestRunIndexWritesArtifacts verifies direct indexing honors caller-selected artifact paths.
func TestRunIndexWritesArtifacts(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root)

	out := filepath.Join(root, "build", "api_index.json")
	diagnostics := filepath.Join(root, "build", "api_index.diagnostics.json")
	openAPI := filepath.Join(root, "build", "openapi.json")
	runIndexAtPaths(t, paths{
		root:        root,
		out:         out,
		diagnostics: diagnostics,
		openAPI:     openAPI,
	})

	for _, p := range []string{out, diagnostics, openAPI} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected artifact %s: %v", p, err)
		}
	}
}

// TestRunIndexSkipsRuntimeDataDir verifies generated and dependency data cannot leak into source discovery.
func TestRunIndexSkipsRuntimeDataDir(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root)
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
	nodeModulesDir := filepath.Join(root, "cmd", "app", "frontend", "node_modules", "package")
	if err := os.MkdirAll(nodeModulesDir, 0o755); err != nil {
		t.Fatalf("mkdir nested node_modules dir: %v", err)
	}
	nodeModuleSource := `package packagefixture
import (
	"net/http"

	"github.com/goforj/web"
)
type Controller struct{}
func (c *Controller) Routes() []any {
	return []any{
		web.NewRoute(http.MethodGet, "/node-modules-leak", c.Leak),
	}
}
func (c *Controller) Leak(ctx any) error { return nil }`
	if err := os.WriteFile(filepath.Join(nodeModulesDir, "ignored.go"), []byte(nodeModuleSource), 0o644); err != nil {
		t.Fatalf("write ignored node_modules file: %v", err)
	}

	out := filepath.Join(root, "build", "api_index.json")
	diagnostics := filepath.Join(root, "build", "api_index.diagnostics.json")
	openAPI := filepath.Join(root, "build", "openapi.json")
	runIndexAtPaths(t, paths{
		root:        root,
		out:         out,
		diagnostics: diagnostics,
		openAPI:     openAPI,
	})
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read api index: %v", err)
	}
	if strings.Contains(string(body), "node-modules-leak") {
		t.Fatalf("nested node_modules route leaked into api index:\n%s", string(body))
	}
}

// TestRunnerDefaultStatusIncludesActiveApp verifies status and artifact selection use the late-bound App.
func TestRunnerDefaultStatusIncludesActiveApp(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root)
	writeRouteComposition(t, root, "customer-portal")
	t.Setenv("FORJ_APP", "customer-portal")

	runner := newTestRunner()

	status, err := runner.RunDefault(Options{Root: root})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !strings.HasPrefix(status, "app customer-portal, changed, ") || !strings.Contains(status, " operation") || !strings.Contains(status, " schema") || !strings.Contains(status, " diagnostic") {
		t.Fatalf("status does not include active app, outcome, and counts: %q", status)
	}

	status, err = runner.RunDefault(Options{Root: root})
	if err != nil {
		t.Fatalf("rerun failed: %v", err)
	}
	if !strings.HasPrefix(status, "app customer-portal, unchanged, ") || !strings.Contains(status, " operation") || !strings.Contains(status, " schema") || !strings.Contains(status, " diagnostic") {
		t.Fatalf("no-change status does not include active app, outcome, and counts: %q", status)
	}
}

// TestRunnerRejectsInvalidExplicitRoot prevents missing source trees from looking like intentional nonparticipation.
func TestRunnerRejectsInvalidExplicitRoot(t *testing.T) {
	fileRoot := filepath.Join(t.TempDir(), "project-file")
	if err := os.WriteFile(fileRoot, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write project-root fixture: %v", err)
	}
	tests := []struct {
		name string
		root string
		want string
	}{
		{name: "missing", root: filepath.Join(t.TempDir(), "missing"), want: "inspect API index project root"},
		{name: "file", root: fileRoot, want: "is not a directory"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := newTestRunner().RunDefault(Options{Root: test.root})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("RunDefault() error = %v, want %q", err, test.want)
			}
		})
	}
}

// TestRunnerSkipsCLIOnlyAppAndRemovesStaleArtifacts verifies explicit nonparticipation clears old contracts.
func TestRunnerSkipsCLIOnlyAppAndRemovesStaleArtifacts(t *testing.T) {
	root := t.TempDir()
	config := `render:
  components:
    cli: true
apps:
  ship:
    components:
      cli: true
`
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	t.Setenv("FORJ_APP", "ship")
	paths := rootDefaultPaths(root, defaultPaths(project.DefaultNamedApp("ship")))
	for _, path := range []string{paths.out, paths.diagnostics, paths.openAPI} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir artifact directory: %v", err)
		}
		if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
			t.Fatalf("write stale artifact: %v", err)
		}
	}

	status, err := newTestRunner().RunDefault(Options{Root: root})
	if err != nil {
		t.Fatalf("run CLI-only API index: %v", err)
	}
	if status != "app ship, cleaned (no web API), 0 operations, 0 schemas, 0 diagnostics" {
		t.Fatalf("unexpected status: %q", status)
	}
	for _, path := range []string{paths.out, paths.diagnostics, paths.openAPI} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected stale artifact %s to be removed, got %v", path, err)
		}
	}
}

// TestRunnerPrepareReturnsNilCandidateForCLIOnlyApp ensures callers can use an interface nil check before publishing or discarding optional work.
func TestRunnerPrepareReturnsNilCandidateForCLIOnlyApp(t *testing.T) {
	root := t.TempDir()
	config := `render:
  components: [cli]
`
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	preparation, err := newTestRunner().Prepare(Options{Root: root})
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	if preparation.Candidate != nil {
		t.Fatalf("Prepare() candidate = %T, want nil for CLI-only App without stale artifacts", preparation.Candidate)
	}
	wantStatus := "app app, skipped (no web API), 0 operations, 0 schemas, 0 diagnostics"
	if preparation.Status != wantStatus {
		t.Fatalf("Prepare() status = %q, want %q", preparation.Status, wantStatus)
	}
}

// TestRunnerRequiresCompositionForWebAPIApp verifies a configured API cannot silently widen to repository scope.
func TestRunnerRequiresCompositionForWebAPIApp(t *testing.T) {
	root := t.TempDir()
	config := `render:
  components:
    cli: true
    web_api: true
`
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	paths := rootDefaultPaths(root, defaultPaths(project.DefaultApp()))
	for _, path := range []string{paths.out, paths.diagnostics, paths.openAPI} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir artifact directory: %v", err)
		}
		if err := os.WriteFile(path, []byte("last valid"), 0o644); err != nil {
			t.Fatalf("write prior artifact: %v", err)
		}
	}

	status, err := newTestRunner().RunDefault(Options{Root: root})
	if err == nil || !strings.Contains(err.Error(), `API index for app "app" requires route composition "app/routes.go"`) {
		t.Fatalf("expected missing composition error, got %v", err)
	}
	if status != "app app, rejected, 0 operations, 0 schemas, 0 diagnostics" {
		t.Fatalf("missing composition status = %q, want rejected outcome", status)
	}
	for _, path := range []string{paths.out, paths.diagnostics, paths.openAPI} {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read preserved artifact %s: %v", path, readErr)
		}
		if string(content) != "last valid" {
			t.Fatalf("artifact %s changed after failed index: %q", path, content)
		}
	}
}

// TestDefaultPathsUsesDefaultAppArtifacts preserves the legacy default-App artifact layout.
func TestDefaultPathsUsesDefaultAppArtifacts(t *testing.T) {
	paths := defaultPaths(project.DefaultApp())

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

// writeFixture keeps these tests focused on runner behavior instead of route discovery setup.
func writeFixture(t *testing.T, root string) {
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

// writeRouteComposition gives named-app default runs the composition marker they require.
func writeRouteComposition(t *testing.T, root string, appName string) {
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

// TestDefaultPathsUsesNamedAppArtifacts verifies named Apps receive isolated artifact directories.
func TestDefaultPathsUsesNamedAppArtifacts(t *testing.T) {
	paths := defaultPaths(project.DefaultNamedApp("customer-portal"))

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

// TestExistingRouteCompositionPathKeepsDefaultFallback verifies convention-only projects may omit composition.
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

// TestExistingRouteCompositionPathSkipsMissingNamedAppFile verifies legacy named Apps remain optional without component metadata.
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

// TestExistingRouteCompositionPathReturnsUnexpectedStatError verifies filesystem failures remain actionable.
func TestExistingRouteCompositionPathReturnsUnexpectedStatError(t *testing.T) {
	app := project.DefaultNamedApp("customer-portal")

	got, err := existingRouteCompositionPath(app, "invalid\x00routes.go")
	if err == nil {
		t.Fatal("expected invalid route composition path to fail")
	}
	if got != "" {
		t.Fatalf("expected no route composition after stat failure, got %q", got)
	}
	if !strings.Contains(err.Error(), `stat route composition for app "customer-portal"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestExistingRouteCompositionPathRejectsDirectory verifies composition must be a concrete source file.
func TestExistingRouteCompositionPathRejectsDirectory(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app", "routes.go")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir route composition path: %v", err)
	}

	got, err := existingRouteCompositionPath(project.DefaultApp(), path)
	if err == nil {
		t.Fatal("expected route composition directory to fail")
	}
	if got != "" {
		t.Fatalf("expected no route composition for directory, got %q", got)
	}
	if !strings.Contains(err.Error(), "must be a file") {
		t.Fatalf("unexpected error: %v", err)
	}
}
