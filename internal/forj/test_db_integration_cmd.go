package forj

import (
	"fmt"
	"io"
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
	tempRoot, err := dbIntegrationTempRoot()
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
	composeProjectName := "goforj-integration-" + variant
	if variant == "mysql" || variant == "postgres" {
		composeCmd, err := detectComposeCommand()
		if err != nil {
			return err
		}
		env := map[string]string{"COMPOSE_PROJECT_NAME": composeProjectName}
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
			"DB_DRIVER":               "mysql",
			"DB_HOST":                 "mysql",
			"DB_PORT":                 "3306",
			"DB_DATABASE":             "db",
			"DB_USERNAME":             "user",
			"DB_PASSWORD":             "password",
			"DB_HOST_IN_DOCKER":       "true",
			"DB_HOST_INTEGRATION":     "mysql",
			"DB_PORT_INTEGRATION":     "3306",
			"DB_DATABASE_INTEGRATION": "db",
			"DB_USERNAME_INTEGRATION": "user",
			"DB_PASSWORD_INTEGRATION": "password",
		}
	case "postgres":
		testEnv = map[string]string{
			"DB_DRIVER":               "postgres",
			"DB_HOST":                 "postgres",
			"DB_PORT":                 "5432",
			"DB_DATABASE":             "app",
			"DB_USERNAME":             "postgres",
			"DB_PASSWORD":             "postgres",
			"DB_HOST_IN_DOCKER":       "true",
			"DB_HOST_INTEGRATION":     "postgres",
			"DB_PORT_INTEGRATION":     "5432",
			"DB_DATABASE_INTEGRATION": "app",
			"DB_USERNAME_INTEGRATION": "postgres",
			"DB_PASSWORD_INTEGRATION": "postgres",
		}
	case "sqlite":
		testEnv = map[string]string{
			"DB_DRIVER":   "sqlite",
			"DB_DATABASE": "./_data/sqlite/app.db",
		}
	}

	if variant == "mysql" || variant == "postgres" {
		if err := cmd.runTaggedTestsInDocker(tempDir, composeProjectName, variant, testEnv); err != nil {
			return err
		}
	} else {
		if err := cmd.runTaggedTests(tempDir, modCache, buildCache, variant, testEnv); err != nil {
			return err
		}
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
		if err := runIntegrationGoTestStep(cmd.Silent, cmd.Verbose, step.name, dir, modCache, buildCache, extraEnv, args); err != nil {
			return err
		}
	}
	return nil
}

func runIntegrationGoTestStep(silent bool, verbose bool, name, dir, modCache, buildCache string, extraEnv map[string]string, args []string) error {
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
	deadline := time.Now().Add(90 * time.Second)
	networkName := strings.TrimSpace(env["COMPOSE_PROJECT_NAME"]) + "_backend"
	var lastErr error

	for time.Now().Before(deadline) {
		switch variant {
		case "mysql":
			// Verify SQL is actually accepting queries inside the DB container.
			localArgs := append(composeCmd[1:], "exec", "-T", "mysql", "sh", "-lc", `mysql -h 127.0.0.1 -uroot -proot -e "SELECT 1" >/dev/null 2>&1`)
			if err := runExec(dir, env, true, composeCmd[0], localArgs...); err != nil {
				lastErr = err
				time.Sleep(1 * time.Second)
				continue
			}
			// Verify cross-container connectivity on the same network used by tests.
			remoteArgs := []string{"run", "--rm", "--network", networkName, "mysql:8.0", "mysqladmin", "ping", "-h", "mysql", "-uuser", "-ppassword", "--silent"}
			if err := runExecWithOutput(".", nil, true, "docker", remoteArgs...); err != nil {
				lastErr = err
				time.Sleep(1 * time.Second)
				continue
			}
			return nil
		case "postgres":
			localArgs := append(composeCmd[1:], "exec", "-T", "postgres", "pg_isready", "-U", "postgres")
			if err := runExec(dir, env, true, composeCmd[0], localArgs...); err != nil {
				lastErr = err
				time.Sleep(1 * time.Second)
				continue
			}
			remoteArgs := []string{"run", "--rm", "--network", networkName, "postgres:16-alpine", "pg_isready", "-h", "postgres", "-U", "postgres"}
			if err := runExecWithOutput(".", nil, true, "docker", remoteArgs...); err != nil {
				lastErr = err
				time.Sleep(1 * time.Second)
				continue
			}
			return nil
		default:
			return nil
		}
	}

	if lastErr != nil {
		return fmt.Errorf("%s did not become ready in time: %w", variant, lastErr)
	}
	return fmt.Errorf("%s did not become ready in time", variant)
}

func runExec(dir string, env map[string]string, silent bool, binary string, args ...string) error {
	command := execx.Command(binary, args...).Dir(dir).EnvAppend(env)
	if !silent {
		command = command.StdoutWriter(os.Stdout).StderrWriter(os.Stderr)
	}
	res, err := command.Run()
	if err != nil || !res.OK() {
		stderr := strings.TrimSpace(res.Stderr)
		stdout := strings.TrimSpace(res.Stdout)
		if stderr != "" {
			return fmt.Errorf("%s %s failed: %s", binary, strings.Join(args, " "), stderr)
		}
		if stdout != "" {
			return fmt.Errorf("%s %s failed: %s", binary, strings.Join(args, " "), stdout)
		}
		if err != nil {
			return fmt.Errorf("%s %s failed: %w", binary, strings.Join(args, " "), err)
		}
		return fmt.Errorf("%s %s failed with exit code %d", binary, strings.Join(args, " "), res.ExitCode)
	}
	return nil
}

