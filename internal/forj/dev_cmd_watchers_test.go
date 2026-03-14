package forj

import (
	"reflect"
	"strings"
	"testing"
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

func contains(s, sub string) bool { return strings.Contains(s, sub) }
