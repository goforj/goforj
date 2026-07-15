package forj

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goforj/execx"
	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/testexec"
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

// integrationStep names one command so execution and failure reporting cannot drift apart.
type integrationStep struct {
	name string
	args []string
}

// integrationExecutor owns the output policy and Go caches shared by every command in one integration run.
type integrationExecutor struct {
	logger  *logger.AppLogger
	silent  bool
	verbose bool
	caches  testexec.GoCaches
}

// dbIntegrationVariantSpec defines the component selection and runtime environment for one rendered database target.
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

// Signature exposes integration validation as a maintainer-only command.
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
	executor := integrationExecutor{
		logger:  cmd.logger,
		silent:  cmd.Silent,
		verbose: cmd.Verbose,
		caches:  testexec.NewGoCaches(modCache, buildCache),
	}
	suite := strings.TrimSpace(strings.ToLower(cmd.Suite))
	target := strings.TrimSpace(strings.ToLower(cmd.Target))
	variant := strings.TrimSpace(strings.ToLower(cmd.Variant))

	if !cmd.Silent {
		testkit.PrintSection(fmt.Sprintf("Integration Suite: %s", suite))
	}

	switch suite {
	case "framework":
		return cmd.runFrameworkSuite(executor, target)
	case "rendered":
		return cmd.runRenderedSuite(executor, target, variant)
	case "all":
		if err := cmd.runFrameworkSuite(executor, target); err != nil {
			return err
		}
		return cmd.runRenderedSuite(executor, target, variant)
	default:
		return fmt.Errorf("unknown integration suite %q", suite)
	}
}

// runFrameworkSuite runs integration-tagged tests against this repository.
func (cmd *TestIntegrationCmd) runFrameworkSuite(executor integrationExecutor, target string) error {
	if target != "" && target != "all" {
		return fmt.Errorf("framework integration does not support target %q; use rendered targets for generated app package tests", target)
	}
	builtForj, err := testkit.BuildForjBinary(executor.caches.ModulePath(), executor.caches.BuildPath())
	if err != nil {
		return err
	}
	defer builtForj.Cleanup()
	forjExec := builtForj.Path
	frameworkEnv := map[string]string{
		"FORJ_INTEGRATION_FORJ_PATH": forjExec,
	}
	redisTeardown, err := cmd.configureFrameworkRedis(frameworkEnv)
	if err != nil {
		return err
	}
	if redisTeardown != nil {
		defer redisTeardown()
	}
	if !executor.silent {
		testkit.PrintSubsection("Framework integration preflight")
	}
	preflight := integrationStep{
		name: "framework preflight",
		args: []string{"go", "test", "-run", "^$", "-tags=integration", "./internal/forj", "-count=1"},
	}
	if err := executor.runFrameworkStep(".", preflight, frameworkEnv); err != nil {
		if !executor.silent {
			console.Warnf("framework preflight failed, attempting integration-tagged tidy")
		}
		tidyEnv := map[string]string{
			"GOFLAGS":                    "-tags=integration",
			"FORJ_INTEGRATION_FORJ_PATH": forjExec,
		}
		tidy := integrationStep{name: "framework tidy", args: []string{"go", "mod", "tidy"}}
		if err := executor.runFrameworkStep(".", tidy, tidyEnv); err != nil {
			return err
		}
		if err := executor.runFrameworkStep(".", preflight, frameworkEnv); err != nil {
			return err
		}
	}
	args := []string{"go", "test", "-tags=integration", "./internal/forj", "-count=1"}
	if executor.verbose {
		args = append(args, "-v")
	}
	if !executor.silent {
		testkit.PrintSubsection("Framework integration tests")
	}
	if err := executor.runFrameworkStep(".", integrationStep{name: "framework", args: args}, frameworkEnv); err != nil {
		return err
	}
	return nil
}

// configureFrameworkRedis reuses a caller-provided Redis endpoint before falling back to a testcontainer.
func (cmd *TestIntegrationCmd) configureFrameworkRedis(frameworkEnv map[string]string) (func(), error) {
	redisHost := strings.TrimSpace(os.Getenv("REDIS_HOST"))
	redisPort := strings.TrimSpace(os.Getenv("REDIS_PORT"))
	if redisHost != "" && redisPort != "" {
		if err := testkit.WaitForTCPReadyAddress(redisHost, redisPort, 2*time.Second); err == nil {
			frameworkEnv["REDIS_HOST"] = redisHost
			frameworkEnv["REDIS_PORT"] = redisPort
			return nil, nil
		}
	}
	return testkit.StartRedisTestcontainer(testkit.ConsoleLogf(cmd.Silent), frameworkEnv)
}

// runRenderedSuite runs generated application tests across the selected database variants.
func (cmd *TestIntegrationCmd) runRenderedSuite(executor integrationExecutor, target, variant string) error {
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
		if !executor.silent {
			testkit.PrintSubsection(fmt.Sprintf("Rendered integration variant: %s", dbVariant))
		}
		if err := cmd.runRenderedVariant(executor, dbVariant, target); err != nil {
			return err
		}
	}

	if !executor.silent {
		console.Successf("Integration tests completed")
	}
	return nil
}

// cloneStringMap prevents runtime container overrides from mutating the reusable variant catalog.
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

