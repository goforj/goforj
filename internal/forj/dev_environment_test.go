package forj

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	envx "github.com/goforj/env/v2"
	"github.com/goforj/goforj/project"
)

const devEnvironmentHelperKey = "GO_WANT_DEV_ENVIRONMENT_HELPER"

// TestDevEnvironmentUsesHostFileForInitialLoadReloadAndPreTasks protects Harbor's ordinary dotenv integration boundary.
func TestDevEnvironmentUsesHostFileForInitialLoadReloadAndPreTasks(t *testing.T) {
	if os.Getenv(devEnvironmentHelperKey) == "1" {
		runDevEnvironmentHelper(t)
		return
	}
	if !envx.IsHostEnvironment() && !envx.IsDockerInDocker() {
		t.Skip(".env.host is intentionally inactive inside ordinary application containers")
	}

	root := t.TempDir()
	logPath := filepath.Join(root, "pre-task.log")
	writeDevEnvironmentFiles(t, root, "127.18.0.11", "host-token-one")
	command := exec.Command(os.Args[0], "-test.run=^TestDevEnvironmentUsesHostFileForInitialLoadReloadAndPreTasks$")
	command.Dir = root
	command.Env = devEnvironmentHelperEnvironment(map[string]string{
		devEnvironmentHelperKey: "1",
		"GO_DEV_ENV_LOG":        logPath,
		"HARBOR_TOKEN":          "ambient-token",
		"IP_ADDRESS":            "127.0.0.1",
	})
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("development environment helper failed: %v\n%s", err, output)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read pre-task log: %v", err)
	}
	want := "host-token-one|127.18.0.11\nhost-token-two|127.18.0.12\n"
	if string(content) != want {
		t.Fatalf("pre-task environment = %q, want %q", content, want)
	}
}

// runDevEnvironmentHelper exercises the env package's real initial and reload behavior in an isolated process.
func runDevEnvironmentHelper(t *testing.T) {
	if err := loadDevEnvironment(false); err != nil {
		t.Fatalf("initial load: %v", err)
	}
	assertDevHostEnvironment(t, "127.18.0.11", "host-token-one")
	if err := runDevEnvironmentPreTask(os.Getenv("GO_DEV_ENV_LOG")); err != nil {
		t.Fatalf("initial pre task: %v", err)
	}
	writeDevEnvironmentFiles(t, ".", "127.18.0.12", "host-token-two")
	if err := loadDevEnvironment(true); err != nil {
		t.Fatalf("reload: %v", err)
	}
	assertDevHostEnvironment(t, "127.18.0.12", "host-token-two")
	if err := runDevEnvironmentPreTask(os.Getenv("GO_DEV_ENV_LOG")); err != nil {
		t.Fatalf("reloaded pre task: %v", err)
	}
}

// assertDevHostEnvironment proves the host layer wins over both base dotenv and ambient values.
func assertDevHostEnvironment(t *testing.T, address string, token string) {
	t.Helper()
	if got := os.Getenv("IP_ADDRESS"); got != address {
		t.Fatalf("IP_ADDRESS = %q, want %q", got, address)
	}
	if got := os.Getenv("HARBOR_TOKEN"); got != token {
		t.Fatalf("HARBOR_TOKEN = %q, want %q", got, token)
	}
}

// runDevEnvironmentPreTask records the environment inherited by an ordinary configured task.
func runDevEnvironmentPreTask(logPath string) error {
	command := "printf '%s|%s\\n' \"$HARBOR_TOKEN\" \"$IP_ADDRESS\" >> " + shellSingleQuote(logPath)
	return runDevTasks("Host environment pre task", []project.DevTask{{Name: "Record environment", Cmd: command}})
}

// writeDevEnvironmentFiles keeps base conflicts explicit while changing only the authoritative host layer.
func writeDevEnvironmentFiles(t *testing.T, root string, address string, token string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("HARBOR_TOKEN=base-token\nIP_ADDRESS=0.0.0.0\n"), 0o644); err != nil {
		t.Fatalf("write base environment: %v", err)
	}
	contents := "HARBOR_TOKEN=" + token + "\nIP_ADDRESS=" + address + "\n"
	if err := os.WriteFile(filepath.Join(root, ".env.host"), []byte(contents), 0o644); err != nil {
		t.Fatalf("write host environment: %v", err)
	}
}

// devEnvironmentHelperEnvironment creates a deterministic child environment without stale fixture values.
func devEnvironmentHelperEnvironment(overrides map[string]string) []string {
	removed := map[string]struct{}{
		devEnvironmentHelperKey: {},
		"GO_DEV_ENV_LOG":        {},
		"HARBOR_TOKEN":          {},
		"IP_ADDRESS":            {},
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
