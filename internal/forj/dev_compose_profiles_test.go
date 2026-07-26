package forj

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/goforj/goforj/project"
)

// effectiveDevPreTasksForTest keeps lifecycle expectations independent from tools installed on the test host.
func effectiveDevPreTasksForTest(config *project.Config) []project.DevTask {
	return effectiveDevPreTasksWithLegacy(config, true)
}

// effectiveDevDownTasksForTest keeps teardown expectations independent from tools installed on the test host.
func effectiveDevDownTasksForTest(config *project.Config) []project.DevTask {
	return effectiveDevDownTasksWithLegacy(config, true)
}

// TestEffectiveDevPreTasksStartsProfilesAtInvocationTime verifies profile flips do not require regenerated lifecycle config.
func TestEffectiveDevPreTasksStartsProfilesAtInvocationTime(t *testing.T) {
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	config := &project.Config{Render: project.RenderConfig{Components: project.Components{Docker: true}}}
	if got := effectiveDevPreTasksForTest(config); len(got) != 0 {
		t.Fatalf("empty profile tasks = %#v, want none", got)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("COMPOSE_PROFILES=rustfs,opensearch\n"), 0o644); err != nil {
		t.Fatalf("write owner environment: %v", err)
	}
	want := []project.DevTask{{Name: "Run Docker Compose", Cmd: dockerComposeUpDevCommand(config.Render.Components)}}
	if got := effectiveDevPreTasksForTest(config); !reflect.DeepEqual(got, want) {
		t.Fatalf("enabled profile tasks = %#v, want %#v", got, want)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("COMPOSE_PROFILES=owner-defined\n"), 0o644); err != nil {
		t.Fatalf("write owner-defined profile: %v", err)
	}
	if got := effectiveDevPreTasksForTest(config); !reflect.DeepEqual(got, want) {
		t.Fatalf("owner-defined profile tasks = %#v, want Compose to resolve the profile", got)
	}

	custom := project.DevTask{Name: "Run Docker Compose", Cmd: "docker compose up custom"}
	config.Dev.Pre = []project.DevTask{custom}
	if got := effectiveDevPreTasksForTest(config); !reflect.DeepEqual(got, []project.DevTask{custom}) {
		t.Fatalf("custom Compose task changed: %#v", got)
	}
}

// TestEffectiveDevPreTasksHonorsProcessProfileOverride keeps runtime behavior aligned with Compose precedence.
func TestEffectiveDevPreTasksHonorsProcessProfileOverride(t *testing.T) {
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("COMPOSE_PROFILES=rustfs\n"), 0o644); err != nil {
		t.Fatalf("write owner environment: %v", err)
	}
	t.Setenv("COMPOSE_PROFILES", "")

	config := &project.Config{Render: project.RenderConfig{Components: project.Components{Docker: true}}}
	if got := effectiveDevPreTasksForTest(config); len(got) != 0 {
		t.Fatalf("explicit empty process profile tasks = %#v, want none", got)
	}
}

// TestEffectiveDevPreTasksSuppressesStaleProfileOnlyStartup verifies disabling a profile needs no config regeneration.
func TestEffectiveDevPreTasksSuppressesStaleProfileOnlyStartup(t *testing.T) {
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("COMPOSE_PROFILES=\n"), 0o644); err != nil {
		t.Fatalf("write owner environment: %v", err)
	}
	profileOnlyCompose := "services:\n  rustfs:\n    profiles: [rustfs]\n    image: rustfs/rustfs:1.0.0-beta.10\n"
	if err := os.WriteFile(filepath.Join(root, "docker-compose.yml"), []byte(profileOnlyCompose), 0o644); err != nil {
		t.Fatalf("write profile-only Compose file: %v", err)
	}

	generated := project.DevTask{Name: "Run Docker Compose", Cmd: dockerComposeUpDevCommand(project.Components{})}
	config := &project.Config{
		Render: project.RenderConfig{Components: project.Components{Docker: true}},
		Dev:    project.DevConfig{Pre: []project.DevTask{generated}},
	}
	if got := effectiveDevPreTasksForTest(config); len(got) != 0 {
		t.Fatalf("disabled profile tasks = %#v, want stale generated startup suppressed", got)
	}
	if err := os.WriteFile(filepath.Join(root, "docker-compose.override.yml"), []byte("services:\n  owner-tool:\n    image: owner/tool:latest\n"), 0o644); err != nil {
		t.Fatalf("write owner Compose override: %v", err)
	}
	if got := effectiveDevPreTasksForTest(config); !reflect.DeepEqual(got, []project.DevTask{generated}) {
		t.Fatalf("owner override tasks = %#v, want generated Compose startup retained", got)
	}

	unprofiledCompose := "services:\n  mysql:\n    image: mariadb:11\n  rustfs:\n    profiles: [rustfs]\n    image: rustfs/rustfs:1.0.0-beta.10\n"
	if err := os.WriteFile(filepath.Join(root, "docker-compose.yml"), []byte(unprofiledCompose), 0o644); err != nil {
		t.Fatalf("write required-service Compose file: %v", err)
	}
	if got := effectiveDevPreTasksForTest(config); !reflect.DeepEqual(got, []project.DevTask{generated}) {
		t.Fatalf("required-service tasks = %#v, want generated Compose startup retained", got)
	}
}

