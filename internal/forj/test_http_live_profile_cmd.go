package forj

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

// TestHTTPLiveProfileCmd profiles the real rendered HTTP server process under live benchmark load.
type TestHTTPLiveProfileCmd struct {
	logger *logger.AppLogger

	DurationMS  int    `help:"Measured benchmark duration in milliseconds" default:"15000"`
	Concurrency int    `help:"HTTP benchmark concurrency" default:"8"`
	Path        string `help:"Benchmark HTTP path" default:"/-/health"`
	ProfileSecs int    `help:"CPU profile seconds; defaults to the full warmup+measure window" default:"0"`
	Top         int    `help:"Top pprof entries to print per profile" default:"20"`
	HTTPCORS    bool   `help:"Enable HTTP CORS middleware in the rendered server" default:"true"`
	HTTPAccessLogs bool `help:"Enable HTTP access logs in the rendered server" default:"true"`
	InspectEnabled bool `help:"Enable inspect capture in the rendered server" default:"true"`
	MetricsEnabled bool `help:"Enable HTTP metrics middleware in the rendered server" default:"true"`
	HealthMode  string `help:"Health response mode: json, text, or nocontent" default:"json" enum:"json,text,nocontent"`
	Keep        bool   `help:"Keep the rendered temp project after completion" short:"k"`
	Silent      bool   `help:"Suppress command progress output" short:"s"`
}

type httpLiveProfileResult struct {
	URL         string
	DurationMS  int64
	Concurrency int
	Ops         int64
	OpsPerSec   float64
	Errors      int64
	P50MS       float64
	P95MS       float64
	P99MS       float64
	CPUProfile  string
	HeapProfile string
	CPUTop      string
	HeapTop     string
}

func (*TestHTTPLiveProfileCmd) Signature() string {
	return `name:"test:http-live-profile" help:"Profile the real rendered HTTP server under live benchmark load" hidden:""`
}

func NewTestHTTPLiveProfileCmd(logger *logger.AppLogger) *TestHTTPLiveProfileCmd {
	return &TestHTTPLiveProfileCmd{logger: logger}
}

