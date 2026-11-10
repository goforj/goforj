//go:build integration

package forj

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

// TestRenderedGrafanaTraceDefaults exercises real SDK delivery from a generated non-test binary without requiring Docker.
func TestRenderedGrafanaTraceDefaults(t *testing.T) {
	root := t.TempDir()
	testkit.RenderProjectWithForj(t, root, testkit.RenderProjectRequest{Config: project.Config{
		ProjectName: "Grafana Trace Defaults", GoModuleName: "example.com/grafanatraces",
		Render: project.RenderConfig{Components: project.Components{CLI: true, WebAPI: true, Metrics: true, Observability: true, Grafana: true, Docker: true}},
	}})
	binary := filepath.Join(root, "trace-defaults-app")
	build := exec.Command("go", "build", "-o", binary, "./cmd/app")
	build.Dir = root
	build.Env = testkit.IntegrationGoProcessEnv(t, nil)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build generated app: %v\n%s", err, out)
	}
	for _, tc := range []struct {
		name, environment, profiles, exporter     string
		explicit, specific, disabled, invalidPort bool
		wantError                                 bool
		wantLocal, wantExternal                   bool
	}{
		{name: "automatic local", environment: "local", profiles: "grafana", wantLocal: true},
		{name: "explicit otlp without endpoint", environment: "local", profiles: "grafana", exporter: "otlp", wantLocal: true},
		{name: "generic endpoint wins", environment: "local", profiles: "grafana", explicit: true, invalidPort: true, wantExternal: true},
		{name: "trace endpoint wins", environment: "local", profiles: "grafana", specific: true, invalidPort: true, wantExternal: true},
		{name: "explicit none", environment: "local", profiles: "grafana", exporter: "none", invalidPort: true},
		{name: "sdk disabled", environment: "local", profiles: "grafana", disabled: true, invalidPort: true},
		{name: "production", environment: "production", profiles: "grafana", invalidPort: true},
		{name: "staging", environment: "staging", profiles: "grafana", invalidPort: true},
		{name: "testing", environment: "testing", profiles: "grafana", invalidPort: true},
		{name: "inactive profile", environment: "local", profiles: "victoriametrics", invalidPort: true},
		{name: "invalid active port", environment: "local", profiles: "grafana", invalidPort: true, wantError: true},
		{name: "profile substring", environment: "local", profiles: "mygrafana", invalidPort: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var local, external atomic.Int64
			collector := func(counter *atomic.Int64) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil || len(body) == 0 || r.Method != http.MethodPost || r.URL.Path != "/v1/traces" || r.Header.Get("Content-Type") != "application/x-protobuf" {
						t.Errorf("invalid OTLP request: %s %s, body=%d, err=%v", r.Method, r.URL.Path, len(body), err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					counter.Add(1)
					w.Header().Set("Content-Type", "application/x-protobuf")
				}))
			}
			localCollector := collector(&local)
			defer localCollector.Close()
			externalCollector := collector(&external)
			defer externalCollector.Close()
			localURL, err := url.Parse(localCollector.URL)
			if err != nil {
				t.Fatal(err)
			}
			settings := map[string]string{
				"APP_ENV": tc.environment, "COMPOSE_PROFILES": tc.profiles,
				"TEMPO_OTLP_HTTP_PORT": localURL.Port(), "IP_ADDRESS": "127.0.0.1",
				"OTEL_LOGS_EXPORTER": "none", "OTEL_TRACES_EXPORTER": tc.exporter, "OTEL_EXPORTER_OTLP_ENDPOINT": "", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "",
				"OTEL_SDK_DISABLED": "false", "OTEL_EXPORTER_OTLP_PROTOCOL": "http/protobuf",
				"OTEL_TRACES_SAMPLER": "always_on", "LIGHTHOUSE_ENABLED": "false",
			}
			if tc.invalidPort {
				settings["TEMPO_OTLP_HTTP_PORT"] = "invalid"
			}
			if tc.explicit {
				settings["OTEL_EXPORTER_OTLP_ENDPOINT"] = externalCollector.URL
			}
			if tc.specific {
				settings["OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"] = externalCollector.URL + "/v1/traces"
			}
			if tc.disabled {
				settings["OTEL_SDK_DISABLED"] = "true"
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, "hello:world")
			cmd.Dir = root
			cmd.Env = testkit.IntegrationProcessEnv(t, settings)
			out, runErr := cmd.CombinedOutput()
			if (runErr != nil) != tc.wantError {
				t.Fatalf("run generated app: %v, want error=%v\n%s", runErr, tc.wantError, out)
			}
			if (local.Load() > 0) != tc.wantLocal || (external.Load() > 0) != tc.wantExternal {
				t.Fatalf("deliveries local=%d external=%d; want local=%v external=%v", local.Load(), external.Load(), tc.wantLocal, tc.wantExternal)
			}
		})
	}
}

