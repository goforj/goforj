package scenarios

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

func TestLoadEmbeddedScenarioSpecs(t *testing.T) {
	specs, err := loadScenarioSpecs("")
	if err != nil {
		t.Fatalf("load specs: %v", err)
	}
	if len(specs) == 0 {
		t.Fatal("expected embedded specs")
	}
	found := false
	for _, spec := range specs {
		if spec.ID == "json-api-route" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected json-api-route spec")
	}
}

// TestScenarioSpecsDeclareCumulativePrimitiveDependencies keeps each independently rendered scenario truthful about the prior steps it applies.
func TestScenarioSpecsDeclareCumulativePrimitiveDependencies(t *testing.T) {
	specs, err := loadScenarioSpecs("")
	if err != nil {
		t.Fatalf("load specs: %v", err)
	}
	byID := make(map[string]ScenarioSpec, len(specs))
	for _, spec := range specs {
		byID[spec.ID] = spec
	}

	tests := []struct {
		id   string
		want project.Components
	}{
		{id: "json-api-route", want: project.Components{}},
		{id: "cached-user-profile", want: project.Components{Cache: true}},
		{id: "file-upload-storage", want: project.Components{Cache: true, Storage: true}},
		{id: "users-created-event", want: project.Components{Cache: true, Events: true}},
		{id: "reports-generate-job", want: project.Components{Cache: true, Events: true, Storage: true, Jobs: true}},
		{id: "reports-daily-schedule", want: project.Components{Cache: true, Events: true, Storage: true, Jobs: true}},
		{id: "runtime-observability", want: project.Components{Cache: true, Events: true, Storage: true, Jobs: true}},
	}
	for _, test := range tests {
		t.Run(test.id, func(t *testing.T) {
			spec, ok := byID[test.id]
			if !ok {
				t.Fatalf("scenario %q was not loaded", test.id)
			}
			got := project.Components{
				Cache:   spec.App.Components.Cache,
				Events:  spec.App.Components.Events,
				Storage: spec.App.Components.Storage,
				Jobs:    spec.App.Components.Jobs,
			}
			if got != test.want {
				t.Fatalf("scenario primitive components = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestRenderScenarioMarkdownIncludesVerificationBanner(t *testing.T) {
	specs, err := selectedScenarioSpecs("", []string{"json-api-route"}, false)
	if err != nil {
		t.Fatalf("select spec: %v", err)
	}
	body := renderScenarioMarkdown(specs[0])
	for _, token := range []string{
		"::: info Verified Scenario",
		"forj make:controller users",
		"forj build",
		"/api/v1/users/:id",
	} {
		if !strings.Contains(body, token) {
			t.Fatalf("generated markdown missing %q\n%s", token, body)
		}
	}
}

func TestScenarioGenerateCheckDetectsDrift(t *testing.T) {
	dir := t.TempDir()
	if err := Generate(GenerateOptions{OutDir: dir, IDs: []string{"json-api-route"}}); err != nil {
		t.Fatalf("generate: %v", err)
	}

	path := filepath.Join(dir, "json-api-route.md")
	if err := os.WriteFile(path, []byte("stale\n"), 0o644); err != nil {
		t.Fatalf("write stale doc: %v", err)
	}

	if err := Generate(GenerateOptions{OutDir: dir, Check: true, IDs: []string{"json-api-route"}}); err == nil {
		t.Fatal("expected check to detect drift")
	}
}
