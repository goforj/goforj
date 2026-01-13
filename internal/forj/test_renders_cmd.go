package forj

import (
	"errors"
	"fmt"
	"github.com/goforj/goforj/internal/logger"
	"gopkg.in/yaml.v3"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type TestRendersCmd struct {
	logger     *logger.AppLogger
	wireMutex  sync.Mutex
	modInitMux sync.Mutex

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

func (t *stepTimer) Report(label string, log *logger.AppLogger) {
	total := time.Since(t.start)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n⏱ Timing breakdown for %s\n", label))
	b.WriteString("---------------------------------------------------\n")

	for k, v := range t.parts {
		b.WriteString(fmt.Sprintf("%-15s %s\n", k+":", v))
	}

	b.WriteString("---------------------------------------------------\n")
	b.WriteString(fmt.Sprintf("%-15s %s\n\n", "total:", total))

	log.Info().Msg(b.String())
}

func NewTestRendersCmd(logger *logger.AppLogger) *TestRendersCmd {
	return &TestRendersCmd{logger: logger}
}

func (cmd *TestRendersCmd) Run() error {
	combos := buildRenderCombos(cmd.Full)
	cmd.logger.Info().Msgf("Testing %d component combinations", len(combos))
	if len(combos) == 0 {
		return nil
	}

	// Warm cache
	cmd.runCombo(combos[0])

	sem := make(chan struct{}, runtime.NumCPU())
	wg := sync.WaitGroup{}

	for i := 1; i < len(combos); i++ {
		sem <- struct{}{}
		wg.Add(1)
		go func(i int) {
			defer func() {
				<-sem
				wg.Done()
			}()
			cmd.runCombo(combos[i])
		}(i)
	}

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

type renderCombo struct {
	id         string
	components Components
	enabled    []string
}

type featureCombo struct {
	webAPI    bool
	webUI     bool
	scheduler bool
	jobs      bool
}

func buildRenderCombos(full bool) []renderCombo {
	if full {
		return buildFullRenderCombos()
	}
	return buildCuratedRenderCombos()
}

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
			id:         fmt.Sprintf("base_%t%t%t%t", feature.webAPI, feature.webUI, feature.scheduler, feature.jobs),
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

func (cmd *TestRendersCmd) runCombo(combo renderCombo) {
	modCache, buildCache := getCachePaths()

	comboID := combo.id
	dir := fmt.Sprintf("%s/forj/test_project_%s", os.TempDir(), comboID)

	timer := newStepTimer()

	_ = os.RemoveAll(dir)
	_ = os.MkdirAll(dir, 0755)

	cfg := ProjectConfig{
		ProjectName:  fmt.Sprintf("TestProject%s", comboID),
		GoModuleName: "github.com/test/project",
		UpdatedAt:    time.Now().Format(time.RFC3339),
		Dev:          DevConfig{},
		Components:   combo.components,
	}

	cmd.logger.Info().
		Str("combo", comboID).
		Str("components", strings.Join(combo.enabled, ", ")).
		Msg("🔧 Rendering components")

	ymlPath := filepath.Join(dir, ".goforj.yml")

	if err := timer.Track("write_yaml", func() error {
		return WriteYAML(ymlPath, cfg)
	}); err != nil {
		cmd.fail("failed to write config", comboID, &cfg, err)
		return
	}

	if err := timer.Track("mod_init", func() error {
		cmd.modInitMux.Lock()
		defer cmd.modInitMux.Unlock()

		goMod := exec.Command("go", "mod", "init", "github.com/test/project")
		goMod.Dir = dir
		goMod.Env = append(os.Environ(),
			"GOMODCACHE="+modCache,
		)
		return goMod.Run()
	}); err != nil {
		cmd.fail("mod init failed", comboID, &cfg, err)
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
			cmd.logger.Error().
				Str("stdout", stdout.String()).
				Err(errors.New(stderr.String())).
				Msg("🔴 forj render failed")
			return err
		}
		return nil
	}); err != nil {
		cmd.fail("render failed", comboID, &cfg, err)
		return
	}

	if err := timer.Track("wire_gen", func() error {
		wire := exec.Command("wire")
		wire.Dir = filepath.Join(dir, "wire")
		wire.Env = append(os.Environ(),
			"GOMODCACHE="+modCache,
			"GOCACHE="+buildCache,
		)
		return wire.Run()
	}); err != nil {
		cmd.fail("wire failed", comboID, &cfg, err)
		return
	}

	if err := timer.Track("go_build", func() error {
		build := exec.Command("go", "build")
		build.Dir = dir
		build.Env = append(os.Environ(),
			"GOMODCACHE="+modCache,
			"GOCACHE="+buildCache,
		)
		return build.Run()
	}); err != nil {
		cmd.fail("go build failed", comboID, &cfg, err)
		return
	}

	timer.Report(fmt.Sprintf("combo %s (%s)", comboID, strings.Join(combo.enabled, ", ")), cmd.logger)

	cmd.logger.Info().
		Str("components", strings.Join(combo.enabled, ", ")).
		Str("combo", comboID).
		Msg("✅ Passed")
}

func WriteYAML(path string, cfg ProjectConfig) error {
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (cmd *TestRendersCmd) fail(reason, comboID string, cfg *ProjectConfig, err error) {
	event := cmd.logger.Error().Err(err).Str("combo", comboID).Str("reason", reason)

	if cfg != nil {
		if yamlDump, yerr := yaml.Marshal(cfg); yerr == nil {
			event.Str("config_yaml", string(yamlDump))
		}
	}

	event.Msg("❌ Failure")
	os.Exit(1)
}
