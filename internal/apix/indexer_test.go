package apix

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRunIndexesRoutesAndHandlerMetadata(t *testing.T) {
	root := t.TempDir()

	files := map[string]string{
		"go.mod": "module example.com/test\n\ngo 1.24\n",
		"internal/hello/controller.go": `package hello

import (
	"net/http"
	"github.com/labstack/echo/v4"
)

type Controller struct {}

func (c *Controller) Routes() []any {
	return []any{
		http.NewRoute(http.MethodGet, "/hello/:name", c.Hello),
	}
}

type requestPayload struct { Name string ` + "`json:\"name\"`" + ` }
type responsePayload struct { Message string ` + "`json:\"message\"`" + ` }

func (c *Controller) Hello(ctx echo.Context) error {
	name := ctx.Param("name")
	filter := ctx.QueryParam("filter")
	var req requestPayload
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error":"bad request"})
	}
	_ = name
	_ = filter
	return ctx.JSON(http.StatusOK, responsePayload{Message: "ok"})
}
`,
		"internal/router/routes_registry.go": `package router

func ProvideRoutes() []any {
	groups := []any{}
	groups = append(groups, http.NewRouteGroup("/api/v1", nil))
	return groups
}
`,
	}

	for rel, contents := range files {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(contents), 0644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	out := filepath.Join(root, "build", "api_index.json")
	diag := filepath.Join(root, "build", "api_index.diagnostics.json")
	openapi := filepath.Join(root, "build", "openapi.json")

	manifest, err := Run(context.Background(), IndexOptions{
		Root:            root,
		OutPath:         out,
		DiagnosticsPath: diag,
		OpenAPIPath:     openapi,
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(manifest.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(manifest.Operations))
	}

	op := manifest.Operations[0]
	if op.Path != "/api/v1/hello/:name" {
		t.Fatalf("unexpected path: %s", op.Path)
	}
	if op.Method != "GET" {
		t.Fatalf("unexpected method: %s", op.Method)
	}
	if op.Handler.Function != "Hello" {
		t.Fatalf("unexpected handler function: %s", op.Handler.Function)
	}
	if len(op.Inputs.PathParams) != 1 || op.Inputs.PathParams[0].Name != "name" {
		t.Fatalf("expected path param name")
	}
	if len(op.Inputs.QueryParams) != 1 || op.Inputs.QueryParams[0].Name != "filter" {
		t.Fatalf("expected query param filter")
	}
	if op.Inputs.Body == nil || op.Inputs.Body.TypeName != "requestPayload" {
		t.Fatalf("expected request body type requestPayload, got %+v", op.Inputs.Body)
	}
	if len(op.Outputs.Responses) < 2 {
		t.Fatalf("expected at least 2 responses, got %d", len(op.Outputs.Responses))
	}

	for _, path := range []string{out, diag, openapi} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected output file %s: %v", path, err)
		}
	}
}

func TestRunMapsRoutesToSpecificGroupsByControllerOwner(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/test\n\ngo 1.24\n",
		"internal/hello/controller.go": `package hello
import "net/http"
type Controller struct{}
func (c *Controller) Routes() []any {
	return []any{
		http.NewRoute(http.MethodGet, "/things", c.Index),
	}
}
func (c *Controller) Index(ctx any) error { return nil }`,
		"internal/admin/controller.go": `package admin
import "net/http"
type Controller struct{}
func (c *Controller) Routes() []any {
	return []any{
		http.NewRoute(http.MethodGet, "/things", c.Index),
	}
}
func (c *Controller) Index(ctx any) error { return nil }`,
		"internal/router/routes_registry.go": `package router
func ProvideAppRoutes(
	helloController *hello.Controller,
	adminController *admin.Controller,
) *AppRoutes {
	var app []any
	var adminRoutes []any
	app = append(app, helloController.Routes()...)
	adminRoutes = append(adminRoutes, adminController.Routes()...)
	return &AppRoutes{
		app: app,
		admin: adminRoutes,
	}
}
type AppRoutes struct {
	app []any
	admin []any
}
func ProvideRoutes(r *AppRoutes) []any {
	groups := []any{}
	groups = append(groups, http.NewRouteGroup("/api/v1", r.app))
	groups = append(groups, http.NewRouteGroup("/api/admin", r.admin))
	return groups
}
`,
	}
	for rel, contents := range files {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(contents), 0644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	manifest, err := Run(context.Background(), IndexOptions{Root: root})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(manifest.Operations) != 2 {
		t.Fatalf("expected 2 operations, got %d", len(manifest.Operations))
	}

	found := map[string]bool{}
	for _, op := range manifest.Operations {
		found[op.Path] = true
	}
	if !found["/api/v1/things"] {
		t.Fatalf("expected /api/v1/things route")
	}
	if !found["/api/admin/things"] {
		t.Fatalf("expected /api/admin/things route")
	}
}

