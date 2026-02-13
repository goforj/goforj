package forj

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/goforj/execx"
	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
)

// TestDBIntegrationCmd renders a temp project and runs DB-tagged integration tests.
type TestDBIntegrationCmd struct {
	logger *logger.AppLogger

	Variant string `arg:"" optional:"" default:"sqlite" enum:"mysql,postgres,sqlite" help:"Database variant to test"`
	Silent  bool   `help:"Suppress command output" short:"s"`
	Verbose bool   `help:"Enable verbose test output" short:"v"`
	Keep    bool   `help:"Keep the temp directory after completion" short:"k"`
}

func NewTestDBIntegrationCmd(logger *logger.AppLogger) *TestDBIntegrationCmd {
	return &TestDBIntegrationCmd{logger: logger}
}

func (cmd *TestDBIntegrationCmd) Run() error {
	variant := strings.ToLower(strings.TrimSpace(cmd.Variant))
	if variant == "" {
		variant = "sqlite"
	}
	if variant != "mysql" && variant != "postgres" && variant != "sqlite" {
		return fmt.Errorf("unknown variant %q (expected mysql, postgres, or sqlite)", variant)
	}

	modCache, buildCache := getCachePaths()
	tempDir, err := os.MkdirTemp("", "forj_db_integration_")
	if err != nil {
		return err
	}
	if !cmd.Keep {
		defer os.RemoveAll(tempDir)
	}

	if !cmd.Silent {
		console.Actionf("Running test:db-integration (%s)", variant)
		console.Infof("workspace: %s", tempDir)
	}

	if err := cmd.writeConfig(tempDir, variant); err != nil {
		return err
	}
	if err := runStep(cmd.logger, cmd.Silent, "render", tempDir, modCache, buildCache, []string{"forj", "render"}); err != nil {
		return err
	}

	var teardown func() = func() {}
	if variant == "mysql" || variant == "postgres" {
		composeCmd, err := detectComposeCommand()
		if err != nil {
			return err
		}
		projectName := "goforj-integration-" + variant
		env := map[string]string{"COMPOSE_PROJECT_NAME": projectName}
		if !cmd.Silent {
			console.Actionf("Starting docker compose services (%s)", variant)
		}
		_ = runExec(tempDir, env, cmd.Silent, composeCmd[0], append(composeCmd[1:], "down", "-v", "--remove-orphans")...)
		if err := runExec(tempDir, env, cmd.Silent, composeCmd[0], append(composeCmd[1:], "up", "-d")...); err != nil {
			return err
		}
		teardown = func() {
			_ = runExec(tempDir, env, true, composeCmd[0], append(composeCmd[1:], "down", "-v", "--remove-orphans")...)
		}
		if err := waitForDBReady(tempDir, env, composeCmd, variant); err != nil {
			teardown()
			return err
		}
	}
	defer teardown()

	testEnv := map[string]string{}
	switch variant {
	case "mysql":
		testEnv = map[string]string{
			"DB_DRIVER":         "mysql",
			"DB_HOST":           "127.0.0.1",
			"DB_PORT":           "3306",
			"DB_DATABASE":       "db",
			"DB_USERNAME":       "user",
			"DB_PASSWORD":       "password",
			"DB_HOST_IN_DOCKER": "false",
		}
	case "postgres":
		testEnv = map[string]string{
			"DB_DRIVER":         "postgres",
			"DB_HOST":           "127.0.0.1",
			"DB_PORT":           "5432",
			"DB_DATABASE":       "app",
			"DB_USERNAME":       "postgres",
			"DB_PASSWORD":       "postgres",
			"DB_HOST_IN_DOCKER": "false",
		}
	case "sqlite":
		testEnv = map[string]string{
			"DB_DRIVER":   "sqlite",
			"DB_DATABASE": "./_data/sqlite/app.db",
		}
	}

	if err := cmd.runTaggedTests(tempDir, modCache, buildCache, variant, testEnv); err != nil {
		return err
	}

	if !cmd.Silent {
		console.Successf("DB integration tests completed (%s)", variant)
	}
	return nil
}

func (cmd *TestDBIntegrationCmd) writeConfig(dir, variant string) error {
	cfg := project.Config{
		ProjectName:  "Integration" + strings.ToUpper(variant[:1]) + variant[1:],
		GoModuleName: "github.com/test/project",
		UpdatedAt:    "2026-01-01 00:00:00 UTC",
		Components: project.Components{
			CLI: true,
		},
	}
	cfg.Render.QueueDriver = "redis"
	cfg.Render.GoForjVersion = "0.1.0"
	switch variant {
	case "mysql":
		cfg.Components.Docker = true
		cfg.Components.DatabaseMySQL = true
	case "postgres":
		cfg.Components.Docker = true
		cfg.Components.DatabasePostgres = true
	case "sqlite":
		cfg.Components.DatabaseSQLite = true
	}
	return WriteYAML(filepath.Join(dir, ".goforj.yml"), cfg)
}

func (cmd *TestDBIntegrationCmd) runTaggedTests(dir, modCache, buildCache, tag string, extraEnv map[string]string) error {
	steps := []struct {
		name string
		args []string
	}{
		{name: "modelgen", args: []string{"go", "test", "./internal/modelgen", "-tags=integration," + tag}},
		{name: "migrations", args: []string{"go", "test", "./internal/migrations", "-tags=integration," + tag}},
		{name: "dbconns", args: []string{"go", "test", "./internal/dbconns", "-tags=integration," + tag}},
	}
	for _, step := range steps {
		args := append([]string{}, step.args...)
		if cmd.Verbose {
			args = append(args, "-v")
		}
		if !cmd.Silent {
			console.Actionf("Running %s integration tests", step.name)
		}
		if err := runIntegrationGoTestStep(cmd.Silent, step.name, dir, modCache, buildCache, extraEnv, args); err != nil {
			return err
		}
	}
	return nil
}

func runIntegrationGoTestStep(silent bool, name, dir, modCache, buildCache string, extraEnv map[string]string, args []string) error {
	command := execx.Command(args[0], args[1:]...).
		Dir(dir).
		EnvAppend(map[string]string{
			"GOMODCACHE": modCache,
			"GOCACHE":    buildCache,
		}).
		EnvAppend(extraEnv)

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
			return err
		}
		return fmt.Errorf("%s failed with exit code %d", name, res.ExitCode)
	}
	return nil
}

func detectComposeCommand() ([]string, error) {
	if _, err := exec.LookPath("docker-compose"); err == nil {
		return []string{"docker-compose"}, nil
	}
	if _, err := exec.LookPath("docker"); err == nil {
		return []string{"docker", "compose"}, nil
	}
	return nil, fmt.Errorf("docker compose not available")
}

func waitForDBReady(dir string, env map[string]string, composeCmd []string, variant string) error {
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		var args []string
		switch variant {
		case "mysql":
			args = append(composeCmd[1:], "exec", "-T", "mysql", "mysqladmin", "ping", "-uroot", "-proot")
		case "postgres":
			args = append(composeCmd[1:], "exec", "-T", "postgres", "pg_isready", "-U", "postgres")
		default:
			return nil
		}
		if err := runExec(dir, env, true, composeCmd[0], args...); err == nil {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("%s did not become ready in time", variant)
}

func runExec(dir string, env map[string]string, silent bool, binary string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		if !silent {
			console.Errorf("%s %s failed: %s", binary, strings.Join(args, " "), strings.TrimSpace(string(output)))
		}
		return err
	}
	return nil
}
