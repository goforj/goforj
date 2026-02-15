package forj

import (
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

func contains(s, sub string) bool { return strings.Contains(s, sub) }
