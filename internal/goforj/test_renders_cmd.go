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
	logger     *logger.AppLogger
	wireMutex  sync.Mutex
	modInitMux sync.Mutex
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

	// Warm up cache by running first combo serially
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
	dir := fmt.Sprintf("/tmp/goforj/test_project_%s", comboID)

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
		Msgf("🔧 Rendering components")

	ymlPath := filepath.Join(dir, ".goforj.yml")
	if err := WriteYAML(ymlPath, cfg); err != nil {
		cmd.fail("failed to write .goforj.yml", comboID, &cfg, err)
		return
	}

	cmd.modInitMux.Lock()
	goMod := exec.Command("go", "mod", "init", "github.com/test/project")
	goMod.Dir = dir
	goMod.Env = append(os.Environ(), "GOMODCACHE=/tmp/goforj/.cache/mod", "GOCACHE=/tmp/goforj/.cache/build")
	_ = goMod.Run()
	cmd.modInitMux.Unlock()

	render := exec.Command("goforj", "render")
	render.Dir = dir
	render.Env = append(os.Environ(), "GOMODCACHE=/tmp/goforj/.cache/mod", "GOCACHE=/tmp/goforj/.cache/build")
	render.Stderr = os.Stderr
	if err := render.Run(); err != nil {
		cmd.fail("render failed", comboID, &cfg, err)
		return
	}

	cmd.wireMutex.Lock()
	wire := exec.Command("wire")
	wire.Dir = filepath.Join(dir, "wire")
	wire.Stdout = os.Stdout
	wire.Stderr = os.Stderr
	wire.Env = append(os.Environ(), "GOMODCACHE=/tmp/goforj/.cache/mod", "GOCACHE=/tmp/goforj/.cache/build")
	if err := wire.Run(); err != nil {
		cmd.wireMutex.Unlock()
		cmd.fail("wire generate failed", comboID, &cfg, err)
		return
	}
	cmd.wireMutex.Unlock()

	build := exec.Command("go", "build", "./...")
	build.Dir = dir
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	build.Env = append(os.Environ(), "GOMODCACHE=/tmp/goforj/.cache/mod", "GOCACHE=/tmp/goforj/.cache/build")
	if err := build.Run(); err != nil {
		cmd.fail("build failed", comboID, &cfg, err)
		return
	}

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
		}
	}
	event.Msg("❌ Failure")
	os.Exit(1)
}