// TestComposeHasUnprofiledServiceAppliesOverrideProfiles verifies owner overrides can change startup eligibility.
func TestComposeHasUnprofiledServiceAppliesOverrideProfiles(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "docker-compose.yml")
	override := filepath.Join(root, "docker-compose.override.yml")
	if err := os.WriteFile(base, []byte("services:\n  worker:\n    image: owner/worker:latest\n"), 0o644); err != nil {
		t.Fatalf("write base Compose file: %v", err)
	}
	if err := os.WriteFile(override, []byte("services:\n  worker:\n    profiles: [worker]\n"), 0o644); err != nil {
		t.Fatalf("write Compose override: %v", err)
	}
	if unprofiled, inspected := composeHasUnprofiledService(base, override); !inspected || unprofiled {
		t.Fatalf("overridden Compose eligibility = unprofiled:%t inspected:%t, want profiled-only", unprofiled, inspected)
	}
	if err := os.WriteFile(override, []byte("services:\n  owner-tool:\n    image: owner/tool:latest\n"), 0o644); err != nil {
		t.Fatalf("rewrite Compose override: %v", err)
	}
	if unprofiled, inspected := composeHasUnprofiledService(base, override); !inspected || !unprofiled {
		t.Fatalf("owner tool Compose eligibility = unprofiled:%t inspected:%t, want required startup", unprofiled, inspected)
	}
}

// TestEffectiveDevPreTasksFailsOpenForCustomComposeSelection preserves lifecycle work it cannot model safely.
func TestEffectiveDevPreTasksFailsOpenForCustomComposeSelection(t *testing.T) {
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("COMPOSE_PROFILES=\nCOMPOSE_FILE=owner.yml\n"), 0o644); err != nil {
		t.Fatalf("write owner environment: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docker-compose.yml"), []byte("services:\n  rustfs:\n    profiles: [rustfs]\n"), 0o644); err != nil {
		t.Fatalf("write generated Compose file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "owner.yml"), []byte("services:\n  owner-tool:\n    image: owner/tool:latest\n"), 0o644); err != nil {
		t.Fatalf("write owner Compose file: %v", err)
	}

	generated := project.DevTask{Name: "Run Docker Compose", Cmd: dockerComposeUpDevCommand(project.Components{})}
	config := &project.Config{
		Render: project.RenderConfig{Components: project.Components{Docker: true}},
		Dev:    project.DevConfig{Pre: []project.DevTask{generated}},
	}
	if got := effectiveDevPreTasksForTest(config); !reflect.DeepEqual(got, []project.DevTask{generated}) {
		t.Fatalf("custom Compose selection tasks = %#v, want generated task preserved", got)
	}
	config.Dev.Pre = nil
	if got := effectiveDevPreTasksForTest(config); !reflect.DeepEqual(got, []project.DevTask{generated}) {
		t.Fatalf("custom Compose selection without persisted task = %#v, want fail-open startup", got)
	}

	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("COMPOSE_PROFILES=\n"), 0o644); err != nil {
		t.Fatalf("rewrite owner environment: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  owner-tool:\n    image: owner/tool:latest\n"), 0o644); err != nil {
		t.Fatalf("write alternate standard Compose file: %v", err)
	}
	if got := effectiveDevPreTasksForTest(config); !reflect.DeepEqual(got, []project.DevTask{generated}) {
		t.Fatalf("alternate Compose selection tasks = %#v, want fail-open startup", got)
	}
}

