package forj

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/managedenv"
	"github.com/goforj/goforj/project"
)

// TestManagedDevEnvironmentSurvivesDotenvReloadsAndPreTasks covers the parent process and inherited task boundary.
func TestManagedDevEnvironmentSurvivesDotenvReloadsAndPreTasks(t *testing.T) {
	if os.Getenv("GO_WANT_MANAGED_DEV_ENV_HELPER") == "1" {
		runManagedDevEnvironmentHelper(t)
		return
	}

	root := t.TempDir()
	logPath := filepath.Join(root, "pre-task.log")
	writeManagedDevEnvFile(t, filepath.Join(root, ".env"), "dotenv-token", "0.0.0.0", "from-envfile-one")
	command := exec.Command(os.Args[0], "-test.run=^TestManagedDevEnvironmentSurvivesDotenvReloadsAndPreTasks$")
	command.Dir = root
	command.Env = managedDevHelperEnvironment(map[string]string{
		"GO_WANT_MANAGED_DEV_ENV_HELPER": "1",
		"GO_MANAGED_DEV_ENV_LOG":         logPath,
		managedenv.MetadataKey:           "HARBOR_EMPTY,HARBOR_TOKEN,IP_ADDRESS",
		"HARBOR_EMPTY":                   "",
		"HARBOR_TOKEN":                   "launcher-token",
		"IP_ADDRESS":                     "127.18.0.11",
		"UNLISTED_AMBIENT":               "from-shell",
	})
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("managed environment helper failed: %v\n%s", err, output)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read pre-task log: %v", err)
	}
	want := strings.Join([]string{
		"|launcher-token|127.18.0.11|from-envfile-one|",
		"|launcher-token|127.18.0.11|from-envfile-two|",
	}, "\n") + "\n"
	if string(content) != want {
		t.Fatalf("pre-task environment = %q, want %q", content, want)
	}
}

// runManagedDevEnvironmentHelper exercises real env Load/Reload behavior in an isolated process.
func runManagedDevEnvironmentHelper(t *testing.T) {
	managedEnv, err := managedenv.Capture()
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if err := loadDevEnvironment(false, managedEnv); err != nil {
		t.Fatalf("initial load: %v", err)
	}
	assertManagedDevProcessEnvironment(t, "from-envfile-one")
	if err := runManagedDevPreTask(os.Getenv("GO_MANAGED_DEV_ENV_LOG")); err != nil {
		t.Fatalf("initial pre task: %v", err)
	}
	writeManagedDevEnvFile(t, ".env", "reloaded-dotenv-token", "0.0.0.0", "from-envfile-two")
	if err := loadDevEnvironment(true, managedEnv); err != nil {
		t.Fatalf("reload: %v", err)
	}
	assertManagedDevProcessEnvironment(t, "from-envfile-two")
	if err := runManagedDevPreTask(os.Getenv("GO_MANAGED_DEV_ENV_LOG")); err != nil {
		t.Fatalf("reloaded pre task: %v", err)
	}
}

// assertManagedDevProcessEnvironment distinguishes named launcher values from ordinary ambient precedence.
func assertManagedDevProcessEnvironment(t *testing.T, unlistedValue string) {
	t.Helper()
	if got := os.Getenv("HARBOR_EMPTY"); got != "" {
		t.Fatalf("HARBOR_EMPTY = %q", got)
	}
	if got := os.Getenv("HARBOR_TOKEN"); got != "launcher-token" {
		t.Fatalf("HARBOR_TOKEN = %q", got)
	}
	if got := os.Getenv("IP_ADDRESS"); got != "127.18.0.11" {
		t.Fatalf("IP_ADDRESS = %q", got)
	}
	if got := os.Getenv("UNLISTED_AMBIENT"); got != unlistedValue {
		t.Fatalf("UNLISTED_AMBIENT = %q, want %q", got, unlistedValue)
	}
	if _, ok := os.LookupEnv(managedenv.MetadataKey); ok {
		t.Fatalf("%s leaked after dotenv loading", managedenv.MetadataKey)
	}
}

// runManagedDevPreTask records the environment inherited by an ordinary configured task.
func runManagedDevPreTask(logPath string) error {
	command := "printf '%s|%s|%s|%s|%s\\n' \"$HARBOR_EMPTY\" \"$HARBOR_TOKEN\" \"$IP_ADDRESS\" \"$UNLISTED_AMBIENT\" \"$" + managedenv.MetadataKey + "\" >> " + shellSingleQuote(logPath)
	return runDevTasks("Managed environment pre task", []project.DevTask{{Name: "Record environment", Cmd: command}})
}

