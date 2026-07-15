//go:build integration

package forj

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

func TestRenderedMultiAppRuntimesUseDeterministicPorts(t *testing.T) {
	requirePortsAvailable(t, []string{
		"127.0.0.1:3000",
		"127.0.0.1:3001",
		"127.0.0.1:3002",
		"127.0.0.1:10000",
		"127.0.0.1:10001",
		"127.0.0.1:10002",
		"127.0.0.1:10010",
		"127.0.0.1:10011",
		"127.0.0.1:10012",
		"127.0.0.1:10020",
		"127.0.0.1:10021",
		"127.0.0.1:10022",
	})

	projectDir := t.TempDir()
	renderMultiAppRuntimeTestApp(t, projectDir)
	buildMultiAppRuntimeBinaries(t, projectDir, []multiAppRuntimeSpec{
		{name: "app", packagePath: "./cmd/app"},
		{name: "billing", packagePath: "./cmd/billing"},
		{name: "customer-portal", packagePath: "./cmd/customer-portal"},
	})

	apps := []multiAppRuntimeSpec{
		{name: "app", httpPort: "3000", metricsPort: "10000", schedulerMetricsPort: "10001", workerMetricsPort: "10002"},
		{name: "billing", httpPort: "3001", metricsPort: "10010", schedulerMetricsPort: "10011", workerMetricsPort: "10012"},
		{name: "customer-portal", httpPort: "3002", metricsPort: "10020", schedulerMetricsPort: "10021", workerMetricsPort: "10022"},
	}
	procs := make([]*procHandle, 0, len(apps)*3)
	for _, app := range apps {
		procs = append(procs, startAppHTTPRuntime(t, projectDir, app))
		procs = append(procs, startAppSchedulerRuntime(t, projectDir, app))
		procs = append(procs, startAppWorkerRuntime(t, projectDir, app))
	}
	defer func() {
		for _, proc := range procs {
			stopProcAsync(t, proc.name, proc, time.Second)
		}
	}()

	checks := make([]runtimeReadinessCheck, 0, len(apps)*3)
	for _, app := range apps {
		httpProc := findRuntimeProc(t, procs, app.name+" http")
		schedulerProc := findRuntimeProc(t, procs, app.name+" scheduler")
		workerProc := findRuntimeProc(t, procs, app.name+" worker")
		checks = append(checks,
			runtimeReadinessCheck{
				name: app.name + " http",
				run: func() error {
					return assertAppHTTPRuntimeReady(app, httpProc)
				},
			},
			runtimeReadinessCheck{
				name: app.name + " scheduler",
				run: func() error {
					return assertAppMetricsRuntimeReady(app.name+" scheduler", app.schedulerMetricsPort, schedulerProc)
				},
			},
			runtimeReadinessCheck{
				name: app.name + " worker",
				run: func() error {
					return assertAppMetricsRuntimeReady(app.name+" worker", app.workerMetricsPort, workerProc)
				},
			},
		)
	}
	assertRuntimeReadiness(t, checks)
}

type multiAppRuntimeSpec struct {
	name                 string
	packagePath          string
	httpPort             string
	metricsPort          string
	schedulerMetricsPort string
	workerMetricsPort    string
}

type runtimeReadinessCheck struct {
	name string
	run  func() error
}

// renderMultiAppRuntimeTestApp keeps the fixture intentionally small so failures point at app runtime behavior.
func renderMultiAppRuntimeTestApp(t *testing.T, dir string) {
	t.Helper()
	cfg := project.Config{
		ProjectName:  "MultiAppRuntime",
		GoModuleName: "example.com/multiappruntime",
		UpdatedAt:    "2026-06-05 00:00:00 UTC",
		Render: project.RenderConfig{
			Components: project.Components{
				CLI:       true,
				WebAPI:    true,
				Metrics:   true,
				Scheduler: true,
				Jobs:      true,
			},
		},
	}
	if err := testkit.WriteProjectConfig(filepath.Join(dir, ".goforj.yml"), cfg); err != nil {
		t.Fatalf("write .goforj.yml: %v", err)
	}
	for _, app := range []string{"billing", "customer-portal"} {
		if err := writeConventionalAppMarker(dir, app); err != nil {
			t.Fatalf("write %s app marker: %v", app, err)
		}
	}
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir rendered project: %v", err)
	}
	defer func() { _ = os.Chdir(originalWD) }()

	renderer := NewProjectRenderer(logger.NewSilentLogger())
	if err := runQuietly(func() error {
		return renderer.Render(ComponentRenderInput{renderAll: true})
	}); err != nil {
		t.Fatalf("render multi-app runtime app: %v", err)
	}
}

