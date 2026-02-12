package apix

import (
	"context"
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
