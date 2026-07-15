package bench

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/testexec"
	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

// InspectOverheadMeasureCmd renders a temp app and compares HTTP request overhead with inspects off vs on.
type InspectOverheadMeasureCmd struct {
	logger *logger.AppLogger

	Iterations int  `help:"Fixed iterations per benchmark mode" default:"5000"`
	Rounds     int  `help:"Comparison rounds to run and summarize with medians" default:"5"`
	Keep       bool `help:"Keep the rendered temp project after completion" short:"k"`
	Silent     bool `help:"Suppress command progress output" short:"s"`
}

type inspectOverheadBenchResult struct {
	Scenario    string
	Mode        string
	NSPerOp     float64
	BytesPerOp  float64
	AllocsPerOp float64
}

type inspectOverheadRound struct {
	Index   int
	Results map[string]inspectOverheadBenchResult
}

func (*InspectOverheadMeasureCmd) Signature() string {
	return `name:"bench:inspect-overhead" help:"Measure HTTP request overhead with inspects off vs on" hidden:""`
}

func NewInspectOverheadMeasureCmd(logger *logger.AppLogger) *InspectOverheadMeasureCmd {
	return &InspectOverheadMeasureCmd{logger: logger}
}

func (cmd *InspectOverheadMeasureCmd) Run() error {
	if cmd.Iterations <= 0 {
		return fmt.Errorf("iterations must be greater than zero")
	}
	if cmd.Rounds <= 0 {
		return fmt.Errorf("rounds must be greater than zero")
	}

	modCache, buildCache := testkit.GoCachePaths()
	dir, err := os.MkdirTemp("", "forj_inspect_overhead_")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	if !cmd.Keep {
		defer os.RemoveAll(dir)
	}
	workspace := testexec.NewWorkspace(cmd.logger, cmd.Silent, dir, testexec.NewGoCaches(modCache, buildCache))

	if !cmd.Silent {
		testkit.PrintSection("Inspect Overhead")
		console.Actionf("Rendering fixed inspect overhead probe app")
	}

	cfg := project.Config{
		ProjectName:  "Inspect Overhead",
		GoModuleName: "example.com/inspectoverheadapp",
		UpdatedAt:    "2026-05-08 00:00:00 UTC",
		Render: project.RenderConfig{
			Components: project.Components{
				CLI:    true,
				WebAPI: true,
			},
		},
	}

	if err := testkit.WriteProjectConfig(filepath.Join(dir, ".goforj.yml"), cfg); err != nil {
		return err
	}

	builtForj, err := testkit.BuildForjBinary(modCache, buildCache)
	if err != nil {
		return err
	}
	defer builtForj.Cleanup()
	forjExec := builtForj.Path

	if err := workspace.Run("render", forjExec, "render"); err != nil {
		return err
	}
	if err := workspace.Run("build", "go", "build", "./..."); err != nil {
		return err
	}

	rounds := make([]inspectOverheadRound, 0, cmd.Rounds)
	for round := 1; round <= cmd.Rounds; round++ {
		results, err := cmd.runBench(dir, round)
		if err != nil {
			return err
		}
		rounds = append(rounds, inspectOverheadRound{Index: round, Results: results})
	}

	if !cmd.Silent {
		cmd.printComparison(rounds)
		if cmd.Keep {
			cmd.logger.Info().Str("path", dir).Msg("Kept rendered inspect overhead probe project")
		}
	}

	return nil
}

