package project

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestComponentYAMLKeysCoverCatalog protects persisted config when the render catalog gains a component.
func TestComponentYAMLKeysCoverCatalog(t *testing.T) {
	want := map[ComponentKey]struct{}{ComponentDemoApp: {}}
	for _, definition := range ComponentCatalog() {
		want[definition.Key] = struct{}{}
	}
	if len(componentYAMLKeys) != len(want) {
		t.Fatalf("persisted component key count = %d, want %d", len(componentYAMLKeys), len(want))
	}
	for _, key := range componentYAMLKeys {
		if _, exists := want[key]; !exists {
			t.Fatalf("persisted component key %q is absent from the render catalog", key)
		}
		delete(want, key)
	}
	if len(want) != 0 {
		t.Fatalf("render catalog components missing from persisted YAML keys: %#v", want)
	}
}

// TestComponentsYAMLMigratesLegacyMappingsEverywhere verifies project and App mappings load unchanged and rewrite canonically.
func TestComponentsYAMLMigratesLegacyMappingsEverywhere(t *testing.T) {
	input := `render:
  components:
    cli: true
    demo_app: false
    auth: true
    web_api: true
apps:
  worker:
    components:
      cli: true
      jobs: true
    starter_kit: none
`

	var config Config
	if err := yaml.Unmarshal([]byte(input), &config); err != nil {
		t.Fatalf("unmarshal legacy component mappings: %v", err)
	}
	if !config.NeedsComponentMigration() {
		t.Fatal("legacy component mappings were not marked for migration")
	}
	if !config.Render.Components.CLI || !config.Render.Components.Auth || !config.Render.Components.WebAPI || config.Render.Components.DemoApp {
		t.Fatalf("render components changed while loading legacy mapping: %#v", config.Render.Components)
	}
	worker := config.Apps["worker"].Components
	if !worker.CLI || !worker.Jobs {
		t.Fatalf("App components changed while loading legacy mapping: %#v", worker)
	}

	encoded, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("marshal migrated component config: %v", err)
	}
	for _, expected := range []string{
		"components: [cli, auth, web_api]",
		"components: [cli, jobs]",
	} {
		if !strings.Contains(string(encoded), expected) {
			t.Fatalf("migrated YAML omitted %q:\n%s", expected, encoded)
		}
	}

	var roundTripped Config
	if err := yaml.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal migrated component config: %v", err)
	}
	if roundTripped.NeedsComponentMigration() {
		t.Fatal("canonical component sequences were still marked for migration")
	}
	if roundTripped.Render.Components != config.Render.Components || roundTripped.Apps["worker"].Components != worker {
		t.Fatalf("components changed across migration round trip: %#v", roundTripped)
	}
}

// TestComponentsYAMLDropsRetiredLegacyKeysDuringMigration keeps old generated configs renderable without reviving removed components.
func TestComponentsYAMLDropsRetiredLegacyKeysDuringMigration(t *testing.T) {
	input := `render:
  components:
    jobs: true
    stress_test: true
`

	var config Config
	if err := yaml.Unmarshal([]byte(input), &config); err != nil {
		t.Fatalf("unmarshal legacy component mapping: %v", err)
	}
	if !config.NeedsComponentMigration() {
		t.Fatal("legacy component mapping was not marked for migration")
	}
	if !config.Render.Components.Jobs {
		t.Fatal("current component was lost while loading retired legacy key")
	}

	encoded, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("marshal migrated component config: %v", err)
	}
	if strings.Contains(string(encoded), "stress_test") {
		t.Fatalf("retired component survived migration:\n%s", encoded)
	}
}

