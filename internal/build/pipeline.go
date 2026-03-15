package build

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
)

type Step struct {
	Name string
	Run  func() (string, error)
}

type Pipeline struct {
	logger   *logger.AppLogger
	apiIndex *APIIndexRunner
}

type RunOptions struct {
	Timings  bool
	SkipWire bool
}

func NewPipeline(appLogger *logger.AppLogger, apiIndex *APIIndexRunner) Pipeline {
	return Pipeline{
		logger:   appLogger,
		apiIndex: apiIndex,
	}
}

func (p Pipeline) Run(root string, kind string, final Step, opts RunOptions) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	originalWD, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := os.Chdir(absRoot); err != nil {
		return err
	}
	defer func() { _ = os.Chdir(originalWD) }()

	debug := debugEnabled()
	generateStep := Step{Name: "generate", Run: p.generateProjectFiles}
	if debug {
		p.logger.Info().Str("kind", kind).Str("step", generateStep.Name).Msg("Running pipeline step")
	}
	generateStartedAt := time.Now()
	generateStatus, err := generateStep.Run()
	if err != nil {
		return err
	}
	if opts.Timings {
		printStepTiming(kind, generateStep.Name, time.Since(generateStartedAt), generateStatus)
	}

	postGenerateSteps := []Step{}
	if !opts.SkipWire {
		postGenerateSteps = append(postGenerateSteps, Step{Name: "wire", Run: p.runWireGenerate})
	}
	postGenerateSteps = append(postGenerateSteps, Step{Name: "build:api-index", Run: p.runAPIIndex})

	stepResults := make(map[string]struct {
		status   string
		duration time.Duration
	}, len(postGenerateSteps))
	for _, step := range postGenerateSteps {
		if debug {
			p.logger.Info().Str("kind", kind).Str("step", step.Name).Msg("Running pipeline step")
		}
		startedAt := time.Now()
		status, err := step.Run()
		if err != nil {
			return err
		}
		stepResults[step.Name] = struct {
			status   string
			duration time.Duration
		}{
			status:   status,
			duration: time.Since(startedAt),
		}
	}
	for _, step := range postGenerateSteps {
		if opts.Timings {
			result := stepResults[step.Name]
			printStepTiming(kind, step.Name, result.duration, result.status)
		}
	}

	if debug {
		p.logger.Info().Str("kind", kind).Str("step", final.Name).Msg("Running pipeline step")
	}
	finalStartedAt := time.Now()
	finalStatus, err := final.Run()
	if err != nil {
		return err
	}
	if opts.Timings {
		printStepTiming(kind, final.Name, time.Since(finalStartedAt), finalStatus)
	}
	if debug {
		steps := 3
		if !opts.SkipWire {
			steps++
		}
		p.logger.Info().Str("kind", kind).Int("steps", steps).Msg("Pipeline completed")
	}
	return nil
}

func printStepTiming(kind string, stepName string, duration time.Duration, status string) {
	timing := duration.Round(time.Millisecond)
	if strings.TrimSpace(status) != "" {
		fmt.Fprintf(os.Stderr, "forj %s %s: %s (%s)\n", kind, stepName, timing, status)
		return
	}
	fmt.Fprintf(os.Stderr, "forj %s %s: %s\n", kind, stepName, timing)
}

func (p Pipeline) runWireGenerate() (string, error) {
	ran := false
	for _, wirePath := range loadWirePaths() {
		info, err := os.Stat(wirePath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", fmt.Errorf("wire path %q: %w", wirePath, err)
		}
		if !info.IsDir() {
			continue
		}
		ran = true

		status, err := p.runWireCommand(wirePath, debugEnabled())
		if err != nil {
			return "", err
		}
		if status != "" {
			return status, nil
		}
	}
	if !ran {
		return "no changes", nil
	}
	return "", nil
}

func (p Pipeline) runWireCommand(wirePath string, debug bool) (string, error) {
	status, detail, err := runWireCommandQuiet(wirePath)
	if err == nil {
		if debug && strings.TrimSpace(status) != "" {
			fmt.Fprintln(os.Stderr, strings.TrimSpace(status))
		}
		return status, nil
	}
	if shouldRetryWire(detail) {
		retryStatus, retryDetail, retryErr := runWireCommandQuiet(wirePath)
		if retryErr == nil {
			if retryStatus == "" {
				return "retried", nil
			}
			return retryStatus + ", retried", nil
		}
		detail = retryDetail
		err = retryErr
	}
	if detail != "" {
		if debug {
			fmt.Fprintln(os.Stderr, detail)
		}
		return "", fmt.Errorf("wire (%s): %w (%s)", wirePath, err, detail)
	}
	return "", fmt.Errorf("wire (%s): %w", wirePath, err)
}

func runWireCommandQuiet(wirePath string) (string, string, error) {
	cmd := exec.Command("wire")
	cmd.Dir = wirePath
	cmd.Env = append(os.Environ(), "WIRE_INCREMENTAL=1")

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	detail := strings.TrimSpace(stderr.String())
	if detail == "" {
		detail = strings.TrimSpace(stdout.String())
	}
	status := ""
	if err == nil {
		status = strings.TrimSpace(stdout.String())
	}
	return status, detail, err
}

func shouldRetryWire(detail string) bool {
	detail = strings.ToLower(strings.TrimSpace(detail))
	if detail == "" {
		return false
	}
	return strings.Contains(detail, "type-check failed for") &&
		strings.Contains(detail, "could not import")
}

func (p Pipeline) generateProjectFiles() (string, error) {
	generatedFiles, changedFiles, err := generate.GenerateProjectFiles(
		".",
		true,
		hasDir(filepath.Join(".", "internal", "cache")),
		hasDir(filepath.Join(".", "internal", "queue")),
		hasDir(filepath.Join(".", "internal", "dbconns")),
	)
	if err != nil {
		return "", fmt.Errorf("generate project files: %w", err)
	}
	if debugEnabled() {
		p.logger.Info().Int("files", generatedFiles).Msg("Generated project files")
	}
	if changedFiles == 0 {
		return "no changes", nil
	}
	return fmt.Sprintf("%d files", changedFiles), nil
}

func (p Pipeline) runAPIIndex() (string, error) {
	status, err := p.apiIndex.RunDefaultWithStatus()
	if err != nil {
		return "", err
	}
	return status, nil
}

func loadWirePaths() []string {
	config, err := project.LoadProjectConfig()
	if err != nil {
		return []string{"wire"}
	}
	if len(config.Dev.WirePaths) == 0 {
		return []string{"wire"}
	}
	return config.Dev.WirePaths
}

func hasDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func debugEnabled() bool {
	for _, key := range []string{"FORJ_DEBUG", "APP_DEBUG", "DEBUG"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" && value != "0" {
			return true
		}
	}
	return false
}
