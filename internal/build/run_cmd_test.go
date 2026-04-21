package build

import (
	"reflect"
	"testing"
)

func TestRunCmdRunArgsDefaultsToCurrentPackage(t *testing.T) {
	cmd := &RunCmd{}
	if got := cmd.runArgs(); !reflect.DeepEqual(got, []string{"."}) {
		t.Fatalf("expected default run args to target current package, got %#v", got)
	}
}

func TestRunCmdRunArgsPassesAppArgsAfterCurrentPackage(t *testing.T) {
	cmd := &RunCmd{Args: []string{"run", "--port", "4000"}}
	if got := cmd.runArgs(); !reflect.DeepEqual(got, []string{".", "run", "--port", "4000"}) {
		t.Fatalf("expected app args after current package, got %#v", got)
	}
}

func TestRunCmdLaunchCommand(t *testing.T) {
	cmd := &RunCmd{}
	if got := cmd.launchCommand([]string{".", "route:list"}); got != "go run . route:list" {
		t.Fatalf("launch command = %q, want %q", got, "go run . route:list")
	}
}
