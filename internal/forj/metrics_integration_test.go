//go:build integration

package forj

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os/exec"
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

	binPath := buildRenderedDefaultApp(t, projectDir, nil, "build rendered app")

	httpAddr := findFreeAddr(t)
	_, httpPort, err := net.SplitHostPort(httpAddr)
	if err != nil {
		t.Fatalf("split http addr: %v", err)
	}
	metricsAddr := findFreeAddr(t)
	_, metricsPort, err := net.SplitHostPort(metricsAddr)
	if err != nil {
		t.Fatalf("split metrics addr: %v", err)
	}
	setRenderedEnvValue(t, projectDir, "METRICS_API_PORT", metricsPort)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, "http:serve", "--port", httpPort, "--metrics-port", metricsPort)
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
		`http_requests_total{app="app",source="http"} 1`,
		`http_requests_inflight{app="app",source="http"} 0`,
		`http_request_duration_seconds_count{app="app",source="http"} 1`,
		`http_requests_by_route_total{app="app",source="http",method="GET",route="/api/v1/hello",status="200"} 1`,
		`http_request_duration_by_route_seconds_count{app="app",source="http",method="GET",route="/api/v1/hello"} 1`,
	} {
		if !strings.Contains(text, token) {
			t.Fatalf("GET /metrics missing %q\nbody:\n%s", token, text)
		}
	}
	if !strings.Contains(text, `http_requests_total{app="app",source="http"} 1`) {
		t.Fatalf("GET /metrics expected scrape to be excluded from request count\nbody:\n%s", text)
	}
	if !strings.Contains(text, `http_requests_inflight{app="app",source="http"} 0`) {
		t.Fatalf("GET /metrics expected scrape to be excluded from inflight gauge\nbody:\n%s", text)
	}
	if !strings.Contains(text, `http_request_duration_seconds_count{app="app",source="http"} 1`) {
		t.Fatalf("GET /metrics expected scrape to be excluded from latency histogram\nbody:\n%s", text)
	}
	if !strings.Contains(text, `http_requests_by_route_total{app="app",source="http",method="GET",route="/api/v1/hello",status="200"} 1`) {
		t.Fatalf("GET /metrics expected labeled route counter for /api/v1/hello\nbody:\n%s", text)
	}
	if !strings.Contains(text, `http_request_duration_by_route_seconds_count{app="app",source="http",method="GET",route="/api/v1/hello"} 1`) {
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
	selectRenderedDemoSQLite(t, projectDir)

	binPath := buildRenderedDefaultApp(t, projectDir, nil, "build rendered demo app")

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

func TestRenderedDemoAppMonitoringMetrics(t *testing.T) {
	projectDir := t.TempDir()
	testkit.RenderProjectWithForj(t, projectDir, testkit.RenderProjectRequest{
		Config: project.Config{
			ProjectName:  "Monitoring Metrics App",
			GoModuleName: "example.com/monitoringmetricsapp",
			UpdatedAt:    "2026-05-12 00:00:00 UTC",
			Render: project.RenderConfig{
				Components: project.Components{
					CLI:            true,
					WebAPI:         true,
					Metrics:        true,
					DemoApp:        true,
					DatabaseSQLite: true,
				},
			},
		},
	})
	selectRenderedDemoSQLite(t, projectDir)

	binPath := buildRenderedDefaultApp(t, projectDir, nil, "build rendered monitoring metrics app")

	runCommandSuccess(t, projectDir, binPath, nil, "migrate")

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
		t.Fatalf("start rendered monitoring metrics app: %v", err)
	}
	defer stopProcAsync(t, "monitoring-metrics-server", handle, time.Second)

	baseURL := "http://127.0.0.1:" + httpPort
	if !waitForTCP(t, "127.0.0.1:"+httpPort, 3*time.Second) {
		t.Fatalf("server did not accept TCP connections before timeout\n%s", handle.Output())
	}

	client := newRenderedMonitoringHTTPClient(t)
	loginRenderedMonitoringClient(t, client, baseURL)

	sidebarResp, err := client.Get(baseURL + "/api/v1/monitoring/monitors/sidebar?limit=25")
	if err != nil {
		t.Fatalf("get monitoring sidebar endpoint: %v\n%s", err, handle.Output())
	}
	_ = sidebarResp.Body.Close()
	if sidebarResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/monitoring/monitors/sidebar status = %d, want %d\n%s", sidebarResp.StatusCode, http.StatusOK, handle.Output())
	}

	heartbeatsResp, err := client.Get(baseURL + "/api/v1/monitoring/heartbeats?limit=12&ids=1,2,3")
	if err != nil {
		t.Fatalf("get monitoring heartbeats endpoint: %v\n%s", err, handle.Output())
	}
	_ = heartbeatsResp.Body.Close()
	if heartbeatsResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/monitoring/heartbeats status = %d, want %d\n%s", heartbeatsResp.StatusCode, http.StatusOK, handle.Output())
	}

	body := fetchMetricsText(t, baseURL+"/metrics")
	for _, token := range []string{
		`monitoring_sidebar_requests_total{app="app",source="app",filtered="false",has_more="false"} 1`,
		`monitoring_sidebar_rows_returned_count{app="app",source="app",filtered="false"} 1`,
		`monitoring_sidebar_next_offset_count{app="app",source="app",filtered="false"} 1`,
		`monitoring_heartbeats_requests_total{app="app",source="app",scope="scoped"} 1`,
		`monitoring_heartbeats_requested_ids_count{app="app",source="app",scope="scoped"} 1`,
		`monitoring_heartbeats_rows_returned_count{app="app",source="app",scope="scoped"} 1`,
		`monitoring_heartbeats_point_sets_returned_count{app="app",source="app",scope="scoped"} 1`,
	} {
		if !strings.Contains(body, token) {
			t.Fatalf("GET /metrics missing %q\nbody:\n%s\n%s", token, body, handle.Output())
		}
	}
}

