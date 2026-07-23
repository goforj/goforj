package forj

import "testing"

// TestNewRuntimeCommandsCopiesLauncherEnvironment keeps wired command instances independent from the caller's map.
func TestNewRuntimeCommandsCopiesLauncherEnvironment(t *testing.T) {
	launcher := map[string]string{"PROCESS_VALUE": "initial"}
	commands := NewRuntimeCommands(nil, nil, launcher)
	launcher["PROCESS_VALUE"] = "changed"

	if got := commands.dev.inheritedEnv["PROCESS_VALUE"]; got != "initial" {
		t.Fatalf("dev launcher environment = %q, want immutable copy", got)
	}
	if got := commands.devStatus.inheritedEnv["PROCESS_VALUE"]; got != "initial" {
		t.Fatalf("dev status launcher environment = %q, want immutable copy", got)
	}
	commands.dev.inheritedEnv["PROCESS_VALUE"] = "dev-only"
	if got := commands.devStatus.inheritedEnv["PROCESS_VALUE"]; got != "initial" {
		t.Fatalf("dev status launcher environment changed with dev command: %q", got)
	}
}

// TestNewRuntimeCommandsLeavesNilEnvironmentForDirectFallback verifies compatibility construction uses the live process environment.
func TestNewRuntimeCommandsLeavesNilEnvironmentForDirectFallback(t *testing.T) {
	t.Setenv("PROCESS_FALLBACK_VALUE", "from-process")
	commands := NewRuntimeCommands(nil, nil, nil)
	if commands.dev.inheritedEnv != nil || commands.devStatus.inheritedEnv != nil {
		t.Fatalf("nil launcher environment = (%#v, %#v), want nil fields", commands.dev.inheritedEnv, commands.devStatus.inheritedEnv)
	}
	if got := commands.dev.inheritedEnvironment()["PROCESS_FALLBACK_VALUE"]; got != "from-process" {
		t.Fatalf("dev fallback environment = %q", got)
	}
	if got := commands.devStatus.inheritedEnvironment()["PROCESS_FALLBACK_VALUE"]; got != "from-process" {
		t.Fatalf("dev status fallback environment = %q", got)
	}
}
