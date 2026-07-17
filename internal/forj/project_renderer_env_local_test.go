package forj

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/envfile"
	"github.com/goforj/goforj/internal/forj/makeapp"
	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// TestProjectRendererSeedsQueueDriverOnlyInEnv keeps the wizard choice out of durable render configuration.
func TestProjectRendererSeedsQueueDriverOnlyInEnv(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	root := t.TempDir()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	config := &project.Config{
		ProjectName: "QueueSeed", GoModuleName: "example.com/queueseed",
		Render: project.RenderConfig{
			Components: project.Components{CLI: true, Jobs: true},
		},
	}
	encoded, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	legacyConfig := strings.Replace(string(encoded), "render:\n", "render:\n    queue_driver: \"\"\n", 1)
	if err := os.WriteFile(".goforj.yml", []byte(legacyConfig), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	renderer := unitProjectRenderer(t)
	if err := renderer.Render(ComponentRenderInput{
		renderAll:    true,
		resourcePlan: queueDriverResourcePlanForTest(t, config.Render.Components, "nats"),
	}); err != nil {
		t.Fatalf("initial render: %v", err)
	}
	envData, err := os.ReadFile(".env")
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	environmentLines := strings.Split(string(envData), "\n")
	queueDriver, queueDriverSet := envfile.Lookup(environmentLines, "QUEUE_DRIVER")
	supportedDrivers, supportedDriversSet := envfile.Lookup(environmentLines, "QUEUE_SUPPORTED_DRIVERS")
	if !queueDriverSet || queueDriver != "nats" || !supportedDriversSet || supportedDrivers != "nats" {
		t.Fatalf("wizard queue choice was not seeded into .env:\n%s", envData)
	}
	configData, err := os.ReadFile(".goforj.yml")
	if err != nil {
		t.Fatalf("read rewritten config: %v", err)
	}
	if strings.Contains(string(configData), "queue_driver:") {
		t.Fatalf("wizard queue choice leaked into project config:\n%s", configData)
	}

	if err := os.WriteFile(".env", []byte("QUEUE_DRIVER=workerpool\nQUEUE_SUPPORTED_DRIVERS=workerpool\n"), 0o644); err != nil {
		t.Fatalf("write existing .env: %v", err)
	}
	if err := renderer.Render(ComponentRenderInput{
		renderAll:    true,
		resourcePlan: queueDriverResourcePlanForTest(t, config.Render.Components, "redis"),
	}); err != nil {
		t.Fatalf("rerender: %v", err)
	}
	envData, err = os.ReadFile(".env")
	if err != nil {
		t.Fatalf("read rerendered .env: %v", err)
	}
	if !strings.Contains(string(envData), "QUEUE_DRIVER=workerpool\n") || strings.Contains(string(envData), "QUEUE_DRIVER=redis\n") {
		t.Fatalf("rerender replaced runtime queue configuration:\n%s", envData)
	}
}

// TestLegacyQueueDriverDefault retains one-way compatibility without a second active selection path.
func TestLegacyQueueDriverDefault(t *testing.T) {
	tests := []struct {
		name   string
		legacy string
		want   string
	}{
		{name: "legacy config seeds missing selection", legacy: "workerpool", want: "workerpool"},
		{name: "invalid values use portable default", legacy: "invalid", want: "workerpool"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := legacyQueueDriverDefault(test.legacy); got != test.want {
				t.Fatalf("legacyQueueDriverDefault(%q) = %q, want %q", test.legacy, got, test.want)
			}
		})
	}
}

// TestRenderAppOnlyPublishesResourceEnvironmentBeforeAppDefaults prevents App updates from overwriting a staged owner migration.
func TestRenderAppOnlyPublishesResourceEnvironmentBeforeAppDefaults(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	components := project.Components{CLI: true, Jobs: true}
	config := &project.Config{
		ProjectName:  "AppResourceMigration",
		GoModuleName: "example.com/app-resource-migration",
		Render: project.RenderConfig{
			Components: components,
		},
		Apps: map[string]project.AppConfig{
			"worker": {Components: components},
		},
	}
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(".env", []byte("QUEUE_DRIVER=workerpool\n"), 0o600); err != nil {
		t.Fatalf("write owner environment: %v", err)
	}

	renderer := unitProjectRenderer(t)
	if err := renderer.RenderAppOnly(project.DefaultNamedApp("worker"), makeapp.RenderOptions{
		Components: components,
		SkipWire:   true,
	}); err != nil {
		t.Fatalf("render existing App: %v", err)
	}
	environment, err := os.ReadFile(".env")
	if err != nil {
		t.Fatalf("read owner environment: %v", err)
	}
	for _, want := range []string{
		"QUEUE_SUPPORTED_DRIVERS=workerpool",
		"WORKER_QUEUE_DRIVER=workerpool",
	} {
		if !strings.Contains(string(environment), want) {
			t.Fatalf("owner environment omitted %q after App render:\n%s", want, environment)
		}
	}
}

