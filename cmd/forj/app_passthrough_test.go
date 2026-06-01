package main

import (
	"errors"
	"os"
	"testing"
)

func TestLocalAppHelp(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()
	writeGeneratedApp(t, `package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--help" {
		fmt.Println("Usage: app <command>")
		return
	}
}
`)

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
	writeGeneratedApp(t, `package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--help" {
		os.Exit(2)
	}
	fmt.Println("Usage: app fallback")
}
`)

	help, ok := localAppHelp()
	if !ok {
		t.Fatal("expected fallback local app help")
	}
	if help != "Usage: app fallback" {
		t.Fatalf("unexpected fallback help output: %q", help)
	}
}

func TestShouldDelegateToAppCommand(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()
	writeGeneratedAppMarker(t)

	parseErr := errors.New("unexpected argument route:list")
	if !shouldDelegateToAppCommand([]string{"route:list"}, parseErr) {
		t.Fatal("expected unresolved app command to pass through")
	}
	if shouldDelegateToAppCommand([]string{"--bad"}, parseErr) {
		t.Fatal("expected flags to remain owned by forj")
	}
	if shouldDelegateToAppCommand([]string{"route:list"}, errors.New("invalid flag --bad")) {
		t.Fatal("expected non-command parser errors to remain owned by forj")
	}
}

func TestShouldDelegateRequiresGeneratedApp(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()

	if shouldDelegateToAppCommand([]string{"route:list"}, errors.New("unexpected argument route:list")) {
		t.Fatal("expected delegation to require a generated app")
	}
}

func TestIsGeneratedAppDirRequiresProjectMarkers(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()

	if isGeneratedAppDir() {
		t.Fatal("expected empty directory not to be a generated app")
	}
	if err := os.WriteFile(".goforj.yml", []byte("project_name: Test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if isGeneratedAppDir() {
		t.Fatal("expected go.mod to be required")
	}
	if err := os.WriteFile("go.mod", []byte("module example.com/testapp\n\ngo 1.24\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if !isGeneratedAppDir() {
		t.Fatal("expected generated app markers")
	}
}

func TestLocalAppEnvRemovesOnlyCLIDefaults(t *testing.T) {
	previousDefaults := cliDefaultedEnv
	previousAppName, hadAppName := os.LookupEnv("APP_NAME")
	previousAppEnv, hadAppEnv := os.LookupEnv("APP_ENV")
	previousStdoutIsTerminal := localAppStdoutIsTerminal
	defer func() {
		cliDefaultedEnv = previousDefaults
		localAppStdoutIsTerminal = previousStdoutIsTerminal
		restoreEnv("APP_NAME", previousAppName, hadAppName)
		restoreEnv("APP_ENV", previousAppEnv, hadAppEnv)
	}()

	cliDefaultedEnv = map[string]bool{}
	localAppStdoutIsTerminal = func() bool { return false }
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
	if !envHasEntry(env, "FORJ_COMMAND_PREFIX=forj") {
		t.Fatal("expected delegated app env to include the forj command prefix")
	}
}

func TestLocalAppEnvForcesColorWhenParentStdoutIsTerminal(t *testing.T) {
	previousStdoutIsTerminal := localAppStdoutIsTerminal
	previousColor, hadColor := os.LookupEnv("CLICOLOR_FORCE")
	previousNoColor, hadNoColor := os.LookupEnv("NO_COLOR")
	defer func() {
		localAppStdoutIsTerminal = previousStdoutIsTerminal
		restoreEnv("CLICOLOR_FORCE", previousColor, hadColor)
		restoreEnv("NO_COLOR", previousNoColor, hadNoColor)
	}()

	localAppStdoutIsTerminal = func() bool { return true }
	_ = os.Unsetenv("CLICOLOR_FORCE")
	_ = os.Unsetenv("NO_COLOR")

	env := localAppEnv()
	if !envHasEntry(env, "CLICOLOR_FORCE=1") {
		t.Fatalf("expected delegated app env to force color when parent stdout is a terminal")
	}
}

func TestLocalAppEnvRespectsColorOptOut(t *testing.T) {
	previousStdoutIsTerminal := localAppStdoutIsTerminal
	previousColor, hadColor := os.LookupEnv("CLICOLOR_FORCE")
	previousNoColor, hadNoColor := os.LookupEnv("NO_COLOR")
	defer func() {
		localAppStdoutIsTerminal = previousStdoutIsTerminal
		restoreEnv("CLICOLOR_FORCE", previousColor, hadColor)
		restoreEnv("NO_COLOR", previousNoColor, hadNoColor)
	}()

	localAppStdoutIsTerminal = func() bool { return true }
	_ = os.Unsetenv("CLICOLOR_FORCE")
	_ = os.Setenv("NO_COLOR", "1")

	env := localAppEnv()
	if envHasEntry(env, "CLICOLOR_FORCE=1") {
		t.Fatalf("expected NO_COLOR to prevent delegated app color forcing")
	}
}

func TestLocalAppEnvRespectsCommandPrefixOverride(t *testing.T) {
	previousPrefix, hadPrefix := os.LookupEnv("FORJ_COMMAND_PREFIX")
	defer restoreEnv("FORJ_COMMAND_PREFIX", previousPrefix, hadPrefix)

	_ = os.Setenv("FORJ_COMMAND_PREFIX", "./bin/app")

	env := localAppEnv()
	if !envHasEntry(env, "FORJ_COMMAND_PREFIX=./bin/app") {
		t.Fatalf("expected delegated app env to preserve existing command prefix")
	}
	if envHasEntry(env, "FORJ_COMMAND_PREFIX=forj") {
		t.Fatalf("expected existing command prefix to prevent forj override")
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

func writeGeneratedApp(t *testing.T, mainSource string) {
	t.Helper()

	writeGeneratedAppMarker(t)
	if err := os.WriteFile("main.go", []byte(mainSource), 0644); err != nil {
		t.Fatal(err)
	}
}

func writeGeneratedAppMarker(t *testing.T) {
	t.Helper()

	if err := os.WriteFile(".goforj.yml", []byte("project_name: Test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("go.mod", []byte("module example.com/testapp\n\ngo 1.24\n"), 0644); err != nil {
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
