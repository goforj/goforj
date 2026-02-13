package forj

import (
	"bytes"
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

	if verbose && !silent {
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

func (cmd *TestDBIntegrationCmd) runTaggedTestsInDocker(tempDir, composeProjectName, tag string, testEnv map[string]string) error {
	hostTempDir, err := resolveDockerHostPath(tempDir)
	if err != nil {
		return err
	}
	repoRoot, err := gitRepoRoot()
	if err != nil {
		return err
	}
	hostRepoRoot, err := resolveDockerHostPath(repoRoot)
	if err != nil {
		return err
	}

	if !cmd.Silent {
		console.Infof("docker app mount: %s", hostTempDir)
		console.Infof("docker repo mount: %s", hostRepoRoot)
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
	script := "cd /goforj && go build -buildvcs=false -o /tmp/forj ./cmd/forj && cd /app && " + testScript

	args := []string{
		"run", "--rm",
		"-v", "goforj-go-mod-cache:/go/pkg/mod",
		"-v", "goforj-go-build-cache:/root/.cache/go-build",
		"--network", composeProjectName + "_backend",
		"-v", hostTempDir + ":/app",
		"-v", hostRepoRoot + ":/goforj",
		"-w", "/app",
		"-e", "FORJ_BIN=/tmp/forj",
	}
	for key, value := range testEnv {
		args = append(args, "-e", key+"="+value)
	}
	args = append(args, "golang:1.25", "sh", "-c", script)

	if !cmd.Silent {
		console.Actionf("Running dockerized integration tests (%s)", tag)
	}
	return runExecWithOutput(".", nil, cmd.Silent, "docker", args...)
}

func runExecWithOutput(dir string, env map[string]string, silent bool, binary string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	if err != nil {
		trimmed := strings.TrimSpace(output.String())
		if !silent && trimmed != "" {
			fmt.Println(trimmed)
		}
		if trimmed != "" {
			return fmt.Errorf("%s %s failed: %w (%s)", binary, strings.Join(args, " "), err, trimmed)
		}
		return fmt.Errorf("%s %s failed: %w", binary, strings.Join(args, " "), err)
	}
	if !silent {
		trimmed := strings.TrimSpace(output.String())
		if trimmed != "" {
			fmt.Println(trimmed)
		}
	}
	return nil
}

func gitRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("resolve repo root: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func resolveDockerHostPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("empty path")
	}
	if strings.HasPrefix(path, "/actions-runner/") && isDir("/runner") {
		path = "/runner" + strings.TrimPrefix(path, "/actions-runner")
	}
	if !strings.HasPrefix(path, "/runner/") {
		return path, nil
	}

	source, root := runnerMountMeta()
	suffix := strings.TrimPrefix(path, "/runner")
	if strings.HasPrefix(source, "/") && !strings.HasPrefix(source, "/dev/") {
		return source + suffix, nil
	}
	if root == "" || root == "/" {
		return suffix, nil
	}
	return root + suffix, nil
}

func runnerMountMeta() (source string, root string) {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return "/runner", "/"
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		if fields[4] != "/runner" {
			continue
		}
		root = fields[3]
		sep := -1
		for i, field := range fields {
			if field == "-" {
				sep = i
				break
			}
		}
		if sep >= 0 && sep+2 < len(fields) {
			source = fields[sep+2]
		}
		break
	}
	if source == "" {
		source = "/runner"
	}
	if root == "" {
		root = "/"
	}
	return source, root
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