// TestProjectRendererRejectsQueueContractBeforeLegacyCleanup keeps both owner files recoverable on validation failure.
func TestProjectRendererRejectsQueueContractBeforeLegacyCleanup(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	root := t.TempDir()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	config := &project.Config{
		ProjectName: "QueueConflict", GoModuleName: "example.com/queueconflict",
		Render: project.RenderConfig{Components: project.Components{CLI: true, Jobs: true}},
	}
	encoded, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	legacyConfig := strings.Replace(string(encoded), "render:\n", "render:\n    queue_driver: nats\n", 1)
	ownerEnv := "QUEUE_DRIVER=workerpool\nQUEUE_SUPPORTED_DRIVERS=redis\n"
	if err := os.WriteFile(".goforj.yml", []byte(legacyConfig), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(".env", []byte(ownerEnv), 0o600); err != nil {
		t.Fatalf("write environment: %v", err)
	}

	renderer := unitProjectRenderer(t)
	err = renderer.Render(ComponentRenderInput{renderAll: true})
	if err == nil || !strings.Contains(err.Error(), "excludes active QUEUE_DRIVER \"workerpool\"") {
		t.Fatalf("render error = %v, want supported-driver conflict", err)
	}
	configData, err := os.ReadFile(".goforj.yml")
	if err != nil {
		t.Fatalf("read config after failed render: %v", err)
	}
	if string(configData) != legacyConfig {
		t.Fatalf("failed migration rewrote legacy config:\n%s", configData)
	}
	envData, err := os.ReadFile(".env")
	if err != nil {
		t.Fatalf("read environment after failed render: %v", err)
	}
	if string(envData) != ownerEnv {
		t.Fatalf("failed migration rewrote owner environment:\n%s", envData)
	}
}

// TestProjectRendererRetainsLegacyQueueDriverWhenEnvWriteFails protects the only recoverable migration source.
func TestProjectRendererRetainsLegacyQueueDriverWhenEnvWriteFails(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	root := t.TempDir()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	config := &project.Config{
		ProjectName: "QueueWriteFailure", GoModuleName: "example.com/queuewritefailure",
		Render: project.RenderConfig{Components: project.Components{CLI: true, Jobs: true}},
	}
	encoded, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	legacyConfig := strings.Replace(string(encoded), "render:\n", "render:\n    queue_driver: nats\n", 1)
	ownerEnv := "APP_ENV=local\n"
	if err := os.WriteFile(".goforj.yml", []byte(legacyConfig), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(".env", []byte(ownerEnv), 0o600); err != nil {
		t.Fatalf("write environment: %v", err)
	}

	renderer := unitProjectRenderer(t)
	renderer.writeEnvironmentFile = func(string, []byte, os.FileMode) error {
		return errors.New("simulated environment replacement failure")
	}
	err = renderer.Render(ComponentRenderInput{renderAll: true})
	if err == nil || !strings.Contains(err.Error(), "simulated environment replacement failure") {
		t.Fatalf("render error = %v, want environment replacement failure", err)
	}
	configData, err := os.ReadFile(".goforj.yml")
	if err != nil {
		t.Fatalf("read config after failed replacement: %v", err)
	}
	if string(configData) != legacyConfig {
		t.Fatalf("failed replacement rewrote legacy config:\n%s", configData)
	}
	envData, err := os.ReadFile(".env")
	if err != nil {
		t.Fatalf("read environment after failed replacement: %v", err)
	}
	if string(envData) != ownerEnv {
		t.Fatalf("failed replacement rewrote owner environment:\n%s", envData)
	}
}

// TestProjectRendererMigratesLegacyQueueDriverIntoExistingEnv verifies environment persistence precedes YAML cleanup.
func TestProjectRendererMigratesLegacyQueueDriverIntoExistingEnv(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	root := t.TempDir()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	config := &project.Config{
		ProjectName: "QueueLegacy", GoModuleName: "example.com/queuelegacy",
		Render: project.RenderConfig{Components: project.Components{CLI: true, Jobs: true}},
	}
	encoded, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	legacyConfig := strings.Replace(string(encoded), "render:\n", "render:\n    queue_driver: nats\n", 1)
	if err := os.WriteFile(".goforj.yml", []byte(legacyConfig), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(".env", []byte("APP_ENV=local\n"), 0o600); err != nil {
		t.Fatalf("write environment: %v", err)
	}

	renderer := unitProjectRenderer(t)
	if err := renderer.Render(ComponentRenderInput{renderAll: true}); err != nil {
		t.Fatalf("render legacy project: %v", err)
	}
	envData, err := os.ReadFile(".env")
	if err != nil {
		t.Fatalf("read migrated environment: %v", err)
	}
	for _, want := range []string{"QUEUE_DRIVER=nats\n", "QUEUE_SUPPORTED_DRIVERS=nats\n"} {
		if !strings.Contains(string(envData), want) {
			t.Fatalf("migrated environment omitted %q:\n%s", want, envData)
		}
	}
	info, err := os.Stat(".env")
	if err != nil {
		t.Fatalf("stat migrated environment: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("migrated environment mode = %o, want 600", info.Mode().Perm())
	}
	configData, err := os.ReadFile(".goforj.yml")
	if err != nil {
		t.Fatalf("read migrated config: %v", err)
	}
	if strings.Contains(string(configData), "queue_driver:") {
		t.Fatalf("legacy queue driver survived successful environment migration:\n%s", configData)
	}
}

// TestProjectRendererDropsInapplicableLegacyQueueDriver avoids materializing queue state when Jobs is disabled.
func TestProjectRendererDropsInapplicableLegacyQueueDriver(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	root := t.TempDir()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	config := &project.Config{
		ProjectName: "NoJobs", GoModuleName: "example.com/nojobs",
		Render: project.RenderConfig{Components: project.Components{CLI: true}},
	}
	encoded, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	legacyConfig := strings.Replace(string(encoded), "render:\n", "render:\n    queue_driver: nats\n", 1)
	if err := os.WriteFile(".goforj.yml", []byte(legacyConfig), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	renderer := unitProjectRenderer(t)
	if err := renderer.Render(ComponentRenderInput{renderAll: true}); err != nil {
		t.Fatalf("render Jobs-disabled project: %v", err)
	}
	envData, err := os.ReadFile(".env")
	if err != nil {
		t.Fatalf("read environment: %v", err)
	}
	for _, line := range strings.Split(string(envData), "\n") {
		key, _, ok := envfile.ParseAssignment(line)
		if ok && strings.HasPrefix(key, "QUEUE_") {
			t.Fatalf("Jobs-disabled migration created %s:\n%s", key, envData)
		}
	}
	configData, err := os.ReadFile(".goforj.yml")
	if err != nil {
		t.Fatalf("read migrated config: %v", err)
	}
	if strings.Contains(string(configData), "queue_driver:") {
		t.Fatalf("inapplicable legacy queue driver survived migration:\n%s", configData)
	}
}

func TestProjectRendererAlwaysRendersEnvLocalWithInspectDefaults(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	root := t.TempDir()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	cfg := &project.Config{
		ProjectName:  "TraceApp",
		GoModuleName: "example.com/traceapp",
		UpdatedAt:    "2026-05-06 00:00:00 UTC",
		Render: project.RenderConfig{
			Components: project.Components{
				WebAPI: true,
			},
		},
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(".goforj.yml", data, 0o644); err != nil {
		t.Fatalf("write .goforj.yml: %v", err)
	}

	renderer := unitProjectRenderer(t)
	if err := renderer.Render(ComponentRenderInput{renderAll: true}); err != nil {
		t.Fatalf("initial render: %v", err)
	}

	envLocalPath := filepath.Join(root, ".env.local")
	envLocal, err := os.ReadFile(envLocalPath)
	if err != nil {
		t.Fatalf("read .env.local: %v", err)
	}
	text := string(envLocal)
	for _, want := range []string{
		"LIGHTHOUSE_INSPECT_ENABLED=true",
		"LIGHTHOUSE_INSPECT_MAX_TOTAL=1000",
		"LIGHTHOUSE_INSPECT_MAX_INFLIGHT=250",
		"LIGHTHOUSE_INSPECT_MAX_EVENTS=500",
		"LIGHTHOUSE_INSPECT_SAMPLE_RATE=1.0",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf(".env.local missing %q\n%s", want, text)
		}
	}
	if strings.Contains(text, "AUTH_SESSION_IDLE_TTL") {
		t.Fatalf(".env.local contains an inert auth override:\n%s", text)
	}

	if err := os.WriteFile(envLocalPath, []byte("LIGHTHOUSE_INSPECT_ENABLED=false\n"), 0o644); err != nil {
		t.Fatalf("mutate .env.local: %v", err)
	}

	if err := renderer.Render(ComponentRenderInput{renderAll: true}); err != nil {
		t.Fatalf("rerender: %v", err)
	}

	envLocal, err = os.ReadFile(envLocalPath)
	if err != nil {
		t.Fatalf("read rerendered .env.local: %v", err)
	}
	text = string(envLocal)
	if !strings.Contains(text, "LIGHTHOUSE_INSPECT_ENABLED=true") {
		t.Fatalf(".env.local did not restore inspect defaults on rerender\n%s", text)
	}
	if strings.Contains(text, "LIGHTHOUSE_INSPECT_ENABLED=false") {
		t.Fatalf(".env.local retained stale local override after rerender\n%s", text)
	}
}
