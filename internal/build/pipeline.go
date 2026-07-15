package build

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/goforj/goforj/internal/apiindex"
	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
	"golang.org/x/term"
)

// Step names one observable pipeline action and returns its compact status.
type Step struct {
	Name string
	Run  func() (string, error)
}

// Pipeline coordinates source generation, indexing, and the caller's final build or launch step.
type Pipeline struct {
	logger   *logger.AppLogger
	apiIndex apiindex.Preparer
}

const buildProgressMarker = "__FORJ_BUILD_PROGRESS__"

type buildProgressReporter interface {
	Step(index int, total int, step string)
	State(state string)
}

type buildProgressNoop struct{}

func (buildProgressNoop) Step(int, int, string) {}
func (buildProgressNoop) State(string)          {}

type buildProgressMarkerReporter struct{}

func (buildProgressMarkerReporter) Step(index int, total int, step string) {
	fmt.Fprintf(os.Stderr, "%s step %d/%d %s\n", buildProgressMarker, index, total, strings.TrimSpace(step))
}

func (buildProgressMarkerReporter) State(state string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", buildProgressMarker, strings.TrimSpace(state))
}

type buildProgressTTYReporter struct {
	mu          sync.Mutex
	label       string
	running     bool
	clearOnDone bool
	done        chan struct{}
}

func (r *buildProgressTTYReporter) Step(index int, total int, step string) {
	r.mu.Lock()
	r.label = fmt.Sprintf("%d/%d %s", index, total, strings.TrimSpace(step))
	started := r.running
	if !r.running {
		r.running = true
		r.done = make(chan struct{})
	}
	label := r.label
	done := r.done
	r.mu.Unlock()

	if !started {
		go r.loop(done)
	}
	r.renderFrame(0, label)
}

func (r *buildProgressTTYReporter) State(state string) {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return
	}
	done := r.done
	label := r.label
	r.running = false
	r.done = nil
	r.mu.Unlock()

	close(done)
	if strings.EqualFold(strings.TrimSpace(state), "done") {
		if r.clearOnDone {
			fmt.Fprint(os.Stderr, "\r\x1b[2K")
			return
		}
		fmt.Fprintf(os.Stderr, "\r\x1b[2K%s %s", console.SuccessMark(), label)
	} else {
		r.renderFrame(0, label)
	}
	fmt.Fprintln(os.Stderr)
}

func (r *buildProgressTTYReporter) loop(done <-chan struct{}) {
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()

	frame := 0
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			r.mu.Lock()
			label := r.label
			running := r.running
			r.mu.Unlock()
			if !running {
				return
			}
			frame++
			r.renderFrame(frame, label)
		}
	}
}

func (r *buildProgressTTYReporter) renderFrame(frame int, label string) {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	fmt.Fprintf(
		os.Stderr,
		"\r\x1b[2K%s %s",
		console.Colorize(console.ColorGreen, frames[frame%len(frames)]),
		label,
	)
}

// RunOptions controls diagnostics, source selection, and progress behavior for one pipeline invocation.
type RunOptions struct {
	Timings  bool
	SkipWire bool
	// APIIndexStrict fails the pipeline when web indexing reports warnings or errors.
	APIIndexStrict bool
	// BuildTags keeps API indexing on the same conditional source surface as the final Go build.
	BuildTags                []string
	TransientProgress        bool
	ClearProgressBeforeFinal bool
}

// NewPipeline creates a build pipeline whose index candidates share the final step's success boundary.
func NewPipeline(appLogger *logger.AppLogger, apiIndex apiindex.Preparer) Pipeline {
	return Pipeline{
		logger:   appLogger,
		apiIndex: apiIndex,
	}
}

