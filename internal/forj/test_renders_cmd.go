package forj

import (
	"errors"
	"fmt"
	"github.com/goforj/goforj/internal/logger"
	"gopkg.in/yaml.v3"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type TestRendersCmd struct {
	logger     *logger.AppLogger
	wireMutex  sync.Mutex
	modInitMux sync.Mutex
}

// Simple lightweight timing helper
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

// NewTestRendersCmd creates a new command to test all combinations of project configurations.
func NewTestRendersCmd(logger *logger.AppLogger) *TestRendersCmd {
	return &TestRendersCmd{
		logger: logger,
	}
}

func (cmd *TestRendersCmd) Run() error {
	const numCombos = 1 << 5 // 32 combinations of 5 booleans
	const maxWorkers = 4

	cmd.logger.Info().Msgf("Testing %d component combinations", numCombos)

	// Warm up module cache by running one serially
	cmd.runCombo(0)

	sem := make(chan struct{}, maxWorkers)
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

func (cmd *TestRendersCmd) runCombo(i int) {
	comboID := fmt.Sprintf("%v", i)
	dir := fmt.Sprintf("/tmp/forj/test_project_%s", comboID)
	timer := newStepTimer()

	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		cmd.logger.Error().Err(err).Str("combo", comboID).Msg("Failed to create test directory")
		return
	}

	cfg := ProjectConfig{
		ProjectName:  fmt.Sprintf("TestProject%s", comboID),
		GoModuleName: "github.com/test/project",
		UpdatedAt:    time.Now().Format(time.RFC3339),
		PreDev:       []DevTask{},
		DevWatches:   []DevWatch{},
		Components: Components{
			CLI:       true,
			Docker:    true,
			WebAPI:    i&(1<<0) != 0,
			WebUI:     i&(1<<1) != 0,
			Database:  i&(1<<2) != 0,
			Scheduler: i&(1<<3) != 0,
			Jobs:      i&(1<<4) != 0,
		},
	}

	// Enabled component string list (for reporting)
	enabled := []string{"CLI", "Docker"}
	if cfg.Components.WebAPI {
		enabled = append(enabled, "WebAPI")
	}
	if cfg.Components.WebUI {
		enabled = append(enabled, "WebUI")
	}
	if cfg.Components.Database {
		enabled = append(enabled, "Database")
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

	// --- write_yaml ---
	if err := timer.Track("write_yaml", func() error {
		return WriteYAML(ymlPath, cfg)
	}); err != nil {
		cmd.fail("failed to write .goforj.yml", comboID, &cfg, err)
		return
	}

	// --- mod_init ---
	if err := timer.Track("mod_init", func() error {
		cmd.modInitMux.Lock()
		defer cmd.modInitMux.Unlock()

		goMod := exec.Command("go", "mod", "init", "github.com/test/project")
		goMod.Dir = dir
		goMod.Env = os.Environ()
		return goMod.Run()
	}); err != nil {
		cmd.fail("mod init failed", comboID, &cfg, err)
		return
	}

	// --- forj render ---
	if err := timer.Track("forj_render", func() error {
		render := exec.Command("forj", "render")
		render.Dir = dir
		render.Env = os.Environ()

		var stdout, stderr strings.Builder
		render.Stdout = &stdout
		render.Stderr = &stderr

		if err := render.Run(); err != nil {
			cmd.logger.Error().
				Str("stdout", stdout.String()).
				Err(errors.New(stderr.String())).
				Msg("🔴 forj render failed output")
			return err
		}
		return nil
	}); err != nil {
		cmd.fail("render failed", comboID, &cfg, err)
		return
	}

	// --- wire_gen ---
	if err := timer.Track("wire_gen", func() error {
		cmd.wireMutex.Lock()
		defer cmd.wireMutex.Unlock()

		wire := exec.Command("wire")
		wire.Dir = filepath.Join(dir, "wire")
		wire.Env = os.Environ()

		return wire.Run()
	}); err != nil {
		cmd.fail("wire generate failed", comboID, &cfg, err)
		return
	}

	// --- go_build ---
	if err := timer.Track("go_build", func() error {
		build := exec.Command("go", "build", "./...")
		build.Dir = dir
		build.Env = os.Environ()
		return build.Run()
	}); err != nil {
		cmd.fail("build failed", comboID, &cfg, err)
		return
	}

	// Print timing breakdown
	timer.Report(fmt.Sprintf("combo %s (%s)", comboID, strings.Join(enabled, ", ")), cmd.logger)

	cmd.logger.Info().
		Str("components", strings.Join(enabled, ", ")).
		Str("combo", comboID).
		Msg("✅ Passed")
}

// WriteYAML writes the ProjectConfig to the given path in YAML format.
func WriteYAML(path string, cfg ProjectConfig) error {
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// fail logs a failure with the given reason, combo ID, config, and error.
func (cmd *TestRendersCmd) fail(reason, comboID string, cfg *ProjectConfig, err error) {
	event := cmd.logger.Error().Err(err).Str("combo", comboID).Str("reason", reason)

	if cfg != nil {
		yamlDump, yerr := yaml.Marshal(cfg)
		if yerr == nil {
			event.Str("config_yaml", string(yamlDump))
		} else {
			event.Err(yerr).Msg("Failed to marshal config to YAML")
		}
	}
	event.Msg("❌ Failure")
	os.Exit(1)
}
