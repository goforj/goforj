package forj

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
	"gopkg.in/yaml.v3"
)

type TestRendersCmd struct {
	logger *logger.AppLogger

	// Full runs the complete component matrix.
	Full bool `help:"Run the full component matrix"`
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
	console.Infof("Testing %d component combinations", len(combos))
	if len(combos) == 0 {
		return nil
	}

	modCache, buildCache := getCachePaths()

	workerCount := runtime.NumCPU()
	if workerCount < 1 {
		workerCount = 1
	}
	if workerCount > 12 {
		workerCount = 12
	}

	jobs := make(chan renderCombo)
	var wg sync.WaitGroup
	wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go func() {
			defer wg.Done()
			for combo := range jobs {
				root, err := os.MkdirTemp("", "forj_test_project_")
				if err != nil {
					cmd.fail("temp dir failed", combo.id, nil, err)
					continue
				}

				if err := initModule(root, modCache); err != nil {
					_ = os.RemoveAll(root)
					cmd.fail("go mod init failed", combo.id, nil, err)
					continue
				}

				cmd.runCombo(root, modCache, buildCache, combo)
				_ = os.RemoveAll(root)
			}
		}()
	}

	for _, combo := range combos {
		jobs <- combo
	}
	close(jobs)
	wg.Wait()
	return nil
}

// -------------------------------------------------------------
// Cross-platform cache dirs (macOS/Linux/Windows safe)
// -------------------------------------------------------------
func getCachePaths() (string, string) {
	base, err := os.UserCacheDir()
	if err != nil {
		// guaranteed safe fallback
		base = os.TempDir()
	}

	modCache := filepath.Join(base, "goforj", "mod")
	buildCache := filepath.Join(base, "goforj", "build")

	_ = os.MkdirAll(modCache, 0755)
	_ = os.MkdirAll(buildCache, 0755)

	return modCache, buildCache
}

// renderCombo describes a single component combination to render.
type renderCombo struct {
	id         string
	components Components
	enabled    []string
}

// featureCombo captures toggles for non-database components.
type featureCombo struct {
	webAPI    bool
	webUI     bool
	scheduler bool
	jobs      bool
}

// featureID returns a stable, readable id for the feature set.
func featureID(feature featureCombo) string {
	parts := []string{"base"}
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
	const numCombos = 1 << 7
	combos := make([]renderCombo, 0, numCombos)
	for i := 0; i < numCombos; i++ {
		cfg := Components{
			CLI:              true,
			Docker:           true,
			WebAPI:           i&(1<<0) != 0,
			WebUI:            i&(1<<1) != 0,
			DatabaseMySQL:    i&(1<<2) != 0,
			DatabasePostgres: i&(1<<3) != 0,
			DatabaseSQLite:   i&(1<<4) != 0,
			Scheduler:        i&(1<<5) != 0,
			Jobs:             i&(1<<6) != 0,
		}

		if cfg.DatabaseSQLite {
			cfg.DatabaseMySQL = false
			cfg.DatabasePostgres = false
		}
		if cfg.DatabasePostgres {
			cfg.DatabaseMySQL = false
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
		{webAPI: true},
		{webUI: true},
		{scheduler: true},
		{jobs: true},
		{webAPI: true, webUI: true},
		{webAPI: true, scheduler: true},
		{webAPI: true, jobs: true},
		{webUI: true, scheduler: true},
		{webUI: true, jobs: true},
		{scheduler: true, jobs: true},
	}

	dbVariants := []struct {
		name  string
		apply func(*Components)
	}{
		{name: "mysql", apply: func(c *Components) { c.DatabaseMySQL = true }},
		{name: "postgres", apply: func(c *Components) { c.DatabasePostgres = true }},
		{name: "sqlite", apply: func(c *Components) { c.DatabaseSQLite = true }},
	}

	var combos []renderCombo
	for _, feature := range features {
		cfg := Components{
			CLI:       true,
			Docker:    true,
			WebAPI:    feature.webAPI,
			WebUI:     feature.webUI,
			Scheduler: feature.scheduler,
			Jobs:      feature.jobs,
		}
		combos = append(combos, renderCombo{
			id:         featureID(feature),
			components: cfg,
			enabled:    componentLabels(cfg),
		})
	}

	for _, variant := range dbVariants {
		for idx, feature := range features {
			cfg := Components{
				CLI:       true,
				Docker:    true,
				WebAPI:    feature.webAPI,
				WebUI:     feature.webUI,
				Scheduler: feature.scheduler,
				Jobs:      feature.jobs,
			}
			variant.apply(&cfg)
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
func componentLabels(cfg Components) []string {
	enabled := []string{"CLI", "Docker"}
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
	return enabled
}

// initModule creates the go.mod once for the shared render directory.
func initModule(dir, modCache string) error {
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		return nil
	}
	goMod := exec.Command("go", "mod", "init", "github.com/test/project")
	goMod.Dir = dir
	goMod.Env = append(os.Environ(),
		"GOMODCACHE="+modCache,
	)
	if err := goMod.Run(); err != nil {
		return err
	}
	return nil
}

// runCombo renders and validates a single combo using a shared directory.
func (cmd *TestRendersCmd) runCombo(dir, modCache, buildCache string, combo renderCombo) {
	comboID := combo.id
	cfg := ProjectConfig{
		ProjectName:  fmt.Sprintf("TestProject%s", comboID),
		GoModuleName: "github.com/test/project",
		UpdatedAt:    time.Now().Format(time.RFC3339),
		Dev:          DevConfig{},
		Components:   combo.components,
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
			console.Errorf("forj render failed")
			if stderr.Len() > 0 {
				fmt.Printf("%s\n", strings.TrimSpace(stderr.String()))
			}
			if stdout.Len() > 0 {
				fmt.Printf("%s\n", strings.TrimSpace(stdout.String()))
			}
			return err
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
			return fmt.Errorf("wire generate failed: %w (%s)", err, strings.TrimSpace(string(output)))
		}
		return nil
	}); err != nil {
		cmd.fail("wire generate failed", comboID, &cfg, err)
		return
	}

	if err := timer.Track("go_build", func() error {
		build := exec.Command("go", "build")
		build.Dir = dir
		build.Env = append(os.Environ(),
			"GOMODCACHE="+modCache,
			"GOCACHE="+buildCache,
		)
		if output, err := build.CombinedOutput(); err != nil {
			return fmt.Errorf("go build failed: %w (%s)", err, strings.TrimSpace(string(output)))
		}
		return nil
	}); err != nil {
		cmd.fail("go build failed", comboID, &cfg, err)
		return
	}

	timer.Report(fmt.Sprintf("combo %s (%s)", comboID, strings.Join(combo.enabled, ", ")))

	console.Successf("Passed")
}

func WriteYAML(path string, cfg ProjectConfig) error {
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (cmd *TestRendersCmd) fail(reason, comboID string, cfg *ProjectConfig, err error) {
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
