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
		"LIGHTHOUSE_INSPECT_MAX=1000",
		"LIGHTHOUSE_INSPECT_MAX_PER_SOURCE=250",
		"LIGHTHOUSE_INSPECT_MAX_EVENTS=500",
		"LIGHTHOUSE_INSPECT_TTL=6h",
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
