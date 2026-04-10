package forj

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
	"github.com/goforj/goforj/version"
	"gopkg.in/yaml.v3"
)

func formatCommandFailure(command string, err error, stdout, stderr string) error {
	var parts []string
	parts = append(parts, fmt.Sprintf("%s: %v", command, err))
	if trimmed := strings.TrimSpace(stdout); trimmed != "" {
		parts = append(parts, fmt.Sprintf("stdout:\n%s", trimmed))
	}
	if trimmed := strings.TrimSpace(stderr); trimmed != "" {
		parts = append(parts, fmt.Sprintf("stderr:\n%s", trimmed))
	}
	return fmt.Errorf("%s", strings.Join(parts, "\n\n"))
}

type TestRendersCmd struct {
	logger *logger.AppLogger

	// Full runs the complete component matrix.
	Full bool `help:"Run the full component matrix"`
}

func (*TestRendersCmd) Signature() string {
	return `name:"test:renders" help:"Runs all combinations of project configurations to test rendering" hidden:""`
}

type stepTimer struct {
	start time.Time
	parts map[string]time.Duration
}

func newStepTimer() *stepTimer {
	return &stepTimer{
		start: time.Now(),
		parts: make(map[string]time.Duration),
	}
}

func (t *stepTimer) Track(name string, fn func() error) error {
	s := time.Now()
	err := fn()
	t.parts[name] = time.Since(s)
	return err
}

func (t *stepTimer) Report(label string) {
	total := time.Since(t.start)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n⏱ Timing breakdown for %s\n", label))
	b.WriteString("---------------------------------------------------\n")

	for k, v := range t.parts {
		b.WriteString(fmt.Sprintf("%-15s %s\n", k+":", v))
	}

	b.WriteString("---------------------------------------------------\n")
	b.WriteString(fmt.Sprintf("%-15s %s\n\n", "total:", total))

	fmt.Print(b.String())
}

func NewTestRendersCmd(logger *logger.AppLogger) *TestRendersCmd {
	return &TestRendersCmd{logger: logger}
}

func (cmd *TestRendersCmd) Run() error {
	combos := buildRenderCombos(cmd.Full)
	totalCombos := len(combos)
	combos, shardLabel, err := shardRenderCombos(combos)
	if err != nil {
		return err
	}
	console.Infof("Testing %d component combinations%s", len(combos), shardLabel)
	if len(combos) == 0 {
		console.Warnf("No render combinations selected%s", shardLabel)
		return nil
	}

	modCache, buildCache := testkit.GoCachePaths()
	workspaceRoot := testRenderWorkspaceRoot()
	if err := os.MkdirAll(workspaceRoot, 0755); err != nil {
		return fmt.Errorf("create test render workspace: %w", err)
	}

	workerCount := testRenderWorkerCount()
	console.Infof("Render workers: %d", workerCount)

	jobs := make(chan renderCombo)
	var wg sync.WaitGroup
	wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		workerRoot := filepath.Join(workspaceRoot, fmt.Sprintf("worker-%02d", i))
		go func(root string) {
			defer wg.Done()
			for combo := range jobs {
				_ = os.RemoveAll(root)
				if err := os.MkdirAll(root, 0755); err != nil {
					cmd.fail("workspace dir failed", combo.id, nil, err)
					continue
				}

				if err := initModule(root, modCache, buildCache); err != nil {
					_ = os.RemoveAll(root)
					cmd.fail("go mod init failed", combo.id, nil, err)
					continue
				}

				cmd.runCombo(root, modCache, buildCache, combo)
				_ = os.RemoveAll(root)
			}
		}(workerRoot)
	}

	for _, combo := range combos {
		jobs <- combo
	}
	close(jobs)
	wg.Wait()
	console.Successf("Rendered %d combinations%s", len(combos), shardLabel)
	if totalCombos != len(combos) {
		console.Infof("Shard completed %d/%d combinations", len(combos), totalCombos)
	}
	return nil
}

