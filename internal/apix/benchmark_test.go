package apix

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkRunSyntheticMediumRepo(b *testing.B) {
	benchmarkRunSyntheticRepo(b, 150, 4)
}

func BenchmarkRunSyntheticLargeRepo(b *testing.B) {
	benchmarkRunSyntheticRepo(b, 500, 5)
}

func benchmarkRunSyntheticRepo(b *testing.B, controllerCount, routesPerController int) {
	b.Helper()
	root := b.TempDir()
	if err := writeSyntheticRepo(root, controllerCount, routesPerController); err != nil {
		b.Fatalf("write synthetic repo: %v", err)
	}
	opts := IndexOptions{
		Root:            root,
		OutPath:         filepath.Join(root, "build", "api_index.json"),
		DiagnosticsPath: filepath.Join(root, "build", "api_index.diagnostics.json"),
		OpenAPIPath:     filepath.Join(root, "build", "openapi.json"),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Run(context.Background(), opts); err != nil {
			b.Fatalf("Run failed: %v", err)
		}
	}
}

func writeSyntheticRepo(root string, controllerCount, routesPerController int) error {
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/bench\n\ngo 1.24\n"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("APP_NAME=Synthetic Benchmark\n"), 0o644); err != nil {
		return err
	}
	for c := 0; c < controllerCount; c++ {
		pkg := fmt.Sprintf("c%d", c)
		dir := filepath.Join(root, "internal", pkg)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		src := "package " + pkg + "\n\n" +
			"import (\n\t\"net/http\"\n\t\"github.com/labstack/echo/v4\"\n)\n\n" +
			"type payload struct { Name string `json:\"name\"`; Enabled bool `json:\"enabled,omitempty\"` }\n" +
			"type Controller struct{}\n\n" +
			"func (c *Controller) Routes() []any {\n\treturn []any{\n"
		for r := 0; r < routesPerController; r++ {
			src += fmt.Sprintf("\t\thttp.NewRoute(http.MethodGet, \"/%s/r%d/:id\", c.H%d),\n", pkg, r, r)
		}
		src += "\t}\n}\n\n"
		for r := 0; r < routesPerController; r++ {
			src += fmt.Sprintf("func (c *Controller) H%d(ctx echo.Context) error {\n", r) +
				"\tif ctx.QueryParam(\"full\") == \"1\" {\n" +
				"\t\treturn ctx.JSON(http.StatusOK, map[string]any{\"ok\": true, \"id\": ctx.Param(\"id\"), \"mode\": \"full\"})\n" +
				"\t}\n" +
				"\treturn ctx.JSON(http.StatusOK, map[string]any{\"ok\": true, \"id\": ctx.Param(\"id\")})\n" +
				"}\n\n"
		}
		if err := os.WriteFile(filepath.Join(dir, "controller.go"), []byte(src), 0o644); err != nil {
			return err
		}
	}
	return nil
}
