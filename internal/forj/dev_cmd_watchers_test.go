package forj

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/goforj/execx"
	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/project"
)

func TestBuildWatcherExecUsesExec(t *testing.T) {
	script := buildWatcherExec("./bin/app http:serve")
	if script == "" {
		t.Fatal("expected watcher script to be non-empty")
	}
	if want := "__FORJ_WATCHER_TRIGGER__"; !contains(script, want) {
		t.Fatalf("expected watcher trigger marker in script: %q", script)
	}
	if want := "exec ./bin/app http:serve"; !contains(script, want) {
		t.Fatalf("expected exec for watcher command in script: %q", script)
	}
}

func TestDevWatchesForTargetsExpandsDefaultWatchers(t *testing.T) {
	withConventionalTarget(t, "customer-portal")

	watches := []project.DevWatch{
		{
			Name:  "Build App",
			Watch: "-file .go -xfile app/wire/wire_gen\\.go$",
			Exec:  "forj build -o ./bin/app",
		},
		{
			Name:  "Run App",
			Watch: "-file ./bin/app -file .env",
			Exec:  "./bin/app run",
		},
		{
			Name: "Custom",
			Exec: "echo ok",
		},
	}
	got := devWatchesForTargets(&project.Config{}, watches)
	if len(got) != 5 {
		t.Fatalf("expected expanded watchers, got %#v", got)
	}
	if got[0].Name != "Build App" || got[0].Exec != "forj build -o ./bin/app" {
		t.Fatalf("expected default build watcher first, got %#v", got[0])
	}
	if got[1].Name != "Build customer-portal" || got[1].Exec != "forj build -o ./bin/customer-portal" {
		t.Fatalf("expected named build watcher, got %#v", got[1])
	}
	if !strings.Contains(got[1].Watch, "app/customer-portal/wire/wire_gen\\.go$") {
		t.Fatalf("expected target wire exclusion, got %q", got[1].Watch)
	}
	if got[3].Name != "Run customer-portal" || got[3].Watch != "-file ./bin/customer-portal -file .env" || got[3].Exec != "./bin/customer-portal run" {
		t.Fatalf("expected named run watcher, got %#v", got[3])
	}
	if got[3].Env["FORJ_APP_TARGET"] != "customer-portal" || got[3].Env["FORJ_COMMAND_PREFIX"] != "forj customer-portal" {
		t.Fatalf("expected target env, got %#v", got[3].Env)
	}
	if got[4].Name != "Custom" {
		t.Fatalf("expected custom watcher to be preserved, got %#v", got[4])
	}
	if watches[1].Exec != "./bin/app run" {
		t.Fatalf("expected original watches to remain unchanged, got %q", watches[1].Exec)
	}
}

func TestDevWatchesForTargetsCanScopeToExplicitTarget(t *testing.T) {
	t.Setenv("FORJ_APP_TARGET", "customer-portal")
	got := devWatchesForTargets(nil, []project.DevWatch{
		{Name: "Build App", Watch: "-file .go -xfile app/wire/wire_gen\\.go$", Exec: "forj build -o ./bin/app"},
		{Name: "Run App", Watch: "-file ./bin/app -file .env", Exec: "./bin/app run"},
	})
	if len(got) != 2 {
		t.Fatalf("expected target-scoped watchers, got %#v", got)
	}
	if got[0].Name != "Build customer-portal" || got[1].Name != "Run customer-portal" {
		t.Fatalf("expected target-scoped watcher names, got %#v", got)
	}
}

