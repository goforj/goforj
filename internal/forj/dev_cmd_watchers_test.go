package forj

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/console"
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

func TestDrainWatcherExitsEmitsStoppedLines(t *testing.T) {
	var out bytes.Buffer
	exitCh := make(chan watcherExit, 2)
	exitCh <- watcherExit{name: "API"}
	exitCh <- watcherExit{name: "Scheduler"}

	drainWatcherExits(exitCh, 2, &out, nil)

	got := out.String()
	if !contains(got, "API") || !contains(got, "Scheduler") {
		t.Fatalf("expected watcher names in stopped output, got %q", got)
	}
	if strings.Count(got, "stopped") != 2 {
		t.Fatalf("expected two stopped markers, got %q", got)
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

func contains(s, sub string) bool { return strings.Contains(s, sub) }
