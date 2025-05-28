package goforj

import (
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
	logger *logger.AppLogger
}

// NewTestRendersCmd creates a new command to test all combinations of project configurations.
func NewTestRendersCmd(logger *logger.AppLogger) *TestRendersCmd {
	return &TestRendersCmd{
		logger: logger,
	}
}

func (cmd *TestRendersCmd) Run() error {
	const numCombos = 1 << 5 // 32 combinations
	cmd.logger.Info().Msgf("Testing %d component combinations", numCombos)

	var wg sync.WaitGroup
	errCh := make(chan error, numCombos)

	// Run combo 0 serially to warm up tooling/deps
	if err := cmd.testCombo(0); err != nil {
		cmd.logger.Error().Err(err).Msg("❌ Warm-up combo 0 failed")
		return err
	}

	// Parallelize remaining combos
	for i := 1; i < numCombos; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := cmd.testCombo(i); err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	if len(errCh) > 0 {
		for err := range errCh {
			cmd.logger.Error().Err(err).Msg("❌ Parallel test failure")
		}
		return fmt.Errorf("some combos failed")
	}

	return nil
}

func (cmd *TestRendersCmd) testCombo(i int) error {
	comboID := fmt.Sprintf("%v", i)
	dir := fmt.Sprintf("/tmp/goforj/test_project_%s", comboID)

	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
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
	if err := WriteYAML(ymlPath, cfg); err != nil {
		return fmt.Errorf("write .goforj.yml: %w", err)
	}

	goMod := exec.Command("go", "mod", "init", "github.com/test/project")
	goMod.Dir = dir
	_ = goMod.Run()

	render := exec.Command("goforj", "render")
	render.Dir = dir
	render.Stderr = os.Stderr
	if err := render.Run(); err != nil {
		return fmt.Errorf("render failed: %w", err)
	}

	wire := exec.Command("wire")
	wire.Dir = filepath.Join(dir, "wire")
	wire.Stdout = os.Stdout
	wire.Stderr = os.Stderr
	if err := wire.Run(); err != nil {
		return fmt.Errorf("wire generate failed: %w", err)
	}

	build := exec.Command("go", "build", "./...")
	build.Dir = dir
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	cmd.logger.Info().
		Str("combo", comboID).
		Str("components", strings.Join(enabled, ", ")).
		Msg("✅ Passed")

	return nil
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
		}
	}

	event.Msg("❌ Failure")
	os.Exit(1)
}