func (cmd *InspectOverheadMeasureCmd) runBench(dir string, round int) (map[string]inspectOverheadBenchResult, error) {
	if !cmd.Silent {
		console.Actionf("Running inspect overhead benchmark (round %d/%d)", round, cmd.Rounds)
	}

	benchTime := fmt.Sprintf("%dx", cmd.Iterations)
	execCmd := exec.CommandContext(
		context.Background(),
		"go",
		"test",
		"./internal/http",
		"-run", "^$",
		"-bench", "BenchmarkHTTPRequestInspectModes",
		"-benchtime", benchTime,
		"-count", "1",
	)
	execCmd.Dir = dir
	execCmd.Env = testkit.ProcessGoEnv("", nil)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr
	if err := execCmd.Run(); err != nil {
		return nil, fmt.Errorf("run inspect overhead benchmark: %w\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	results, err := parseInspectOverheadBenchOutput(stdout.String())
	if err != nil {
		return nil, fmt.Errorf("parse inspect overhead benchmark: %w\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	return results, nil
}

func parseInspectOverheadBenchOutput(stdout string) (map[string]inspectOverheadBenchResult, error) {
	results := map[string]inspectOverheadBenchResult{}
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "BenchmarkHTTPRequestInspectModes/") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}
		name := strings.TrimPrefix(fields[0], "BenchmarkHTTPRequestInspectModes/")
		if idx := strings.LastIndex(name, "-"); idx > 0 {
			name = name[:idx]
		}
		parts := strings.Split(name, "/")
		if len(parts) < 2 {
			return nil, fmt.Errorf("unexpected benchmark name %q", name)
		}
		scenario := parts[0]
		mode := parts[1]
		nsPerOp, err := strconv.ParseFloat(fields[2], 64)
		if err != nil {
			return nil, fmt.Errorf("parse ns/op for %s: %w", mode, err)
		}
		bytesPerOp, err := strconv.ParseFloat(fields[4], 64)
		if err != nil {
			return nil, fmt.Errorf("parse B/op for %s: %w", mode, err)
		}
		allocsPerOp, err := strconv.ParseFloat(fields[6], 64)
		if err != nil {
			return nil, fmt.Errorf("parse allocs/op for %s: %w", mode, err)
		}
		key := scenario + "/" + mode
		results[key] = inspectOverheadBenchResult{
			Scenario:    scenario,
			Mode:        mode,
			NSPerOp:     nsPerOp,
			BytesPerOp:  bytesPerOp,
			AllocsPerOp: allocsPerOp,
		}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no inspect overhead benchmark results found")
	}
	return results, nil
}

func (cmd *InspectOverheadMeasureCmd) printComparison(rounds []inspectOverheadRound) {
	byMode := map[string][]inspectOverheadBenchResult{}
	for _, round := range rounds {
		for mode, result := range round.Results {
			byMode[mode] = append(byMode[mode], result)
		}
	}
	for _, scenario := range []string{"minimal_get", "json_post"} {
		disabled := medianInspectOverhead(byMode[scenario+"/disabled"])
		enabled := medianInspectOverhead(byMode[scenario+"/enabled_publish_only"])
		rows := [][]string{
			{"Mode", "ns/op", "Delta %", "allocs/op", "Alloc delta %", "B/op", "Bytes delta %", "Rounds"},
			{
				"disabled",
				fmt.Sprintf("%.1f", disabled.NSPerOp),
				"baseline",
				fmt.Sprintf("%.1f", disabled.AllocsPerOp),
				"baseline",
				fmt.Sprintf("%.1f", disabled.BytesPerOp),
				"baseline",
				fmt.Sprintf("%d", len(byMode[scenario+"/disabled"])),
			},
			{
				"enabled_publish_only",
				fmt.Sprintf("%.1f", enabled.NSPerOp),
				formatPercentDelta(disabled.NSPerOp, enabled.NSPerOp),
				fmt.Sprintf("%.1f", enabled.AllocsPerOp),
				formatPercentDelta(disabled.AllocsPerOp, enabled.AllocsPerOp),
				fmt.Sprintf("%.1f", enabled.BytesPerOp),
				formatPercentDelta(disabled.BytesPerOp, enabled.BytesPerOp),
				fmt.Sprintf("%d", len(byMode[scenario+"/enabled_publish_only"])),
			},
		}
		fmt.Fprintf(os.Stdout, "HTTP Request Inspect Overhead (%s)\n", scenario)
		printASCIITable(os.Stdout, rows)
		fmt.Fprintln(os.Stdout)
	}
}

func medianInspectOverhead(results []inspectOverheadBenchResult) inspectOverheadBenchResult {
	if len(results) == 0 {
		return inspectOverheadBenchResult{}
	}
	base := results[0]
	nsValues := make([]float64, 0, len(results))
	allocValues := make([]float64, 0, len(results))
	byteValues := make([]float64, 0, len(results))
	for _, result := range results {
		nsValues = append(nsValues, result.NSPerOp)
		allocValues = append(allocValues, result.AllocsPerOp)
		byteValues = append(byteValues, result.BytesPerOp)
	}
	base.NSPerOp = medianFloat64Inspect(nsValues)
	base.AllocsPerOp = medianFloat64Inspect(allocValues)
	base.BytesPerOp = medianFloat64Inspect(byteValues)
	return base
}

func medianFloat64Inspect(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64{}, values...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}
