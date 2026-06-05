package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestShouldDelegateToAppCommand(t *testing.T) {
	parseErr := errors.New("unexpected argument route:list")
	if !shouldDelegateToAppCommand([]string{"route:list"}, parseErr, true) {
		t.Fatal("expected unresolved app command to pass through")
	}
	if shouldDelegateToAppCommand([]string{"--bad"}, parseErr, true) {
		t.Fatal("expected flags to remain owned by forj")
	}
	if shouldDelegateToAppCommand([]string{"route:list"}, errors.New("invalid flag --bad"), true) {
		t.Fatal("expected non-command parser errors to remain owned by forj")
	}
	if shouldDelegateToAppCommand([]string{"route:list"}, parseErr, false) {
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

func TestAppRootHelpArgsUsesAppHelp(t *testing.T) {
	args := appRootHelpArgs()
	if len(args) != 1 || args[0] != "--help" {
		t.Fatalf("expected app root help args to request app help, got %#v", args)
	}
}

func TestResolveTargetPrefixUsesConventionalSourceTarget(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()
	writeGeneratedAppMarker(t)
	writeMain := filepath.Join("cmd", "reporting", "main.go")
	if err := os.MkdirAll(filepath.Dir(writeMain), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(writeMain, []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	previousNativeNames := cliNativeCommandNames
	defer func() { cliNativeCommandNames = previousNativeNames }()
	cliNativeCommandNames = []string{"build", "dev"}

	target, remaining, ok := resolveTargetPrefix([]string{"reporting", "build"}, true)
	if !ok || target != "reporting" || len(remaining) != 1 || remaining[0] != "build" {
		t.Fatalf("target prefix = (%q, %#v, %t), want reporting, [build], true", target, remaining, ok)
	}
}

func TestResolveTargetPrefixPreservesNativeCommandPrecedence(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()
	writeGeneratedAppMarker(t)
	writeMain := filepath.Join("cmd", "build", "main.go")
	if err := os.MkdirAll(filepath.Dir(writeMain), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(writeMain, []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	previousNativeNames := cliNativeCommandNames
	defer func() { cliNativeCommandNames = previousNativeNames }()
	cliNativeCommandNames = []string{"build"}

	_, _, ok := resolveTargetPrefix([]string{"build"}, true)
	if ok {
		t.Fatal("expected native build command to keep precedence over a conventional target")
	}
}

func TestResolveTargetPrefixPreservesNativeCommandPrecedenceOverBinary(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()
	writeGeneratedAppMarker(t)
	if err := os.MkdirAll("bin", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("bin", "build"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	previousNativeNames := cliNativeCommandNames
	defer func() { cliNativeCommandNames = previousNativeNames }()
	cliNativeCommandNames = []string{"build"}

	_, _, ok := resolveTargetPrefix([]string{"build"}, true)
	if ok {
		t.Fatal("expected native build command to keep precedence over a built target binary")
	}
}

func TestWithTargetEnvOverridesExistingTargetIdentity(t *testing.T) {
	env := withTargetEnv([]string{
		"FORJ_COMMAND_PREFIX=forj billing",
		"FORJ_APP_TARGET=billing",
		"APP_TARGET=billing",
		"KEEP=value",
	}, "reporting")

	for _, want := range []string{
		"FORJ_COMMAND_PREFIX=forj reporting",
		"FORJ_APP_TARGET=reporting",
		"APP_TARGET=reporting",
		"KEEP=value",
	} {
		if !envHasEntry(env, want) {
			t.Fatalf("expected env to include %q, got %#v", want, env)
		}
	}
}

func TestDelegatedAppEnvRemovesOnlyCLIDefaults(t *testing.T) {
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

	env := delegatedAppEnv()
	if envHasEntry(env, "APP_NAME=GoForj") {
		t.Fatal("expected CLI-defaulted APP_NAME to be removed")
	}
	if !envHasEntry(env, "APP_ENV=testing") {
		t.Fatal("expected caller-provided APP_ENV to be preserved")
	}
	if !envHasEntry(env, "FORJ_COMMAND_PREFIX=forj") {
		t.Fatal("expected delegated app env to include the forj command prefix")
	}
}

func TestDelegatedAppEnvRespectsCommandPrefixOverride(t *testing.T) {
	previousPrefix, hadPrefix := os.LookupEnv("FORJ_COMMAND_PREFIX")
	defer restoreEnv("FORJ_COMMAND_PREFIX", previousPrefix, hadPrefix)

	_ = os.Setenv("FORJ_COMMAND_PREFIX", "./bin/app")

	env := delegatedAppEnv()
	if !envHasEntry(env, "FORJ_COMMAND_PREFIX=./bin/app") {
		t.Fatalf("expected delegated app env to preserve existing command prefix")
	}
	if envHasEntry(env, "FORJ_COMMAND_PREFIX=forj") {
		t.Fatalf("expected existing command prefix to prevent forj override")
	}
}

func TestDelegatedAppEnvIncludesNativeCommandNames(t *testing.T) {
	previousNativeNames := cliNativeCommandNames
	defer func() {
		cliNativeCommandNames = previousNativeNames
	}()

	cliNativeCommandNames = []string{"build", "dev"}

	env := delegatedAppEnv()
	if !envHasEntry(env, "FORJ_NATIVE_COMMAND_NAMES=build,dev") {
		t.Fatalf("expected delegated app env to include native command names, got %#v", env)
	}
}

func TestShouldForceDelegatedAppColor(t *testing.T) {
	previousNoColor, hadNoColor := os.LookupEnv("NO_COLOR")
	previousForce, hadForce := os.LookupEnv("CLICOLOR_FORCE")
	defer func() {
		restoreEnv("NO_COLOR", previousNoColor, hadNoColor)
		restoreEnv("CLICOLOR_FORCE", previousForce, hadForce)
	}()

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("CLICOLOR_FORCE")
	if !shouldForceDelegatedAppColor(true) {
		t.Fatal("expected TTY delegated app output to force color")
	}
	if shouldForceDelegatedAppColor(false) {
		t.Fatal("expected non-TTY delegated app output not to force color")
	}

	_ = os.Setenv("NO_COLOR", "1")
	if shouldForceDelegatedAppColor(true) {
		t.Fatal("expected NO_COLOR to prevent forced color")
	}

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Setenv("CLICOLOR_FORCE", "1")
	if shouldForceDelegatedAppColor(true) {
		t.Fatal("expected existing CLICOLOR_FORCE to prevent duplicate forced color")
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

func envHasEntry(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}
