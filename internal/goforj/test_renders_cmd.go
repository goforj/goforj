package goforj

import (
	"fmt"
	"github.com/goforj/goforj/internal/logger"
	"gopkg.in/yaml.v3"
	"os"
	"os/exec"
	"path/filepath"
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
		comboID := fmt.Sprintf("%05b", i)
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

		ymlPath := filepath.Join(dir, ".goforj.yml")
		cmd.logger.Info().
			Any("cfg", cfg).
			Str("ymlPath", ymlPath).
			Str("combo", comboID).
			Msg("Yaml")
		if err := WriteYAML(ymlPath, cfg); err != nil {
			cmd.logger.Error().Err(err).Str("combo", comboID).Msg("Failed to write .goforj.yml")
			continue
		}

		// Optional: init go.mod to satisfy any render logic expecting it
		goMod := exec.Command("go", "mod", "init", "github.com/test/project")
		goMod.Dir = dir
		_ = goMod.Run() // silent fail ok

		// Run `goforj render`
		render := exec.Command("goforj", "render")
		render.Dir = dir
		render.Stdout = os.Stdout
		render.Stderr = os.Stderr
		if err := render.Run(); err != nil {
			cmd.logger.Error().Err(err).Str("combo", comboID).Msg("Render failed")
			continue
		}

		// Run `go build ./...`
		build := exec.Command("go", "build", "./...")
		build.Dir = dir
		build.Stdout = os.Stdout
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			cmd.logger.Error().Err(err).Str("combo", comboID).Msg("Build failed")
			continue
		}

		cmd.logger.Info().Str("combo", comboID).Msg("✅ Passed")
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
