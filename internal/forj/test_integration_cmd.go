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
	"github.com/goforj/goforj/version"
)

// TestIntegrationCmd runs integration tests for the GoForj CLI.
type TestIntegrationCmd struct {
	logger *logger.AppLogger

	// Suite chooses which integration pipeline to run.
	Suite string `arg:"" optional:"" default:"all" enum:"framework,rendered,all" help:"Integration suite to run"`

	// Target narrows the package target within the selected suite.
	Target string `help:"Integration target to run" default:"all" enum:"all,auth,makecmd,modelgen,migrations,database"`

	// Variant selects the DB variant. Defaults to all rendered variants so no-arg runs exercise the full matrix.
	Variant string `help:"Database variant selection" default:"all" enum:"sqlite,mysql,postgres,all"`

	// Silent suppresses shadow-printed commands.
	Silent bool `help:"Suppress command output" short:"s"`

	// Verbose enables verbose test output.
	Verbose bool `help:"Enable verbose test output" short:"v"`
}

type integrationStep struct {
	name string
	args []string
}

type dbIntegrationVariantSpec struct {
	applyConfig func(*project.Components)
	testEnv     map[string]string
}

var dbIntegrationVariantSpecs = map[string]dbIntegrationVariantSpec{
	"mysql": {
		applyConfig: func(components *project.Components) {
			components.DatabaseMySQL = true
		},
		testEnv: map[string]string{
			"DB_DRIVER":               "mysql",
			"DB_HOST":                 "127.0.0.1",
			"DB_PORT":                 "3306",
			"DB_DATABASE":             "db",
			"DB_USERNAME":             "user",
			"DB_PASSWORD":             "password",
			"DB_HOST_INTEGRATION":     "127.0.0.1",
			"DB_PORT_INTEGRATION":     "3306",
			"DB_DATABASE_INTEGRATION": "db",
			"DB_USERNAME_INTEGRATION": "user",
			"DB_PASSWORD_INTEGRATION": "password",
		},
	},
	"postgres": {
		applyConfig: func(components *project.Components) {
			components.DatabasePostgres = true
		},
		testEnv: map[string]string{
			"DB_DRIVER":               "postgres",
			"DB_HOST":                 "127.0.0.1",
			"DB_PORT":                 "5432",
			"DB_DATABASE":             "app",
			"DB_USERNAME":             "postgres",
			"DB_PASSWORD":             "postgres",
			"DB_HOST_INTEGRATION":     "127.0.0.1",
			"DB_PORT_INTEGRATION":     "5432",
			"DB_DATABASE_INTEGRATION": "app",
			"DB_USERNAME_INTEGRATION": "postgres",
			"DB_PASSWORD_INTEGRATION": "postgres",
		},
	},
	"sqlite": {
		applyConfig: func(components *project.Components) {
			components.DatabaseSQLite = true
		},
		testEnv: map[string]string{
			"DB_DRIVER":   "sqlite",
			"DB_DATABASE": "./_data/sqlite/app.db",
		},
	},
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
		testkit.PrintSection(fmt.Sprintf("Integration Suite: %s", suite))
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
	redisTeardown, err := testkit.StartRedisTestcontainer(testkit.ConsoleLogf(cmd.Silent), frameworkEnv)
	if err != nil {
		return err
	}
	if redisTeardown != nil {
		defer redisTeardown()
	}
	if !cmd.Silent {
		testkit.PrintSubsection("Framework integration preflight")
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
		testkit.PrintSubsection("Framework integration tests")
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

	for _, dbVariant := range variants {
		if !cmd.Silent {
			testkit.PrintSubsection(fmt.Sprintf("Rendered integration variant: %s", dbVariant))
		}
		if err := cmd.runRenderedVariant(dbVariant, target); err != nil {
			return err
		}
	}

	if !cmd.Silent {
		console.Successf("Integration tests completed")
	}
	return nil
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func (cmd *TestIntegrationCmd) writeRenderedIntegrationConfig(dir, variant string, spec dbIntegrationVariantSpec) error {
	cfg := project.Config{
		ProjectName:  "Integration" + strings.ToUpper(variant[:1]) + variant[1:],
		GoModuleName: "github.com/test/project",
		UpdatedAt:    "2026-01-01 00:00:00 UTC",
		Render: project.RenderConfig{
			QueueDriver:   "redis",
			GoForjVersion: version.Semver(),
			Components: project.Components{
				CLI:    true,
				WebAPI: true,
				Auth:   true,
				Docker: true,
			},
		},
	}
	if spec.applyConfig != nil {
		spec.applyConfig(&cfg.Render.Components)
	}
	return WriteYAML(filepath.Join(dir, ".goforj.yml"), cfg)
}

func renderedIntegrationSteps(tag, target string) ([]integrationStep, error) {
	all := []integrationStep{
		{name: "auth", args: []string{"go", "test", "./internal/auth", "-tags=integration," + tag}},
		{name: "makecmd", args: []string{"go", "test", "./internal/makecmd", "-tags=integration," + tag}},
		{name: "migrations", args: []string{"go", "test", "./migrations", "-tags=integration," + tag}},
		{name: "database", args: []string{"go", "test", "./internal/database", "-tags=integration," + tag}},
	}
	target = strings.TrimSpace(strings.ToLower(target))
	if target == "modelgen" {
		target = "makecmd"
	}
	if target == "" || target == "all" {
		return all, nil
	}
	for _, step := range all {
		if step.name == target {
			return []integrationStep{step}, nil
		}
	}
	return nil, fmt.Errorf("unknown rendered integration target %q", target)
}

func (cmd *TestIntegrationCmd) runRenderedTaggedTests(dir, modCache, buildCache, tag, target string, extraEnv map[string]string) error {
	steps, err := renderedIntegrationSteps(tag, target)
	if err != nil {
		return err
	}
	for _, step := range steps {
		args := append([]string{}, step.args...)
		if cmd.Verbose {
			args = append(args, "-v")
		}
		if !cmd.Silent {
			testkit.PrintSubsection(fmt.Sprintf("%s integration tests", step.name))
		}
		if err := runIntegrationGoTestStep(cmd.Silent, step.name, dir, modCache, buildCache, extraEnv, args); err != nil {
			return err
		}
	}
	return nil
}

func (cmd *TestIntegrationCmd) runRenderedVariant(variant, target string) error {
	spec, ok := dbIntegrationVariantSpecs[variant]
	if !ok {
		return fmt.Errorf("unknown variant %q (expected mysql, postgres, or sqlite)", variant)
	}

	modCache, buildCache := testkit.GoCachePaths()
	tempRoot, err := testkit.TempRoot("FORJ_DB_INTEGRATION_TMPDIR")
	if err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp(tempRoot, "forj_db_integration_")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	if !cmd.Silent {
		testkit.PrintSection(fmt.Sprintf("Rendered App Integration: %s", variant))
		console.Infof("workspace: %s", tempDir)
	}

	if err := cmd.writeRenderedIntegrationConfig(tempDir, variant, spec); err != nil {
		return err
	}
	forjExec, cleanup, err := repoForjExecutable(modCache, buildCache)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := runStep(cmd.logger, cmd.Silent, "render", tempDir, modCache, buildCache, []string{forjExec, "render"}); err != nil {
		return err
	}

	testEnv := cloneStringMap(spec.testEnv)
	stack, err := testkit.StartRenderedComposeServices(tempDir, testkit.ConsoleLogf(cmd.Silent))
	if err != nil {
		return err
	}
	defer stack.Stop()
	if err := stack.ApplyHostEnvOverrides([]string{filepath.Join(tempDir, ".env")}); err != nil {
		return err
	}
	for key, value := range stack.EnvOverrides() {
		testEnv[key] = value
	}

	if err := cmd.runRenderedTaggedTests(tempDir, modCache, buildCache, variant, target, testEnv); err != nil {
		return err
	}

	if !cmd.Silent {
		console.Successf("DB integration tests completed (%s)", variant)
	}
	return nil
}

func runIntegrationGoTestStep(silent bool, name, dir, modCache, buildCache string, extraEnv map[string]string, args []string) error {
	args = ensureGoTestVerbose(args)
	command := execx.Command(args[0], args[1:]...).
		Dir(dir).
		EnvAppend(map[string]string{
			"GOMODCACHE": modCache,
			"GOCACHE":    buildCache,
		}).
		EnvAppend(extraEnv)

	if !silent {
		command = command.StdoutWriter(os.Stdout).StderrWriter(os.Stderr)
	}

	if !silent {
		command = command.ShadowPrint(
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

	res, err := command.Run()
	if err != nil || !res.OK() {
		if !silent {
			console.Errorf("%s failed", name)
		}
		if err != nil {
			stderr := strings.TrimSpace(res.Stderr)
			stdout := strings.TrimSpace(res.Stdout)
			if stderr != "" {
				return fmt.Errorf("%s: %w (%s)", name, err, stderr)
			}
			if stdout != "" {
				return fmt.Errorf("%s: %w (%s)", name, err, stdout)
			}
			return fmt.Errorf("%s: %w", name, err)
		}
		stderr := strings.TrimSpace(res.Stderr)
		stdout := strings.TrimSpace(res.Stdout)
		if stderr != "" {
			return fmt.Errorf("%s failed with exit code %d (%s)", name, res.ExitCode, stderr)
		}
		if stdout != "" {
			return fmt.Errorf("%s failed with exit code %d (%s)", name, res.ExitCode, stdout)
		}
		return fmt.Errorf("%s failed with exit code %d", name, res.ExitCode)
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