func TestRunFallsBackToUnprefixedPathWhenGroupMappingMissing(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/test\n\ngo 1.24\n",
		"internal/hello/controller.go": `package hello
import "net/http"
type Controller struct{}
func (c *Controller) Routes() []any {
	return []any{
		http.NewRoute(http.MethodGet, "/raw", c.Raw),
	}
}
func (c *Controller) Raw(ctx any) error { return nil }`,
		"internal/router/routes_registry.go": `package router
func ProvideRoutes() []any {
	groups := []any{}
	groups = append(groups, http.NewRouteGroup("/api/v1", nil))
	groups = append(groups, http.NewRouteGroup("/api/admin", nil))
	return groups
}
`,
	}
	for rel, contents := range files {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(contents), 0644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	manifest, err := Run(context.Background(), IndexOptions{Root: root})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(manifest.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(manifest.Operations))
	}
	if manifest.Operations[0].Path != "/raw" {
		t.Fatalf("expected unprefixed fallback path /raw, got %s", manifest.Operations[0].Path)
	}
}

func TestRunEmitsAmbiguousHandlerDiagnostic(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/test\n\ngo 1.24\n",
		"internal/a/handler.go": `package a
func Ping(ctx any) error { return nil }`,
		"internal/b/handler.go": `package b
func Ping(ctx any) error { return nil }`,
		"internal/router/routes.go": `package router
import "net/http"
func Routes() []any {
	return []any{
		http.NewRoute(http.MethodGet, "/ping", Ping),
	}
}
`,
	}
	for rel, contents := range files {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(contents), 0644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	manifest, err := Run(context.Background(), IndexOptions{Root: root})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	found := false
	for _, d := range manifest.Diagnostics {
		if d.Code == "handler_ambiguous" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected handler_ambiguous diagnostic, got %+v", manifest.Diagnostics)
	}
}

func TestRunExtractsStringAndNoContentResponses(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/test\n\ngo 1.24\n",
		"internal/hello/controller.go": `package hello
import (
	"net/http"
	"github.com/labstack/echo/v4"
)
type Controller struct{}
func (c *Controller) Routes() []any {
	return []any{
		http.NewRoute(http.MethodGet, "/status", c.Status),
	}
}
func (c *Controller) Status(ctx echo.Context) error {
	if ctx.QueryParam("fmt") == "text" {
		return ctx.String(http.StatusOK, "ok")
	}
	return ctx.NoContent(http.StatusNoContent)
}`,
	}
	for rel, contents := range files {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(contents), 0644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	manifest, err := Run(context.Background(), IndexOptions{Root: root})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(manifest.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(manifest.Operations))
	}
	op := manifest.Operations[0]
	has200 := false
	has204 := false
	for _, r := range op.Outputs.Responses {
		if r.StatusCode == 200 {
			has200 = true
		}
		if r.StatusCode == 204 {
			has204 = true
		}
	}
	if !has200 || !has204 {
		t.Fatalf("expected 200 and 204 responses, got %+v", op.Outputs.Responses)
	}
}

