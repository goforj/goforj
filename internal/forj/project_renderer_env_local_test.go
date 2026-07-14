package forj

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/logger"
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
		Render: project.RenderConfig{Components: project.Components{CLI: true, Jobs: true}},
	}
	encoded, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	legacyConfig := strings.Replace(string(encoded), "render:\n", "render:\n    queue_driver: \"\"\n", 1)
	if err := os.WriteFile(".goforj.yml", []byte(legacyConfig), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	renderer := NewProjectRenderer(logger.NewSilentLogger())
	if err := renderer.Render(ComponentRenderInput{renderAll: true, queueDriver: "nats"}); err != nil {
		t.Fatalf("initial render: %v", err)
	}
	envData, err := os.ReadFile(".env")
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	if !strings.Contains(string(envData), "QUEUE_DRIVER=nats ") || !strings.Contains(string(envData), "QUEUE_SUPPORTED_DRIVERS=nats ") {
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
	if err := renderer.Render(ComponentRenderInput{renderAll: true, queueDriver: "redis"}); err != nil {
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

// TestResolveQueueDriverSeed keeps the wizard selection transient while retaining one-way legacy migration.
func TestResolveQueueDriverSeed(t *testing.T) {
	tests := []struct {
		name     string
		selected string
		legacy   string
		want     string
	}{
		{name: "wizard selection wins", selected: " NATS ", legacy: "redis", want: "nats"},
		{name: "legacy config seeds missing selection", legacy: "workerpool", want: "workerpool"},
		{name: "invalid values use default", selected: "unknown", legacy: "invalid", want: "redis"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resolveQueueDriverSeed(test.selected, test.legacy); got != test.want {
				t.Fatalf("resolveQueueDriverSeed(%q, %q) = %q, want %q", test.selected, test.legacy, got, test.want)
			}
		})
	}
}

// TestReconcileQueueDriverEnvPreservesOwnership verifies migration fills only missing values and rejects owner conflicts.
func TestReconcileQueueDriverEnvPreservesOwnership(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		seed          string
		wantActive    string
		wantFragments []string
		wantUnchanged bool
		wantError     string
	}{
		{
			name:          "existing environment wins",
			input:         "QUEUE_DRIVER=workerpool\nQUEUE_SUPPORTED_DRIVERS=workerpool,redis\n",
			seed:          "nats",
			wantActive:    "workerpool",
			wantUnchanged: true,
		},
		{
			name:          "seed fills missing active",
			input:         "APP_ENV=local\nQUEUE_SUPPORTED_DRIVERS=nats,redis\n",
			seed:          "nats",
			wantActive:    "nats",
			wantFragments: []string{"QUEUE_DRIVER=nats\n", "QUEUE_SUPPORTED_DRIVERS=nats,redis\n"},
		},
		{
			name:          "seed fills missing pair",
			input:         "APP_ENV=local\n",
			seed:          "workerpool",
			wantActive:    "workerpool",
			wantFragments: []string{"QUEUE_DRIVER=workerpool\n", "QUEUE_SUPPORTED_DRIVERS=workerpool\n"},
		},
		{
			name:      "owner contract excludes active",
			input:     "QUEUE_DRIVER=workerpool\nQUEUE_SUPPORTED_DRIVERS=redis\n",
			seed:      "nats",
			wantError: "excludes active QUEUE_DRIVER \"workerpool\"",
		},
		{
			name:      "owner active is unknown",
			input:     "QUEUE_DRIVER=custom\nQUEUE_SUPPORTED_DRIVERS=custom\n",
			seed:      "redis",
			wantError: "selects unsupported driver \"custom\"",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			updated, active, changed, err := reconcileQueueDriverEnv([]byte(test.input), test.seed)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("reconcile error = %v, want %q", err, test.wantError)
				}
				if changed || updated != nil {
					t.Fatalf("invalid owner contract returned a rewrite: changed=%t data=%q", changed, updated)
				}
				return
			}
			if err != nil {
				t.Fatalf("reconcile queue environment: %v", err)
			}
			if active != test.wantActive {
				t.Fatalf("active driver = %q, want %q", active, test.wantActive)
			}
			if test.wantUnchanged {
				if changed || string(updated) != test.input {
					t.Fatalf("owner environment changed: changed=%t data=%q", changed, updated)
				}
				return
			}
			if !changed {
				t.Fatal("missing queue values did not request an environment write")
			}
			for _, fragment := range test.wantFragments {
				if !strings.Contains(string(updated), fragment) {
					t.Fatalf("updated environment omitted %q:\n%s", fragment, updated)
				}
			}
		})
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

	renderer := NewProjectRenderer(logger.NewSilentLogger())
	err = renderer.Render(ComponentRenderInput{renderAll: true, queueDriver: "redis"})
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

	renderer := NewProjectRenderer(logger.NewSilentLogger())
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

	renderer := NewProjectRenderer(logger.NewSilentLogger())
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

	renderer := NewProjectRenderer(logger.NewSilentLogger())
	if err := renderer.Render(ComponentRenderInput{renderAll: true}); err != nil {
		t.Fatalf("render Jobs-disabled project: %v", err)
	}
	envData, err := os.ReadFile(".env")
	if err != nil {
		t.Fatalf("read environment: %v", err)
	}
	for _, line := range strings.Split(string(envData), "\n") {
		key, _, ok := parseEnvLine(line)
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

	renderer := NewProjectRenderer(logger.NewSilentLogger())
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
