package forj

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

// TestHTTPRuntimeProfileCmd renders a temp app and profiles HTTP runtime benchmark modes.
type TestHTTPRuntimeProfileCmd struct {
	logger *logger.AppLogger

	BenchTime string `help:"Go benchmark benchtime, for example 3s or 50000x" default:"3s"`
	Modes     string `help:"Comma-separated benchmark modes" default:"baseline,metrics_only,logging_only,inspect_only,full_stack"`
	Top       int    `help:"Top pprof entries to print per profile" default:"20"`
	Keep      bool   `help:"Keep the rendered temp project after completion" short:"k"`
	Silent    bool   `help:"Suppress command progress output" short:"s"`
}

type httpRuntimeProfileResult struct {
	Mode          string
	NSPerOp       float64
	BytesPerOp    float64
	AllocsPerOp   float64
	CPUProfile    string
	MemProfile    string
	CPUTop        string
	AllocSpaceTop string
}

func (*TestHTTPRuntimeProfileCmd) Signature() string {
	return `name:"test:http-runtime-profile" help:"Profile HTTP runtime benchmark modes" hidden:""`
}

func NewTestHTTPRuntimeProfileCmd(logger *logger.AppLogger) *TestHTTPRuntimeProfileCmd {
	return &TestHTTPRuntimeProfileCmd{logger: logger}
}

func (cmd *TestHTTPRuntimeProfileCmd) Run() error {
	modes, err := parseHTTPRuntimeProfileModes(cmd.Modes)
	if err != nil {
		return err
	}
	if cmd.Top <= 0 {
		return fmt.Errorf("top must be greater than zero")
	}

	modCache, buildCache := testkit.GoCachePaths()
	dir, err := os.MkdirTemp("", "forj_http_runtime_profile_")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	if !cmd.Keep {
		defer os.RemoveAll(dir)
	}

	if !cmd.Silent {
		testkit.PrintSection("HTTP Runtime Profile")
		console.Actionf("Rendering fixed HTTP runtime benchmark app")
	}

	cfg := project.Config{
		ProjectName:  "HTTP Runtime Profile",
		GoModuleName: "example.com/httpruntimeprofileapp",
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

	testBinary := filepath.Join(dir, "http_runtime_bench.test")
	if err := runStep(cmd.logger, cmd.Silent, "bench-compile", dir, modCache, buildCache, []string{"go", "test", "-c", "-o", testBinary, "./internal/http"}); err != nil {
		return err
	}

	profileDir := filepath.Join(dir, "_profiles")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		return fmt.Errorf("create profile dir: %w", err)
	}

	results := make([]httpRuntimeProfileResult, 0, len(modes))
	for _, mode := range modes {
		result, err := cmd.runMode(dir, testBinary, profileDir, mode)
		if err != nil {
			return err
		}
		results = append(results, result)
	}

	if !cmd.Silent {
		cmd.printResults(results)
		if cmd.Keep {
			cmd.logger.Info().Str("path", dir).Msg("Kept rendered HTTP runtime profile project")
		}
	}

	return nil
}

func parseHTTPRuntimeProfileModes(raw string) ([]string, error) {
	allowed := map[string]struct{}{
		"baseline":     {},
		"metrics_only": {},
		"logging_only": {},
		"inspect_only": {},
		"full_stack":   {},
	}
	items := strings.Split(strings.TrimSpace(raw), ",")
	if len(items) == 1 && strings.TrimSpace(items[0]) == "" {
		return nil, fmt.Errorf("at least one mode is required")
	}
	out := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		mode := strings.ToLower(strings.TrimSpace(item))
		if mode == "" {
			continue
		}
		if _, ok := allowed[mode]; !ok {
			return nil, fmt.Errorf("unsupported mode %q", mode)
		}
		if _, ok := seen[mode]; ok {
			continue
		}
		seen[mode] = struct{}{}
		out = append(out, mode)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one mode is required")
	}
	return out, nil
}

