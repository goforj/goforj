package forj

import (
	"fmt"
	"os"
	"strings"

	"github.com/goforj/execx"
)

// TestIntegrationCmd runs integration tests for the GoForj CLI.
type TestIntegrationCmd struct {
	// Silent suppresses shadow-printed commands.
	Silent bool `help:"Suppress command output" short:"s"`

	// Verbose enables verbose test output.
	Verbose bool `help:"Enable verbose test output" short:"v"`
}

// NewTestIntegrationCmd creates a new TestIntegrationCmd instance.
func NewTestIntegrationCmd() *TestIntegrationCmd {
	return &TestIntegrationCmd{}
}

// Run executes integration tests for the model generator.
func (cmd *TestIntegrationCmd) Run() error {
	modCache, buildCache := getCachePaths()

	if !cmd.Silent {
		fmt.Printf("%s Running test:integration\n", actionMark())
	}

	args := []string{"go", "test", "./internal/modelgen", "-tags=integration,mysql"}
	if cmd.Verbose {
		args = append(args, "-v")
	}
	if err := runIntegrationStep(cmd.Silent, cmd.Verbose, "integration", ".", modCache, buildCache, args); err != nil {
		return err
	}

	if !cmd.Silent {
		fmt.Printf("%s Integration tests completed\n", successMark())
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
					return fmt.Sprintf("%s %s", actionMark(), ev.Command)
				case execx.ShadowAfter:
					return fmt.Sprintf("%s %s (%s)", infoMark(), ev.Command, ev.Duration)
				default:
					return fmt.Sprintf("%s %s", infoMark(), ev.Command)
				}
			}),
		)
	}

	res, err := cmd.Run()
	if err != nil || !res.OK() {
		if !silent {
			fmt.Printf("%s %s failed\n", errorMark(), name)
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
