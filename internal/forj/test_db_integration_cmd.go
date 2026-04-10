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

// RenderedIntegrationRunner renders temp projects and runs integration tests in generated apps.
type RenderedIntegrationRunner struct {
	logger *logger.AppLogger

	Target  string `name:"target" optional:"" default:"all" enum:"all,auth,modelgen,migrations,database" help:"Rendered package target to run"`
	Variant string `arg:"" optional:"" default:"sqlite" enum:"mysql,postgres,sqlite" help:"Database variant to test"`
	Silent  bool   `help:"Suppress command output" short:"s"`
	Verbose bool   `help:"Enable verbose test output" short:"v"`
	Keep    bool   `help:"Keep the temp directory after completion" short:"k"`
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

func NewRenderedIntegrationRunner(logger *logger.AppLogger) *RenderedIntegrationRunner {
	return &RenderedIntegrationRunner{logger: logger}
}

func (cmd *RenderedIntegrationRunner) Run() error {
	variant := strings.ToLower(strings.TrimSpace(cmd.Variant))
	if variant == "" {
		variant = "sqlite"
	}
	return cmd.runRenderedVariant(variant, strings.ToLower(strings.TrimSpace(cmd.Target)))
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

func (cmd *RenderedIntegrationRunner) writeConfig(dir, variant string, spec dbIntegrationVariantSpec) error {
	cfg := project.Config{
		ProjectName:  "Integration" + strings.ToUpper(variant[:1]) + variant[1:],
		GoModuleName: "github.com/test/project",
		UpdatedAt:    "2026-01-01 00:00:00 UTC",
		Render: project.RenderConfig{
			QueueDriver:   "redis",
			GoForjVersion: "0.1.0",
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
		{name: "modelgen", args: []string{"go", "test", "./internal/modelgen", "-tags=integration," + tag}},
		{name: "migrations", args: []string{"go", "test", "./migrations", "-tags=integration," + tag}},
		{name: "database", args: []string{"go", "test", "./internal/database", "-tags=integration," + tag}},
	}
	target = strings.TrimSpace(strings.ToLower(target))
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

func (cmd *RenderedIntegrationRunner) runTaggedTests(dir, modCache, buildCache, tag, target string, extraEnv map[string]string) error {
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
			printIntegrationSubsection(fmt.Sprintf("%s integration tests", step.name))
		}
		if err := runIntegrationGoTestStep(cmd.Silent, step.name, dir, modCache, buildCache, extraEnv, args); err != nil {
			return err
		}
	}
	return nil
}

func (cmd *RenderedIntegrationRunner) runRenderedVariant(variant, target string) error {
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
	if !cmd.Keep {
		defer os.RemoveAll(tempDir)
	}

	if !cmd.Silent {
		printIntegrationSection(fmt.Sprintf("Rendered App Integration: %s", variant))
		console.Infof("workspace: %s", tempDir)
	}

	if err := cmd.writeConfig(tempDir, variant, spec); err != nil {
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
	stack, err := testkit.StartRenderedComposeServices(tempDir, integrationLogf(cmd.Silent))
	if err != nil {
		return err
	}
	defer stack.Stop()
	for key, value := range stack.EnvOverrides() {
		testEnv[key] = value
	}
	if err := stack.ApplyHostEnvOverrides([]string{filepath.Join(tempDir, ".env.host")}); err != nil {
		return err
	}

	if err := cmd.runTaggedTests(tempDir, modCache, buildCache, variant, target, testEnv); err != nil {
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