func TestDevWatchesForTargetsDropsRemovedConventionalTarget(t *testing.T) {
	withConventionalTarget(t, "billing")
	base := []project.DevWatch{
		{Name: "Build App", Watch: "-file .go", Exec: "forj build -o ./bin/app"},
		{Name: "Run App", Watch: "-file ./bin/app", Exec: "./bin/app run"},
	}
	if got := devWatchesForTargets(&project.Config{}, base); len(got) != 4 {
		t.Fatalf("expected billing watchers before removal, got %#v", got)
	}
	if err := os.RemoveAll(filepath.Join("cmd", "billing")); err != nil {
		t.Fatalf("remove billing target: %v", err)
	}

	got := devWatchesForTargets(&project.Config{}, base)
	if len(got) != 2 {
		t.Fatalf("expected only default watchers after removal, got %#v", got)
	}
	for _, watch := range got {
		if strings.Contains(watch.Name, "billing") || strings.Contains(watch.Exec, "billing") {
			t.Fatalf("did not expect stale billing watcher after removal: %#v", got)
		}
	}
}

func TestDevBuildCommandsBuildEveryTarget(t *testing.T) {
	withConventionalTarget(t, "customer-portal")

	got := devBuildCommands(&project.Config{
		Dev: project.DevConfig{
			Watches: []project.DevWatch{
				{Name: "Build App", Exec: "FORJ_BUILD=1 forj build --race -o ./bin/app"},
			},
		},
	})
	want := []string{"FORJ_BUILD=1 forj build --race -o ./bin/app ./cmd/app", "FORJ_BUILD=1 forj build --race -o ./bin/customer-portal ./cmd/customer-portal"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected dev build commands: got %#v want %#v", got, want)
	}
}

func TestDevInitialBuildCommandsBuildEveryTarget(t *testing.T) {
	withConventionalTarget(t, "customer-portal")
	if err := os.MkdirAll("bin", 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join("bin", "app"), []byte("binary"), 0o755); err != nil {
		t.Fatalf("write app binary: %v", err)
	}

	got := devInitialBuildCommands(&project.Config{})
	want := []string{"forj build -o ./bin/app ./cmd/app", "forj build -o ./bin/customer-portal ./cmd/customer-portal"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected initial dev build commands: got %#v want %#v", got, want)
	}
}

func TestDevBuildCommandForTargetRewritesExistingPackageArgument(t *testing.T) {
	target := project.DefaultNamedAppTarget("billing")
	got := devBuildCommandForTarget("forj build --tags dev -o ./bin/app ./cmd/app", target)
	want := "forj build --tags dev -o ./bin/billing ./cmd/billing"
	if got != want {
		t.Fatalf("target build command = %q, want %q", got, want)
	}
}

func TestDevBuildJobsKeepTargetLabels(t *testing.T) {
	withConventionalTarget(t, "billing")

	got := devBuildJobs(&project.Config{}, false)
	if len(got) != 2 {
		t.Fatalf("expected default and billing build jobs, got %#v", got)
	}
	if got[0].target.Name != "app" || got[1].target.Name != "billing" {
		t.Fatalf("unexpected build job targets: %#v", got)
	}
}