func newRenderedMonitoringHTTPClient(t *testing.T) *http.Client {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("new cookie jar: %v", err)
	}
	return &http.Client{Jar: jar}
}

func loginRenderedMonitoringClient(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	if client == nil {
		t.Fatal("login client is nil")
	}

	body, err := json.Marshal(map[string]string{
		"login":    "admin",
		"password": "admin",
	})
	if err != nil {
		t.Fatalf("marshal login body: %v", err)
	}
	resp, err := client.Post(baseURL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		loginBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("login status = %d, want %d\nbody:\n%s", resp.StatusCode, http.StatusOK, string(loginBody))
	}
}

func TestRenderedJobsSourceMetrics(t *testing.T) {
	redisHost, redisPort := testkit.EnsureIntegrationRedis(t)
	queueEnv := map[string]string{
		"QUEUE_DRIVER": "redis",
		"QUEUE_NAME":   "default",
		"QUEUE_ADDR":   net.JoinHostPort(redisHost, redisPort),
		"REDIS_HOST":   redisHost,
		"REDIS_PORT":   redisPort,
	}

	projectDir := t.TempDir()
	testkit.RenderProjectWithForj(t, projectDir, testkit.RenderProjectRequest{
		Config: project.Config{
			ProjectName:  "Jobs Metrics App",
			GoModuleName: "example.com/jobsmetricsapp",
			UpdatedAt:    "2026-05-06 00:00:00 UTC",
			Render: project.RenderConfig{
				Components: project.Components{
					CLI:            true,
					WebAPI:         true,
					Metrics:        true,
					Jobs:           true,
					DemoApp:        true,
					DatabaseSQLite: true,
				},
			},
		},
		EnvOverrides: queueEnv,
	})
	selectRenderedDemoSQLite(t, projectDir)
	for key, value := range queueEnv {
		setRenderedEnvValue(t, projectDir, key, value)
	}

	binPath := buildRenderedDefaultApp(t, projectDir, nil, "build rendered jobs app")

	runCommandSuccess(t, projectDir, binPath, queueEnv, "migrate")
	runCommandSuccess(t, projectDir, binPath, queueEnv, "monitor:seed", "--count=1")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	metricsAddr := findFreeAddr(t)
	_, metricsPort, err := net.SplitHostPort(metricsAddr)
	if err != nil {
		t.Fatalf("split jobs metrics addr: %v", err)
	}

	workerCmd := exec.CommandContext(ctx, binPath, "queue:work", "--metrics-port", metricsPort)
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

	if !waitForOutputContains(worker, []string{"Queue worker started", "driver=redis"}, 5*time.Second) {
		t.Fatalf("jobs worker did not report ready state before timeout\n%s", worker.Output())
	}
	if !waitForTCP(t, "127.0.0.1:"+metricsPort, 5*time.Second) {
		t.Fatalf("jobs metrics endpoint did not accept TCP connections before timeout\n%s", worker.Output())
	}

	enqueueOut := runCommandSuccess(t, projectDir, binPath, queueEnv, "monitor:poll")

	metricsURL := "http://127.0.0.1:" + metricsPort + "/metrics"
	jobsMetric := regexp.MustCompile(`queue_jobs_by_job_total\{[^\n]*source="jobs"[^\n]*job_name="monitoring:check"[^\n]*status="succeeded"\}\s+[1-9]`)
	if !waitForMetricsMatch(t, metricsURL, jobsMetric, 20*time.Second) {
		body := fetchMetricsText(t, metricsURL)
		t.Fatalf("jobs metrics missing jobs-scoped monitoring:check success counter\nenqueue:\n%s\nbody:\n%s\n%s", string(enqueueOut), body, worker.Output())
	}
}

func TestRenderedSchedulerSourceMetrics(t *testing.T) {
	redisHost, redisPort := testkit.EnsureIntegrationRedis(t)
	validQueueEnv := map[string]string{
		"QUEUE_DRIVER": "redis",
		"QUEUE_NAME":   "default",
		"QUEUE_ADDR":   net.JoinHostPort(redisHost, redisPort),
		"REDIS_HOST":   redisHost,
		"REDIS_PORT":   redisPort,
	}
	invalidQueueEnv := map[string]string{
		"QUEUE_DRIVER": "redis",
		"QUEUE_NAME":   "default",
		"QUEUE_ADDR":   "127.0.0.1:1",
		"REDIS_HOST":   "127.0.0.1",
		"REDIS_PORT":   "1",
	}

	projectDir := t.TempDir()
	testkit.RenderProjectWithForj(t, projectDir, testkit.RenderProjectRequest{
		Config: project.Config{
			ProjectName:  "Scheduler Metrics App",
			GoModuleName: "example.com/schedulermetricsapp",
			UpdatedAt:    "2026-05-06 00:00:00 UTC",
			Render: project.RenderConfig{
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
	selectRenderedDemoSQLite(t, projectDir)

	binPath := buildRenderedDefaultApp(t, projectDir, nil, "build rendered scheduler app")

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
	schedulerCmd.Env = testkit.IntegrationProcessEnv(t, mergeEnv(invalidQueueEnv, map[string]string{
		"MONITOR_POLL_INTERVAL_SECONDS": "1",
	}))
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
