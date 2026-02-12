package forj

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goforj/goforj/internal/logger"
)

func TestApiIndexCmdRunWritesArtifacts(t *testing.T) {
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

	cmd := NewApiIndexCmd(logger.NewSilentLogger())
	cmd.Root = root
	cmd.Out = filepath.Join(root, "build", "api_index.json")
	cmd.Diagnostics = filepath.Join(root, "build", "api_index.diagnostics.json")
	cmd.OpenAPI = filepath.Join(root, "build", "openapi.json")

	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	for _, p := range []string{cmd.Out, cmd.Diagnostics, cmd.OpenAPI} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected artifact %s: %v", p, err)
		}
	}
}
