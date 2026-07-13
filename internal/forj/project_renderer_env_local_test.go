package forj

import (
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