func (cmd *TestHTTPLiveProfileCmd) Run() error {
	if cmd.DurationMS <= 0 {
		return fmt.Errorf("duration-ms must be greater than zero")
	}
	if cmd.Concurrency <= 0 {
		return fmt.Errorf("concurrency must be greater than zero")
	}
	if cmd.Top <= 0 {
		return fmt.Errorf("top must be greater than zero")
	}

	duration := time.Duration(cmd.DurationMS) * time.Millisecond
	concurrency := liveBenchmarkConcurrency(cmd.Concurrency, 8, 512)

	modCache, buildCache := testkit.GoCachePaths()
	dir, err := os.MkdirTemp("", "forj_http_live_profile_")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	if !cmd.Keep {
		defer os.RemoveAll(dir)
	}

	if !cmd.Silent {
		testkit.PrintSection("HTTP Live Profile")
		console.Actionf("Rendering fixed live HTTP profile app")
	}

	cfg := project.Config{
		ProjectName:  "HTTP Live Profile",
		GoModuleName: "example.com/httpliveprofileapp",
		UpdatedAt:    "2026-05-21 00:00:00 UTC",
		Render: project.RenderConfig{
			Components: project.Components{
				CLI:     true,
				WebAPI:  true,
				Metrics: true,
			},
		},
	}
	if err := WriteYAML(filepath.Join(dir, ".goforj.yml"), cfg); err != nil {
		return err
	}

	forjExec, cleanup, err := repoForjExecutable(modCache, buildCache)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := runStep(cmd.logger, cmd.Silent, "render", dir, modCache, buildCache, []string{forjExec, "render"}); err != nil {
		return err
	}
	if err := cmd.applyLocalWebReplace(dir); err != nil {
		return err
	}
	if err := cmd.customizeRenderedHTTPApp(dir); err != nil {
		return err
	}
	if err := cmd.writePprofSupport(dir); err != nil {
		return err
	}

	binPath := filepath.Join(dir, "app")
	if err := runStep(cmd.logger, cmd.Silent, "build", dir, modCache, buildCache, []string{"go", "build", "-o", binPath, "."}); err != nil {
		return err
	}

	httpAddr, err := findFreeAddr()
	if err != nil {
		return err
	}
	_, httpPort, err := net.SplitHostPort(httpAddr)
	if err != nil {
		return fmt.Errorf("split http addr: %w", err)
	}
	pprofAddr, err := findFreeAddr()
	if err != nil {
		return err
	}
	metricsAddr, err := findFreeAddr()
	if err != nil {
		return err
	}
	_, metricsPort, err := net.SplitHostPort(metricsAddr)
	if err != nil {
		return fmt.Errorf("split metrics addr: %w", err)
	}

	baseURL := "http://127.0.0.1:" + httpPort
	targetURL := normalizeLiveHTTPBenchmarkURL(baseURL, cmd.Path)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	execCmd := exec.CommandContext(ctx, binPath, "http:serve", "--port", httpPort, "--metrics-port", metricsPort)
	execCmd.Dir = dir
	execCmd.Env = testkit.ProcessEnv("", map[string]string{
		"APP_ENV":                 "local",
		"APP_URL":                 baseURL,
		"HTTP_ACCESS_LOG_ENABLED": strconv.FormatBool(cmd.HTTPAccessLogs),
		"LIGHTHOUSE_INSPECT_ENABLED": strconv.FormatBool(cmd.InspectEnabled),
		"METRICS_HTTP_ENABLED":    strconv.FormatBool(cmd.MetricsEnabled),
		"METRICS_API_PORT":        metricsPort,
		"FORJ_PPROF_ADDR":         pprofAddr,
	})
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr
	if err := execCmd.Start(); err != nil {
		return fmt.Errorf("start rendered app: %w", err)
	}
	defer func() {
		cancel()
		_ = execCmd.Wait()
	}()

	if err := waitForTCP("127.0.0.1:"+httpPort, 10*time.Second); err != nil {
		return fmt.Errorf("wait for http port: %w\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if err := waitForHTTPStatus(targetURL, cmd.expectedHealthStatus(), 10*time.Second); err != nil {
		return fmt.Errorf("wait for health endpoint: %w\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if err := waitForTCP(pprofAddr, 10*time.Second); err != nil {
		return fmt.Errorf("wait for pprof port: %w\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	profileDir := filepath.Join(dir, "_profiles")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		return fmt.Errorf("create profile dir: %w", err)
	}
	cpuPath := filepath.Join(profileDir, "live.cpu.pprof")
	heapPath := filepath.Join(profileDir, "live.heap.pprof")

	profileDuration := cmd.liveProfileDuration(duration)
	cpuErrCh := make(chan error, 1)
	go func() {
		cpuErrCh <- fetchPprof("http://"+pprofAddr+"/debug/pprof/profile?seconds="+strconv.Itoa(int(math.Ceil(profileDuration.Seconds()))), cpuPath)
	}()

	started := time.Now()
	result := runLiveHTTPBenchmark(ctx, targetURL, duration, concurrency, started)

	if err := <-cpuErrCh; err != nil {
		return fmt.Errorf("capture cpu profile: %w", err)
	}
	if err := fetchPprof("http://"+pprofAddr+"/debug/pprof/heap", heapPath); err != nil {
		return fmt.Errorf("capture heap profile: %w", err)
	}

	cpuTop, err := runPprofTop(dir, binPath, cpuPath, cmd.Top)
	if err != nil {
		return fmt.Errorf("pprof cpu top: %w", err)
	}
	heapTop, err := runPprofAllocSpaceTop(dir, binPath, heapPath, cmd.Top)
	if err != nil {
		return fmt.Errorf("pprof heap top: %w", err)
	}

	if !cmd.Silent {
		cmd.printResult(httpLiveProfileResult{
			URL:         targetURL,
			DurationMS:  result.DurationMS,
			Concurrency: result.Concurrency,
			Ops:         result.Ops,
			OpsPerSec:   result.OpsPerSec,
			Errors:      result.Errors,
			P50MS:       result.P50MS,
			P95MS:       result.P95MS,
			P99MS:       result.P99MS,
			CPUProfile:  cpuPath,
			HeapProfile: heapPath,
			CPUTop:      cpuTop,
			HeapTop:     heapTop,
		})
		if cmd.Keep {
			cmd.logger.Info().Str("path", dir).Msg("Kept rendered HTTP live profile project")
		}
	}

	return nil
}

func (cmd *TestHTTPLiveProfileCmd) liveProfileDuration(duration time.Duration) time.Duration {
	if cmd.ProfileSecs > 0 {
		return time.Duration(cmd.ProfileSecs) * time.Second
	}
	return duration + liveBenchmarkWarmup(duration)
}

func (cmd *TestHTTPLiveProfileCmd) expectedHealthStatus() int {
	if strings.EqualFold(strings.TrimSpace(cmd.HealthMode), "nocontent") {
		return http.StatusNoContent
	}
	return http.StatusOK
}

func (cmd *TestHTTPLiveProfileCmd) writePprofSupport(dir string) error {
	const source = `package http

import (
	nethttp "net/http"
	_ "net/http/pprof"
	"os"
	"strings"
)

func init() {
	addr := strings.TrimSpace(os.Getenv("FORJ_PPROF_ADDR"))
	if addr == "" {
		return
	}
	go func() {
		_ = nethttp.ListenAndServe(addr, nil)
	}()
}
`
	path := filepath.Join(dir, "internal", "http", "forj_pprof_support.go")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		return fmt.Errorf("write pprof support file: %w", err)
	}
	return nil
}

func (cmd *TestHTTPLiveProfileCmd) customizeRenderedHTTPApp(dir string) error {
	if err := cmd.customizeRenderedHealth(dir); err != nil {
		return err
	}
	if err := cmd.customizeRenderedCORS(dir); err != nil {
		return err
	}
	if err := cmd.customizeRenderedEnvFlags(dir); err != nil {
		return err
	}
	return nil
}

func (cmd *TestHTTPLiveProfileCmd) applyLocalWebReplace(dir string) error {
	const localWebPath = "/workspace/code/web"
	info, err := os.Stat(localWebPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat local web repo: %w", err)
	}
	if !info.IsDir() {
		return nil
	}
	return runStep(cmd.logger, cmd.Silent, "go mod replace web", dir, "", "", []string{
		"go", "mod", "edit", "-replace", "github.com/goforj/web=" + localWebPath,
	})
}

func (cmd *TestHTTPLiveProfileCmd) customizeRenderedHealth(dir string) error {
	mode := strings.ToLower(strings.TrimSpace(cmd.HealthMode))
	if mode == "json" {
		return nil
	}
	path := filepath.Join(dir, "internal", "http", "health.go")
	input, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read rendered health file: %w", err)
	}
	original := "func (s *Server) healthStatus(r web.Context) error {\n\treturn r.JSON(http.StatusOK, map[string]string{\"status\": \"ok\"})\n}"
	replacement := "func (s *Server) healthStatus(r web.Context) error {\n\treturn r.Text(http.StatusOK, \"ok\")\n}"
	if mode == "nocontent" {
		replacement = "func (s *Server) healthStatus(r web.Context) error {\n\treturn r.NoContent(http.StatusNoContent)\n}"
	}
	updated := strings.Replace(string(input), original, replacement, 1)
	if updated == string(input) {
		return fmt.Errorf("customize rendered health file: expected healthStatus block not found")
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("write rendered health file: %w", err)
	}
	return nil
}

func (cmd *TestHTTPLiveProfileCmd) customizeRenderedCORS(dir string) error {
	if cmd.HTTPCORS {
		return nil
	}
	path := filepath.Join(dir, "internal", "http", "cors.go")
	const source = `package http

import "github.com/goforj/web"

func (s *Server) mountCors(router web.Router) error {
	return nil
}
`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		return fmt.Errorf("write rendered cors file: %w", err)
	}
	return nil
}

