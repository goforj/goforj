package forj

import (
	"reflect"
	"testing"

	"github.com/goforj/goforj/internal/devwatch"
)

// TestIsolateDevRuntimeEnvironmentsKeepsDotenvStateOutOfAppChildren proves framework configuration
// cannot outrank an App's own dotenv load merely because the framework launched it.
func TestIsolateDevRuntimeEnvironmentsKeepsDotenvStateOutOfAppChildren(t *testing.T) {
	inherited := processEnvironment{
		"PATH":         "/launcher/bin",
		"USER_SETTING": "launcher",
	}
	runtimeEnvironment := map[string]string{
		"FORJ_APP":     "harbord",
		"USER_SETTING": "app-override",
	}
	compiled := []devCompiledWatcher{
		{
			Kind: devWatcherAppRun,
			Command: devwatch.Command{
				Shell: "./bin/harbord --foreground",
				Env:   runtimeEnvironment,
			},
		},
		{
			Kind: devWatcherCustom,
			Command: devwatch.Command{
				Shell: "docker compose up",
				Env:   map[string]string{"TASK_SETTING": "compose"},
			},
		},
		{
			Kind:   devWatcherAppRun,
			Legacy: true,
			Command: devwatch.Command{
				Shell: "./bin/legacy",
				Env:   map[string]string{"LEGACY_SETTING": "preserved"},
			},
		},
	}

	isolated := isolateDevRuntimeEnvironments(compiled, inherited)

	wantRuntime := map[string]string{
		"FORJ_APP":     "harbord",
		"PATH":         "/launcher/bin",
		"USER_SETTING": "app-override",
	}
	if !isolated[0].Command.ReplaceEnv || !reflect.DeepEqual(isolated[0].Command.Env, wantRuntime) {
		t.Fatalf("runtime command = %#v, want isolated environment %#v", isolated[0].Command, wantRuntime)
	}
	if isolated[1].Command.ReplaceEnv || !reflect.DeepEqual(isolated[1].Command.Env, compiled[1].Command.Env) {
		t.Fatalf("custom command = %#v, want existing environment behavior", isolated[1].Command)
	}
	if isolated[2].Command.ReplaceEnv || !reflect.DeepEqual(isolated[2].Command.Env, compiled[2].Command.Env) {
		t.Fatalf("legacy runtime command = %#v, want existing environment behavior", isolated[2].Command)
	}
	if !reflect.DeepEqual(inherited, processEnvironment{"PATH": "/launcher/bin", "USER_SETTING": "launcher"}) {
		t.Fatalf("launcher environment mutated: %#v", inherited)
	}
	if !reflect.DeepEqual(runtimeEnvironment, map[string]string{"FORJ_APP": "harbord", "USER_SETTING": "app-override"}) {
		t.Fatalf("runtime overrides mutated: %#v", runtimeEnvironment)
	}
}

// TestMergeDevRuntimeEnvironmentUsesWindowsKeySemantics proves explicit App configuration wins even
// when the launcher used a differently cased spelling of the same Windows environment key.
func TestMergeDevRuntimeEnvironmentUsesWindowsKeySemantics(t *testing.T) {
	environment := mergeDevRuntimeEnvironment(
		processEnvironment{
			"Path":      `C:\launcher`,
			"UNCHANGED": "launcher",
		},
		map[string]string{
			"PATH": `C:\app`,
		},
		true,
	)

	want := map[string]string{
		"PATH":      `C:\app`,
		"UNCHANGED": "launcher",
	}
	if !reflect.DeepEqual(environment, want) {
		t.Fatalf("merged Windows environment = %#v, want %#v", environment, want)
	}
}