func TestDevDatabasesForTargetsIncludesNamedTargetDatabase(t *testing.T) {
	withConventionalTarget(t, "billing")
	t.Setenv("DB_DRIVER", "mysql")
	t.Setenv("DB_DATABASE", "db")
	t.Setenv("BILLING_DB_DATABASE", "billing")

	got, err := devDatabasesForTargets(&project.Config{
		Render: project.RenderConfig{
			Components: project.Components{DatabaseMySQL: true},
		},
		AppTargets: map[string]project.AppTargetConfig{
			"billing": {
				Components: project.Components{DatabaseMySQL: true},
			},
		},
	}, activeDevTargets())
	if err != nil {
		t.Fatalf("devDatabasesForTargets returned error: %v", err)
	}
	want := []devDatabase{
		{Target: "billing", Driver: "mysql", Name: "billing"},
		{Target: "app", Driver: "mysql", Name: "db"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dev databases = %#v, want %#v", got, want)
	}
}

func TestDevDatabasesForTargetsSupportsMixedDrivers(t *testing.T) {
	withConventionalTarget(t, "reporting")
	t.Setenv("DB_DRIVER", "mysql")
	t.Setenv("DB_DATABASE", "db")
	t.Setenv("REPORTING_DB_DRIVER", "postgres")
	t.Setenv("REPORTING_DB_DATABASE", "reporting")

	got, err := devDatabasesForTargets(&project.Config{
		Render: project.RenderConfig{
			Components: project.Components{DatabaseMySQL: true, DatabasePostgres: true},
		},
		AppTargets: map[string]project.AppTargetConfig{
			"reporting": {
				Components: project.Components{DatabasePostgres: true},
			},
		},
	}, activeDevTargets())
	if err != nil {
		t.Fatalf("devDatabasesForTargets returned error: %v", err)
	}
	want := []devDatabase{
		{Target: "app", Driver: "mysql", Name: "db"},
		{Target: "reporting", Driver: "postgres", Name: "reporting"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dev databases = %#v, want %#v", got, want)
	}
}

func TestDevDatabasesForTargetsRejectsUnsafeDatabaseNames(t *testing.T) {
	withConventionalTarget(t, "billing")
	t.Setenv("DB_DRIVER", "mysql")
	t.Setenv("DB_DATABASE", "db")
	t.Setenv("BILLING_DB_DATABASE", "billing-prod")

	_, err := devDatabasesForTargets(&project.Config{
		Render: project.RenderConfig{
			Components: project.Components{DatabaseMySQL: true},
		},
		AppTargets: map[string]project.AppTargetConfig{
			"billing": {
				Components: project.Components{DatabaseMySQL: true},
			},
		},
	}, activeDevTargets())
	if err == nil {
		t.Fatal("expected unsafe database name to return an error")
	}
	if !strings.Contains(err.Error(), "BILLING_DB_DATABASE") {
		t.Fatalf("expected target env key in error, got %v", err)
	}
}

func TestCreateDatabaseScriptsIncludeAllDatabases(t *testing.T) {
	mysqlScript := mysqlCreateDatabasesScript([]string{"billing", "db"})
	for _, want := range []string{
		`mysqladmin ping`,
		`for db in billing db`,
		"CREATE DATABASE IF NOT EXISTS \\`$db\\`;",
		"GRANT ALL PRIVILEGES ON \\`$db\\`.* TO '$MARIADB_USER'@'%';",
		`FLUSH PRIVILEGES;`,
	} {
		if !strings.Contains(mysqlScript, want) {
			t.Fatalf("mysql script missing %q:\n%s", want, mysqlScript)
		}
	}

	postgresScript := postgresCreateDatabasesScript([]string{"app", "reporting"})
	for _, want := range []string{
		`pg_isready`,
		`for db in app reporting`,
		`CREATE DATABASE \"$db\";`,
	} {
		if !strings.Contains(postgresScript, want) {
			t.Fatalf("postgres script missing %q:\n%s", want, postgresScript)
		}
	}
}

func TestDevAutoMigrateUsesUnqualifiedFrameworkPrefix(t *testing.T) {
	if got := devAutoMigrateShellCommand(); got != "./bin/app migrate" {
		t.Fatalf("auto-migrate command = %q, want ./bin/app migrate", got)
	}
	if got := devAutoMigrateEnv()["FORJ_COMMAND_PREFIX"]; got != "forj" {
		t.Fatalf("auto-migrate prefix = %q, want forj", got)
	}
}

func TestDevAutoMigrateKeepsExplicitTargetBinary(t *testing.T) {
	t.Setenv("FORJ_APP_TARGET", "billing")

	if got := devAutoMigrateShellCommand(); got != "./bin/billing migrate" {
		t.Fatalf("auto-migrate command = %q, want ./bin/billing migrate", got)
	}
	if got := devAutoMigrateEnv()["FORJ_COMMAND_PREFIX"]; got != "forj" {
		t.Fatalf("auto-migrate prefix = %q, want forj", got)
	}
}

func TestRunDevBuildRunsTargetsInParallel(t *testing.T) {
	withConventionalTarget(t, "billing")

	config := &project.Config{
		Dev: project.DevConfig{
			Watches: []project.DevWatch{
				{Name: "Build App", Exec: `bash -c "sleep 0.5; mkdir -p bin; touch ./bin/app"`},
			},
		},
	}

	start := time.Now()
	var out bytes.Buffer
	var errOut bytes.Buffer
	if err := runDevBuild(config, &out, &errOut); err != nil {
		t.Fatalf("runDevBuild returned error: %v\nstdout:\n%s\nstderr:\n%s", err, out.String(), errOut.String())
	}
	if elapsed := time.Since(start); elapsed > 900*time.Millisecond {
		t.Fatalf("expected target builds to run in parallel, elapsed %s", elapsed)
	}
	if _, err := os.Stat(filepath.Join("bin", "app")); err != nil {
		t.Fatalf("expected app binary marker: %v", err)
	}
	if _, err := os.Stat(filepath.Join("bin", "billing")); err != nil {
		t.Fatalf("expected billing binary marker: %v", err)
	}
}

func TestRunDevBuildBuffersFailureOutputByTarget(t *testing.T) {
	withConventionalTarget(t, "billing")

	config := &project.Config{
		Dev: project.DevConfig{
			Watches: []project.DevWatch{
				{Name: "Build App", Exec: `test ./bin/app = ./bin/billing && echo billing failed >&2 && exit 7 || echo app ok`},
			},
		},
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	err := runDevBuild(config, &out, &errOut)
	if err == nil {
		t.Fatal("expected runDevBuild to fail")
	}
	if !strings.Contains(err.Error(), "billing") {
		t.Fatalf("expected target name in error, got %v", err)
	}
	if !strings.Contains(out.String(), "Build failed for billing") {
		t.Fatalf("expected target failure heading, got stdout %q", out.String())
	}
	if !strings.Contains(errOut.String(), "billing failed") {
		t.Fatalf("expected buffered stderr, got %q", errOut.String())
	}
}

func TestRunDevBuildDoesNotReplayProgressMarkersOnFailure(t *testing.T) {
	withConventionalTarget(t, "billing")

	config := &project.Config{
		Dev: project.DevConfig{
			Watches: []project.DevWatch{
				{Name: "Build App", Exec: `test ./bin/app = ./bin/billing && printf "__FORJ_BUILD_PROGRESS__ step 3/4 build:api-index\nreal failure\n" >&2 && exit 7 || true`},
			},
		},
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	err := runDevBuild(config, &out, &errOut)
	if err == nil {
		t.Fatal("expected runDevBuild to fail")
	}
	if strings.Contains(out.String(), buildProgressMarker) || strings.Contains(errOut.String(), buildProgressMarker) {
		t.Fatalf("expected progress markers to be stripped, stdout %q stderr %q", out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), "real failure") {
		t.Fatalf("expected non-protocol failure output to remain, got %q", errOut.String())
	}
}

func TestWriteDevTargetBuildLineSkipsSingleDefaultTarget(t *testing.T) {
	var out bytes.Buffer
	writeDevTargetBuildLine(&out, []project.AppTarget{project.DefaultAppTarget()})
	if out.Len() != 0 {
		t.Fatalf("expected no target line for default-only dev, got %q", out.String())
	}
}

func TestWriteDevTargetBuildLineShowsExpandedTargets(t *testing.T) {
	var out bytes.Buffer
	writeDevTargetBuildLine(&out, []project.AppTarget{
		project.DefaultAppTarget(),
		project.DefaultNamedAppTarget("billing"),
	})
	if !contains(out.String(), "Building app targets: app, billing") {
		t.Fatalf("expected target build note, got %q", out.String())
	}
}

func TestDevRuntimeWatcherTargetsReturnsRunTargets(t *testing.T) {
	got := devRuntimeWatcherTargets([]project.DevWatch{
		{Name: "Build App"},
		{Name: "Run App"},
		{Name: "Build billing"},
		{Name: "Run billing"},
	})
	want := []string{"app", "billing"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime watcher targets = %#v, want %#v", got, want)
	}
}

func TestDevAppTargetNamesUsesActiveTargetsOnly(t *testing.T) {
	got := devAppTargetNames([]project.AppTarget{project.DefaultAppTarget(), project.DefaultAppTarget()})
	want := []string{"app"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("active target names = %#v, want %#v", got, want)
	}
}

func TestDecorateDevAppLogTargetColumnAddsTargetAfterTimestamp(t *testing.T) {
	line := "23:35:28.370 HTTP         Starting HTTP server"
	got := stripANSI(decorateDevAppLogTargetColumn(line, "billing", len("billing"), true))
	want := "23:35:28.370 billing HTTP         Starting HTTP server"
	if got != want {
		t.Fatalf("decorated log line = %q, want %q", got, want)
	}
}

func TestDecorateDevAppLogTargetColumnSkipsSingleAppMode(t *testing.T) {
	line := "23:35:28.370 HTTP         Starting HTTP server"
	got := decorateDevAppLogTargetColumn(line, "app", len("app"), false)
	if got != line {
		t.Fatalf("expected single-app log line to remain unchanged, got %q", got)
	}
}

func TestDecorateDevAppLogTargetColumnHandlesColoredTimestamp(t *testing.T) {
	line := "\x1b[90m23:35:28.370\x1b[0m HTTP         Starting HTTP server"
	got := stripANSI(decorateDevAppLogTargetColumn(line, "app", len("billing"), true))
	want := "23:35:28.370 app     HTTP         Starting HTTP server"
	if got != want {
		t.Fatalf("decorated colored log line = %q, want %q", got, want)
	}
}

func TestDefaultDevAppTargetColorDiffersFromTimestampGray(t *testing.T) {
	if devDefaultAppTargetColor == console.ColorGray {
		t.Fatal("expected default app target color to differ from timestamp gray")
	}
}

func withConventionalTarget(t *testing.T, name string) {
	t.Helper()
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })

	targetMain := filepath.Join("cmd", name, "main.go")
	if err := os.MkdirAll(filepath.Dir(targetMain), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(targetMain, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write target main: %v", err)
	}
}

func TestBuildWatcherCommandArgsPreservesEnvGlobPattern(t *testing.T) {
	args := buildWatcherCommandArgs("-file .go -file .env -file .env.* -xdir forj -postpone", buildWatcherExec("./bin/app run"))
	got := strings.Join(args, " ")
	if !contains(got, ".env.*") {
		t.Fatalf("expected watcher args to preserve literal env glob, got %q", got)
	}
	if contains(got, ".env.local") {
		t.Fatalf("expected watcher args not to expand hidden env files, got %q", got)
	}
	if len(args) < 3 || args[len(args)-3] != "sh" || args[len(args)-2] != "-c" {
		t.Fatalf("expected watcher args to end with shell runner, got %#v", args)
	}
}

func TestSplitWatcherEnvAssignments(t *testing.T) {
	env, cmd := splitWatcherEnvAssignments("FEATURE_FLAG=1 FOO=bar my-command --verbose")
	if env["FEATURE_FLAG"] != "1" || env["FOO"] != "bar" {
		t.Fatalf("expected leading env assignments to be extracted, got %#v", env)
	}
	if cmd != "my-command --verbose" {
		t.Fatalf("expected remaining command to be preserved, got %q", cmd)
	}
}

func TestShellSplitArgsPreservesQuotedFragments(t *testing.T) {
	args, err := shellSplitArgs(`-file .env.* -xfile "wire/wire_gen\.go$" -xdir '_data'`)
	if err != nil {
		t.Fatalf("shellSplitArgs returned error: %v", err)
	}
	want := []string{"-file", ".env.*", "-xfile", `wire/wire_gen\.go$`, "-xdir", "_data"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("unexpected args: got %#v want %#v", args, want)
	}
}

func TestCopyEnvMapProducesIndependentClone(t *testing.T) {
	original := map[string]string{"FOO": "bar"}
	cloned := copyEnvMap(original)
	cloned["FOO"] = "baz"
	if reflect.DeepEqual(original, cloned) {
		t.Fatalf("expected clone to be independent of original")
	}
	if original["FOO"] != "bar" {
		t.Fatalf("expected original map to remain unchanged, got %#v", original)
	}
}

func TestContainsErrorWordTreatsSelectorCompileErrorsAsBuildErrors(t *testing.T) {
	line := `internal/hello/controller.go:12:2: use of package caches not in selector`
	if !containsErrorWord(line) {
		t.Fatalf("expected selector compile error to trigger watcher error alert")
	}
}

func TestContainsErrorWordTreatsBuildAppCommandFailuresAsBuildErrors(t *testing.T) {
	line := `Test › Build App › Error executing command error="go build: exit status 1 (# test/wire\nwire/wire_gen.go:83:66: too many arguments in call to hello.NewController)"`
	if !containsErrorWord(line) {
		t.Fatalf("expected build app command failure to trigger watcher error alert")
	}
}

func TestFormatWatcherLifecycleLine(t *testing.T) {
	line := formatWatcherLifecycleLine("API", watcherStateStarted)
	if !contains(line, "API") {
		t.Fatalf("expected watcher name in lifecycle line: %q", line)
	}
	if !contains(line, "started") {
		t.Fatalf("expected state in lifecycle line: %q", line)
	}
	if !contains(line, console.SuccessMark()) {
		t.Fatalf("expected success mark in started lifecycle line: %q", line)
	}
}

func TestFormatWatcherLifecycleSummary(t *testing.T) {
	line := formatWatcherLifecycleSummary([]string{"Build App", "Wire", "API"}, watcherStateStarted)
	if !contains(line, "Watchers") {
		t.Fatalf("expected watcher summary label in lifecycle line: %q", line)
	}
	if !contains(line, "started") {
		t.Fatalf("expected started state in summary line: %q", line)
	}
	if !contains(line, "Build App, Wire, API") {
		t.Fatalf("expected watcher names in summary line: %q", line)
	}
	if !contains(line, console.SuccessMark()) {
		t.Fatalf("expected success mark in started summary line: %q", line)
	}
}

func TestDrainWatcherExitsEmitsStoppedLines(t *testing.T) {
	var out bytes.Buffer
	exitCh := make(chan watcherExit, 2)
	exitCh <- watcherExit{name: "API"}
	exitCh <- watcherExit{name: "Scheduler"}

	drainWatcherExits(exitCh, 2, &out, nil, false)

	got := out.String()
	if !contains(got, "API") || !contains(got, "Scheduler") {
		t.Fatalf("expected watcher names in stopped output, got %q", got)
	}
	if strings.Count(got, "stopped") != 2 {
		t.Fatalf("expected two stopped markers, got %q", got)
	}
}

func TestDrainWatcherExitsEmitsStoppedSummaryWhenCollapsed(t *testing.T) {
	var out bytes.Buffer
	exitCh := make(chan watcherExit, 2)
	exitCh <- watcherExit{name: "API"}
	exitCh <- watcherExit{name: "Scheduler"}

	drainWatcherExits(exitCh, 2, &out, nil, true)

	got := out.String()
	if !contains(got, "Watchers") {
		t.Fatalf("expected watcher summary label in stopped output, got %q", got)
	}
	if !contains(got, "API, Scheduler") {
		t.Fatalf("expected watcher names in stopped summary, got %q", got)
	}
	if strings.Count(got, "stopped") != 1 {
		t.Fatalf("expected one stopped summary line, got %q", got)
	}
}

func TestStopWatchersEmitsStoppingSummaryWhenCollapsed(t *testing.T) {
	var out bytes.Buffer
	watchers := []runningWatcher{
		{name: "Build App", proc: &execx.Process{}},
		{name: "Wire", proc: &execx.Process{}},
		{name: "API", proc: &execx.Process{}},
	}

	stopWatchers(watchers, 0, &out, nil, true)

	got := out.String()
	if !contains(got, "Watchers") {
		t.Fatalf("expected watcher summary label in stopping output, got %q", got)
	}
	if !contains(got, "stopping") {
		t.Fatalf("expected stopping state in summary line, got %q", got)
	}
	if !contains(got, "Build App, Wire, API") {
		t.Fatalf("expected watcher names in stopping summary, got %q", got)
	}
	if strings.Count(got, "stopping") != 1 {
		t.Fatalf("expected one stopping summary line, got %q", got)
	}
}

func TestDecorateWatcherLineFormatsTriggerAsStarting(t *testing.T) {
	line := decorateWatcherLine("__FORJ_WATCHER_TRIGGER__", "API", "./bin/app http:serve")
	if !contains(line, "Starting") {
		t.Fatalf("expected starting label in trigger line: %q", line)
	}
	if !contains(line, "API") {
		t.Fatalf("expected watcher name in trigger line: %q", line)
	}
	if !contains(line, "./bin/app http:serve") {
		t.Fatalf("expected command in trigger line: %q", line)
	}
}

func TestDecorateWatcherLineFormatsANSIWrappedTriggerAsStarting(t *testing.T) {
	line := decorateWatcherLine("\x1b[32m__FORJ_WATCHER_TRIGGER__\x1b[0m", "API", "./bin/app http:serve")
	if !contains(line, "Starting") {
		t.Fatalf("expected starting label in trigger line: %q", line)
	}
	if contains(line, "__FORJ_WATCHER_TRIGGER__") {
		t.Fatalf("expected raw trigger marker to be hidden, got %q", line)
	}
}

func TestFormatWatcherNameList(t *testing.T) {
	got := formatWatcherNameList([]project.DevWatch{
		{Name: "Build App"},
		{Name: "Wire"},
		{Name: "API"},
	})
	if got != "Build App, Wire, API" {
		t.Fatalf("unexpected watcher summary: %q", got)
	}
}

func TestDevwatchLifecycleStateEmitsStartupSeparatorOnceAfterExpectedTriggers(t *testing.T) {
	state := newDevwatchLifecycleState(3, nil)
	if state.noteStartupTrigger() {
		t.Fatal("expected first trigger not to emit")
	}
	if state.noteStartupTrigger() {
		t.Fatal("expected second trigger not to emit")
	}
	if !state.noteStartupTrigger() {
		t.Fatal("expected third trigger to emit")
	}
	if state.noteStartupTrigger() {
		t.Fatal("expected separator emission only once")
	}
}

func TestCountImmediateStartupWatchers(t *testing.T) {
	got := countImmediateStartupWatchers([]project.DevWatch{
		{Name: "Build App", Watch: "-file .go -postpone"},
		{Name: "Run App", Watch: "-file ./bin/app"},
	})
	if got != 1 {
		t.Fatalf("expected 1 immediate startup watcher, got %d", got)
	}
}

func TestDevEnvFilesChangedDetectsCreateUpdateAndDelete(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	initial, err := snapshotDevEnvFiles()
	if err != nil {
		t.Fatalf("snapshot initial env files: %v", err)
	}
	if len(initial) != 0 {
		t.Fatalf("expected empty initial env snapshot, got %#v", initial)
	}

	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("FEATURE=1\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	created, err := snapshotDevEnvFiles()
	if err != nil {
		t.Fatalf("snapshot created env files: %v", err)
	}
	if !devEnvFilesChanged(initial, created) {
		t.Fatal("expected create to change env snapshot")
	}

	time.Sleep(2 * time.Millisecond)
	if err := os.WriteFile(envPath, []byte("FEATURE=2\n"), 0o644); err != nil {
		t.Fatalf("update .env: %v", err)
	}
	updated, err := snapshotDevEnvFiles()
	if err != nil {
		t.Fatalf("snapshot updated env files: %v", err)
	}
	if !devEnvFilesChanged(created, updated) {
		t.Fatal("expected update to change env snapshot")
	}

	if err := os.Remove(envPath); err != nil {
		t.Fatalf("remove .env: %v", err)
	}
	deleted, err := snapshotDevEnvFiles()
	if err != nil {
		t.Fatalf("snapshot deleted env files: %v", err)
	}
	if !devEnvFilesChanged(updated, deleted) {
		t.Fatal("expected delete to change env snapshot")
	}
}

func TestConsumeSuppressedDevEnvTrigger(t *testing.T) {
	for consumeSuppressedDevEnvTrigger() {
	}
	suppressNextDevEnvTrigger()
	if !consumeSuppressedDevEnvTrigger() {
		t.Fatal("expected suppressed env trigger to be consumed")
	}
	if consumeSuppressedDevEnvTrigger() {
		t.Fatal("expected suppression token to be one-shot")
	}
}

func TestDevwatchLifecycleStateBuildsRestartTargets(t *testing.T) {
	state := newDevwatchLifecycleState(0, []string{"Run App"})
	if state == nil {
		t.Fatal("expected lifecycle state")
	}
	if _, ok := state.restartExpected["Run App"]; !ok {
		t.Fatal("expected restart target to include Run App")
	}
}

func TestDevwatchLifecycleStateEmitsRestartSeparatorsAroundRunApp(t *testing.T) {
	state := newDevwatchLifecycleState(0, []string{"Run App"})
	if state == nil {
		t.Fatal("expected lifecycle state")
	}
	state.startupEmitted = true
	if state.noteRestartTrigger("Run App") != "" {
		t.Fatal("expected no labeled restart separator before shutdown")
	}
	if !state.noteRestartShutdown("Run App", "Test › API › Shutting down HTTP server #http.Server.Serve") {
		t.Fatal("expected first shutdown line to emit shutdown separator")
	}
	if state.noteRestartShutdown("Run App", "Test › Jobs › Queue worker shut down #jobs.Worker.StartWithContext") {
		t.Fatal("expected shutdown separator to emit once per restart")
	}
	if got := state.noteRestartTrigger("Run App"); got != "Start" {
		t.Fatalf("expected labeled start separator after shutdown, got %q", got)
	}
	if got := state.noteRestartTrigger("Run App"); got != "" {
		t.Fatalf("expected unlabeled trigger before next shutdown, got %q", got)
	}
}

func TestIsRuntimeShutdownLine(t *testing.T) {
	for _, line := range []string{
		"Test › API › Shutting down HTTP server #http.Server.Serve",
		"Test › API › HTTP server shut down #http.Server.Serve",
		"Test › Scheduler › Shutting down scheduler... #scheduler.Scheduler.StartWithContext",
		"Test › Scheduler › Scheduler shut down #scheduler.Scheduler.StartWithContext",
		"Test › Jobs › Shutting down queue worker... #jobs.Worker.StartWithContext",
		"Test › Jobs › Queue worker shut down #jobs.Worker.StartWithContext",
		"asynq: pid=1 INFO: Starting graceful shutdown",
		"asynq: pid=1 INFO: Waiting for all workers to finish...",
		"asynq: pid=1 INFO: All workers have finished",
		"asynq: pid=1 INFO: Exiting",
	} {
		if !isRuntimeShutdownLine(line) {
			t.Fatalf("expected runtime shutdown line: %q", line)
		}
	}
	if isRuntimeShutdownLine("Test › API › Starting HTTP server · port 3000 #http.Server.Serve") {
		t.Fatal("expected startup line not to be classified as shutdown output")
	}
}

func TestFormatBuildProgressStatus(t *testing.T) {
	line := ansiCSI.ReplaceAllString(formatBuildProgressStatus("2/4", "wire"), "")
	if !contains(line, "2/4") {
		t.Fatalf("expected step count in status line: %q", line)
	}
	if !contains(line, "wire") {
		t.Fatalf("expected step name in status line: %q", line)
	}
}

func TestHandleBuildProgressLineConsumesNamedTargetBuildMarkers(t *testing.T) {
	var out bytes.Buffer
	if !handleBuildProgressLine(&out, "Build billing", "__FORJ_BUILD_PROGRESS__ step 2/4 wire") {
		t.Fatal("expected named target build progress marker to be handled")
	}
	if strings.Contains(out.String(), buildProgressMarker) {
		t.Fatalf("did not expect raw progress marker in output: %q", out.String())
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
