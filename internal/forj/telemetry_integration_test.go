//go:build integration

package forj

import (
	"os"
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
			packages := []string{"./internal/logger", "./internal/inspects", "./internal/http", "./app/wire"}
			if full {
				components = project.Components{CLI: true, DemoApp: true, Mail: true, Auth: true, OAuth: true, WebAPI: true, WebUI: true, Metrics: true, Observability: true, Grafana: true, Docker: true, DatabaseSQLite: true, Scheduler: true, Cache: true, Events: true, Storage: true, Jobs: true}
				if err := writeConventionalAppMarker(root, "admin"); err != nil {
					t.Fatal(err)
				}
				apps["admin"] = project.AppConfig{Components: components}
				packages = append(packages, "./app/admin/wire", "./internal/queues", "./internal/events", "./internal/observability", "./internal/metrics", "./internal/lighthouse")
			}
			testkit.RenderProjectWithForj(t, root, testkit.RenderProjectRequest{Config: project.Config{
				ProjectName: "Telemetry Test", GoModuleName: "example.com/telemetrytest", Render: project.RenderConfig{Components: components}, Apps: apps,
			}})
			if !full {
				addTelemetryCLIPrivacyFixture(t, root)
			}
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

// addTelemetryCLIPrivacyFixture exercises app-owned global flags without changing the generated RootCmd schema.
func addTelemetryCLIPrivacyFixture(t *testing.T, root string) {
	t.Helper()
	commandsPath := filepath.Join(root, "app", "commands.go")
	commands, err := os.ReadFile(commandsPath)
	if err != nil {
		t.Fatalf("read app-owned commands for telemetry privacy fixture: %v", err)
	}
	const marker = "type Commands struct {"
	if strings.Count(string(commands), marker) != 1 {
		t.Fatal("app-owned Commands marker must occur once")
	}
	const fields = "\n\tToken string `name:\"token\" help:\"Authentication token\"`\n\tReviewSecretCmd telemetryPrivacyCommand `cmd:\"\" name:\"review:secret\"`"
	updated := strings.Replace(string(commands), marker, marker+fields, 1)
	updated += "\n// telemetryPrivacyCommand models an app-owned command with a sensitive positional value.\ntype telemetryPrivacyCommand struct { Value string `arg:\"\"` }\n\n// Run completes without interpreting the fixture input.\nfunc (*telemetryPrivacyCommand) Run() error { return nil }\n"
	if err := os.WriteFile(commandsPath, []byte(updated), 0o644); err != nil {
		t.Fatalf("write app-owned telemetry privacy command: %v", err)
	}
	const privacyTest = `package wire

import (
	"context"
	"example.com/telemetrytest/app"
	"example.com/telemetrytest/internal/inspects"
	"example.com/telemetrytest/internal/logger"
	"example.com/telemetrytest/internal/runtime"
	"github.com/alecthomas/kong"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"strings"
	"testing"
)

// TestAppRunDoesNotExportCLIValues verifies automatic CLI span names exclude application flag and positional values.
func TestAppRunDoesNotExportCLIValues(t *testing.T) {
	args := []string{"--token=private-cli-secret", "review:secret", "private-positional-secret"}
	root := &app.RootCmd{}
	parser, err := kong.New(root)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parser.Parse(args)
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.Command(); got != "review:secret <value>" {
		t.Fatalf("parsed grammar name=%q", got)
	}
	t.Setenv("LIGHTHOUSE_ENABLED", "true")
	t.Setenv("LIGHTHOUSE_INSPECT_ENABLED", "true")
	manager := inspects.NewManager()
	exporter := &cliPrivacySpanExporter{InMemoryExporter: tracetest.NewInMemoryExporter()}
	manager.TracerProvider().RegisterSpanProcessor(sdktrace.NewSimpleSpanProcessor(exporter))
	timeouts := runtime.NewTimeouts()
	application := App{rootCmd: &app.RootCmd{}, inspects: manager, logger: logger.NewSilentLogger(), lifecycle: runtime.NewLifecycle(timeouts), appTimeouts: timeouts}
	if err := application.Run(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported %d spans, want CLI execution", len(spans))
	}
	if strings.Contains(spans[0].Name, "private-") {
		t.Fatalf("automatic CLI span exported raw arguments: %q", spans[0].Name)
	}
	if spans[0].Name != "review:secret <value>" {
		t.Fatalf("CLI span name=%q", spans[0].Name)
	}
}

// cliPrivacySpanExporter retains evidence after App.Run shuts down its SDK.
type cliPrivacySpanExporter struct { *tracetest.InMemoryExporter }

// Shutdown leaves exported spans available for the privacy assertion.
func (*cliPrivacySpanExporter) Shutdown(context.Context) error { return nil }
`
	if err := os.WriteFile(filepath.Join(root, "app", "wire", "cli_privacy_test.go"), []byte(privacyTest), 0o644); err != nil {
		t.Fatalf("write telemetry CLI privacy test: %v", err)
	}
}
