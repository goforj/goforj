package forj

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/goforj/execx"
	"github.com/goforj/goforj/internal/devwatch"
	"github.com/goforj/goforj/project"
)

const (
	devEnvironmentHelperKey        = "GO_WANT_DEV_ENVIRONMENT_HELPER"
	devEnvironmentWatcherHelperKey = "GO_WANT_DEV_ENVIRONMENT_WATCHER_HELPER"
)

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
	inherited := processEnvironment(snapshotProcessEnv())
	if err := loadDevEnvironment(false, inherited); err != nil {
		t.Fatalf("initial load: %v", err)
	}
	assertDevInheritedEnvironment(t, "dotenv-one")
	if err := runDevEnvironmentPreTask(t, os.Getenv("GO_DEV_ENV_LOG")); err != nil {
		t.Fatalf("initial pre task: %v", err)
	}
	writeDevEnvironmentFiles(t, ".", "127.18.0.12", "host-token-two")
	if err := loadDevEnvironment(true, inherited); err != nil {
		t.Fatalf("reload: %v", err)
	}
	assertDevInheritedEnvironment(t, "dotenv-two")
	if err := runDevEnvironmentPreTask(t, os.Getenv("GO_DEV_ENV_LOG")); err != nil {
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

// runDevEnvironmentPreTask records inheritance through the real task runner while isolating pre-script Windows loader failures.
func runDevEnvironmentPreTask(t *testing.T, logPath string) error {
	t.Helper()
	command := "printf '%s|%s|%s\\n' \"$INHERITED_TOKEN\" \"$SERVICE_ADDRESS\" \"$DOTENV_ONLY\" >> " + shellSingleQuote(logPath)
	task := project.DevTask{Name: "Record environment", Cmd: command}
	stdin, err := os.Open(os.DevNull)
	if err != nil {
		return err
	}
	defer stdin.Close()
	var output *devTaskOutputTail
	result, err := runDevEnvironmentFixtureTask(t, runtime.GOOS, func() (execx.Result, error) {
		output = newDevTaskOutputTail(40)
		result, _, err := runDevTask(task, stdin, output, true)
		return result, err
	})
	if err != nil {
		return devTaskFailureError(task.Name, fmt.Sprintf("failed: %v", err), output.String())
	}
	if !result.OK() {
		return devTaskFailureError(task.Name, fmt.Sprintf("failed with exit code %d", result.ExitCode), output.String())
	}
	return nil
}

// runDevEnvironmentFixtureTask bounds retries to Windows DLL initialization failures with no command output, never application failures.
func runDevEnvironmentFixtureTask(t *testing.T, goos string, run func() (execx.Result, error)) (execx.Result, error) {
	t.Helper()
	const dllInitFailed = 0xC0000142
	for attempt := 1; ; attempt++ {
		result, err := run()
		if goos != "windows" || err != nil || uint32(result.ExitCode) != dllInitFailed || result.Stdout != "" || result.Stderr != "" || attempt == 3 {
			return result, err
		}
		t.Logf("Windows shell failed before script startup (STATUS_DLL_INIT_FAILED), retry %d/2", attempt)
		time.Sleep(25 * time.Millisecond)
	}
}

// TestDevEnvironmentFixtureOnlyRetriesWindowsShellInitialization keeps infrastructure recovery from hiding real hook failures.
func TestDevEnvironmentFixtureOnlyRetriesWindowsShellInitialization(t *testing.T) {
	failure := errors.New("could not launch")
	status := uint32(0xC0000142)
	dllExit := int(status)
	for _, test := range []struct {
		name       string
		goos       string
		result     execx.Result
		err        error
		persistent bool
		wantCalls  int
	}{
		{name: "Windows loader recovers", goos: "windows", result: execx.Result{ExitCode: dllExit}, wantCalls: 2},
		{name: "Windows loader remains broken", goos: "windows", result: execx.Result{ExitCode: dllExit}, persistent: true, wantCalls: 3},
		{name: "signed Windows loader status", goos: "windows", result: execx.Result{ExitCode: -1073741502}, wantCalls: 2},
		{name: "Unix failure", goos: "linux", result: execx.Result{ExitCode: dllExit}, wantCalls: 1},
		{name: "ordinary failure", goos: "windows", result: execx.Result{ExitCode: 19}, wantCalls: 1},
		{name: "execution error", goos: "windows", result: execx.Result{ExitCode: dllExit}, err: failure, wantCalls: 1},
		{name: "stdout already produced", goos: "windows", result: execx.Result{ExitCode: dllExit, Stdout: "hook started"}, wantCalls: 1},
		{name: "stderr already produced", goos: "windows", result: execx.Result{ExitCode: dllExit, Stderr: "hook failed"}, wantCalls: 1},
		{name: "success", goos: "windows", wantCalls: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			result, err := runDevEnvironmentFixtureTask(t, test.goos, func() (execx.Result, error) {
				calls++
				if calls == 1 || test.persistent {
					return test.result, test.err
				}
				return execx.Result{}, nil
			})
			if calls != test.wantCalls {
				t.Fatalf("task calls = %d, want %d", calls, test.wantCalls)
			}
			if calls == 1 || test.persistent {
				if result.ExitCode != test.result.ExitCode || !errors.Is(err, test.err) {
					t.Fatalf("failure changed: result=%#v error=%v", result, err)
				}
			} else if err != nil || !result.OK() {
				t.Fatalf("recovered task failed: result=%#v error=%v", result, err)
			}
		})
	}
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
