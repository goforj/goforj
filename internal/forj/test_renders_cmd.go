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
	const numCombos = 1 << 6

	cmd.logger.Info().Msgf("Testing %d component combinations", numCombos)

	// Warm cache
	cmd.runCombo(0)

	sem := make(chan struct{}, runtime.NumCPU())
	wg := sync.WaitGroup{}

	for i := 1; i < numCombos; i++ {
		sem <- struct{}{}
		wg.Add(1)
		go func(i int) {
			defer func() {
				<-sem
				wg.Done()
			}()
			cmd.runCombo(i)
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

func (cmd *TestRendersCmd) runCombo(i int) {
	modCache, buildCache := getCachePaths()

	comboID := fmt.Sprintf("%v", i)
	dir := fmt.Sprintf("%s/forj/test_project_%s", os.TempDir(), comboID)

	timer := newStepTimer()

	_ = os.RemoveAll(dir)
	_ = os.MkdirAll(dir, 0755)

	cfg := ProjectConfig{
		ProjectName:  fmt.Sprintf("TestProject%s", comboID),
		GoModuleName: "github.com/test/project",
		UpdatedAt:    time.Now().Format(time.RFC3339),
		Dev:          DevConfig{},
		Components: Components{
			CLI:           true,
			Docker:        true,
			WebAPI:        i&(1<<0) != 0,
			WebUI:         i&(1<<1) != 0,
			DatabaseMySQL: i&(1<<2) != 0,
			DatabasePostgres: i&(1<<3) != 0,
			Scheduler:     i&(1<<4) != 0,
			Jobs:          i&(1<<5) != 0,
		},
	}

	if cfg.Components.DatabasePostgres {
		cfg.Components.DatabaseMySQL = false
	}

	enabled := []string{"CLI", "Docker"}
	if cfg.Components.WebAPI {
		enabled = append(enabled, "WebAPI")
	}
	if cfg.Components.WebUI {
		enabled = append(enabled, "WebUI")
	}
	if cfg.Components.DatabaseMySQL {
		enabled = append(enabled, "Database (MySQL)")
	}
	if cfg.Components.DatabasePostgres {
		enabled = append(enabled, "Database (Postgres)")
	}
	if cfg.Components.Scheduler {
		enabled = append(enabled, "Scheduler")
	}
	if cfg.Components.Jobs {
		enabled = append(enabled, "Jobs")
	}

	cmd.logger.Info().
		Str("combo", comboID).
		Str("components", strings.Join(enabled, ", ")).
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
		build := exec.Command("go", "build", "./...")
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

	timer.Report(fmt.Sprintf("combo %s (%s)", comboID, strings.Join(enabled, ", ")), cmd.logger)

	cmd.logger.Info().
		Str("components", strings.Join(enabled, ", ")).
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
