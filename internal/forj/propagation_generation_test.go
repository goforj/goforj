package forj

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// TestRenderedMessagePropagationSurvivesGeneration prevents duplicate adapters when a new app renders and then regenerates.
func TestRenderedMessagePropagationSurvivesGeneration(t *testing.T) {
	t.Setenv("EVENTS_DRIVER", "inproc")
	t.Setenv("QUEUE_DRIVER", "workerpool")
	root := t.TempDir()
	config := &project.Config{
		ProjectName: "AsyncAdapters", GoModuleName: "example.com/asyncadapters",
		Render: project.RenderConfig{Components: project.Components{CLI: true, Events: true, Jobs: true}},
	}
	content, err := yaml.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := unitProjectRenderer(t).Render(ComponentRenderInput{root: root, renderAll: true}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "internal", "inspects", "propagation_gen.go")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, generator := range []func(string) (int, error){generate.GenerateEventFiles, generate.GenerateQueueFiles} {
		if _, err := generator(root); err != nil {
			t.Fatal(err)
		}
		if changed, err := generator(root); err != nil || changed != 0 {
			t.Fatalf("repeated generation changed=%d, error=%v", changed, err)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("generation changed the rendered adapter: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "internal", "inspects", "propagation.go")); !os.IsNotExist(err) {
		t.Fatalf("render created a second adapter: %v", err)
	}
}
