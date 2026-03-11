package forj

import (
	"fmt"
	"os"
	"strings"

	"github.com/goforj/env/v2"
	"github.com/goforj/execx"
	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/str"
)

// TestIntegrationCmd runs integration tests for the GoForj CLI.
type TestIntegrationCmd struct {
	// Silent suppresses shadow-printed commands.
	Silent bool `help:"Suppress command output" short:"s"`

	// Verbose enables verbose test output.
	Verbose bool `help:"Enable verbose test output" short:"v"`
}

func (*TestIntegrationCmd) Signature() string {
	return `name:"test:integration" help:"Run integration tests" hidden:""`
}

// NewTestIntegrationCmd creates a new TestIntegrationCmd instance.
func NewTestIntegrationCmd() *TestIntegrationCmd {
	return &TestIntegrationCmd{}
}

// Run executes integration tests for the model generator.
func (cmd *TestIntegrationCmd) Run() error {
	modCache, buildCache := getCachePaths()

	if !cmd.Silent {
		console.Actionf("Running test:integration")
	}

	_ = os.Setenv("APP_ENV", "local")
	if err := env.Load(); err != nil {
		return err
	}
	driver := str.Of(env.Get("DB_DRIVER", "")).TrimSpace().ToLower().String()
	tag := "mysql"
	switch driver {
	case "postgres", "postgresql":
		tag = "postgres"
	case "sqlite", "sqlite3":
		tag = "sqlite"
	case "mysql", "mariadb", "":
		tag = "mysql"
	}

	steps := []struct {
		name string
		args []string
	}{
		{
			name: "modelgen",
			args: []string{"go", "test", "./internal/modelgen", "-tags=integration," + tag},
		},
		{
			name: "migrations",
			args: []string{"go", "test", "./migrations", "-tags=integration," + tag},
		},
		{
			name: "dbconns",
			args: []string{"go", "test", "./internal/dbconns", "-tags=integration," + tag},
		},
	}

	for _, step := range steps {
		args := step.args
		if cmd.Verbose {
			args = append(args, "-v")
		}
		if !cmd.Silent {
			console.Actionf("Running %s integration tests", step.name)
		}
		if err := runIntegrationStep(cmd.Silent, cmd.Verbose, step.name, ".", modCache, buildCache, args); err != nil {
			return err
		}
	}

	if !cmd.Silent {
		console.Successf("Integration tests completed")
	}
	return nil
}

// runIntegrationStep executes a command with cache isolation and semantic output.
func runIntegrationStep(silent bool, verbose bool, name, dir, modCache, buildCache string, args []string) error {
	cmd := execx.Command(args[0], args[1:]...).
		Dir(dir).
		EnvAppend(map[string]string{
			"GOMODCACHE": modCache,
			"GOCACHE":    buildCache,
		})

	if verbose && !silent {
		cmd = cmd.StdoutWriter(os.Stdout).StderrWriter(os.Stderr)
	}

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
		return fmt.Errorf("%s", errMsg)
	}
	return nil
}
