package scenarios

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestLoadEmbeddedScenarioSpecs verifies the shipped catalog is non-empty and includes the golden-path entry point.
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

// TestValidateScenarioCatalog verifies malformed IDs and dependency graphs fail before execution can consume the catalog.
func TestValidateScenarioCatalog(t *testing.T) {
	tests := []struct {
		name    string
		specs   []ScenarioSpec
		wantErr string
	}{
		{
			name:    "absolute ID",
			specs:   []ScenarioSpec{{ID: "/tmp/escape"}},
			wantErr: `scenario ID "/tmp/escape" must be a safe slug`,
		},
		{
			name:    "traversal ID",
			specs:   []ScenarioSpec{{ID: "../escape"}},
			wantErr: `scenario ID "../escape" must be a safe slug`,
		},
		{
			name:    "forward path separator",
			specs:   []ScenarioSpec{{ID: "group/scenario"}},
			wantErr: `scenario ID "group/scenario" must be a safe slug`,
		},
		{
			name:    "backward path separator",
			specs:   []ScenarioSpec{{ID: `group\scenario`}},
			wantErr: `scenario ID "group\\scenario" must be a safe slug`,
		},
		{
			name:    "duplicate ID",
			specs:   []ScenarioSpec{{ID: "first"}, {ID: "first"}},
			wantErr: `duplicate scenario ID "first"`,
		},
		{
			name:    "unknown dependency",
			specs:   []ScenarioSpec{{ID: "first", DependsOn: []string{"missing"}}},
			wantErr: `scenario "first" depends on unknown scenario "missing"`,
		},
		{
			name: "dependency cycle",
			specs: []ScenarioSpec{
				{ID: "first", DependsOn: []string{"second"}},
				{ID: "second", DependsOn: []string{"third"}},
				{ID: "third", DependsOn: []string{"first"}},
			},
			wantErr: "scenario dependency cycle: first -> second -> third -> first",
		},
		{
			name: "valid DAG",
			specs: []ScenarioSpec{
				{ID: "base"},
				{ID: "cache", DependsOn: []string{"base"}},
				{ID: "events", DependsOn: []string{"base"}},
				{ID: "app", DependsOn: []string{"cache", "events"}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateScenarioCatalog(test.specs)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("validate catalog: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validate catalog succeeded, want error containing %q", test.wantErr)
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validate catalog error = %q, want substring %q", err, test.wantErr)
			}
		})
	}
}

// TestUnsafeScenarioIDCannotMutateOutsideRoots verifies generation and execution reject a catalog before ID-derived paths are touched.
func TestUnsafeScenarioIDCannotMutateOutsideRoots(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, specDir, operationRoot string) error
	}{
		{
			name: "generate",
			run: func(t *testing.T, specDir, operationRoot string) error {
				t.Helper()
				return Generate(GenerateOptions{SpecDir: specDir, OutDir: operationRoot})
			},
		},
		{
			name: "validate",
			run: func(t *testing.T, specDir, operationRoot string) error {
				t.Helper()
				return Validate(ValidateOptions{SpecDir: specDir, WorkDir: operationRoot, ForjExec: "unused"})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := t.TempDir()
			specDir := filepath.Join(base, "specs")
			operationRoot := filepath.Join(base, "operation")
			outsideRoot := filepath.Join(base, "outside")
			if err := os.MkdirAll(outsideRoot, 0o755); err != nil {
				t.Fatalf("create outside root: %v", err)
			}
			sentinelPath := filepath.Join(outsideRoot, "sentinel.txt")
			if err := os.WriteFile(sentinelPath, []byte("preserve me"), 0o644); err != nil {
				t.Fatalf("write sentinel: %v", err)
			}
			outsideMarkdownPath := filepath.Join(base, "outside.md")
			if err := os.WriteFile(outsideMarkdownPath, []byte("preserve this too"), 0o644); err != nil {
				t.Fatalf("write markdown sentinel: %v", err)
			}
			writeScenarioSpecFixture(t, specDir, "unsafe.yaml", "id: ../outside\ntitle: Unsafe scenario\n")

			err := test.run(t, specDir, operationRoot)
			if err == nil {
				t.Fatal("operation succeeded with an unsafe scenario ID")
			}
			body, readErr := os.ReadFile(sentinelPath)
			if readErr != nil {
				t.Fatalf("read sentinel after rejection: %v", readErr)
			}
			if string(body) != "preserve me" {
				t.Fatalf("sentinel content = %q, want it unchanged", body)
			}
			markdownBody, readErr := os.ReadFile(outsideMarkdownPath)
			if readErr != nil {
				t.Fatalf("read markdown sentinel after rejection: %v", readErr)
			}
			if string(markdownBody) != "preserve this too" {
				t.Fatalf("markdown sentinel content = %q, want it unchanged", markdownBody)
			}
			if _, statErr := os.Stat(operationRoot); !os.IsNotExist(statErr) {
				t.Fatalf("operation root was touched before catalog validation: %v", statErr)
			}
		})
	}
}

// writeScenarioSpecFixture writes an external catalog fixture so public entry points exercise the same loader as user-provided specs.
func writeScenarioSpecFixture(t *testing.T, specDir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("create spec dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(specDir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write scenario spec: %v", err)
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

// TestRenderScenarioMarkdownIncludesVerificationBanner verifies generated documentation describes its executable contract.
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

// TestScenarioGenerateCheckDetectsDrift verifies check mode compares rendered content without overwriting it.
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
