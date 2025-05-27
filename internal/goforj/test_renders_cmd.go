package goforj

import (
	"fmt"
	"github.com/goforj/goforj/internal/logger"
	"gopkg.in/yaml.v3"
	"os"
	"os/exec"
	"path/filepath"
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

	for i := 0; i < numCombos; i++ {
		dir := fmt.Sprintf("/tmp/goforj/test_project_%05b", i)
		_ = os.RemoveAll(dir) // Clean previous run
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}

		projectConfig := ProjectConfig{
			ProjectName:  fmt.Sprintf("TestProject%05b", i),
			GoModuleName: "github.com/test/project",
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
		if err := WriteYAML(ymlPath, projectConfig); err != nil {
			fmt.Printf("❌ Failed to write config for combo %05b: %v\n", i, err)
			continue
		}

		// Run `goforj render`
		render := exec.Command("goforj", "render")
		render.Dir = dir
		render.Stdout = os.Stdout
		render.Stderr = os.Stderr
		if err := render.Run(); err != nil {
			fmt.Printf("❌ Render failed for combo %05b\n", i)
			continue
		}

		// Run `go build ./...`
		build := exec.Command("go", "build", "./...")
		build.Dir = dir
		build.Stdout = os.Stdout
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			fmt.Printf("❌ Build failed for combo %05b\n", i)
			continue
		}

		fmt.Printf("✅ Passed combo %05b\n", i)
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