func shardRenderCombos(combos []renderCombo) ([]renderCombo, string, error) {
	count := 1
	if v := strings.TrimSpace(os.Getenv("FORJ_TEST_RENDERS_SHARD_COUNT")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, "", fmt.Errorf("invalid FORJ_TEST_RENDERS_SHARD_COUNT=%q (must be integer >= 1)", v)
		}
		count = n
	}
	if count == 1 {
		return combos, "", nil
	}

	index := 0
	if v := strings.TrimSpace(os.Getenv("FORJ_TEST_RENDERS_SHARD_INDEX")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return nil, "", fmt.Errorf("invalid FORJ_TEST_RENDERS_SHARD_INDEX=%q (must be integer >= 0)", v)
		}
		index = n
	}
	if index >= count {
		return nil, "", fmt.Errorf(
			"invalid shard config: FORJ_TEST_RENDERS_SHARD_INDEX=%d must be < FORJ_TEST_RENDERS_SHARD_COUNT=%d",
			index,
			count,
		)
	}

	filtered := make([]renderCombo, 0, len(combos)/count+1)
	for i, combo := range combos {
		if i%count == index {
			filtered = append(filtered, combo)
		}
	}
	label := fmt.Sprintf(" (shard %d/%d · total %d)", index+1, count, len(combos))
	return filtered, label, nil
}

func testRenderWorkspaceRoot() string {
	if v := strings.TrimSpace(os.Getenv("FORJ_TEST_RENDERS_DIR")); v != "" {
		return v
	}
	return filepath.Join(os.TempDir(), "forj_test_renders")
}

