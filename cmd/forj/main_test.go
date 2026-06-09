package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
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

func TestTargetPrefixedNativeCommandStaysInSourceMode(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()
	writeGeneratedAppMarker(t)
	writeSourceTarget(t, "billing")
	if err := os.MkdirAll("bin", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("bin", "billing"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	previousNativeNames := cliNativeCommandNames
	defer func() { cliNativeCommandNames = previousNativeNames }()
	cliNativeCommandNames = []string{"build", "dev"}

	target, remaining, ok := resolveTargetPrefix([]string{"billing", "build", "-o", "./bin/billing"}, true)
	if !ok || target != "billing" {
		t.Fatalf("target prefix = (%q, %#v, %t), want billing target", target, remaining, ok)
	}
	if !shouldRunTargetNativeCommand(remaining) {
		t.Fatalf("expected target-prefixed native command to stay source-scoped, got %#v", remaining)
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

func TestConventionalAppHelpTargetsIncludesSourceAndBinaryTargets(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()
	writeGeneratedAppMarker(t)
	writeSourceTarget(t, "admin")
	writeSourceTarget(t, "billing")
	writeSourceTarget(t, "build")
	writeSourceTarget(t, "wire")
	if err := os.MkdirAll("bin", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("bin", "reporting"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	previousNativeNames := cliNativeCommandNames
	defer func() { cliNativeCommandNames = previousNativeNames }()
	cliNativeCommandNames = []string{"build"}

	targets := conventionalAppHelpTargets(true)
	want := []string{"admin", "app", "billing", "reporting"}
	if len(targets) != len(want) {
		t.Fatalf("help targets = %#v, want %#v", targets, want)
	}
	for i := range want {
		if targets[i] != want[i] {
			t.Fatalf("help targets = %#v, want %#v", targets, want)
		}
	}
}

func TestConventionalAppHelpTargetsSkipsNonGeneratedProjects(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()
	writeSourceTarget(t, "billing")

	if targets := conventionalAppHelpTargets(false); len(targets) != 0 {
		t.Fatalf("expected no generated app help targets, got %#v", targets)
	}
}

func TestRunAppHelpForTargetShellsThroughRootBinary(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()

	scriptPath, err := filepath.Abs("fake-forj")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho \"$1|$2|$FORJ_APP_TARGET|$APP_TARGET\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	previousArg0 := os.Args[0]
	defer func() { os.Args[0] = previousArg0 }()
	os.Args[0] = scriptPath

	output, err := runAppHelpForTarget("billing", true)
	if err != nil {
		t.Fatalf("run app help: %v\n%s", err, output)
	}
	if output != "billing|--help|billing|billing\n" {
		t.Fatalf("unexpected help command output: %q", output)
	}
}

func TestRunAppHelpForTargetMarksMultiAppHelp(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()

	scriptPath, err := filepath.Abs("fake-forj")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho \"$FORJ_MULTI_APP_HELP\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	previousArg0 := os.Args[0]
	defer func() { os.Args[0] = previousArg0 }()
	os.Args[0] = scriptPath

	output, err := runAppHelpForTarget("app", true)
	if err != nil {
		t.Fatalf("run app help: %v\n%s", err, output)
	}
	if output != "1\n" {
		t.Fatalf("unexpected multi-app marker: %q", output)
	}
}

func TestCompactGeneratedAppHelpDeduplicatesSharedCommands(t *testing.T) {
	results := []appHelpResult{
		{target: "app", output: strings.Join([]string{
			"test · app",
			"",
			"app",
			"  about    Show environment",
			"  health   Query readiness",
			"monitor",
			"  seed     Seed monitors",
			"",
		}, "\n")},
		{target: "billing", output: strings.Join([]string{
			"test · billing",
			"",
			"app",
			"  about    Show environment",
			"  health   Query readiness",
			"queue",
			"  invoice  Process invoices",
			"",
		}, "\n")},
		{target: "reporting", output: strings.Join([]string{
			"test · reporting",
			"",
			"app",
			"  about    Show environment",
			"  health   Query readiness",
			"",
		}, "\n")},
	}

	output, ok := compactGeneratedAppHelp(results)
	if !ok {
		t.Fatal("expected compact help output")
	}
	for _, want := range []string{
		"test · available in all apps",
		"  about   Show environment",
		"  health  Query readiness",
		"test · app",
		"  seed  Seed monitors",
		"test · billing",
		"  invoice  Process invoices",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected compact help to include %q, got:\n%s", want, output)
		}
	}
	for _, unexpected := range []string{"app only", "billing only", "test · reporting"} {
		if strings.Contains(output, unexpected) {
			t.Fatalf("expected compact help to omit %q, got:\n%s", unexpected, output)
		}
	}
}

func TestPrintTargetUsageHelpOnlyForMultiApp(t *testing.T) {
	single := captureStdout(t, func() {
		printTargetUsageHelp([]string{"app"})
	})
	if single != "" {
		t.Fatalf("expected no target usage for single app, got:\n%s", single)
	}

	multi := captureStdout(t, func() {
		printTargetUsageHelp([]string{"app", "billing"})
	})
	for _, want := range []string{
		"app usage",
		"forj <app> <command>",
		"Run a command for a specific app",
		"forj <app> build",
		"Build a specific app binary",
		"forj dev",
		"Build and run all apps in development",
	} {
		if !strings.Contains(multi, want) {
			t.Fatalf("expected target usage help to include %q, got:\n%s", want, multi)
		}
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

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	previous := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = write
	defer func() {
		os.Stdout = previous
	}()

	fn()

	if err := write.Close(); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if _, err := io.Copy(&out, read); err != nil {
		t.Fatal(err)
	}
	return out.String()
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

func writeSourceTarget(t *testing.T, target string) {
	t.Helper()

	writeMain := filepath.Join("cmd", target, "main.go")
	if err := os.MkdirAll(filepath.Dir(writeMain), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(writeMain, []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
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

func envHasEntry(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}
