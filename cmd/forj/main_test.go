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

func TestResolveAppPrefixUsesConventionalSourceApp(t *testing.T) {
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

	appName, remaining, ok := resolveAppPrefix([]string{"reporting", "build"}, true)
	if !ok || appName != "reporting" || len(remaining) != 1 || remaining[0] != "build" {
		t.Fatalf("app prefix = (%q, %#v, %t), want reporting, [build], true", appName, remaining, ok)
	}
}

func TestAppPrefixedNativeCommandStaysInSourceMode(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()
	writeGeneratedAppMarker(t)
	writeSourceApp(t, "billing")
	if err := os.MkdirAll("bin", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("bin", "billing"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	previousNativeNames := cliNativeCommandNames
	defer func() { cliNativeCommandNames = previousNativeNames }()
	cliNativeCommandNames = []string{"build", "dev"}

	appName, remaining, ok := resolveAppPrefix([]string{"billing", "build", "-o", "./bin/billing"}, true)
	if !ok || appName != "billing" {
		t.Fatalf("app prefix = (%q, %#v, %t), want billing app", appName, remaining, ok)
	}
	if !shouldRunAppNativeCommand(remaining) {
		t.Fatalf("expected app-prefixed native command to stay source-scoped, got %#v", remaining)
	}
}

func TestAppPrefixedBackupCommandStaysInSourceMode(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()
	writeGeneratedAppMarker(t)
	writeSourceApp(t, "billing")
	previousNativeNames := cliNativeCommandNames
	defer func() { cliNativeCommandNames = previousNativeNames }()
	cliNativeCommandNames = []string{"backup:create", "backup:list", "backup:plan", "backup:prune", "backup:restore", "backup:verify"}

	appName, remaining, ok := resolveAppPrefix([]string{"billing", "backup:create"}, true)
	if !ok || appName != "billing" {
		t.Fatalf("app prefix = (%q, %#v, %t), want billing app", appName, remaining, ok)
	}
	if !shouldRunAppNativeCommand(remaining) {
		t.Fatalf("expected app-prefixed backup command to stay source-scoped, got %#v", remaining)
	}
}

func TestBackupCommandUsesFrameworkProcessWithAppEnvironment(t *testing.T) {
	if !shouldRunFrameworkCommandWithAppEnv([]string{"backup:create"}) {
		t.Fatal("expected backup command to remain framework-owned")
	}
	if shouldRunFrameworkCommandWithAppEnv([]string{"route:list"}) {
		t.Fatal("expected generated app command to remain app-owned")
	}
	if got := appEnvPrefix("customer-portal"); got != "CUSTOMER_PORTAL" {
		t.Fatalf("app env prefix = %q, want CUSTOMER_PORTAL", got)
	}
}

func TestAppPrefixedSourceCommandWinsOverBuiltBinary(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()
	writeGeneratedAppMarker(t)
	writeSourceApp(t, "billing")
	if err := os.MkdirAll("bin", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("bin", "billing"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	previousNativeNames := cliNativeCommandNames
	defer func() { cliNativeCommandNames = previousNativeNames }()
	cliNativeCommandNames = []string{"build", "dev"}

	if !shouldRunAppThroughSource("billing", []string{"make:controller", "checkout"}, true) {
		t.Fatal("expected source app commands to avoid stale built app binaries")
	}
}

func TestAppPrefixedBinaryOnlyCommandUsesBuiltBinary(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()
	if err := os.MkdirAll("bin", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("bin", "billing"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	previousNativeNames := cliNativeCommandNames
	defer func() { cliNativeCommandNames = previousNativeNames }()
	cliNativeCommandNames = []string{"build", "dev"}

	if shouldRunAppThroughSource("billing", []string{"route:list"}, false) {
		t.Fatal("expected binary-only app commands to delegate to the built app binary")
	}
}

func TestResolveAppPrefixPreservesNativeCommandPrecedence(t *testing.T) {
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

	_, _, ok := resolveAppPrefix([]string{"build"}, true)
	if ok {
		t.Fatal("expected native build command to keep precedence over a conventional app")
	}
}

func TestResolveAppPrefixPreservesNativeCommandPrecedenceOverBinary(t *testing.T) {
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

	_, _, ok := resolveAppPrefix([]string{"build"}, true)
	if ok {
		t.Fatal("expected native build command to keep precedence over a built app binary")
	}
}

func TestConventionalAppHelpIncludesSourceAndBinaryApps(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()
	writeGeneratedAppMarker(t)
	writeSourceApp(t, "admin")
	writeSourceApp(t, "billing")
	writeSourceApp(t, "build")
	writeSourceApp(t, "wire")
	if err := os.MkdirAll("bin", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("bin", "reporting"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	previousNativeNames := cliNativeCommandNames
	defer func() { cliNativeCommandNames = previousNativeNames }()
	cliNativeCommandNames = []string{"build"}

	apps := conventionalAppHelpApps(true)
	want := []string{"admin", "app", "billing", "reporting"}
	if len(apps) != len(want) {
		t.Fatalf("help apps = %#v, want %#v", apps, want)
	}
	for i := range want {
		if apps[i] != want[i] {
			t.Fatalf("help apps = %#v, want %#v", apps, want)
		}
	}
}

func TestConventionalAppHelpSkipsNonGeneratedProjects(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()
	writeSourceApp(t, "billing")

	if apps := conventionalAppHelpApps(false); len(apps) != 0 {
		t.Fatalf("expected no generated app help entries, got %#v", apps)
	}
}

func TestRunAppHelpForAppShellsThroughRootBinary(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()

	scriptPath, err := filepath.Abs("fake-forj")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho \"$1|$2|$FORJ_APP\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	previousArg0 := os.Args[0]
	defer func() { os.Args[0] = previousArg0 }()
	os.Args[0] = scriptPath

	output, err := runAppHelpForApp("billing", true)
	if err != nil {
		t.Fatalf("run app help: %v\n%s", err, output)
	}
	if output != "billing|--help|billing\n" {
		t.Fatalf("unexpected help command output: %q", output)
	}
}

func TestRunAppHelpForAppMarksMultiAppHelp(t *testing.T) {
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

	output, err := runAppHelpForApp("app", true)
	if err != nil {
		t.Fatalf("run app help: %v\n%s", err, output)
	}
	if output != "1\n" {
		t.Fatalf("unexpected multi-app marker: %q", output)
	}
}

func TestPrintGeneratedAppHelpIgnoresUnavailableAppHelp(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()

	scriptPath, err := filepath.Abs("fake-forj")
	if err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
case "$1" in
  app)
    printf '%s\n\n%s\n  %s\n' 'test · app' 'app' 'about    Show environment'
    exit 0
    ;;
  ship-smoke)
    echo 'custom binary has no framework help' >&2
    exit 1
    ;;
esac
exit 1
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	previousArg0 := os.Args[0]
	defer func() { os.Args[0] = previousArg0 }()
	os.Args[0] = scriptPath

	output := captureStdout(t, func() {
		if err := printGeneratedAppHelp([]string{"app", "ship-smoke"}); err != nil {
			t.Fatalf("print app help: %v", err)
		}
	})
	if !strings.Contains(output, "test · app") {
		t.Fatalf("expected generated app help to be printed, got:\n%s", output)
	}
	if strings.Contains(output, "ship-smoke") || strings.Contains(output, "custom binary") {
		t.Fatalf("expected unavailable app help to be ignored, got:\n%s", output)
	}
}

func TestGeneratedAppHelpResultsIgnoreCustomHelp(t *testing.T) {
	results := generatedAppHelpResults([]appHelpResult{
		{
			app: "app",
			output: strings.Join([]string{
				"test · app",
				"",
				"app",
				"  about    Show environment",
				"",
			}, "\n"),
		},
		{
			app:    "ship-smoke",
			output: "Usage: ship-smoke [options]\n",
		},
	})

	if len(results) != 1 || results[0].app != "app" {
		t.Fatalf("filtered app help = %#v, want only app", results)
	}
}

func TestCompactGeneratedAppHelpDeduplicatesSharedCommands(t *testing.T) {
	results := []appHelpResult{
		{app: "app", output: strings.Join([]string{
			"test · app",
			"",
			"app",
			"  about    Show environment",
			"  health   Query readiness",
			"monitor",
			"  seed     Seed monitors",
			"",
		}, "\n")},
		{app: "billing", output: strings.Join([]string{
			"test · billing",
			"",
			"app",
			"  about    Show environment",
			"  health   Query readiness",
			"queue",
			"  invoice  Process invoices",
			"",
		}, "\n")},
		{app: "reporting", output: strings.Join([]string{
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

func TestPrintAppUsageHelpOnlyForMultiApp(t *testing.T) {
	single := captureStdout(t, func() {
		printAppUsageHelp([]string{"app"})
	})
	if single != "" {
		t.Fatalf("expected no app usage for single app, got:\n%s", single)
	}

	multi := captureStdout(t, func() {
		printAppUsageHelp([]string{"app", "billing"})
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
			t.Fatalf("expected app usage help to include %q, got:\n%s", want, multi)
		}
	}
}

func TestWithAppEnvOverridesExistingAppIdentity(t *testing.T) {
	env := withAppEnv([]string{
		"FORJ_COMMAND_PREFIX=forj billing",
		"FORJ_APP=billing",
		"KEEP=value",
	}, "reporting")

	for _, want := range []string{
		"FORJ_COMMAND_PREFIX=forj reporting",
		"FORJ_APP=reporting",
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

func writeSourceApp(t *testing.T, appName string) {
	t.Helper()

	writeMain := filepath.Join("cmd", appName, "main.go")
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
