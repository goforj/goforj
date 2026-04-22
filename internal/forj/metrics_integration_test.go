//go:build integration

package forj

import (
	"context"
	"io"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

func TestRenderedAppMetricsEndpoint(t *testing.T) {
	projectDir := t.TempDir()
	renderMetricsTestApp(t, projectDir)

	binPath := filepath.Join(t.TempDir(), "app")
	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	buildCmd.Dir = projectDir
	buildCmd.Env = testkit.IntegrationGoProcessEnv(t, nil)
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build rendered app: %v\n%s", err, out)
	}

	httpAddr := findFreeAddr(t)
	_, httpPort, err := net.SplitHostPort(httpAddr)
	if err != nil {
		t.Fatalf("split http addr: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, "http:serve", "--port", httpPort)
	cmd.Dir = projectDir
	cmd.Env = testkit.IntegrationProcessEnv(t, nil)
	handle := &procHandle{
		name:   "api",
		cmd:    cmd,
		cancel: cancel,
	}
	cmd.Stdout = &handle.stdout
	cmd.Stderr = &handle.stderr
	if err := handle.Start(); err != nil {
		t.Fatalf("start rendered app: %v", err)
	}
	defer stopProcAsync(t, "metrics-server", handle, time.Second)

	baseURL := "http://127.0.0.1:" + httpPort
	if !waitForTCP(t, "127.0.0.1:"+httpPort, 3*time.Second) {
		t.Fatalf("server did not accept TCP connections before timeout\n%s", handle.Output())
	}

	helloResp, err := http.Get(baseURL + "/api/v1/hello")
	if err != nil {
		t.Fatalf("get hello endpoint: %v\n%s", err, handle.Output())
	}
	_ = helloResp.Body.Close()
	if helloResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/hello status = %d, want %d\n%s", helloResp.StatusCode, http.StatusOK, handle.Output())
	}

	resp, err := http.Get(baseURL + "/metrics")
	if err != nil {
		t.Fatalf("get metrics endpoint: %v\n%s", err, handle.Output())
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read metrics response: %v", err)
	}
	text := string(body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want %d\nbody:\n%s\n%s", resp.StatusCode, http.StatusOK, text, handle.Output())
	}
	if got := resp.Header.Get("Content-Type"); got != "text/plain; version=0.0.4; charset=utf-8" {
		t.Fatalf("GET /metrics content-type = %q, want %q", got, "text/plain; version=0.0.4; charset=utf-8")
	}
	for _, token := range []string{
		"# TYPE http_requests_total counter",
		"# TYPE http_requests_inflight gauge",
		"# TYPE http_responses_client_errors_total counter",
		"# TYPE http_responses_server_errors_total counter",
		"# TYPE http_request_duration_seconds histogram",
	} {
		if !strings.Contains(text, token) {
			t.Fatalf("GET /metrics missing %q\nbody:\n%s", token, text)
		}
	}
	if !strings.Contains(text, "http_requests_total 1") {
		t.Fatalf("GET /metrics expected scrape to be excluded from request count\nbody:\n%s", text)
	}
	if !strings.Contains(text, "http_requests_inflight 0") {
		t.Fatalf("GET /metrics expected scrape to be excluded from inflight gauge\nbody:\n%s", text)
	}
	if !strings.Contains(text, "http_request_duration_seconds_count 1") {
		t.Fatalf("GET /metrics expected scrape to be excluded from latency histogram\nbody:\n%s", text)
	}
	if !strings.Contains(text, `http_requests_by_route_total{method="GET",route="/api/v1/hello",status="200"} 1`) {
		t.Fatalf("GET /metrics expected labeled route counter for /api/v1/hello\nbody:\n%s", text)
	}
	if !strings.Contains(text, `http_request_duration_by_route_seconds_count{method="GET",route="/api/v1/hello"} 1`) {
		t.Fatalf("GET /metrics expected labeled route histogram for /api/v1/hello\nbody:\n%s", text)
	}
}

func renderMetricsTestApp(t *testing.T, dir string) {
	t.Helper()

	testkit.RenderProjectWithForj(t, dir, testkit.RenderProjectRequest{
		Config: project.Config{
			ProjectName:  "Metrics Test App",
			GoModuleName: "example.com/metricstestapp",
			UpdatedAt:    "2026-04-21 00:00:00 UTC",
			Render: project.RenderConfig{
				Components: project.Components{
					CLI:     true,
					WebAPI:  true,
					Metrics: true,
				},
			},
		},
		ModuleReplaces: map[string]string{
			"github.com/goforj/metrics": testkit.LocalSiblingRepoPath(t, "metrics"),
		},
	})
}
