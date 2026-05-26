package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalAppHelp(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()
	writeAppScript(t, "echo 'Usage: app <command>'\n")

	help, ok := localAppHelp()
	if !ok {
		t.Fatal("expected local app help")
	}
	if help != "Usage: app <command>" {
		t.Fatalf("unexpected help output: %q", help)
	}
}

func TestLocalAppHelpFallsBackToNoArgs(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()
	writeAppScript(t, "if [ \"$1\" = \"--help\" ]; then exit 2; fi\necho 'Usage: app fallback'\n")

	help, ok := localAppHelp()
	if !ok {
		t.Fatal("expected fallback local app help")
	}
	if help != "Usage: app fallback" {
		t.Fatalf("unexpected fallback help output: %q", help)
	}
}

func TestShouldPassThroughToLocalApp(t *testing.T) {
	restore := chdirTemp(t)
	writeAppScript(t, "exit 0\n")
	defer restore()

	parseErr := errors.New("unexpected argument route:list")
	if !shouldPassThroughToLocalApp([]string{"route:list"}, parseErr) {
		t.Fatal("expected unresolved app command to pass through")
	}
	if shouldPassThroughToLocalApp([]string{"--bad"}, parseErr) {
		t.Fatal("expected flags to remain owned by forj")
	}
	if shouldPassThroughToLocalApp([]string{"route:list"}, errors.New("invalid flag --bad")) {
		t.Fatal("expected non-command parser errors to remain owned by forj")
	}
}

func TestShouldPassThroughRequiresLocalApp(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()

	if shouldPassThroughToLocalApp([]string{"route:list"}, errors.New("unexpected argument route:list")) {
		t.Fatal("expected pass-through to require ./bin/app")
	}
}

func TestRunLocalAppPassesArguments(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()
	writeAppScript(t, "printf '%s\\n' \"$*\" > app.args\n")

	if err := runLocalApp([]string{"api", "--port", "3000"}); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile("app.args")
	if err != nil {
		t.Fatal(err)
	}
	if string(args) != "api --port 3000\n" {
		t.Fatalf("unexpected passed args: %q", string(args))
	}
}

func TestLocalAppEnvRemovesOnlyCLIDefaults(t *testing.T) {
	previousDefaults := cliDefaultedEnv
	previousAppName, hadAppName := os.LookupEnv("APP_NAME")
	previousAppEnv, hadAppEnv := os.LookupEnv("APP_ENV")
	defer func() {
		cliDefaultedEnv = previousDefaults
		restoreEnv("APP_NAME", previousAppName, hadAppName)
		restoreEnv("APP_ENV", previousAppEnv, hadAppEnv)
	}()

	cliDefaultedEnv = map[string]bool{}
	_ = os.Unsetenv("APP_NAME")
	_ = os.Unsetenv("APP_ENV")

	setCLIDefaultEnv("APP_NAME", "GoForj")
	_ = os.Setenv("APP_ENV", "testing")

	env := localAppEnv()
	if envHasKey(env, "APP_NAME") {
		t.Fatal("expected CLI-defaulted APP_NAME to be removed")
	}
	if !envHasEntry(env, "APP_ENV=testing") {
		t.Fatal("expected caller-provided APP_ENV to be preserved")
	}
}

func chdirTemp(t *testing.T) func() {
	t.Helper()

	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	return func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatal(err)
		}
	}
}

func writeAppScript(t *testing.T, body string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Join("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n" + body
	if err := os.WriteFile(filepath.Join("bin", "app"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func restoreEnv(key, value string, existed bool) {
	if !existed {
		_ = os.Unsetenv(key)
		return
	}
	_ = os.Setenv(key, value)
}

func envHasKey(env []string, key string) bool {
	for _, entry := range env {
		if entry == key || len(entry) > len(key) && entry[:len(key)+1] == key+"=" {
			return true
		}
	}
	return false
}

func envHasEntry(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}
