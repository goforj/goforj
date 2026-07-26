package forj

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/devwatch"
	"github.com/goforj/goforj/project"
)

const (
	devEnvironmentHelperKey        = "GO_WANT_DEV_ENVIRONMENT_HELPER"
	devEnvironmentWatcherHelperKey = "GO_WANT_DEV_ENVIRONMENT_WATCHER_HELPER"
)

// TestDevEnvironmentPreservesInheritedValuesAcrossLoadReloadAndChildren protects process-level overrides from dotenv layers.
func TestDevEnvironmentPreservesInheritedValuesAcrossLoadReloadAndChildren(t *testing.T) {
	if os.Getenv(devEnvironmentWatcherHelperKey) == "1" {
		if _, err := fmt.Fprintf(os.Stdout, "%s|%s", os.Getenv("INHERITED_TOKEN"), os.Getenv("DOTENV_ONLY")); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	if os.Getenv(devEnvironmentHelperKey) == "1" {
		runDevEnvironmentHelper(t)
		return
	}
	root := t.TempDir()
	logPath := filepath.Join(root, "pre-task.log")
	writeDevEnvironmentFiles(t, root, "127.18.0.11", "host-token-one")
	command := exec.Command(os.Args[0], "-test.run=^TestDevEnvironmentPreservesInheritedValuesAcrossLoadReloadAndChildren$")
	command.Dir = root
	command.Env = devEnvironmentHelperEnvironment(map[string]string{
		devEnvironmentHelperKey: "1",
		"GO_DEV_ENV_LOG":        logPath,
		"INHERITED_TOKEN":       "process-token",
		"SERVICE_ADDRESS":       "127.0.0.1",
		"APP_ENV":               "launcher",
	})
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("development environment helper failed: %v\n%s", err, output)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read pre-task log: %v", err)
	}
	want := "process-token|127.0.0.1|dotenv-one\nprocess-token|127.0.0.1|dotenv-two\n"
	if string(content) != want {
		t.Fatalf("pre-task environment = %q, want %q", content, want)
	}
}

// runDevEnvironmentHelper exercises the env package's real initial and reload behavior in an isolated process.
func runDevEnvironmentHelper(t *testing.T) {
	inherited := captureProcessEnvironment()
	if err := loadDevEnvironment(false, inherited); err != nil {
		t.Fatalf("initial load: %v", err)
	}
	assertDevInheritedEnvironment(t, "dotenv-one")
	if err := runDevEnvironmentPreTask(os.Getenv("GO_DEV_ENV_LOG")); err != nil {
		t.Fatalf("initial pre task: %v", err)
	}
	writeDevEnvironmentFiles(t, ".", "127.18.0.12", "host-token-two")
	if err := loadDevEnvironment(true, inherited); err != nil {
		t.Fatalf("reload: %v", err)
	}
	assertDevInheritedEnvironment(t, "dotenv-two")
	if err := runDevEnvironmentPreTask(os.Getenv("GO_DEV_ENV_LOG")); err != nil {
		t.Fatalf("reloaded pre task: %v", err)
	}
	runDevEnvironmentWatcherChild(t)
}

// assertDevInheritedEnvironment verifies dotenv retains keys not set by the process.
func assertDevInheritedEnvironment(t *testing.T, dotenvValue string) {
	t.Helper()
	if got := os.Getenv("SERVICE_ADDRESS"); got != "127.0.0.1" {
		t.Fatalf("SERVICE_ADDRESS = %q", got)
	}
	if got := os.Getenv("INHERITED_TOKEN"); got != "process-token" {
		t.Fatalf("INHERITED_TOKEN = %q", got)
	}
	if got := os.Getenv("DOTENV_ONLY"); got != dotenvValue {
		t.Fatalf("DOTENV_ONLY = %q, want %q", got, dotenvValue)
	}
	if got := os.Getenv("APP_ENV"); got != "launcher" {
		t.Fatalf("APP_ENV = %q, want inherited launcher value", got)
	}
}

// runDevEnvironmentPreTask records the environment inherited by an ordinary configured task.
func runDevEnvironmentPreTask(logPath string) error {
	command := "printf '%s|%s|%s\\n' \"$INHERITED_TOKEN\" \"$SERVICE_ADDRESS\" \"$DOTENV_ONLY\" >> " + shellSingleQuote(logPath)
	return runDevTasks("Host environment pre task", []project.DevTask{
		{
			Name: "Record environment",
			Cmd:  command,
		},
	})
}

// writeDevEnvironmentFiles keeps .env.host as a fallback while process values remain authoritative.
func writeDevEnvironmentFiles(t *testing.T, root string, address string, token string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("APP_ENV=staging\nINHERITED_TOKEN=base-token\nSERVICE_ADDRESS=0.0.0.0\nDOTENV_ONLY=dotenv-"+strings.TrimPrefix(token, "host-token-")+"\n"), 0o644); err != nil {
		t.Fatalf("write base environment: %v", err)
	}
	contents := "INHERITED_TOKEN=" + token + "\nSERVICE_ADDRESS=" + address + "\n"
	if err := os.WriteFile(filepath.Join(root, ".env.host"), []byte(contents), 0o644); err != nil {
		t.Fatalf("write host environment: %v", err)
	}
}

// devEnvironmentHelperEnvironment creates a deterministic child environment without stale fixture values.
func devEnvironmentHelperEnvironment(overrides map[string]string) []string {
	removed := map[string]struct{}{
		devEnvironmentHelperKey: {},
		"GO_DEV_ENV_LOG":        {},
		"INHERITED_TOKEN":       {},
		"SERVICE_ADDRESS":       {},
		"APP_ENV":               {},
	}
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if _, remove := removed[key]; ok && remove {
			continue
		}
		environment = append(environment, entry)
	}
	for key, value := range overrides {
		environment = append(environment, key+"="+value)
	}
	return environment
}

// runDevEnvironmentWatcherChild verifies native watcher children inherit the restored process environment.
func runDevEnvironmentWatcherChild(t *testing.T) {
	t.Helper()
	var output strings.Builder
	supervisor := devwatch.NewSupervisor(devwatch.SupervisorOptions{})
	defer supervisor.Close()
	exit, err := supervisor.Run(context.Background(), "environment", devwatch.Command{
		Args:   []string{os.Args[0], "-test.run=^TestDevEnvironmentPreservesInheritedValuesAcrossLoadReloadAndChildren$"},
		Env:    map[string]string{devEnvironmentWatcherHelperKey: "1"},
		Stdout: &output,
	})
	if err != nil || !exit.OK() {
		t.Fatalf("watcher child = %#v, %v", exit, err)
	}
	if got := output.String(); got != "process-token|dotenv-two" {
		t.Fatalf("watcher child environment = %q", got)
	}
}