func (cmd *TestDBIntegrationCmd) runTaggedTestsInDocker(tempDir, composeProjectName, tag string, testEnv map[string]string) error {
	const testImage = "golang:1.25"

	containerForjBin, err := stageForjBinaryForDocker(tempDir)
	if err != nil {
		return err
	}
	containerName := fmt.Sprintf("forj-dbtest-%s-%d", tag, time.Now().UnixNano())
	defer func() {
		_ = runExecWithOutput(".", nil, true, "docker", "rm", "-f", containerName)
	}()

	if !cmd.Silent {
		console.Infof("docker forj bin: %s", containerForjBin)
		console.Infof("docker image: %s", testImage)
	}

	testArgs := []string{
		"go test ./internal/modelgen -tags=integration," + tag,
		"go test ./internal/migrations -tags=integration," + tag,
		"go test ./internal/dbconns -tags=integration," + tag,
	}
	if cmd.Verbose {
		for i := range testArgs {
			testArgs[i] += " -v"
		}
	}
	testScript := strings.Join(testArgs, " && ")
	script := "set -euo pipefail; cd /app && " + testScript

	createArgs := []string{
		"create",
		"--name", containerName,
		"-v", "goforj-go-mod-cache:/go/pkg/mod",
		"-v", "goforj-go-build-cache:/root/.cache/go-build",
		"--network", composeProjectName + "_backend",
		"-w", "/app",
		"--entrypoint", "sh",
		"-e", "FORJ_BIN=" + containerForjBin,
	}
	for key, value := range testEnv {
		createArgs = append(createArgs, "-e", key+"="+value)
	}
	createArgs = append(createArgs, testImage, "-c", "sleep 3600")

	if !cmd.Silent {
		console.Actionf("Running dockerized integration tests (%s)", tag)
	}
	if err := runExecWithOutput(".", nil, cmd.Silent, "docker", createArgs...); err != nil {
		return err
	}
	if err := runExecWithOutput(".", nil, cmd.Silent, "docker", "cp", tempDir+string(os.PathSeparator)+".", containerName+":/app"); err != nil {
		return err
	}
	if err := runExecWithOutput(".", nil, cmd.Silent, "docker", "start", containerName); err != nil {
		return err
	}
	return runExecWithOutput(".", nil, cmd.Silent, "docker", "exec", containerName, "sh", "-lc", script)
}

func runExecWithOutput(dir string, env map[string]string, silent bool, binary string, args ...string) error {
	command := execx.Command(binary, args...).Dir(dir).EnvAppend(env)
	if !silent {
		command = command.StdoutWriter(os.Stdout).StderrWriter(os.Stderr)
	}
	res, err := command.Run()
	if err != nil || !res.OK() {
		stderr := strings.TrimSpace(res.Stderr)
		stdout := strings.TrimSpace(res.Stdout)
		if stderr != "" && stdout != "" {
			return fmt.Errorf("%s %s failed (%s): %s", binary, strings.Join(args, " "), stderr, stdout)
		}
		if stderr != "" {
			return fmt.Errorf("%s %s failed: %s", binary, strings.Join(args, " "), stderr)
		}
		if stdout != "" {
			return fmt.Errorf("%s %s failed: %s", binary, strings.Join(args, " "), stdout)
		}
		if err != nil {
			return fmt.Errorf("%s %s failed: %w", binary, strings.Join(args, " "), err)
		}
		return fmt.Errorf("%s %s failed with exit code %d", binary, strings.Join(args, " "), res.ExitCode)
	}
	return nil
}

func stageForjBinaryForDocker(tempDir string) (string, error) {
	source, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve current forj binary: %w", err)
	}
	srcFile, err := os.Open(source)
	if err != nil {
		return "", fmt.Errorf("open current forj binary: %w", err)
	}
	defer srcFile.Close()

	destDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("create temp bin dir: %w", err)
	}
	destPath := filepath.Join(destDir, "forj")
	destFile, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("create staged forj binary: %w", err)
	}
	if _, err := io.Copy(destFile, srcFile); err != nil {
		_ = destFile.Close()
		return "", fmt.Errorf("copy staged forj binary: %w", err)
	}
	if err := destFile.Close(); err != nil {
		return "", fmt.Errorf("close staged forj binary: %w", err)
	}
	if err := os.Chmod(destPath, 0o755); err != nil {
		return "", fmt.Errorf("chmod staged forj binary: %w", err)
	}
	return "/app/bin/forj", nil
}

func dbIntegrationTempRoot() (string, error) {
	if override := strings.TrimSpace(os.Getenv("FORJ_DB_INTEGRATION_TMPDIR")); override != "" {
		if err := os.MkdirAll(override, 0o755); err != nil {
			return "", fmt.Errorf("create FORJ_DB_INTEGRATION_TMPDIR: %w", err)
		}
		return override, nil
	}

	// In containerized runners talking to host docker.sock, /tmp isn't usually
	// host-visible; use cwd so bind-mount paths are valid to the docker daemon.
	if _, err := os.Stat("/.dockerenv"); err == nil {
		wd, err := os.Getwd()
		if err == nil {
			root := filepath.Join(wd, ".tmp", "db_integration")
			if mkErr := os.MkdirAll(root, 0o755); mkErr != nil {
				return "", fmt.Errorf("create db integration temp root: %w", mkErr)
			}
			return root, nil
		}
	}

	root := os.TempDir()
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("create os temp dir: %w", err)
	}
	return root, nil
}