// writeManagedDevEnvFile replaces conflicting dotenv values between the initial load and reload.
func writeManagedDevEnvFile(t *testing.T, path string, token string, address string, unlisted string) {
	t.Helper()
	contents := strings.Join([]string{
		"HARBOR_EMPTY=dotenv-nonempty",
		"HARBOR_TOKEN=" + token,
		"IP_ADDRESS=" + address,
		"UNLISTED_AMBIENT=" + unlisted,
		managedenv.MetadataKey + "=SHOULD_NOT_SURVIVE",
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// managedDevHelperEnvironment creates a deterministic child env without stale values from the parent test process.
func managedDevHelperEnvironment(overrides map[string]string) []string {
	removed := map[string]struct{}{
		managedenv.MetadataKey: {}, "HARBOR_EMPTY": {}, "HARBOR_TOKEN": {}, "IP_ADDRESS": {}, "UNLISTED_AMBIENT": {},
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

// TestCompileDevWatchersForcesManagedValuesAndScopesMetadata verifies every watcher gets values but only Apps get restoration metadata.
func TestCompileDevWatchersForcesManagedValuesAndScopesMetadata(t *testing.T) {
	managedEnv := captureManagedEnvironmentForTest(t, "launcher-token", "127.18.0.12")
	config := &project.Config{Dev: project.DevConfig{
		Apps: map[string]project.DevApp{project.DefaultAppName: {
			Build: &project.DevAppCommand{Env: map[string]string{
				"HARBOR_TOKEN": "build-config", "IP_ADDRESS": "0.0.0.0", "API_HTTP_PORT": "3001", managedenv.MetadataKey: "BAD",
			}},
			Run: &project.DevAppCommand{Exec: "run", Shorthand: true, Env: map[string]string{
				"HARBOR_TOKEN": "run-config", "IP_ADDRESS": "0.0.0.0", "API_HTTP_HOST": "0.0.0.0", "API_HTTP_PORT": "3002",
			}},
			SPAs: map[string]project.DevSPA{"portal": {Path: "./frontend"}},
		}},
		Watches: []project.DevWatch{{
			Name: "Custom", Include: []string{".go"}, Exec: "echo custom",
			Env: map[string]string{"HARBOR_TOKEN": "custom-config", "IP_ADDRESS": "0.0.0.0", "API_HTTP_PORT": "3003", managedenv.MetadataKey: "BAD"},
		}},
	}}

	watchers, err := compileDevWatchersWithManagedEnvironment(config, managedEnv)
	if err != nil {
		t.Fatalf("compileDevWatchersWithManagedEnvironment() error = %v", err)
	}
	wantPorts := map[devWatcherKind]string{
		devWatcherAppBuild: "3001",
		devWatcherAppRun:   "3002",
		devWatcherCustom:   "3003",
	}
	seenKinds := map[devWatcherKind]bool{}
	for _, watcher := range watchers {
		seenKinds[watcher.Kind] = true
		if watcher.Command.Env["HARBOR_TOKEN"] != "launcher-token" || watcher.Command.Env["IP_ADDRESS"] != "127.18.0.12" {
			t.Fatalf("%s environment did not force managed values: %#v", watcher.Name, watcher.Command.Env)
		}
		if wantPort, ok := wantPorts[watcher.Kind]; ok && watcher.Command.Env["API_HTTP_PORT"] != wantPort {
			t.Fatalf("%s API_HTTP_PORT = %q, want %q", watcher.Name, watcher.Command.Env["API_HTTP_PORT"], wantPort)
		}
		marker, hasMarker := watcher.Command.Env[managedenv.MetadataKey]
		if watcher.Kind == devWatcherAppRun {
			if !hasMarker || marker != "API_HTTP_HOST,HARBOR_TOKEN,IP_ADDRESS" {
				t.Fatalf("App run marker = %q, present %v", marker, hasMarker)
			}
			if watcher.Command.Env["API_HTTP_HOST"] != "127.18.0.12" {
				t.Fatalf("App run API_HTTP_HOST = %q", watcher.Command.Env["API_HTTP_HOST"])
			}
			continue
		}
		if hasMarker {
			t.Fatalf("%s leaked private marker: %#v", watcher.Name, watcher.Command.Env)
		}
	}
	for _, kind := range []devWatcherKind{devWatcherAppBuild, devWatcherAppRun, devWatcherSPABuild, devWatcherCustom} {
		if !seenKinds[kind] {
			t.Fatalf("watcher kind %q was not compiled", kind)
		}
	}
}

// TestManagedEnvironmentPropagatesToInitialAndRebuildCommands exercises both App build entrypoints.
func TestManagedEnvironmentPropagatesToInitialAndRebuildCommands(t *testing.T) {
	managedEnv := captureManagedEnvironmentForTest(t, "launcher-token", "127.18.0.13")
	root := t.TempDir()
	t.Chdir(root)
	logPath := filepath.Join(root, "builds.log")
	command := "printf '%s|%s|%s|%s\\n' \"$HARBOR_TOKEN\" \"$IP_ADDRESS\" \"$API_HTTP_PORT\" \"$" + managedenv.MetadataKey + "\" >> " + shellSingleQuote(logPath)
	config := &project.Config{Dev: project.DevConfig{Apps: map[string]project.DevApp{
		project.DefaultAppName: {Build: &project.DevAppCommand{Exec: command, Env: map[string]string{
			"HARBOR_TOKEN": "configured", "IP_ADDRESS": "0.0.0.0", "API_HTTP_PORT": "3004", managedenv.MetadataKey: "BAD",
		}}},
	}}}

	if err := runDevInitialBuildWithManagedEnvironment(config, io.Discard, io.Discard, managedEnv); err != nil {
		t.Fatalf("initial build: %v", err)
	}
	if err := runDevBuildWithManagedEnvironment(config, io.Discard, io.Discard, managedEnv); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Repeat("launcher-token|127.18.0.13|3004|\n", 2)
	if string(content) != want {
		t.Fatalf("build environments = %q, want %q", content, want)
	}
}

// TestAutoMigrateReceivesGeneratedAppManagedMetadata verifies setup Apps can restore values after their own LoadEnv.
func TestAutoMigrateReceivesGeneratedAppManagedMetadata(t *testing.T) {
	managedEnv := captureManagedEnvironmentForTest(t, "launcher-token", "127.18.0.14")
	root := t.TempDir()
	t.Chdir(root)
	if err := os.MkdirAll("bin", 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "migrate.log")
	script := "#!/bin/sh\nprintf '%s|%s|%s|%s\\n' \"$HARBOR_TOKEN\" \"$IP_ADDRESS\" \"$API_HTTP_HOST\" \"$" + managedenv.MetadataKey + "\" > " + shellSingleQuote(logPath) + "\n"
	if err := os.WriteFile(filepath.Join("bin", "app"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	config := &project.Config{
		Dev:    project.DevConfig{AutoMigrate: true},
		Render: project.RenderConfig{Components: project.Components{DatabaseSQLite: true}},
	}

	if err := runDevAppSetupWithManagedEnvironment(config, io.Discard, io.Discard, managedEnv); err != nil {
		t.Fatalf("runDevAppSetupWithManagedEnvironment() error = %v", err)
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "launcher-token|127.18.0.14|127.18.0.14|API_HTTP_HOST,HARBOR_TOKEN,IP_ADDRESS\n"
	if string(content) != want {
		t.Fatalf("auto-migrate environment = %q, want %q", content, want)
	}
}

// TestDevCmdRejectsMalformedManagedMetadataBeforeProjectWork verifies the private contract fails closed.
func TestDevCmdRejectsMalformedManagedMetadataBeforeProjectWork(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HARBOR_TOKEN", "launcher-token")
	t.Setenv("IP_ADDRESS", "127.18.0.15")
	t.Setenv(managedenv.MetadataKey, "IP_ADDRESS,HARBOR_TOKEN")
	err := (&DevCmd{}).Run()
	if err == nil || !strings.Contains(err.Error(), "must be sorted") {
		t.Fatalf("DevCmd.Run() error = %v, want malformed marker", err)
	}
	if _, ok := os.LookupEnv(managedenv.MetadataKey); ok {
		t.Fatalf("%s survived rejected startup", managedenv.MetadataKey)
	}
}

// captureManagedEnvironmentForTest uses the production contract rather than constructing an impossible Set directly.
func captureManagedEnvironmentForTest(t *testing.T, token string, address string) managedenv.Set {
	t.Helper()
	t.Setenv("HARBOR_TOKEN", token)
	t.Setenv("IP_ADDRESS", address)
	t.Setenv(managedenv.MetadataKey, "HARBOR_TOKEN,IP_ADDRESS")
	set, err := managedenv.Capture()
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	return set
}
