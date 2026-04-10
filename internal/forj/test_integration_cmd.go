package forj

import (
	"fmt"
	"os"
	"strings"

	"github.com/goforj/execx"
	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/testkit"
)

// TestIntegrationCmd runs integration tests for the GoForj CLI.
type TestIntegrationCmd struct {
	logger *logger.AppLogger

	// Suite chooses which integration pipeline to run.
	Suite string `arg:"" optional:"" default:"all" enum:"framework,rendered,all" help:"Integration suite to run"`

	// Target narrows the package target within the selected suite.
	Target string `help:"Integration target to run" default:"all" enum:"all,auth,modelgen,migrations,database"`

	// Variant selects the DB variant. Defaults to all rendered variants so no-arg runs exercise the full matrix.
	Variant string `help:"Database variant selection" default:"all" enum:"sqlite,mysql,postgres,all"`

	// Silent suppresses shadow-printed commands.
	Silent bool `help:"Suppress command output" short:"s"`

	// Verbose enables verbose test output.
	Verbose bool `help:"Enable verbose test output" short:"v"`
}

func (*TestIntegrationCmd) Signature() string {
	return `name:"test:integration" help:"Run integration tests" hidden:""`
}

// NewTestIntegrationCmd creates a new TestIntegrationCmd instance.
func NewTestIntegrationCmd(logger *logger.AppLogger) *TestIntegrationCmd {
	return &TestIntegrationCmd{logger: logger}
}

// Run executes integration tests for the model generator.
func (cmd *TestIntegrationCmd) Run() error {
	modCache, buildCache := testkit.GoCachePaths()
	suite := strings.TrimSpace(strings.ToLower(cmd.Suite))
	target := strings.TrimSpace(strings.ToLower(cmd.Target))
	variant := strings.TrimSpace(strings.ToLower(cmd.Variant))

	if !cmd.Silent {
		printIntegrationSection(fmt.Sprintf("Integration Suite: %s", suite))
	}

	switch suite {
	case "framework":
		return cmd.runFrameworkSuite(modCache, buildCache, target, variant)
	case "rendered":
		return cmd.runRenderedSuite(target, variant)
	case "all":
		if err := cmd.runFrameworkSuite(modCache, buildCache, target, variant); err != nil {
			return err
		}
		return cmd.runRenderedSuite(target, variant)
	default:
		return fmt.Errorf("unknown integration suite %q", suite)
	}
}

func (cmd *TestIntegrationCmd) runFrameworkSuite(modCache, buildCache string, target, variant string) error {
	if target != "" && target != "all" {
		return fmt.Errorf("framework integration does not support target %q; use rendered targets for generated app package tests", target)
	}
	forjExec, cleanup, err := repoForjExecutable(modCache, buildCache)
	if err != nil {
		return err
	}
	defer cleanup()
	frameworkEnv := map[string]string{
		"FORJ_INTEGRATION_FORJ_PATH": forjExec,
	}
	redisTeardown, err := startRedisTestcontainer(cmd.Silent, frameworkEnv)
	if err != nil {
		return err
	}
	if redisTeardown != nil {
		defer redisTeardown()
	}
	if !cmd.Silent {
		printIntegrationSubsection("Framework integration preflight")
	}
	preflightArgs := []string{"go", "test", "-run", "^$", "-tags=integration", "./internal/forj", "-count=1"}
	if err := runIntegrationStepWithEnv(cmd.Silent, cmd.Verbose, "framework preflight", ".", modCache, buildCache, frameworkEnv, preflightArgs); err != nil {
		if !cmd.Silent {
			console.Warnf("framework preflight failed, attempting integration-tagged tidy")
		}
		tidyEnv := map[string]string{
			"GOFLAGS":                    "-tags=integration",
			"FORJ_INTEGRATION_FORJ_PATH": forjExec,
		}
		if err := runIntegrationStepWithEnv(cmd.Silent, cmd.Verbose, "framework tidy", ".", modCache, buildCache, tidyEnv, []string{"go", "mod", "tidy"}); err != nil {
			return err
		}
		if err := runIntegrationStepWithEnv(cmd.Silent, cmd.Verbose, "framework preflight", ".", modCache, buildCache, frameworkEnv, preflightArgs); err != nil {
			return err
		}
	}
	args := []string{"go", "test", "-tags=integration", "./internal/forj", "-count=1"}
	if cmd.Verbose {
		args = append(args, "-v")
	}
	if !cmd.Silent {
		printIntegrationSubsection("Framework integration tests")
	}
	if err := runIntegrationStepWithEnv(cmd.Silent, cmd.Verbose, "framework", ".", modCache, buildCache, frameworkEnv, args); err != nil {
		return err
	}
	return nil
}

func (cmd *TestIntegrationCmd) runRenderedSuite(target, variant string) error {
	var variants []string
	switch variant {
	case "", "all":
		variants = []string{"sqlite", "mysql", "postgres"}
	case "sqlite", "mysql", "postgres":
		variants = []string{variant}
	default:
		return fmt.Errorf("unsupported rendered integration variant %q", variant)
	}

	renderedCmd := NewRenderedIntegrationRunner(cmd.logger)
	renderedCmd.Target = target
	renderedCmd.Silent = cmd.Silent
	renderedCmd.Verbose = cmd.Verbose
	for _, dbVariant := range variants {
		renderedCmd.Variant = dbVariant
		if !cmd.Silent {
			printIntegrationSubsection(fmt.Sprintf("Rendered integration variant: %s", dbVariant))
		}
		if err := renderedCmd.Run(); err != nil {
			return err
		}
	}

	if !cmd.Silent {
		console.Successf("Integration tests completed")
	}
	return nil
}

func ensureGoTestVerbose(args []string) []string {
	if len(args) < 2 || args[0] != "go" || args[1] != "test" {
		return args
	}
	for _, arg := range args[2:] {
		if arg == "-v" {
			return args
		}
	}
	updated := append([]string{}, args[:2]...)
	updated = append(updated, "-v")
	updated = append(updated, args[2:]...)
	return updated
}

func runIntegrationStepWithEnv(silent bool, verbose bool, name, dir, modCache, buildCache string, extraEnv map[string]string, args []string) error {
	args = ensureGoTestVerbose(args)
	cmd := execx.Command(args[0], args[1:]...).
		Dir(dir).
		EnvAppend(map[string]string{
			"GOMODCACHE": modCache,
			"GOCACHE":    buildCache,
			"GOFLAGS":    "",
			"GOWORK":     "off",
		}).
		EnvAppend(extraEnv)

	if !silent {
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
		stderr := strings.TrimSpace(res.Stderr)
		stdout := strings.TrimSpace(res.Stdout)
		rawOutput := strings.TrimSpace(strings.Join([]string{stdout, stderr}, "\n"))
		switch {
		case rawOutput != "":
			return fmt.Errorf("%s", rawOutput)
		case err != nil:
			return err
		default:
			return fmt.Errorf("command failed")
		}
	}
	return nil
}
