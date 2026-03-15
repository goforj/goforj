package forj

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/goforj/execx"
	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/project"
)

func TestBuildWatcherExecUsesExecAndPrefix(t *testing.T) {
	script := buildWatcherExec("Test › API", "./bin/app http:serve")
	if script == "" {
		t.Fatal("expected watcher script to be non-empty")
	}
	if want := "__FORJ_WATCHER_TRIGGER__"; !contains(script, want) {
		t.Fatalf("expected watcher trigger marker in script: %q", script)
	}
	if want := "export APP_LOG_PREFIX="; !contains(script, want) {
		t.Fatalf("expected APP_LOG_PREFIX export in script: %q", script)
	}
	if want := "exec ./bin/app http:serve"; !contains(script, want) {
		t.Fatalf("expected exec for watcher command in script: %q", script)
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
	line := `internal/hello/controller.go:12:2: use of package cache not in selector`
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
	if !contains(line, "GoForj Watcher") {
		t.Fatalf("expected watcher label in lifecycle line: %q", line)
	}
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
	if !contains(line, "GoForj Watchers") {
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
	if !contains(got, "GoForj Watchers") {
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
	if !contains(got, "GoForj Watchers") {
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
	if !contains(line, "GoForj Watcher") {
		t.Fatalf("expected watcher label in trigger line: %q", line)
	}
	if !contains(line, "API") {
		t.Fatalf("expected watcher name in trigger line: %q", line)
	}
	if !contains(line, "starting") {
		t.Fatalf("expected starting state in trigger line: %q", line)
	}
	if !contains(line, "./bin/app http:serve") {
		t.Fatalf("expected command in trigger line: %q", line)
	}
}

func TestDecorateWatcherLineFormatsANSIWrappedTriggerAsStarting(t *testing.T) {
	line := decorateWatcherLine("\x1b[32m__FORJ_WATCHER_TRIGGER__\x1b[0m", "API", "./bin/app http:serve")
	if !contains(line, "GoForj Watcher") {
		t.Fatalf("expected watcher label in trigger line: %q", line)
	}
	if !contains(line, "starting") {
		t.Fatalf("expected starting state in trigger line: %q", line)
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

func TestDevwatchStartupStateEmitsSeparatorOnceAfterExpectedTriggers(t *testing.T) {
	state := &devwatchStartupState{expected: 3}
	if state.noteTrigger() {
		t.Fatal("expected first trigger not to emit")
	}
	if state.noteTrigger() {
		t.Fatal("expected second trigger not to emit")
	}
	if !state.noteTrigger() {
		t.Fatal("expected third trigger to emit")
	}
	if state.noteTrigger() {
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

func TestWatcherGroupNames(t *testing.T) {
	state := newDevwatchRestartState([]string{"Run App"})
	if state == nil {
		t.Fatal("expected restart state")
	}
}

func TestDevwatchRestartStateEmitsSeparatorOnFirstWatcherInBurst(t *testing.T) {
	state := newDevwatchRestartState([]string{"Run App"})
	if state == nil {
		t.Fatal("expected restart state")
	}
	if state.noteTrigger("Run App") != "" {
		t.Fatal("expected no labeled restart separator before shutdown")
	}
	if !state.noteShutdown("Run App", "Test › API › Shutting down HTTP server #http.Server.Serve") {
		t.Fatal("expected first shutdown line to emit shutdown separator")
	}
	if state.noteShutdown("Run App", "Test › Jobs › Queue worker shut down #jobs.Worker.StartWithContext") {
		t.Fatal("expected shutdown separator to emit once per restart")
	}
	if got := state.noteTrigger("Run App"); got != "Start" {
		t.Fatalf("expected labeled start separator after shutdown, got %q", got)
	}
	if got := state.noteTrigger("Run App"); got != "" {
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

func contains(s, sub string) bool { return strings.Contains(s, sub) }
