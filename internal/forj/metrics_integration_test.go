//go:build integration

package forj

import (
	"context"
	"io"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"regexp"
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
		name:   "http",
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
		"# TYPE http_request_duration_seconds histogram",
		`http_requests_total{source="http"} 1`,
		`http_requests_inflight{source="http"} 0`,
		`http_request_duration_seconds_count{source="http"} 1`,
		`http_requests_by_route_total{source="http",method="GET",route="/api/v1/hello",status="200"} 1`,
		`http_request_duration_by_route_seconds_count{source="http",method="GET",route="/api/v1/hello"} 1`,
	} {
		if !strings.Contains(text, token) {
			t.Fatalf("GET /metrics missing %q\nbody:\n%s", token, text)
		}
	}
	if !strings.Contains(text, `http_requests_total{source="http"} 1`) {
		t.Fatalf("GET /metrics expected scrape to be excluded from request count\nbody:\n%s", text)
	}
	if !strings.Contains(text, `http_requests_inflight{source="http"} 0`) {
		t.Fatalf("GET /metrics expected scrape to be excluded from inflight gauge\nbody:\n%s", text)
	}
	if !strings.Contains(text, `http_request_duration_seconds_count{source="http"} 1`) {
		t.Fatalf("GET /metrics expected scrape to be excluded from latency histogram\nbody:\n%s", text)
	}
	if !strings.Contains(text, `http_requests_by_route_total{source="http",method="GET",route="/api/v1/hello",status="200"} 1`) {
		t.Fatalf("GET /metrics expected labeled route counter for /api/v1/hello\nbody:\n%s", text)
	}
	if !strings.Contains(text, `http_request_duration_by_route_seconds_count{source="http",method="GET",route="/api/v1/hello"} 1`) {
		t.Fatalf("GET /metrics expected labeled route histogram for /api/v1/hello\nbody:\n%s", text)
	}
}

func TestRenderedDemoAppStartupSourceMetrics(t *testing.T) {
	projectDir := t.TempDir()
	testkit.RenderProjectWithForj(t, projectDir, testkit.RenderProjectRequest{
		Config: project.Config{
			ProjectName:  "Startup Metrics App",
			GoModuleName: "example.com/startupmetricsapp",
			UpdatedAt:    "2026-05-06 00:00:00 UTC",
			Render: project.RenderConfig{
				Components: project.Components{
					WebAPI:         true,
					Metrics:        true,
					DemoApp:        true,
					Jobs:           true,
					DatabaseSQLite: true,
				},
			},
		},
	})

	binPath := filepath.Join(t.TempDir(), "app")
	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	buildCmd.Dir = projectDir
	buildCmd.Env = testkit.IntegrationGoProcessEnv(t, nil)
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build rendered demo app: %v\n%s", err, out)
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
		name:   "http",
		cmd:    cmd,
		cancel: cancel,
	}
	cmd.Stdout = &handle.stdout
	cmd.Stderr = &handle.stderr
	if err := handle.Start(); err != nil {
		t.Fatalf("start rendered demo app: %v", err)
	}
	defer stopProcAsync(t, "startup-metrics-server", handle, time.Second)

	baseURL := "http://127.0.0.1:" + httpPort
	if !waitForTCP(t, "127.0.0.1:"+httpPort, 3*time.Second) {
		t.Fatalf("server did not accept TCP connections before timeout\n%s", handle.Output())
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

	startupCacheMetric := regexp.MustCompile(`cache_operations_total\{[^\n]*source="startup"`)
	if !startupCacheMetric.MatchString(text) {
		t.Fatalf("GET /metrics missing startup-scoped cache metrics\nbody:\n%s", text)
	}
}

func TestRenderedJobsSourceMetrics(t *testing.T) {
	redisHost, redisPort := testkit.EnsureIntegrationRedis(t)
	queueEnv := map[string]string{
		"QUEUE_DRIVER":        "redis",
		"QUEUE_DEFAULT_QUEUE": "default",
		"QUEUE_ADDR":          net.JoinHostPort(redisHost, redisPort),
		"REDIS_HOST":          redisHost,
		"REDIS_PORT":          redisPort,
	}

	projectDir := t.TempDir()
	testkit.RenderProjectWithForj(t, projectDir, testkit.RenderProjectRequest{
		Config: project.Config{
			ProjectName:  "Jobs Metrics App",
			GoModuleName: "example.com/jobsmetricsapp",
			UpdatedAt:    "2026-05-06 00:00:00 UTC",
			Render: project.RenderConfig{
				QueueDriver: "redis",
				Components: project.Components{
					CLI:            true,
					WebAPI:         true,
					Jobs:           true,
					DemoApp:        true,
					DatabaseSQLite: true,
				},
			},
		},
		EnvOverrides: queueEnv,
	})

	binPath := filepath.Join(t.TempDir(), "app")
	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	buildCmd.Dir = projectDir
	buildCmd.Env = testkit.IntegrationGoProcessEnv(t, nil)
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build rendered jobs app: %v\n%s", err, out)
	}

	runCommandSuccess(t, projectDir, binPath, queueEnv, "migrate")
	runCommandSuccess(t, projectDir, binPath, queueEnv, "monitor:seed")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	workerCmd := exec.CommandContext(ctx, binPath, "queue:work")
	workerCmd.Dir = projectDir
	workerCmd.Env = testkit.IntegrationProcessEnv(t, queueEnv)
	worker := &procHandle{
		name:   "jobs-worker",
		cmd:    workerCmd,
		cancel: cancel,
	}
	workerCmd.Stdout = &worker.stdout
	workerCmd.Stderr = &worker.stderr
	if err := worker.Start(); err != nil {
		t.Fatalf("start jobs worker: %v", err)
	}
	defer stopProcAsync(t, "jobs-worker", worker, time.Second)

	if !waitForOutputContains(worker, []string{"Queue worker started", "driver redis"}, 5*time.Second) {
		t.Fatalf("jobs worker did not report ready state before timeout\n%s", worker.Output())
	}

	enqueueOut := runCommandSuccess(t, projectDir, binPath, queueEnv, "monitor:poll")

	if !waitForOutputContains(worker, []string{"source jobs", "queue_event process_succeeded", "job_name monitoring:check"}, 10*time.Second) {
		t.Fatalf("jobs worker output missing jobs-scoped queue success log\nenqueue:\n%s\n%s", string(enqueueOut), worker.Output())
	}
}