// writeRenderedIntegrationConfig publishes the minimal generated App needed to exercise one database variant.
func (cmd *TestIntegrationCmd) writeRenderedIntegrationConfig(dir, variant string, spec dbIntegrationVariantSpec) error {
	cfg := project.Config{
		ProjectName:  "Integration" + strings.ToUpper(variant[:1]) + variant[1:],
		GoModuleName: "github.com/test/project",
		UpdatedAt:    "2026-01-01 00:00:00 UTC",
		Render: project.RenderConfig{
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
	return testkit.WriteProjectConfig(filepath.Join(dir, ".goforj.yml"), cfg)
}

// renderedIntegrationSteps maps a user target to deterministic generated-package test commands.
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

// runRenderedTaggedTests runs each selected generated-package test through the shared integration environment.
func (executor integrationExecutor) runRenderedTaggedTests(dir, tag, target string, extraEnv map[string]string) error {
	steps, err := renderedIntegrationSteps(tag, target)
	if err != nil {
		return err
	}
	for _, step := range steps {
		step.args = append([]string{}, step.args...)
		if executor.verbose {
			step.args = append(step.args, "-v")
		}
		if !executor.silent {
			testkit.PrintSubsection(fmt.Sprintf("%s integration tests", step.name))
		}
		if err := executor.runGoTestStep(dir, step, extraEnv); err != nil {
			return err
		}
	}
	return nil
}

// runRenderedVariant renders and tests one database-specific application workspace.
func (cmd *TestIntegrationCmd) runRenderedVariant(executor integrationExecutor, variant, target string) error {
	spec, ok := dbIntegrationVariantSpecs[variant]
	if !ok {
		return fmt.Errorf("unknown variant %q (expected mysql, postgres, or sqlite)", variant)
	}

	tempRoot, err := testkit.TempRoot("FORJ_DB_INTEGRATION_TMPDIR")
	if err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp(tempRoot, "forj_db_integration_")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	if !executor.silent {
		testkit.PrintSection(fmt.Sprintf("Rendered App Integration: %s", variant))
		console.Infof("workspace: %s", tempDir)
	}

	if err := cmd.writeRenderedIntegrationConfig(tempDir, variant, spec); err != nil {
		return err
	}
	builtForj, err := testkit.BuildForjBinary(executor.caches.ModulePath(), executor.caches.BuildPath())
	if err != nil {
		return err
	}
	defer builtForj.Cleanup()
	forjExec := builtForj.Path
	if err := executor.workspace(tempDir).Run("render", forjExec, "render"); err != nil {
		return err
	}

	testEnv := cloneStringMap(spec.testEnv)
	stack, err := testkit.StartRenderedComposeServices(tempDir, testkit.ConsoleLogf(executor.silent))
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

	if err := executor.runRenderedTaggedTests(tempDir, variant, target, testEnv); err != nil {
		return err
	}

	if !executor.silent {
		console.Successf("DB integration tests completed (%s)", variant)
	}
	return nil
}

// runGoTestStep runs one rendered-package test with shared caches and preserves detailed package failure context.
func (executor integrationExecutor) runGoTestStep(dir string, step integrationStep, extraEnv map[string]string) error {
	args := ensureGoTestVerbose(step.args)
	command := execx.Command(args[0], args[1:]...).
		Dir(dir).
		EnvAppend(map[string]string{
			"GOMODCACHE": executor.caches.ModulePath(),
			"GOCACHE":    executor.caches.BuildPath(),
		}).
		EnvAppend(extraEnv)

	if !executor.silent {
		command = command.StdoutWriter(os.Stdout).StderrWriter(os.Stderr)
	}

	if !executor.silent {
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
		if !executor.silent {
			console.Errorf("%s failed", step.name)
		}
		if err != nil {
			stderr := strings.TrimSpace(res.Stderr)
			stdout := strings.TrimSpace(res.Stdout)
			if stderr != "" {
				return fmt.Errorf("%s: %w (%s)", step.name, err, stderr)
			}
			if stdout != "" {
				return fmt.Errorf("%s: %w (%s)", step.name, err, stdout)
			}
			return fmt.Errorf("%s: %w", step.name, err)
		}
		stderr := strings.TrimSpace(res.Stderr)
		stdout := strings.TrimSpace(res.Stdout)
		if stderr != "" {
			return fmt.Errorf("%s failed with exit code %d (%s)", step.name, res.ExitCode, stderr)
		}
		if stdout != "" {
			return fmt.Errorf("%s failed with exit code %d (%s)", step.name, res.ExitCode, stdout)
		}
		return fmt.Errorf("%s failed with exit code %d", step.name, res.ExitCode)
	}
	return nil
}

// ensureGoTestVerbose keeps long integration runs observable even when the command flag is omitted.
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

// runFrameworkStep runs one repository command in the isolated integration module environment.
func (executor integrationExecutor) runFrameworkStep(dir string, step integrationStep, extraEnv map[string]string) error {
	args := ensureGoTestVerbose(step.args)
	cmd := execx.Command(args[0], args[1:]...).
		Dir(dir).
		EnvAppend(map[string]string{
			"GOMODCACHE": executor.caches.ModulePath(),
			"GOCACHE":    executor.caches.BuildPath(),
			"GOFLAGS":    "",
			"GOWORK":     "off",
		}).
		EnvAppend(extraEnv)

	if !executor.silent {
		cmd = cmd.StdoutWriter(os.Stdout).StderrWriter(os.Stderr)
	}

	if !executor.silent {
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
		if !executor.silent {
			console.Errorf("%s failed", step.name)
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

// workspace binds non-streaming integration commands to the executor's shared policy.
func (executor integrationExecutor) workspace(dir string) *testexec.Workspace {
	return testexec.NewWorkspace(executor.logger, executor.silent, dir, executor.caches)
}