// TestRenderedGrafanaLogDefaults exercises real SDK delivery from a generated non-test binary without requiring Docker.
func TestRenderedGrafanaLogDefaults(t *testing.T) {
	root := t.TempDir()
	testkit.RenderProjectWithForj(t, root, testkit.RenderProjectRequest{Config: project.Config{
		ProjectName: "Grafana Trace Defaults", GoModuleName: "example.com/grafanatraces",
		Render: project.RenderConfig{Components: project.Components{CLI: true, WebAPI: true, Metrics: true, Observability: true, Grafana: true, Docker: true}},
	}})
	binary := filepath.Join(root, "trace-defaults-app")
	build := exec.Command("go", "build", "-o", binary, "./cmd/app")
	build.Dir = root
	build.Env = testkit.IntegrationGoProcessEnv(t, nil)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build generated app: %v\n%s", err, out)
	}
	for _, tc := range []struct {
		name, environment, profiles, exporter     string
		explicit, specific, disabled, invalidPort bool
		wantError                                 bool
		wantLocal, wantExternal                   bool
	}{
		{name: "automatic local", environment: "local", profiles: "grafana", wantLocal: true},
		{name: "explicit otlp without endpoint", environment: "local", profiles: "grafana", exporter: "otlp", wantLocal: true},
		{name: "generic endpoint wins when enabled", environment: "local", profiles: "grafana", exporter: "otlp", explicit: true, invalidPort: true, wantExternal: true},
		{name: "trace-only generic endpoint", environment: "local", profiles: "grafana", explicit: true, invalidPort: true},
		{name: "trace endpoint wins", environment: "local", profiles: "grafana", specific: true, invalidPort: true, wantExternal: true},
		{name: "explicit none", environment: "local", profiles: "grafana", exporter: "none", invalidPort: true},
		{name: "sdk disabled", environment: "local", profiles: "grafana", disabled: true, invalidPort: true},
		{name: "production", environment: "production", profiles: "grafana", invalidPort: true},
		{name: "staging", environment: "staging", profiles: "grafana", invalidPort: true},
		{name: "testing", environment: "testing", profiles: "grafana", invalidPort: true},
		{name: "inactive profile", environment: "local", profiles: "victoriametrics", invalidPort: true},
		{name: "invalid active port", environment: "local", profiles: "grafana", invalidPort: true, wantError: true},
		{name: "profile substring", environment: "local", profiles: "mygrafana", invalidPort: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var local, external atomic.Int64
			collector := func(counter *atomic.Int64) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil || len(body) == 0 || r.Method != http.MethodPost || r.URL.Path != "/v1/logs" || r.Header.Get("Content-Type") != "application/x-protobuf" {
						t.Errorf("invalid OTLP request: %s %s, body=%d, err=%v", r.Method, r.URL.Path, len(body), err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					counter.Add(1)
					w.Header().Set("Content-Type", "application/x-protobuf")
				}))
			}
			localCollector := collector(&local)
			defer localCollector.Close()
			externalCollector := collector(&external)
			defer externalCollector.Close()
			localURL, err := url.Parse(localCollector.URL)
			if err != nil {
				t.Fatal(err)
			}
			settings := map[string]string{
				"APP_ENV": tc.environment, "COMPOSE_PROFILES": tc.profiles,
				"TEMPO_OTLP_HTTP_PORT": localURL.Port(), "IP_ADDRESS": "127.0.0.1",
				"OTEL_TRACES_EXPORTER": "none", "OTEL_LOGS_EXPORTER": tc.exporter, "OTEL_EXPORTER_OTLP_ENDPOINT": "", "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT": "",
				"OTEL_SDK_DISABLED": "false", "OTEL_EXPORTER_OTLP_PROTOCOL": "http/protobuf",
				"OTEL_TRACES_SAMPLER": "always_on", "LIGHTHOUSE_ENABLED": "false",
			}
			if tc.invalidPort {
				settings["TEMPO_OTLP_HTTP_PORT"] = "invalid"
			}
			if tc.explicit {
				settings["OTEL_EXPORTER_OTLP_ENDPOINT"] = externalCollector.URL
			}
			if tc.specific {
				settings["OTEL_EXPORTER_OTLP_LOGS_ENDPOINT"] = externalCollector.URL + "/v1/logs"
			}
			if tc.disabled {
				settings["OTEL_SDK_DISABLED"] = "true"
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, "hello:world")
			cmd.Dir = root
			cmd.Env = testkit.IntegrationProcessEnv(t, settings)
			out, runErr := cmd.CombinedOutput()
			if (runErr != nil) != tc.wantError {
				t.Fatalf("run generated app: %v, want error=%v\n%s", runErr, tc.wantError, out)
			}
			if (local.Load() > 0) != tc.wantLocal || (external.Load() > 0) != tc.wantExternal {
				t.Fatalf("deliveries local=%d external=%d; want local=%v external=%v", local.Load(), external.Load(), tc.wantLocal, tc.wantExternal)
			}
		})
	}
}
