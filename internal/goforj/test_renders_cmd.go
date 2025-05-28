package goforj

import (
	"fmt"
	"github.com/goforj/goforj/internal/logger"
	"gopkg.in/yaml.v3"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	const numCombos = 1 << 5 // 32 combinations of 5 booleans
	cmd.logger.Info().Msgf("Testing %d component combinations", numCombos)

	for i := 0; i < numCombos; i++ {
		comboID := fmt.Sprintf("%v", i)
		dir := fmt.Sprintf("/tmp/goforj/test_project_%s", comboID)

		_ = os.RemoveAll(dir)
		if err := os.MkdirAll(dir, 0755); err != nil {
			cmd.logger.Error().Err(err).Str("combo", comboID).Msg("Failed to create test directory")
			continue
		}

		cfg := ProjectConfig{
			ProjectName:  fmt.Sprintf("TestProject%s", comboID),
			GoModuleName: "github.com/test/project",
			UpdatedAt:    time.Now().Format(time.RFC3339),
			PreDev:       []DevTask{},
			DevWatches:   []DevWatch{},
			Components: Components{
				CLI:       true,          // always on
				Docker:    true,          // always on
				WebAPI:    i&(1<<0) != 0, // variable
				WebUI:     i&(1<<1) != 0, // variable
				Database:  i&(1<<2) != 0, // variable
				Scheduler: i&(1<<3) != 0, // variable
				Jobs:      i&(1<<4) != 0, // variable
			},
		}

		enabled := []string{}
		if cfg.Components.CLI {
			enabled = append(enabled, "CLI")
		}
		if cfg.Components.Docker {
			enabled = append(enabled, "Docker")
		}
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
			Str("components", fmt.Sprintf("%s", enabled)).
			Msgf("🔧 Rendering components [%s]", strings.Join(enabled, ", "))

		ymlPath := filepath.Join(dir, ".goforj.yml")
		if err := WriteYAML(ymlPath, cfg); err != nil {
			cmd.fail("failed to write .goforj.yml", comboID, &cfg, err)
			continue
		}

		goMod := exec.Command("go", "mod", "init", "github.com/test/project")
		goMod.Dir = dir
		_ = goMod.Run()

		render := exec.Command("goforj", "render")
		render.Dir = dir
		render.Stderr = os.Stderr
		if err := render.Run(); err != nil {
			cmd.fail("render failed", comboID, &cfg, err)
			continue
		}

		wire := exec.Command("wire")
		wire.Dir = filepath.Join(dir, "wire")
		wire.Stdout = os.Stdout
		wire.Stderr = os.Stderr
		if err := wire.Run(); err != nil {
			cmd.fail("wire generate failed", comboID, &cfg, err)
			continue
		}

		build := exec.Command("go", "build", "./...")
		build.Dir = dir
		build.Stdout = os.Stdout
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			cmd.fail("build failed", comboID, &cfg, err)
			continue
		}

		cmd.logger.Info().
			Str("components", fmt.Sprintf("%s", enabled)).
			Str("combo", comboID).
			Msg("✅ Passed")
	}

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
