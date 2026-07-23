package forj

import (
	"reflect"
	"testing"

	"github.com/goforj/goforj/internal/launcher"
)

// TestNewRuntimeCommandsSnapshotsLauncherEnvironment keeps runtime commands isolated from each other's snapshots.
func TestNewRuntimeCommandsSnapshotsLauncherEnvironment(t *testing.T) {
	launcherEnvironment := &launcher.Environment{}
	launcherEnvironment.Set(map[string]string{"PROCESS_VALUE": "initial"})
	commands := NewRuntimeCommands(nil, nil, launcherEnvironment)

	if got := commands.dev.inheritedEnvironment()["PROCESS_VALUE"]; got != "initial" {
		t.Fatalf("dev launcher environment = %q, want initial", got)
	}
	if got := commands.devStatus.inheritedEnvironment()["PROCESS_VALUE"]; got != "initial" {
		t.Fatalf("dev status launcher environment = %q, want initial", got)
	}
	devSnapshot := commands.dev.inheritedEnvironment()
	devSnapshot["PROCESS_VALUE"] = "dev-only"
	if got := commands.devStatus.inheritedEnvironment()["PROCESS_VALUE"]; got != "initial" {
		t.Fatalf("dev status launcher environment changed with dev snapshot: %q", got)
	}
}

// TestRuntimeCommandsWithoutExplicitLauncherUseProvidedEnvironment preserves direct-construction compatibility.
func TestRuntimeCommandsWithoutExplicitLauncherUseProvidedEnvironment(t *testing.T) {
	commands := NewRuntimeCommands(nil, nil, nil)
	want := processEnvironment(launcher.Provide().Snapshot())
	if got := commands.dev.inheritedEnvironment(); !reflect.DeepEqual(got, want) {
		t.Fatalf("dev launcher environment = %#v, want provided snapshot", got)
	}
	if got := commands.devStatus.inheritedEnvironment(); !reflect.DeepEqual(got, want) {
		t.Fatalf("dev status launcher environment = %#v, want provided snapshot", got)
	}
}