// buildMultiAppRuntimeBinaries builds entrypoint packages directly so the test focuses on compiled runtime defaults.
func buildMultiAppRuntimeBinaries(t *testing.T, projectDir string, apps []multiAppRuntimeSpec) {
	t.Helper()
	binDir := filepath.Join(projectDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("create bin dir: %v", err)
	}
	args := []string{"build", "-o", binDir}
	for _, app := range apps {
		args = append(args, app.packagePath)
	}
	buildCmd := exec.Command("go", args...)
	buildCmd.Dir = projectDir
	buildCmd.Env = testkit.IntegrationGoProcessEnv(t, nil)
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build runtime binaries: %v\n%s", err, out)
	}
}

// startAppHTTPRuntime starts the generated binary without flags so ports must come from compiled app defaults.
func startAppHTTPRuntime(t *testing.T, projectDir string, app multiAppRuntimeSpec) *procHandle {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	binPath := filepath.Join(projectDir, "bin", app.name)
	cmd := exec.CommandContext(ctx, binPath, "http:serve")
	cmd.Dir = projectDir
	cmd.Env = testkit.IntegrationProcessEnv(t, nil)
	handle := &procHandle{
		name:   app.name + " http",
		cmd:    cmd,
		cancel: cancel,
	}
	cmd.Stdout = &handle.stdout
	cmd.Stderr = &handle.stderr
	if err := handle.Start(); err != nil {
		cancel()
		t.Fatalf("start %s HTTP runtime: %v", app.name, err)
	}
	return handle
}

// startAppSchedulerRuntime starts schedule:run so the scheduler metrics listener must use app defaults.
func startAppSchedulerRuntime(t *testing.T, projectDir string, app multiAppRuntimeSpec) *procHandle {
	t.Helper()
	return startAppRuntimeCommand(t, projectDir, app, "scheduler", "schedule:run")
}

// startAppWorkerRuntime starts queue:work so the worker metrics listener must use app defaults.
func startAppWorkerRuntime(t *testing.T, projectDir string, app multiAppRuntimeSpec) *procHandle {
	t.Helper()
	return startAppRuntimeCommand(t, projectDir, app, "worker", "queue:work")
}

// startAppRuntimeCommand starts a long-running generated runtime command for one app binary.
func startAppRuntimeCommand(t *testing.T, projectDir string, app multiAppRuntimeSpec, runtimeName string, command string) *procHandle {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	binPath := filepath.Join(projectDir, "bin", app.name)
	cmd := exec.CommandContext(ctx, binPath, command)
	cmd.Dir = projectDir
	cmd.Env = testkit.IntegrationProcessEnv(t, nil)
	handle := &procHandle{
		name:   app.name + " " + runtimeName,
		cmd:    cmd,
		cancel: cancel,
	}
	cmd.Stdout = &handle.stdout
	cmd.Stderr = &handle.stderr
	if err := handle.Start(); err != nil {
		cancel()
		t.Fatalf("start %s %s runtime: %v", app.name, runtimeName, err)
	}
	return handle
}

// assertAppHTTPRuntimeReady checks both user traffic and the dedicated metrics listener for one app.
func assertAppHTTPRuntimeReady(app multiAppRuntimeSpec, proc *procHandle) error {
	baseURL := "http://127.0.0.1:" + app.httpPort
	metricsURL := "http://127.0.0.1:" + app.metricsPort + "/metrics"

	if err := waitForHTTPStatus(proc, baseURL+"/-/health", http.StatusOK, 10*time.Second); err != nil {
		return err
	}
	if err := waitForHTTPStatus(proc, baseURL+"/api/v1/hello", http.StatusOK, 10*time.Second); err != nil {
		return err
	}
	return waitForHTTPMetricLabels(proc, metricsURL, app.name, 10*time.Second)
}

// assertAppMetricsRuntimeReady checks a non-HTTP runtime's dedicated metrics listener.
func assertAppMetricsRuntimeReady(name string, port string, proc *procHandle) error {
	return waitForHTTPStatus(proc, "http://127.0.0.1:"+port+"/metrics", http.StatusOK, 10*time.Second)
}