func testRenderWorkerCount() int {
	if v := strings.TrimSpace(os.Getenv("FORJ_TEST_RENDERS_WORKERS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	workerCount := runtime.NumCPU()
	if workerCount < 1 {
		workerCount = 1
	}
	if workerCount > 12 {
		workerCount = 12
	}
	return workerCount
}

// renderCombo describes a single component combination to render.
type renderCombo struct {
	id         string
	components project.Components
	enabled    []string
}

// featureCombo captures toggles for non-database components.
type featureCombo struct {
	auth       bool
	webAPI     bool
	webUI      bool
	scheduler  bool
	jobs       bool
	stressTest bool
}

// featureID returns a stable, readable id for the feature set.
func featureID(feature featureCombo) string {
	parts := []string{"base"}
	if feature.auth {
		parts = append(parts, "auth")
	}
	if feature.webAPI {
		parts = append(parts, "webapi")
	}
	if feature.webUI {
		parts = append(parts, "webui")
	}
	if feature.scheduler {
		parts = append(parts, "scheduler")
	}
	if feature.jobs {
		parts = append(parts, "jobs")
	}
	if feature.stressTest {
		parts = append(parts, "stresstest")
	}
	return strings.Join(parts, "_")
}

// buildRenderCombos builds the render matrix for the run.
func buildRenderCombos(full bool) []renderCombo {
	if full {
		return buildFullRenderCombos()
	}
	return buildCuratedRenderCombos()
}

// buildFullRenderCombos returns the full component matrix.
func buildFullRenderCombos() []renderCombo {
	const numCombos = 1 << 9
	combos := make([]renderCombo, 0, numCombos)
	for i := 0; i < numCombos; i++ {
		cfg := project.Components{
			CLI:              true,
			Docker:           true,
			Auth:             i&(1<<0) != 0,
			WebAPI:           i&(1<<1) != 0,
			WebUI:            i&(1<<2) != 0,
			DatabaseMySQL:    i&(1<<3) != 0,
			DatabasePostgres: i&(1<<4) != 0,
			DatabaseSQLite:   i&(1<<5) != 0,
			Scheduler:        i&(1<<6) != 0,
			Jobs:             i&(1<<7) != 0,
			StressTest:       i&(1<<8) != 0,
		}

		if cfg.StressTest && !cfg.Jobs {
			cfg.StressTest = false
		}

		if cfg.DatabaseSQLite {
			cfg.DatabaseMySQL = false
			cfg.DatabasePostgres = false
		}
		if cfg.DatabasePostgres {
			cfg.DatabaseMySQL = false
		}
		if err := cfg.ValidateRenderContract(); err != nil {
			continue
		}

		combos = append(combos, renderCombo{
			id:         fmt.Sprintf("%v", i),
			components: cfg,
			enabled:    componentLabels(cfg),
		})
	}
	return combos
}

// buildCuratedRenderCombos returns a curated pairwise set of combos.
func buildCuratedRenderCombos() []renderCombo {
	features := []featureCombo{
		{},
		{auth: true},
		{webAPI: true},
		{webUI: true},
		{scheduler: true},
		{jobs: true},
		{auth: true, webAPI: true},
		{auth: true, webUI: true},
		{webAPI: true, webUI: true},
		{webAPI: true, scheduler: true},
		{webAPI: true, jobs: true},
		{webUI: true, scheduler: true},
		{webUI: true, jobs: true},
		{scheduler: true, jobs: true},
		{jobs: true, stressTest: true},
		{scheduler: true, jobs: true, stressTest: true},
	}

	dbVariants := []struct {
		name  string
		apply func(*project.Components)
	}{
		{name: "mysql", apply: func(c *project.Components) { c.DatabaseMySQL = true }},
		{name: "postgres", apply: func(c *project.Components) { c.DatabasePostgres = true }},
		{name: "sqlite", apply: func(c *project.Components) { c.DatabaseSQLite = true }},
	}

	var combos []renderCombo
	for _, feature := range features {
		cfg := project.Components{
			CLI:        true,
			Docker:     true,
			Auth:       feature.auth,
			WebAPI:     feature.webAPI,
			WebUI:      feature.webUI,
			Scheduler:  feature.scheduler,
			Jobs:       feature.jobs,
			StressTest: feature.stressTest && feature.jobs,
		}
		if err := cfg.ValidateRenderContract(); err != nil {
			continue
		}
		combos = append(combos, renderCombo{
			id:         featureID(feature),
			components: cfg,
			enabled:    componentLabels(cfg),
		})
	}

	for _, variant := range dbVariants {
		for idx, feature := range features {
			cfg := project.Components{
				CLI:        true,
				Docker:     true,
				Auth:       feature.auth,
				WebAPI:     feature.webAPI,
				WebUI:      feature.webUI,
				Scheduler:  feature.scheduler,
				Jobs:       feature.jobs,
				StressTest: feature.stressTest && feature.jobs,
			}
			variant.apply(&cfg)
			if err := cfg.ValidateRenderContract(); err != nil {
				continue
			}
			combos = append(combos, renderCombo{
				id:         fmt.Sprintf("%s_%02d", variant.name, idx),
				components: cfg,
				enabled:    componentLabels(cfg),
			})
		}
	}

	return combos
}

// componentLabels returns the human-friendly component labels for logging.
func componentLabels(cfg project.Components) []string {
	enabled := []string{"CLI", "Docker"}
	if cfg.Auth {
		enabled = append(enabled, "Auth")
	}
	if cfg.WebAPI {
		enabled = append(enabled, "WebAPI")
	}
	if cfg.WebUI {
		enabled = append(enabled, "WebUI")
	}
	if cfg.DatabaseMySQL {
		enabled = append(enabled, "Database (MySQL)")
	}
	if cfg.DatabasePostgres {
		enabled = append(enabled, "Database (Postgres)")
	}
	if cfg.DatabaseSQLite {
		enabled = append(enabled, "Database (SQLite)")
	}
	if cfg.Scheduler {
		enabled = append(enabled, "Scheduler")
	}
	if cfg.Jobs {
		enabled = append(enabled, "Jobs")
	}
	if cfg.StressTest {
		enabled = append(enabled, "Stress Test")
	}
	return enabled
}

// initModule creates the go.mod once for the shared render directory.
func initModule(dir, modCache, buildCache string) error {
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		return nil
	}
	goMod := exec.Command("go", "mod", "init", "github.com/test/project")
	goMod.Dir = dir
	goMod.Env = append(os.Environ(),
		"GOMODCACHE="+modCache,
		"GOCACHE="+buildCache,
	)
	if err := goMod.Run(); err != nil {
		return err
	}
	return nil
}

// runCombo renders and validates a single combo using a shared directory.
func (cmd *TestRendersCmd) runCombo(dir, modCache, buildCache string, combo renderCombo) {
	comboID := combo.id
	cfg := project.Config{
		ProjectName:  fmt.Sprintf("TestProject%s", comboID),
		GoModuleName: "github.com/test/project",
		UpdatedAt:    time.Now().Format(time.RFC3339),
		Dev:          project.DevConfig{},
		Render: project.RenderConfig{
			Components: combo.components,
		},
	}

	console.Actionf("Rendering components %s", strings.Join(combo.enabled, ", "))

	timer := newStepTimer()
	ymlPath := filepath.Join(dir, ".goforj.yml")

	if err := timer.Track("write_yaml", func() error {
		return WriteYAML(ymlPath, cfg)
	}); err != nil {
		cmd.fail("failed to write config", comboID, &cfg, err)
		return
	}

	if err := timer.Track("forj_render", func() error {
		render := exec.Command("forj", "render")
		render.Dir = dir
		render.Env = append(os.Environ(),
			"GOMODCACHE="+modCache,
			"GOCACHE="+buildCache,
		)

		var stdout, stderr strings.Builder
		render.Stdout = &stdout
		render.Stderr = &stderr

		if err := render.Run(); err != nil {
			return formatCommandFailure("forj render", err, stdout.String(), stderr.String())
		}
		if renderDebugEnabled() {
			if stdout.Len() > 0 {
				fmt.Printf("%s\n", strings.TrimSpace(stdout.String()))
			}
			if stderr.Len() > 0 {
				fmt.Printf("%s\n", strings.TrimSpace(stderr.String()))
			}
		}
		return nil
	}); err != nil {
		cmd.fail("render failed", comboID, &cfg, err)
		return
	}

	if err := timer.Track("wire_gen", func() error {
		wireCmd := exec.Command("wire")
		wireCmd.Dir = filepath.Join(dir, "wire")
		wireCmd.Env = append(os.Environ(),
			"GOMODCACHE="+modCache,
			"GOCACHE="+buildCache,
		)
		if output, err := wireCmd.CombinedOutput(); err != nil {
			return formatCommandFailure("wire generate", err, string(output), "")
		}
		return nil
	}); err != nil {
		cmd.fail("wire generate failed", comboID, &cfg, err)
		return
	}

	if err := timer.Track("go_build", func() error {
		binDir := filepath.Join(dir, "bin")
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			return fmt.Errorf("create bin dir: %w", err)
		}
		args := []string{"build"}
		if renderBuildTraceEnabled() {
			args = append(args, "-x")
		}
		args = append(args, "-o", filepath.Join(binDir, "app"))
		build := exec.Command("go", args...)
		build.Dir = dir
		build.Env = append(os.Environ(),
			"GOMODCACHE="+modCache,
			"GOCACHE="+buildCache,
		)
		output, err := build.CombinedOutput()
		if err != nil {
			return formatCommandFailure("go build", err, string(output), "")
		}
		if renderBuildTraceEnabled() {
			console.Infof("go build trace for combo %s:\n%s", comboID, strings.TrimSpace(string(output)))
		}
		return nil
	}); err != nil {
		cmd.fail("go build failed", comboID, &cfg, err)
		return
	}

	timer.Report(fmt.Sprintf("combo %s (%s)", comboID, strings.Join(combo.enabled, ", ")))

	console.Successf("Passed")
}

func WriteYAML(path string, cfg project.Config) error {
	if cfg.Render.QueueDriver == "" {
		cfg.Render.QueueDriver = "redis"
	}
	if strings.TrimSpace(cfg.Render.GoForjVersion) == "" {
		cfg.Render.GoForjVersion = version.Semver()
	}
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (cmd *TestRendersCmd) fail(reason, comboID string, cfg *project.Config, err error) {
	console.Errorf("Failure")
	console.Infof("reason: %s", reason)
	console.Infof("combo: %s", comboID)
	if err != nil {
		console.Infof("error: %v", err)
	}
	if cfg != nil {
		if yamlDump, yerr := yaml.Marshal(cfg); yerr == nil {
			console.Infof("config:\n%s", string(yamlDump))
		}
	}
	os.Exit(1)
}

func renderBuildTraceEnabled() bool {
	if !renderDebugEnabled() {
		return false
	}
	for _, key := range []string{"FORJ_RENDER_BUILD_TRACE", "FORJ_RENDER_GO_BUILD_TRACE"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" && value != "0" {
			return true
		}
	}
	return false
}