// TestEffectiveDevPreTasksFailsOpenForComposeEnvironmentIndirection keeps Compose-native dotenv inputs from hiding lifecycle work.
func TestEffectiveDevPreTasksFailsOpenForComposeEnvironmentIndirection(t *testing.T) {
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("COMPOSE_PROFILES=${DEV_PROFILES}\n"), 0o644); err != nil {
		t.Fatalf("write interpolated owner environment: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docker-compose.yml"), []byte("services:\n  rustfs:\n    profiles: [rustfs]\n"), 0o644); err != nil {
		t.Fatalf("write generated Compose file: %v", err)
	}
	t.Setenv("DEV_PROFILES", "rustfs")

	generated := project.DevTask{Name: "Run Docker Compose", Cmd: dockerComposeUpDevCommand(project.Components{})}
	config := &project.Config{Render: project.RenderConfig{Components: project.Components{Docker: true}}}
	if got := effectiveDevPreTasksForTest(config); !reflect.DeepEqual(got, []project.DevTask{generated}) {
		t.Fatalf("interpolated Compose profiles tasks = %#v, want fail-open startup", got)
	}

	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("COMPOSE_PROFILES=\nCOMPOSE_ENV_FILES=profiles.env\n"), 0o644); err != nil {
		t.Fatalf("write alternate Compose environment selection: %v", err)
	}
	if got := effectiveDevPreTasksForTest(config); !reflect.DeepEqual(got, []project.DevTask{generated}) {
		t.Fatalf("alternate Compose environment tasks = %#v, want fail-open startup", got)
	}
}

// TestEffectiveDevDownTasksAlwaysCoversDockerProjects verifies removed profiles cannot strand prior containers.
func TestEffectiveDevDownTasksAlwaysCoversDockerProjects(t *testing.T) {
	config := &project.Config{Render: project.RenderConfig{Components: project.Components{Docker: true}}}
	want := []project.DevTask{{Name: "Docker Compose Down", Cmd: dockerComposeDownDevCommand()}}
	if got := effectiveDevDownTasksForTest(config); !reflect.DeepEqual(got, want) {
		t.Fatalf("Docker down tasks = %#v, want %#v", got, want)
	}

	config.Dev.Down = []project.DevTask{{Name: "Docker Compose Down", Cmd: "docker-compose down"}}
	if got := effectiveDevDownTasksForTest(config); !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy generated down tasks = %#v, want %#v", got, want)
	}

	custom := project.DevTask{Name: "Docker Compose Down", Cmd: "docker compose down --timeout 5"}
	config.Dev.Down = []project.DevTask{custom}
	if got := effectiveDevDownTasksForTest(config); !reflect.DeepEqual(got, []project.DevTask{custom}) {
		t.Fatalf("custom down task changed: %#v", got)
	}

	config.Render.Components.Docker = false
	config.Dev.Down = nil
	if got := effectiveDevDownTasksForTest(config); len(got) != 0 {
		t.Fatalf("Docker-disabled down tasks = %#v, want none", got)
	}
}

// TestResolveDevComposeCommandUsesTheAvailableFrontend preserves task arguments across Docker installations.
func TestResolveDevComposeCommandUsesTheAvailableFrontend(t *testing.T) {
	tests := []struct {
		name      string
		command   string
		useLegacy bool
		want      string
	}{
		{
			name:    "plugin only",
			command: `docker-compose --profile "*" down`,
			want:    `docker compose --profile "*" down`,
		},
		{
			name:      "owner plugin command preserved",
			command:   "docker compose up -d",
			useLegacy: true,
			want:      "docker compose up -d",
		},
		{
			name:      "configured frontend available",
			command:   "docker-compose up -d",
			useLegacy: true,
			want:      "docker-compose up -d",
		},
		{
			name:    "unrelated owner task",
			command: "docker info",
			want:    "docker info",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := resolveDevComposeCommand(test.command, test.useLegacy)
			if got != test.want {
				t.Fatalf("resolveDevComposeCommand() = %q, want %q", got, test.want)
			}
		})
	}
}