// Run executes the configured steps from the project root and publishes API artifacts after the final step succeeds.
func (p Pipeline) Run(root string, kind string, final Step, opts RunOptions) error {
	if err := apiindex.ValidateGOFLAGS(os.Getenv("GOFLAGS")); err != nil {
		return err
	}
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
	progress := newBuildProgressReporter(debug, opts)
	var pendingAPIIndex apiindex.Candidate
	defer func() {
		if pendingAPIIndex != nil {
			pendingAPIIndex.Discard()
		}
	}()
	generateStep := Step{Name: "generate", Run: p.generateProjectFiles}
	steps := make([]Step, 0, 4)
	steps = append(steps, generateStep)
	if buildUsesTemplHTMX() {
		steps = append(steps, Step{Name: "templ", Run: p.runTemplGenerate})
	}
	if !opts.SkipWire {
		steps = append(steps, Step{Name: "wire", Run: p.runWireGenerate})
	}
	steps = append(steps, Step{Name: "build:api-index", Run: func() (string, error) {
		candidate, status, err := p.prepareAPIIndex(opts.APIIndexStrict, opts.BuildTags...)
		if err != nil {
			return "", err
		}
		pendingAPIIndex = candidate
		return status, nil
	}})
	steps = append(steps, final)
	progressState := "done"
	defer func() {
		progress.State(progressState)
	}()

	if debug {
		p.logger.Info().Str("kind", kind).Str("step", generateStep.Name).Msg("Running pipeline step")
	}
	progress.Step(1, len(steps), generateStep.Name)
	generateStartedAt := time.Now()
	generateStatus, err := generateStep.Run()
	if err != nil {
		progressState = "failed"
		return err
	}
	if opts.Timings {
		printStepTiming(kind, generateStep.Name, time.Since(generateStartedAt), generateStatus)
	}

	postGenerateSteps := steps[1 : len(steps)-1]

	stepResults := make(map[string]struct {
		status   string
		duration time.Duration
	}, len(postGenerateSteps))
	for idx, step := range postGenerateSteps {
		if debug {
			p.logger.Info().Str("kind", kind).Str("step", step.Name).Msg("Running pipeline step")
		}
		progress.Step(idx+2, len(steps), step.Name)
		startedAt := time.Now()
		status, err := step.Run()
		if err != nil {
			progressState = "failed"
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
	progress.Step(len(steps), len(steps), final.Name)
	if opts.ClearProgressBeforeFinal {
		progress.State("done")
	}
	finalStartedAt := time.Now()
	finalStatus, err := runFinalAndPublishAPIIndex(final, pendingAPIIndex)
	if err != nil {
		progressState = "failed"
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

// runFinalAndPublishAPIIndex keeps the previous active contract until compilation or process startup succeeds.
func runFinalAndPublishAPIIndex(final Step, pending apiindex.Candidate) (string, error) {
	status, err := final.Run()
	if err != nil {
		return "", err
	}
	if pending != nil {
		if err := pending.Publish(); err != nil {
			return "", err
		}
	}
	return status, nil
}

// prepareAPIIndex applies the command's diagnostics policy while keeping pipeline status formatting centralized.
func (p Pipeline) prepareAPIIndex(strict bool, buildTags ...string) (apiindex.Candidate, string, error) {
	prepared, status, err := p.apiIndex.Prepare(apiindex.Options{Strict: strict, BuildTags: append([]string(nil), buildTags...)})
	if err != nil {
		return nil, status, fmt.Errorf("%s: %w", status, err)
	}
	return prepared, status, nil
}

func buildProgressEnabled() bool {
	value := strings.TrimSpace(os.Getenv("FORJ_BUILD_PROGRESS"))
	return value != "" && value != "0" && !strings.EqualFold(value, "false")
}

func newBuildProgressReporter(debug bool, opts RunOptions) buildProgressReporter {
	if buildProgressEnabled() {
		return buildProgressMarkerReporter{}
	}
	if debug || opts.Timings || !term.IsTerminal(int(os.Stderr.Fd())) {
		return buildProgressNoop{}
	}
	return &buildProgressTTYReporter{clearOnDone: opts.TransientProgress}
}

func printStepTiming(kind string, stepName string, duration time.Duration, status string) {
	timing := duration.Round(time.Millisecond)
	if strings.TrimSpace(status) != "" {
		fmt.Fprintf(os.Stderr, "forj %s %s: %s (%s)\n", kind, stepName, timing, status)
		return
	}
	fmt.Fprintf(os.Stderr, "forj %s %s: %s\n", kind, stepName, timing)
}

// buildUsesTemplHTMX limits template generation to the selected App's configured starter kit so unrelated Apps do not add build work.
func buildUsesTemplHTMX() bool {
	cfg, err := project.LoadProjectConfig()
	if err != nil {
		return false
	}
	app := ActiveApp()
	starterKit := cfg.Render.StarterKit
	if app.Name != project.DefaultAppName {
		if appConfig, ok := cfg.Apps[app.Name]; ok {
			starterKit = appConfig.StarterKit
		}
	}
	return project.NormalizeStarterKit(starterKit) == project.StarterKitTemplHTMX
}

func (p Pipeline) runTemplGenerate() (string, error) {
	cmd := exec.Command("go", "run", "github.com/a-h/templ/cmd/templ@v0.3.1020", "generate")
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail != "" {
			printBuildFailureDetail(detail)
		}
		return "", fmt.Errorf("templ generate: %w", err)
	}
	return "generated", nil
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
		printBuildFailureDetail(detail)
		return "", fmt.Errorf("wire (%s): %w", wirePath, err)
	}
	return "", fmt.Errorf("wire (%s): %w", wirePath, err)
}

func printBuildFailureDetail(detail string) {
	trimmed := strings.TrimSpace(detail)
	if trimmed == "" {
		return
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, trimmed)
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

// generateProjectFiles uses durable component intent when available so stale generated directories cannot reactivate optional primitives.
func (p Pipeline) generateProjectFiles() (string, error) {
	storageEnabled := hasDir(filepath.Join(".", "internal", "storages"))
	eventsEnabled := hasDir(filepath.Join(".", "internal", "events"))
	config, err := project.LoadProjectConfig()
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("load project generation config: %w", err)
	}
	if config != nil {
		components := project.ProjectComponents(config)
		storageEnabled = components.Storage
		eventsEnabled = components.Events
	}
	generatedFiles, changedFiles, err := generate.GenerateProjectFiles(
		".",
		storageEnabled,
		hasDir(filepath.Join(".", "internal", "caches")),
		hasDir(filepath.Join(".", "internal", "queues")),
		eventsEnabled,
		hasDir(filepath.Join(".", "internal", "database")),
		hasDir(filepath.Join(".", "containers", "observability", "vmagent")),
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

// loadWirePaths reads project-configured Wire roots and falls back to the generated app layout.
func loadWirePaths() []string {
	if targetName := requestedAppName(); targetName != "" {
		if !project.IsSafeAppName(targetName) {
			return defaultWirePaths()
		}
		target := project.DefaultNamedApp(targetName)
		if hasDir(target.WireDir) {
			return []string{target.WireDir}
		}
	}
	config, err := project.LoadProjectConfig()
	if err != nil {
		return defaultWirePaths()
	}
	if len(config.Dev.WirePaths) == 0 {
		return defaultWirePaths()
	}
	return config.Dev.WirePaths
}

// defaultWirePaths prefers app/wire so rendered projects do not depend on the legacy root wire directory.
func defaultWirePaths() []string {
	if hasDir(filepath.Join("app", "wire")) {
		return []string{filepath.Join("app", "wire")}
	}
	return []string{"wire"}
}

// hasDir treats missing paths as a normal layout probe instead of an error.
func hasDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func debugEnabled() bool {
	for _, key := range []string{"FORJ_DEBUG", "DEBUG"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" && value != "0" {
			return true
		}
	}
	return false
}