func (cmd *TestHTTPRuntimeProfileCmd) runMode(dir, testBinary, profileDir, mode string) (httpRuntimeProfileResult, error) {
	if !cmd.Silent {
		console.Actionf("Profiling HTTP runtime mode %s", mode)
	}

	cpuPath := filepath.Join(profileDir, mode+".cpu.pprof")
	memPath := filepath.Join(profileDir, mode+".mem.pprof")
	benchPattern := fmt.Sprintf("^BenchmarkHTTPRuntimeModes/health_route/%s$", mode)

	execCmd := exec.Command(
		testBinary,
		"-test.run", "^$",
		"-test.bench", benchPattern,
		"-test.benchmem",
		"-test.benchtime", cmd.BenchTime,
		"-test.count", "1",
		"-test.cpuprofile", cpuPath,
		"-test.memprofile", memPath,
	)
	execCmd.Dir = dir
	execCmd.Env = testkit.ProcessGoEnv("", nil)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr
	if err := execCmd.Run(); err != nil {
		return httpRuntimeProfileResult{}, fmt.Errorf(
			"run HTTP runtime benchmark (%s): %w\nstdout:\n%s\nstderr:\n%s",
			mode,
			err,
			stdout.String(),
			stderr.String(),
		)
	}

	nsPerOp, bytesPerOp, allocsPerOp, err := parseHTTPRuntimeBenchmarkOutput(stdout.String(), mode)
	if err != nil {
		return httpRuntimeProfileResult{}, fmt.Errorf("parse benchmark output for %s: %w\nstdout:\n%s", mode, err, stdout.String())
	}

	cpuTop, err := runPprofTop(dir, testBinary, cpuPath, cmd.Top)
	if err != nil {
		return httpRuntimeProfileResult{}, fmt.Errorf("pprof cpu top for %s: %w", mode, err)
	}
	allocTop, err := runPprofAllocSpaceTop(dir, testBinary, memPath, cmd.Top)
	if err != nil {
		return httpRuntimeProfileResult{}, fmt.Errorf("pprof alloc_space top for %s: %w", mode, err)
	}

	return httpRuntimeProfileResult{
		Mode:          mode,
		NSPerOp:       nsPerOp,
		BytesPerOp:    bytesPerOp,
		AllocsPerOp:   allocsPerOp,
		CPUProfile:    cpuPath,
		MemProfile:    memPath,
		CPUTop:        cpuTop,
		AllocSpaceTop: allocTop,
	}, nil
}

func parseHTTPRuntimeBenchmarkOutput(stdout string, mode string) (float64, float64, float64, error) {
	want := "BenchmarkHTTPRuntimeModes/health_route/" + mode
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, want) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}
		nsPerOp, err := strconv.ParseFloat(fields[2], 64)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("parse ns/op: %w", err)
		}
		bytesPerOp, err := strconv.ParseFloat(fields[4], 64)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("parse B/op: %w", err)
		}
		allocsPerOp, err := strconv.ParseFloat(fields[6], 64)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("parse allocs/op: %w", err)
		}
		return nsPerOp, bytesPerOp, allocsPerOp, nil
	}
	return 0, 0, 0, fmt.Errorf("benchmark line not found")
}

func runPprofTop(dir, binaryPath, profilePath string, top int) (string, error) {
	execCmd := exec.Command(
		"go",
		"tool",
		"pprof",
		"-top",
		"-nodecount",
		strconv.Itoa(top),
		binaryPath,
		profilePath,
	)
	execCmd.Dir = dir
	execCmd.Env = testkit.ProcessGoEnv("", nil)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr
	if err := execCmd.Run(); err != nil {
		return "", fmt.Errorf("run pprof top: %w\nstderr:\n%s", err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

func runPprofAllocSpaceTop(dir, binaryPath, profilePath string, top int) (string, error) {
	execCmd := exec.Command(
		"go",
		"tool",
		"pprof",
		"-top",
		"-alloc_space",
		"-nodecount",
		strconv.Itoa(top),
		binaryPath,
		profilePath,
	)
	execCmd.Dir = dir
	execCmd.Env = testkit.ProcessGoEnv("", nil)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr
	if err := execCmd.Run(); err != nil {
		return "", fmt.Errorf("run pprof alloc_space top: %w\nstderr:\n%s", err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (cmd *TestHTTPRuntimeProfileCmd) printResults(results []httpRuntimeProfileResult) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].NSPerOp < results[j].NSPerOp
	})

	baseline := 0.0
	for _, result := range results {
		if result.Mode == "baseline" {
			baseline = result.NSPerOp
			break
		}
	}

	rows := [][]string{
		{"Mode", "ns/op", "Delta %", "allocs/op", "B/op"},
	}
	for _, result := range results {
		delta := "baseline"
		if baseline > 0 && result.Mode != "baseline" {
			delta = formatPercentDelta(baseline, result.NSPerOp)
		}
		rows = append(rows, []string{
			result.Mode,
			fmt.Sprintf("%.1f", result.NSPerOp),
			delta,
			fmt.Sprintf("%.1f", result.AllocsPerOp),
			fmt.Sprintf("%.1f", result.BytesPerOp),
		})
	}

	fmt.Fprintln(os.Stdout, "HTTP Runtime Benchmark Modes")
	printASCIITable(os.Stdout, rows)
	fmt.Fprintln(os.Stdout)

	for _, result := range results {
		fmt.Fprintf(os.Stdout, "[%s] CPU Top\n", strings.ToUpper(result.Mode))
		fmt.Fprintln(os.Stdout, result.CPUTop)
		fmt.Fprintln(os.Stdout)
		fmt.Fprintf(os.Stdout, "[%s] Alloc Space Top\n", strings.ToUpper(result.Mode))
		fmt.Fprintln(os.Stdout, result.AllocSpaceTop)
		fmt.Fprintln(os.Stdout)
		fmt.Fprintf(os.Stdout, "profiles: cpu=%s mem=%s\n\n", result.CPUProfile, result.MemProfile)
	}
}
