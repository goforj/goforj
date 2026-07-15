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
	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

// LoggerOverheadMeasureCmd renders a temp app and benchmarks the generated logger layer.
type LoggerOverheadMeasureCmd struct {
	logger *logger.AppLogger

	Iterations int  `help:"Fixed iterations per benchmark mode" default:"200000"`
	Rounds     int  `help:"Comparison rounds to run and summarize with medians" default:"3"`
	Keep       bool `help:"Keep the rendered temp project after completion" short:"k"`
	Silent     bool `help:"Suppress command progress output" short:"s"`
}

type loggerOverheadBenchResult struct {
	Scenario    string
	Mode        string
	NSPerOp     float64
	BytesPerOp  float64
	AllocsPerOp float64
}

type loggerOverheadRound struct {
	Index   int
	Results map[string]loggerOverheadBenchResult
}

func (*LoggerOverheadMeasureCmd) Signature() string {
	return `name:"bench:logger-overhead" help:"Measure generated logger overhead against direct zerolog" hidden:""`
}

func NewLoggerOverheadMeasureCmd(logger *logger.AppLogger) *LoggerOverheadMeasureCmd {
	return &LoggerOverheadMeasureCmd{logger: logger}
}

func (cmd *LoggerOverheadMeasureCmd) Run() error {
	if cmd.Iterations <= 0 {
		return fmt.Errorf("iterations must be greater than zero")
	}
	if cmd.Rounds <= 0 {
		return fmt.Errorf("rounds must be greater than zero")
	}

	modCache, buildCache := testkit.GoCachePaths()
	dir, err := os.MkdirTemp("", "forj_logger_overhead_")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	if !cmd.Keep {
		defer os.RemoveAll(dir)
	}

	if !cmd.Silent {
		testkit.PrintSection("Logger Overhead")
		console.Actionf("Rendering fixed logger overhead probe app")
	}

	cfg := project.Config{
		ProjectName:  "Logger Overhead",
		GoModuleName: "example.com/loggeroverheadapp",
		UpdatedAt:    "2026-05-21 00:00:00 UTC",
		Render: project.RenderConfig{
			Components: project.Components{
				CLI: true,
			},
		},
	}
	if err := testkit.WriteProjectConfig(filepath.Join(dir, ".goforj.yml"), cfg); err != nil {
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
	if err := runStep(cmd.logger, cmd.Silent, "build", dir, modCache, buildCache, []string{"go", "build", "./..."}); err != nil {
		return err
	}

	rounds := make([]loggerOverheadRound, 0, cmd.Rounds)
	for round := 1; round <= cmd.Rounds; round++ {
		results, err := cmd.runBench(dir, round)
		if err != nil {
			return err
		}
		rounds = append(rounds, loggerOverheadRound{Index: round, Results: results})
	}

	if !cmd.Silent {
		cmd.printComparison(rounds)
		if cmd.Keep {
			cmd.logger.Info().Str("path", dir).Msg("Kept rendered logger overhead probe project")
		}
	}
	return nil
}

func (cmd *LoggerOverheadMeasureCmd) runBench(dir string, round int) (map[string]loggerOverheadBenchResult, error) {
	if !cmd.Silent {
		console.Actionf("Running logger overhead benchmark (round %d/%d)", round, cmd.Rounds)
	}
	benchTime := fmt.Sprintf("%dx", cmd.Iterations)
	execCmd := exec.CommandContext(
		context.Background(),
		"go",
		"test",
		"./internal/logger",
		"-run", "^$",
		"-bench", "BenchmarkLoggerOverhead",
		"-benchmem",
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
		return nil, fmt.Errorf("run logger overhead benchmark: %w\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	results, err := parseLoggerOverheadBenchOutput(stdout.String())
	if err != nil {
		return nil, fmt.Errorf("parse logger overhead benchmark: %w\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	return results, nil
}

func parseLoggerOverheadBenchOutput(stdout string) (map[string]loggerOverheadBenchResult, error) {
	results := map[string]loggerOverheadBenchResult{}
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "BenchmarkLoggerOverhead/") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}
		name := strings.TrimPrefix(fields[0], "BenchmarkLoggerOverhead/")
		if idx := strings.LastIndex(name, "-"); idx > 0 {
			name = name[:idx]
		}
		parts := strings.Split(name, "/")
		if len(parts) < 3 {
			return nil, fmt.Errorf("unexpected benchmark name %q", name)
		}
		scenario := parts[0] + "/" + parts[1]
		mode := strings.Join(parts[2:], "/")
		nsPerOp, err := strconv.ParseFloat(fields[2], 64)
		if err != nil {
			return nil, fmt.Errorf("parse ns/op for %s: %w", name, err)
		}
		bytesPerOp, err := strconv.ParseFloat(fields[4], 64)
		if err != nil {
			return nil, fmt.Errorf("parse B/op for %s: %w", name, err)
		}
		allocsPerOp, err := strconv.ParseFloat(fields[6], 64)
		if err != nil {
			return nil, fmt.Errorf("parse allocs/op for %s: %w", name, err)
		}
		key := scenario + "/" + mode
		results[key] = loggerOverheadBenchResult{
			Scenario:    scenario,
			Mode:        mode,
			NSPerOp:     nsPerOp,
			BytesPerOp:  bytesPerOp,
			AllocsPerOp: allocsPerOp,
		}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no logger overhead benchmark results found")
	}
	return results, nil
}

func (cmd *LoggerOverheadMeasureCmd) printComparison(rounds []loggerOverheadRound) {
	byMode := map[string][]loggerOverheadBenchResult{}
	for _, round := range rounds {
		for mode, result := range round.Results {
			byMode[mode] = append(byMode[mode], result)
		}
	}

	for _, scenario := range []string{"http_access/repeated", "http_access/unique"} {
		base := medianLoggerOverhead(byMode[scenario+"/zerolog_direct"])
		rows := [][]string{
			{"Mode", "ns/op", "Delta %", "allocs/op", "Alloc delta %", "B/op", "Bytes delta %", "Rounds"},
			{
				"zerolog_direct",
				fmt.Sprintf("%.1f", base.NSPerOp),
				"baseline",
				fmt.Sprintf("%.1f", base.AllocsPerOp),
				"baseline",
				fmt.Sprintf("%.1f", base.BytesPerOp),
				"baseline",
				fmt.Sprintf("%d", len(byMode[scenario+"/zerolog_direct"])),
			},
		}
		modeNames := make([]string, 0, 8)
		prefix := scenario + "/"
		for key := range byMode {
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			mode := strings.TrimPrefix(key, prefix)
			if mode == "zerolog_direct" {
				continue
			}
			modeNames = append(modeNames, mode)
		}
		sort.Strings(modeNames)
		for _, mode := range modeNames {
			median := medianLoggerOverhead(byMode[prefix+mode])
			rows = append(rows, []string{
				mode,
				fmt.Sprintf("%.1f", median.NSPerOp),
				formatPercentDelta(base.NSPerOp, median.NSPerOp),
				fmt.Sprintf("%.1f", median.AllocsPerOp),
				formatPercentDelta(base.AllocsPerOp, median.AllocsPerOp),
				fmt.Sprintf("%.1f", median.BytesPerOp),
				formatPercentDelta(base.BytesPerOp, median.BytesPerOp),
				fmt.Sprintf("%d", len(byMode[prefix+mode])),
			})
		}
		fmt.Fprintf(os.Stdout, "Logger Overhead (%s)\n", scenario)
		printASCIITable(os.Stdout, rows)
		fmt.Fprintln(os.Stdout)
	}
}

func medianLoggerOverhead(results []loggerOverheadBenchResult) loggerOverheadBenchResult {
	if len(results) == 0 {
		return loggerOverheadBenchResult{}
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
	base.NSPerOp = medianFloat64Logger(nsValues)
	base.AllocsPerOp = medianFloat64Logger(allocValues)
	base.BytesPerOp = medianFloat64Logger(byteValues)
	return base
}

func medianFloat64Logger(values []float64) float64 {
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