func TestRunWritesExpectedJSONShape(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/test\n\ngo 1.24\n",
		"internal/hello/controller.go": `package hello
import "net/http"
type Controller struct{}
func (c *Controller) Routes() []any {
	return []any{
		http.NewRoute(http.MethodGet, "/shape", c.Shape),
	}
}
func (c *Controller) Shape(ctx any) error { return nil }`,
	}
	for rel, contents := range files {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(contents), 0644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	out := filepath.Join(root, "build", "api_index.json")
	_, err := Run(context.Background(), IndexOptions{Root: root, OutPath: out})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read out: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if decoded["version"] == nil || decoded["operations"] == nil || decoded["diagnostics"] == nil {
		t.Fatalf("missing required top-level keys in manifest: %v", decoded)
	}
}

func TestRunExtractsPathParamsFromRouteTemplate(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/test\n\ngo 1.24\n",
		"internal/hello/controller.go": `package hello
import "net/http"
type Controller struct{}
func (c *Controller) Routes() []any {
	return []any{
		http.NewRoute(http.MethodGet, "/teams/:teamID/users/:userID", c.Show),
	}
}
func (c *Controller) Show(ctx any) error { return nil }`,
	}
	for rel, contents := range files {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(contents), 0644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	manifest, err := Run(context.Background(), IndexOptions{Root: root})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(manifest.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(manifest.Operations))
	}
	op := manifest.Operations[0]
	if len(op.Inputs.PathParams) != 2 {
		t.Fatalf("expected 2 path params, got %+v", op.Inputs.PathParams)
	}
	if op.Inputs.PathParams[0].Name != "teamID" || op.Inputs.PathParams[1].Name != "userID" {
		t.Fatalf("unexpected path params: %+v", op.Inputs.PathParams)
	}
}

func TestRunExtractsQueryParamsFromQueryParamsGetPattern(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/test\n\ngo 1.24\n",
		"internal/hello/controller.go": `package hello
import (
	"net/http"
	"github.com/labstack/echo/v4"
)
type Controller struct{}
func (c *Controller) Routes() []any {
	return []any{
		http.NewRoute(http.MethodGet, "/search", c.Search),
	}
}
func (c *Controller) Search(ctx echo.Context) error {
	_ = ctx.QueryParams().Get("page")
	return ctx.NoContent(http.StatusNoContent)
}`,
	}
	for rel, contents := range files {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(contents), 0644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	manifest, err := Run(context.Background(), IndexOptions{Root: root})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(manifest.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(manifest.Operations))
	}
	op := manifest.Operations[0]
	if len(op.Inputs.QueryParams) != 1 || op.Inputs.QueryParams[0].Name != "page" {
		t.Fatalf("expected query param page, got %+v", op.Inputs.QueryParams)
	}
}

func TestRunEmitsDynamicParamDiagnostics(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/test\n\ngo 1.24\n",
		"internal/hello/controller.go": `package hello
import (
	"net/http"
	"github.com/labstack/echo/v4"
)
type Controller struct{}
func (c *Controller) Routes() []any {
	return []any{
		http.NewRoute(http.MethodGet, "/search/:id", c.Search),
	}
}
func (c *Controller) Search(ctx echo.Context) error {
	key := "q"
	_ = ctx.QueryParam(key)
	headerKey := "X-Request-ID"
	_ = ctx.Request().Header.Get(headerKey)
	paramName := "id"
	_ = ctx.Param(paramName)
	return ctx.NoContent(http.StatusNoContent)
}`,
	}
	for rel, contents := range files {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(contents), 0644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	manifest, err := Run(context.Background(), IndexOptions{Root: root})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	codes := map[string]bool{}
	for _, d := range manifest.Diagnostics {
		if d.Code == "dynamic_param_key" {
			codes[d.Message] = true
		}
	}
	if len(codes) < 3 {
		t.Fatalf("expected dynamic param diagnostics, got %+v", manifest.Diagnostics)
	}
}