func TestRenderedSchedulerSourceMetrics(t *testing.T) {
	redisHost, redisPort := testkit.EnsureIntegrationRedis(t)
	validQueueEnv := map[string]string{
		"QUEUE_DRIVER":        "redis",
		"QUEUE_DEFAULT_QUEUE": "default",
		"QUEUE_ADDR":          net.JoinHostPort(redisHost, redisPort),
		"REDIS_HOST":          redisHost,
		"REDIS_PORT":          redisPort,
	}
	invalidQueueEnv := map[string]string{
		"QUEUE_DRIVER":        "redis",
		"QUEUE_DEFAULT_QUEUE": "default",
		"QUEUE_ADDR":          "127.0.0.1:1",
		"REDIS_HOST":          "127.0.0.1",
		"REDIS_PORT":          "1",
	}

	projectDir := t.TempDir()
	testkit.RenderProjectWithForj(t, projectDir, testkit.RenderProjectRequest{
		Config: project.Config{
			ProjectName:  "Scheduler Metrics App",
			GoModuleName: "example.com/schedulermetricsapp",
			UpdatedAt:    "2026-05-06 00:00:00 UTC",
			Render: project.RenderConfig{
				QueueDriver: "redis",
				Components: project.Components{
					CLI:            true,
					WebAPI:         true,
					Jobs:           true,
					Scheduler:      true,
					Metrics:        true,
					DemoApp:        true,
					DatabaseSQLite: true,
				},
			},
		},
		EnvOverrides: validQueueEnv,
	})

	binPath := filepath.Join(t.TempDir(), "app")
	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	buildCmd.Dir = projectDir
	buildCmd.Env = testkit.IntegrationGoProcessEnv(t, nil)
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build rendered scheduler app: %v\n%s", err, out)
	}

	runCommandSuccess(t, projectDir, binPath, validQueueEnv, "migrate")
	runCommandSuccess(t, projectDir, binPath, validQueueEnv, "monitor:seed")

	schedulerAddr := findFreeAddr(t)
	_, schedulerPort, err := net.SplitHostPort(schedulerAddr)
	if err != nil {
		t.Fatalf("split scheduler addr: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	schedulerCmd := exec.CommandContext(ctx, binPath, "schedule:run", "--metrics-port", schedulerPort)
	schedulerCmd.Dir = projectDir
	schedulerCmd.Env = testkit.IntegrationProcessEnv(t, invalidQueueEnv)
	schedulerProc := &procHandle{
		name:   "scheduler",
		cmd:    schedulerCmd,
		cancel: cancel,
	}
	schedulerCmd.Stdout = &schedulerProc.stdout
	schedulerCmd.Stderr = &schedulerProc.stderr
	if err := schedulerProc.Start(); err != nil {
		t.Fatalf("start scheduler runtime: %v", err)
	}
	defer stopProcAsync(t, "scheduler", schedulerProc, time.Second)

	if !waitForTCP(t, "127.0.0.1:"+schedulerPort, 5*time.Second) {
		t.Fatalf("scheduler metrics endpoint did not accept TCP connections before timeout\n%s", schedulerProc.Output())
	}

	metricsURL := "http://127.0.0.1:" + schedulerPort + "/metrics"
	schedulerMetric := regexp.MustCompile(`scheduler_runs_by_job_total\{[^\n]*source="scheduler"[^\n]*job_name="monitor:poll"[^\n]*status="failed"\}\s+[1-9]`)
	if !waitForMetricsMatch(t, metricsURL, schedulerMetric, 40*time.Second) {
		body := fetchMetricsText(t, metricsURL)
		t.Fatalf("scheduler metrics missing scheduler-scoped monitor:poll failure counter\nbody:\n%s\n%s", body, schedulerProc.Output())
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
	})
}

func waitForMetricsMatch(t *testing.T, url string, pattern *regexp.Regexp, timeout time.Duration) bool {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		body := fetchMetricsText(t, url)
		if pattern.MatchString(body) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func fetchMetricsText(t *testing.T, url string) string {
	t.Helper()

	resp, err := http.Get(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	return string(body)
}

func waitForOutputContains(proc *procHandle, tokens []string, timeout time.Duration) bool {
	if proc == nil {
		return false
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out := ansiEscapeRe.ReplaceAllString(proc.Output(), "")
		matched := true
		for _, token := range tokens {
			if !strings.Contains(out, token) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func mergeEnv(base map[string]string, extra map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(extra))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range extra {
		merged[key] = value
	}
	return merged
}

func runCommandSuccess(t *testing.T, projectDir, binPath string, envOverrides map[string]string, args ...string) []byte {
	t.Helper()

	cmd := exec.Command(binPath, args...)
	cmd.Dir = projectDir
	cmd.Env = testkit.IntegrationProcessEnv(t, envOverrides)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return out
}
