package forj

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goforj/execx"
	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

// TestRenderCmd renders a full project to a temp dir and verifies build + tests.
type TestRenderCmd struct {
	logger *logger.AppLogger

	// Silent suppresses shadow-printed commands.
	Silent bool `help:"Suppress command output" short:"s"`

	// Keep preserves the temp directory after completion.
	Keep bool `help:"Keep the temp directory after completion" short:"k"`
}

func (*TestRenderCmd) Signature() string {
	return `name:"test:render" help:"Render full project and run build/tests" hidden:""`
}

// NewTestRenderCmd creates a new TestRenderCmd instance.
func NewTestRenderCmd(logger *logger.AppLogger) *TestRenderCmd {
	return &TestRenderCmd{logger: logger}
}

// Run executes the render, build, and test steps against a temp project.
func (cmd *TestRenderCmd) Run() error {
	modCache, buildCache := testkit.GoCachePaths()

	dir, err := os.MkdirTemp("", "forj_render_")
	if err != nil {
		return err
	}
	if !cmd.Keep {
		defer os.RemoveAll(dir)
	}

	cfg := project.Config{
		ProjectName:  "Test Render",
		GoModuleName: "github.com/test/project",
		Dev: project.DevConfig{
			Pre:               []project.DevTask{},
			Down:              []project.DevTask{},
			DownOnExit:        false,
			SoundOnWatchError: false,
			Watches:           []project.DevWatch{},
		},
		App: project.AppConfig{
			DefaultTarget: project.DefaultAppTargetName,
			Targets: []project.AppTarget{
				project.DefaultAppTarget(),
				project.DefaultNamedAppTarget("customer-portal"),
			},
		},
		Render: project.RenderConfig{
			Components: project.Components{
				CLI:           true,
				Docker:        true,
				WebAPI:        true,
				WebUI:         true,
				DatabaseMySQL: true,
				Scheduler:     true,
				Jobs:          true,
			},
		},
	}

	ymlPath := filepath.Join(dir, ".goforj.yml")
	if err := WriteYAML(ymlPath, cfg); err != nil {
		return err
	}

	if !cmd.Silent {
		console.Actionf("Running test:render")
	}
	forjExec, cleanup, err := repoForjExecutable(modCache, buildCache)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := runStep(cmd.logger, cmd.Silent, "render", dir, modCache, buildCache, []string{forjExec, "render"}); err != nil {
		return err
	}
	if err := runStep(cmd.logger, cmd.Silent, "build", dir, modCache, buildCache, []string{"go", "build", "./..."}); err != nil {
		return err
	}
	if err := runStep(cmd.logger, cmd.Silent, "build customer-portal", dir, modCache, buildCache, []string{forjExec, "customer-portal", "build"}); err != nil {
		return err
	}
	if err := runStep(cmd.logger, cmd.Silent, "test", dir, modCache, buildCache, []string{"go", "test", "./..."}); err != nil {
		return err
	}

	if !cmd.Silent {
		console.Successf("Render/build/test completed")
	}
	if !cmd.Silent {
		cmd.logger.Info().Str("path", dir).Msg("Render/build/test completed")
	}
	return nil
}

// runStep executes a command with cache isolation and logs failures.
func runStep(log *logger.AppLogger, silent bool, name, dir, modCache, buildCache string, args []string) error {
	cmd := execx.Command(args[0], args[1:]...).
		Dir(dir).
		EnvAppend(map[string]string{
			"GOMODCACHE": modCache,
			"GOCACHE":    buildCache,
		})

	if !silent {
		cmd = cmd.ShadowPrint(
			execx.WithFormatter(func(ev execx.ShadowEvent) string {
				switch ev.Phase {
				case execx.ShadowBefore:
					return fmt.Sprintf("%s %s", console.ActionMark(), ev.Command)
				case execx.ShadowAfter:
					return fmt.Sprintf("%s %s (%s)", console.InfoMark(), ev.Command, ev.Duration)
				default:
					return fmt.Sprintf("%s %s", console.InfoMark(), ev.Command)
				}
			}),
		)
	}

	res, err := cmd.Run()
	if err != nil || !res.OK() {
		if !silent {
			console.Errorf("%s failed", name)
		}
		if err == nil {
			err = fmt.Errorf("command failed with exit code %d", res.ExitCode)
		}
		errMsg := strings.TrimSpace(res.Stderr)
		if errMsg == "" {
			errMsg = err.Error()
		}
		if errMsg == "" {
			errMsg = "command failed"
		}
		log.Error().
			Str("step", name).
			Str("stdout", strings.TrimSpace(res.Stdout)).
			Err(fmt.Errorf("%s", errMsg)).
			Msg("Step failed")
		return err
	}
	return nil
}
