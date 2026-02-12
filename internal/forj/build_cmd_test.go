package forj

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goforj/goforj/internal/logger"
)

func TestBuildCmdRunExecutesBuildPipelines(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/test\n\ngo 1.24\n",
		"internal/hello/controller.go": `package hello
import "net/http"
type Controller struct{}
func (c *Controller) Routes() []any {
	return []any{
		http.NewRoute(http.MethodGet, "/hello", c.Hello),
	}
}
func (c *Controller) Hello(ctx any) error { return nil }`,
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

	appLogger := logger.NewSilentLogger()
	api := NewApiIndexCmd(appLogger)
	api.Root = root
	api.Out = filepath.Join(root, "build", "api_index.json")
	api.Diagnostics = filepath.Join(root, "build", "api_index.diagnostics.json")
	api.OpenAPI = filepath.Join(root, "build", "openapi.json")

	build := NewBuildCmd(appLogger, api)
	if err := build.Run(); err != nil {
		t.Fatalf("build run failed: %v", err)
	}

	for _, p := range []string{api.Out, api.Diagnostics, api.OpenAPI} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected artifact %s: %v", p, err)
		}
	}
}