// TestProjectConfigNeedsComponentMigrationFindsEachScope verifies App-only legacy mappings cannot hide behind canonical render config.
func TestProjectConfigNeedsComponentMigrationFindsEachScope(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name: "render mapping",
			input: `render:
  components:
    cli: true
`,
			want: true,
		},
		{
			name: "App mapping",
			input: `render:
  components: [cli]
apps:
  api:
    components: [web_api]
  worker:
    components:
      jobs: true
`,
			want: true,
		},
		{
			name: "canonical sequences",
			input: `render:
  components: [cli]
apps:
  api:
    components: [web_api]
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var config Config
			if err := yaml.Unmarshal([]byte(test.input), &config); err != nil {
				t.Fatalf("unmarshal project config: %v", err)
			}
			if got := config.NeedsComponentMigration(); got != test.want {
				t.Fatalf("NeedsComponentMigration() = %t, want %t", got, test.want)
			}
		})
	}
}

// TestComponentsYAMLSequenceRoundTripUsesCanonicalOrder verifies user ordering never destabilizes generated configuration.
func TestComponentsYAMLSequenceRoundTripUsesCanonicalOrder(t *testing.T) {
	input := `render:
  components: [jobs, web_api, demo_app, cli, docker, mail]
`

	var config Config
	if err := yaml.Unmarshal([]byte(input), &config); err != nil {
		t.Fatalf("unmarshal component sequence: %v", err)
	}
	if config.NeedsComponentMigration() {
		t.Fatal("component sequence was incorrectly marked for migration")
	}
	encoded, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("marshal component sequence: %v", err)
	}
	if !strings.Contains(string(encoded), "components: [cli, demo_app, mail, web_api, docker, jobs]") {
		t.Fatalf("components were not emitted in canonical order:\n%s", encoded)
	}

	var roundTripped Config
	if err := yaml.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal round-tripped component sequence: %v", err)
	}
	if roundTripped.Render.Components != config.Render.Components {
		t.Fatalf("components changed across sequence round trip: %#v", roundTripped.Render.Components)
	}
}

// TestComponentsYAMLPreservesEmptySequence verifies empty selections remain visible instead of becoming an omitted mapping.
func TestComponentsYAMLPreservesEmptySequence(t *testing.T) {
	input := `render:
  components: []
apps:
  ship:
    components: []
    starter_kit: none
`

	var config Config
	if err := yaml.Unmarshal([]byte(input), &config); err != nil {
		t.Fatalf("unmarshal empty component sequences: %v", err)
	}
	if config.NeedsComponentMigration() {
		t.Fatal("empty component sequences were incorrectly marked for migration")
	}
	if config.Render.Components != (Components{}) || config.Apps["ship"].Components != (Components{}) {
		t.Fatalf("empty component sequences decoded non-empty: %#v", config)
	}
	encoded, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("marshal empty component sequences: %v", err)
	}
	if count := strings.Count(string(encoded), "components: []"); count != 2 {
		t.Fatalf("empty component sequence count = %d, want 2:\n%s", count, encoded)
	}
}

// TestComponentsYAMLRejectsInvalidSequenceEntries provides focused errors for typos and repeated selections.
func TestComponentsYAMLRejectsInvalidSequenceEntries(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "unknown", input: "render:\n  components: [cli, web]\n", want: `unknown component "web"; valid components:`},
		{name: "retired canonical", input: "render:\n  components: [stress_test]\n", want: `unknown component "stress_test"; valid components:`},
		{name: "duplicate", input: "render:\n  components: [cli, web_api, cli]\n", want: `duplicate component "cli"; list each component once`},
		{name: "non string", input: "render:\n  components: [cli, true]\n", want: "entry 2 must be a component name"},
		{name: "unknown legacy key", input: "render:\n  components:\n    cli: true\n    custom_plugin: true\n", want: `unknown component "custom_plugin" in legacy mapping; valid components:`},
		{name: "duplicate legacy key", input: "render:\n  components:\n    cli: true\n    cli: false\n", want: `duplicate component "cli" in legacy mapping; define each component once`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var config Config
			err := yaml.Unmarshal([]byte(test.input), &config)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("unmarshal error = %v, want %q", err, test.want)
			}
		})
	}
}

// TestComponentsJSONRemainsBooleanObject protects Lighthouse and other JSON consumers from the YAML migration shape.
func TestComponentsJSONRemainsBooleanObject(t *testing.T) {
	original := Components{CLI: true, DemoApp: true, WebAPI: true, Jobs: true}
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal component JSON: %v", err)
	}
	expected := `{"cli":true,"demo_app":true,"mail":false,"auth":false,"oauth":false,"web_api":true,"web_ui":false,"metrics":false,"observability":false,"grafana":false,"docker":false,"database_mysql":false,"database_postgres":false,"database_sqlite":false,"scheduler":false,"jobs":true}`
	if string(encoded) != expected {
		t.Fatalf("component JSON = %s, want %s", encoded, expected)
	}
	var roundTripped Components
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal component JSON: %v", err)
	}
	if roundTripped != original {
		t.Fatalf("component JSON round trip = %#v, want %#v", roundTripped, original)
	}
}