// assertRuntimeReadiness runs listener checks concurrently because every runtime has already been started.
func assertRuntimeReadiness(t *testing.T, checks []runtimeReadinessCheck) {
	t.Helper()
	errs := make(chan error, len(checks))
	for _, check := range checks {
		check := check
		go func() {
			if check.run == nil {
				errs <- fmt.Errorf("%s readiness check is missing", check.name)
				return
			}
			if err := check.run(); err != nil {
				errs <- fmt.Errorf("%s: %w", check.name, err)
				return
			}
			errs <- nil
		}()
	}
	for range checks {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
}

// findRuntimeProc keeps readiness assertions tied to stable process labels.
func findRuntimeProc(t *testing.T, procs []*procHandle, name string) *procHandle {
	t.Helper()
	for _, proc := range procs {
		if proc != nil && proc.name == name {
			return proc
		}
	}
	t.Fatalf("runtime process %q was not started", name)
	return nil
}

// waitForHTTPStatus fails early when the child process exits, which makes port conflicts easier to diagnose.
func waitForHTTPStatus(proc *procHandle, url string, wantStatus int, timeout time.Duration) error {
	client := &http.Client{Timeout: 300 * time.Millisecond}
	deadline := time.Now().Add(timeout)
	var lastStatus int
	var lastErr error
	for time.Now().Before(deadline) {
		if err := proc.ExitError(); err != nil {
			return fmt.Errorf("%s exited before %s became ready: %w", proc.name, url, err)
		}
		resp, err := client.Get(url)
		if err == nil {
			lastStatus = resp.StatusCode
			_ = resp.Body.Close()
			if resp.StatusCode == wantStatus {
				return nil
			}
		} else {
			lastErr = err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("%s did not return status %d before timeout; last status=%d last error=%v\n%s", url, wantStatus, lastStatus, lastErr, proc.Output())
}

// waitForHTTPMetricLabels proves the generated app identity reaches runtime metrics, not just port selection.
func waitForHTTPMetricLabels(proc *procHandle, url string, appName string, timeout time.Duration) error {
	client := &http.Client{Timeout: 300 * time.Millisecond}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := proc.ExitError(); err != nil {
			return fmt.Errorf("%s exited before %s exposed app HTTP metrics: %w", proc.name, url, err)
		}
		body, err := getHTTPBody(client, url)
		if err != nil {
			lastErr = err
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if hasHTTPMetricLabels(body, appName) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("%s did not expose HTTP metric labels app=%q source=http before timeout; last error=%v\n%s", url, appName, lastErr, proc.Output())
}

// getHTTPBody keeps metric assertions small while preserving the short request timeout used by readiness checks.
func getHTTPBody(client *http.Client, url string) (string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// hasHTTPMetricLabels scans complete Prometheus samples so label order changes do not make the smoke test brittle.
func hasHTTPMetricLabels(body string, appName string) bool {
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "http_requests_total{") {
			continue
		}
		if strings.Contains(line, `app="`+appName+`"`) &&
			strings.Contains(line, `source="http"`) &&
			!strings.Contains(line, `source="app"`) {
			return true
		}
	}
	return false
}

// requirePortsAvailable avoids making opt-in integration runs fail just because a developer already has local services open.
func requirePortsAvailable(t *testing.T, addrs []string) {
	t.Helper()
	listeners := make([]net.Listener, 0, len(addrs))
	for _, addr := range addrs {
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			for _, existing := range listeners {
				_ = existing.Close()
			}
			t.Skipf("default runtime port %s is unavailable: %v", addr, err)
		}
		listeners = append(listeners, listener)
	}
	for _, listener := range listeners {
		if err := listener.Close(); err != nil {
			t.Fatalf("release default runtime port reservation: %v", err)
		}
	}
	for _, addr := range addrs {
		if !waitForPortRelease(addr, time.Second) {
			t.Fatalf("default runtime port %s did not release after preflight check", addr)
		}
	}
}

// waitForPortRelease narrows the race between the preflight listener check and starting generated binaries.
func waitForPortRelease(addr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		listener, err := net.Listen("tcp", addr)
		if err == nil {
			_ = listener.Close()
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// runQuietly keeps renderer status output from obscuring integration failure logs.
func runQuietly(fn func() error) error {
	originalStdout := os.Stdout
	originalStderr := os.Stderr
	stdoutReader, stdoutWriter, stdoutErr := os.Pipe()
	if stdoutErr != nil {
		return stdoutErr
	}
	stderrReader, stderrWriter, stderrErr := os.Pipe()
	if stderrErr != nil {
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		return stderrErr
	}

	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(io.Discard, stdoutReader)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(io.Discard, stderrReader)
		done <- struct{}{}
	}()

	os.Stdout = stdoutWriter
	os.Stderr = stderrWriter
	err := fn()
	os.Stdout = originalStdout
	os.Stderr = originalStderr

	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	<-done
	<-done
	_ = stdoutReader.Close()
	_ = stderrReader.Close()
	return err
}
