//go:build integration

package forj

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

// TestRenderedTelemetryContract validates OTel export, Lighthouse projection, HTTP instrumentation, and app shutdown in real generated code.
func TestRenderedTelemetryContract(t *testing.T) {

	for _, full := range []bool{false, true} {
		name := "minimal"
		if full {
			name = "full-multi-app"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			components := project.Components{CLI: true, WebAPI: true, Metrics: true}
			apps := map[string]project.AppConfig{}
			packages := []string{"./internal/inspects", "./internal/http", "./app/wire"}
			if full {
				components = project.Components{CLI: true, DemoApp: true, Mail: true, Auth: true, OAuth: true, WebAPI: true, WebUI: true, Metrics: true, Observability: true, Grafana: true, Docker: true, DatabaseSQLite: true, Scheduler: true, Cache: true, Events: true, Storage: true, Jobs: true}
				if err := writeConventionalAppMarker(root, "admin"); err != nil {
					t.Fatal(err)
				}
				apps["admin"] = project.AppConfig{Components: components}
				packages = append(packages, "./app/admin/wire", "./internal/queues", "./internal/observability")
			}
			testkit.RenderProjectWithForj(t, root, testkit.RenderProjectRequest{Config: project.Config{
				ProjectName: "Telemetry Test", GoModuleName: "example.com/telemetrytest", Render: project.RenderConfig{Components: components}, Apps: apps,
			}})
			args := append([]string{"test", "-race"}, packages...)
			cmd := exec.Command("go", args...)
			cmd.Dir = filepath.Clean(root)
			cmd.Env = testkit.IntegrationGoProcessEnv(t, map[string]string{"GOWORK": "off"})
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("rendered telemetry contract: %v\n%s", err, out)
			} else {
				t.Logf("%s", out)
			}
			modules := exec.Command("go", "list", "-m", "-f", "{{if .Replace}}{{.Path}}{{end}}", "all")
			modules.Dir = root
			modules.Env = cmd.Env
			if out, err := modules.CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "" {
				t.Fatalf("rendered modules must resolve published versions without replacements: %v\n%s", err, out)
			}
			if !full {
				bench := exec.Command("go", "test", "-run", "^$", "-bench", "^BenchmarkInspectBeginFinish$|^BenchmarkHTTPRequestInspectModes$", "-benchtime=100ms", "-benchmem", "./internal/inspects", "./internal/http")
				bench.Dir = root
				bench.Env = cmd.Env
				if out, err := bench.CombinedOutput(); err != nil {
					t.Fatalf("inspect benchmark: %v\n%s", err, out)
				} else {
					t.Logf("%s", out)
				}
			}
		})
	}
}