func (cmd *TestHTTPLiveProfileCmd) customizeRenderedEnvFlags(dir string) error {
	path := filepath.Join(dir, ".env.local")
	input, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read rendered env.local: %w", err)
	}
	lines := strings.Split(string(input), "\n")
	updatedInspect := false
	updatedMetrics := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "LIGHTHOUSE_INSPECT_ENABLED=") {
			lines[i] = "LIGHTHOUSE_INSPECT_ENABLED=" + strconv.FormatBool(cmd.InspectEnabled)
			updatedInspect = true
			continue
		}
		if strings.HasPrefix(trimmed, "METRICS_HTTP_ENABLED=") {
			lines[i] = "METRICS_HTTP_ENABLED=" + strconv.FormatBool(cmd.MetricsEnabled)
			updatedMetrics = true
		}
	}
	if !updatedInspect {
		lines = append(lines, "LIGHTHOUSE_INSPECT_ENABLED="+strconv.FormatBool(cmd.InspectEnabled))
	}
	if !updatedMetrics {
		lines = append(lines, "METRICS_HTTP_ENABLED="+strconv.FormatBool(cmd.MetricsEnabled))
	}
	updated := strings.Join(lines, "\n")
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("write rendered env.local: %w", err)
	}
	return nil
}

func (cmd *TestHTTPLiveProfileCmd) printResult(result httpLiveProfileResult) {
	rows := [][]string{
		{"Metric", "Value"},
		{"Target URL", result.URL},
		{"Concurrency", fmt.Sprintf("%d", result.Concurrency)},
		{"Duration", fmt.Sprintf("%dms", result.DurationMS)},
		{"Ops", fmt.Sprintf("%d", result.Ops)},
		{"Ops/sec", fmt.Sprintf("%.2f", result.OpsPerSec)},
		{"Errors", fmt.Sprintf("%d", result.Errors)},
		{"P50/P95/P99", fmt.Sprintf("%.2f / %.2f / %.2f ms", result.P50MS, result.P95MS, result.P99MS)},
	}
	fmt.Fprintln(os.Stdout, "Live HTTP Benchmark")
	printASCIITable(os.Stdout, rows)
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "[LIVE] CPU Top")
	fmt.Fprintln(os.Stdout, result.CPUTop)
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "[LIVE] Alloc Space Top")
	fmt.Fprintln(os.Stdout, result.HeapTop)
	fmt.Fprintf(os.Stdout, "\nprofiles: cpu=%s heap=%s\n", result.CPUProfile, result.HeapProfile)
}

