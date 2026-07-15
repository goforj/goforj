package cmd

import "testing"

// TestEffectiveLaunchArgsDefaultsRuntimeAppsToRun verifies bare runtime binaries enter the standalone host.
func TestEffectiveLaunchArgsDefaultsRuntimeAppsToRun(t *testing.T) {
	got := EffectiveLaunchArgs(nil, true)
	if len(got) != 1 || got[0] != "run" {
		t.Fatalf("expected no-arg launch to default to run, got %v", got)
	}
}

// TestEffectiveLaunchArgsPreservesCLIOnlyNoArgLaunches verifies command-only binaries retain root help behavior.
func TestEffectiveLaunchArgsPreservesCLIOnlyNoArgLaunches(t *testing.T) {
	got := EffectiveLaunchArgs(nil, false)
	if len(got) != 0 {
		t.Fatalf("expected CLI-only no-arg launch to remain empty, got %v", got)
	}
}

// TestEffectiveLaunchArgsPreservesExplicitArgs verifies explicit command selection always takes precedence.
func TestEffectiveLaunchArgsPreservesExplicitArgs(t *testing.T) {
	args := []string{"about", "--json"}
	got := EffectiveLaunchArgs(args, true)
	if len(got) != len(args) {
		t.Fatalf("expected explicit args to bypass default launch, got %v", got)
	}
	for index := range args {
		if got[index] != args[index] {
			t.Fatalf("expected explicit args to remain unchanged, got %v", got)
		}
	}
}