func fetchPprof(url, path string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("GET %s status %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, resp.Body)
	return err
}

func findFreeAddr() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("find free addr: %w", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr, nil
}

func waitForTCP(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for tcp %s", addr)
}

func waitForHTTPStatus(url string, want int, timeout time.Duration) error {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
			_ = resp.Body.Close()
			if resp.StatusCode == want {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s status %d", url, want)
}

func normalizeLiveHTTPBenchmarkURL(baseURL string, path string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = "http://127.0.0.1:3000"
	}
	path = strings.TrimSpace(path)
	if path == "" {
		path = "/-/health"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return strings.TrimRight(baseURL, "/") + path
}

func liveBenchmarkConcurrency(value, fallback, max int) int {
	if value <= 0 {
		value = fallback
	}
	if value > max {
		value = max
	}
	return value
}

func liveBenchmarkWarmup(duration time.Duration) time.Duration {
	if duration <= 0 {
		return 0
	}
	w := duration / 5
	if w > time.Second {
		w = time.Second
	}
	if w < 200*time.Millisecond {
		w = 200 * time.Millisecond
	}
	return w
}

func runLiveHTTPBenchmark(ctx context.Context, target string, duration time.Duration, concurrency int, started time.Time) httpRuntimeBenchmarkSummary {
	warmup := liveBenchmarkWarmup(duration)
	measureStart := started.Add(warmup)
	deadline := measureStart.Add(duration)
	client := liveBenchmarkHTTPClient()
	var ops int64
	var errs int64
	latencies := make([]float64, 0, 2048)
	var latMu sync.Mutex
	var wg sync.WaitGroup
	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			transportErrStreak := 0
			for {
				if ctx.Err() != nil {
					return
				}
				now := time.Now()
				if now.After(deadline) {
					return
				}
				measured := !now.Before(measureStart)
				opStarted := time.Now()
				remaining := time.Until(deadline)
				if remaining <= 50*time.Millisecond {
					return
				}
				requestTimeout := 5 * time.Second
				if remaining > 0 && remaining < requestTimeout {
					requestTimeout = remaining
				}
				reqCtx, reqCancel := context.WithTimeout(context.Background(), requestTimeout)
				req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, target, nil)
				if err != nil {
					reqCancel()
					if measured {
						atomic.AddInt64(&errs, 1)
					}
					continue
				}
				resp, err := client.Do(req)
				if err != nil {
					reqCancel()
					if measured {
						if time.Now().After(deadline) {
							return
						}
						atomic.AddInt64(&errs, 1)
					}
					transportErrStreak++
					backoff := liveTransportErrorBackoff(transportErrStreak)
					if backoff > 0 {
						time.Sleep(backoff)
					}
					continue
				}
				transportErrStreak = 0
				_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
				_ = resp.Body.Close()
				reqCancel()
				if resp.StatusCode >= 400 {
					if measured {
						atomic.AddInt64(&errs, 1)
					}
					continue
				}
				if measured {
					atomic.AddInt64(&ops, 1)
					ms := float64(time.Since(opStarted)) / float64(time.Millisecond)
					latMu.Lock()
					latencies = append(latencies, ms)
					latMu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	completed := time.Now()
	elapsed := completed.Sub(measureStart)
	if elapsed <= 0 {
		elapsed = time.Millisecond
	}
	p50, p95, p99 := liveLatencyPercentiles(latencies)
	return httpRuntimeBenchmarkSummary{
		DurationMS:  int64(duration / time.Millisecond),
		Concurrency: concurrency,
		Ops:         ops,
		OpsPerSec:   float64(ops) / elapsed.Seconds(),
		Errors:      errs,
		P50MS:       p50,
		P95MS:       p95,
		P99MS:       p99,
	}
}

type httpRuntimeBenchmarkSummary struct {
	DurationMS  int64
	Concurrency int
	Ops         int64
	OpsPerSec   float64
	Errors      int64
	P50MS       float64
	P95MS       float64
	P99MS       float64
}

var (
	liveHTTPClientOnce sync.Once
	liveHTTPClientInst *http.Client
)

func liveBenchmarkHTTPClient() *http.Client {
	liveHTTPClientOnce.Do(func() {
		transport := &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          512,
			MaxIdleConnsPerHost:   256,
			MaxConnsPerHost:       256,
			IdleConnTimeout:       2 * time.Minute,
			TLSHandshakeTimeout:   5 * time.Second,
			ExpectContinueTimeout: time.Second,
		}
		liveHTTPClientInst = &http.Client{Transport: transport}
	})
	return liveHTTPClientInst
}

func liveTransportErrorBackoff(streak int) time.Duration {
	if streak <= 1 {
		return 0
	}
	if streak > 6 {
		streak = 6
	}
	return time.Duration(1<<streak) * time.Millisecond
}

func liveLatencyPercentiles(raw []float64) (float64, float64, float64) {
	if len(raw) == 0 {
		return 0, 0, 0
	}
	sorted := append([]float64(nil), raw...)
	sort.Float64s(sorted)
	pick := func(q float64) float64 {
		if q <= 0 {
			return sorted[0]
		}
		if q >= 1 {
			return sorted[len(sorted)-1]
		}
		idx := int(float64(len(sorted)-1) * q)
		if idx < 0 {
			idx = 0
		}
		if idx >= len(sorted) {
			idx = len(sorted) - 1
		}
		return sorted[idx]
	}
	return pick(0.50), pick(0.95), pick(0.99)
}
