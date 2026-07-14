package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

func TestWriteYAMLPreservesRenderComponents(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".goforj.yml")
	cfg := project.Config{
		ProjectName:  "RenderPreferred",
		GoModuleName: "example.com/renderpreferred",
		Render: project.RenderConfig{
			Components: project.Components{
				WebAPI:         true,
				Auth:           true,
				DatabaseSQLite: true,
			},
		},
	}

	if err := WriteYAML(path, cfg); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	loaded := readWrittenConfig(t, path)
	if !loaded.Render.Components.WebAPI || !loaded.Render.Components.Auth || !loaded.Render.Components.DatabaseSQLite {
		t.Fatalf("render components not preserved: %#v", loaded.Render.Components)
	}
}

func TestWriteYAMLPreservesRawComponentDependencies(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".goforj.yml")
	cfg := project.Config{
		ProjectName:  "DependencyShape",
		GoModuleName: "example.com/dependencyshape",
		Render: project.RenderConfig{
			Components: project.Components{
				Auth:           true,
				WebAPI:         true,
				DatabaseSQLite: true,
			},
		},
	}

	if err := WriteYAML(path, cfg); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	loaded := readWrittenConfig(t, path)
	if !loaded.Render.Components.Auth {
		t.Fatalf("expected auth to remain selected")
	}
	if loaded.Render.Components.Mail {
		t.Fatalf("expected raw yaml to preserve mail=false, got %#v", loaded.Render.Components)
	}
}

func TestWriteYAMLAppliesDefaultsWithoutMutatingComponents(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".goforj.yml")
	cfg := project.Config{
		ProjectName:  "Defaults",
		GoModuleName: "example.com/defaults",
		Render: project.RenderConfig{
			Components: project.Components{
				CLI:           true,
				WebAPI:        true,
				DatabaseMySQL: true,
			},
		},
	}

	if err := WriteYAML(path, cfg); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	loaded := readWrittenConfig(t, path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written config: %v", err)
	}
	if strings.Contains(string(data), "queue_driver:") {
		t.Fatalf("test render config persisted wizard-only queue choice:\n%s", data)
	}
	if !loaded.Render.Components.CLI || !loaded.Render.Components.WebAPI || !loaded.Render.Components.DatabaseMySQL {
		t.Fatalf("render components changed unexpectedly: %#v", loaded.Render.Components)
	}
}

func TestBuildRenderCombosSkipsInvalidAuthSelections(t *testing.T) {
	for _, profile := range []string{renderProfileSmoke, renderProfilePR, renderProfileFull} {
		for _, combo := range buildRenderCombos(profile) {
			if combo.components.Auth && !combo.components.WebAPI {
				t.Fatalf("%s combo includes invalid auth selection: %#v", profile, combo.components)
			}
			if err := combo.components.ValidateRenderContract(); err != nil {
				t.Fatalf("%s combo %q violates render contract: %v", profile, combo.id, err)
			}
		}
	}
}

func TestSelectedRenderProfile(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		full    bool
		want    string
	}{
		{name: "default", want: renderProfilePR},
		{name: "smoke", profile: renderProfileSmoke, want: renderProfileSmoke},
		{name: "pr", profile: renderProfilePR, want: renderProfilePR},
		{name: "full", profile: renderProfileFull, want: renderProfileFull},
		{name: "legacy full flag wins", profile: renderProfileSmoke, full: true, want: renderProfileFull},
		{name: "unknown falls back to pr", profile: "unknown", want: renderProfilePR},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := selectedRenderProfile(tt.profile, tt.full); got != tt.want {
				t.Fatalf("selectedRenderProfile(%q, %v) = %q, want %q", tt.profile, tt.full, got, tt.want)
			}
		})
	}
}

func TestRenderProfilesHaveExpectedCoverageShape(t *testing.T) {
	smoke := buildRenderCombos(renderProfileSmoke)
	pr := buildRenderCombos(renderProfilePR)
	full := buildRenderCombos(renderProfileFull)

	if len(smoke) == 0 {
		t.Fatal("smoke profile should include at least one combo")
	}
	if len(smoke) >= len(pr) {
		t.Fatalf("smoke profile should be smaller than pr profile: smoke=%d pr=%d", len(smoke), len(pr))
	}
	if len(pr) >= len(full) {
		t.Fatalf("pr profile should be smaller than full profile: pr=%d full=%d", len(pr), len(full))
	}
	if !renderCombosInclude(smoke, func(c project.Components) bool {
		return c.Auth && c.WebAPI && c.DatabaseMySQL
	}) {
		t.Fatal("smoke profile should include an auth/webapi/mysql combo")
	}
	if !renderCombosInclude(pr, func(c project.Components) bool {
		return c.DatabaseMySQL
	}) || !renderCombosInclude(pr, func(c project.Components) bool {
		return c.DatabasePostgres
	}) || !renderCombosInclude(pr, func(c project.Components) bool {
		return c.DatabaseSQLite
	}) {
		t.Fatal("pr profile should include mysql, postgres, and sqlite coverage")
	}
}

// TestPRRenderProfileIncludesRecommendedAndLeanPrimitiveSentinels keeps compatibility and opt-out coverage explicit.
func TestPRRenderProfileIncludesRecommendedAndLeanPrimitiveSentinels(t *testing.T) {
	combos := buildRenderCombos(renderProfilePR)
	recommended := project.DefaultSelectedComponents()
	foundRecommended := false
	foundLean := false
	for _, combo := range combos {
		if combo.id == "sentinel_recommended_default" {
			foundRecommended = true
			if combo.components != recommended {
				t.Fatalf("recommended sentinel = %#v, want %#v", combo.components, recommended)
			}
			if !combo.components.Cache || !combo.components.Events || !combo.components.Storage || !combo.components.Jobs {
				t.Fatalf("recommended sentinel omitted a default primitive: %#v", combo.components)
			}
		}
		if !combo.components.Cache && !combo.components.Events && !combo.components.Storage && !combo.components.Jobs {
			foundLean = true
		}
	}
	if !foundRecommended {
		t.Fatal("pr profile should include the recommended-default sentinel")
	}
	if !foundLean {
		t.Fatal("pr profile should include a lean primitive-free sentinel")
	}
}

// TestComponentLabelsIncludeEnabledPrimitiveComponents keeps render diagnostics truthful about the selected matrix shape.
func TestComponentLabelsIncludeEnabledPrimitiveComponents(t *testing.T) {
	wants := []string{"Cache", "Events", "Storage"}
	labels := componentLabels(project.Components{Cache: true, Events: true, Storage: true})
	seen := make(map[string]bool, len(labels))
	for _, label := range labels {
		seen[label] = true
	}
	for _, want := range wants {
		if !seen[want] {
			t.Fatalf("component labels = %#v, want %q", labels, want)
		}
	}

	leanLabels := componentLabels(project.Components{})
	for _, label := range leanLabels {
		for _, unwanted := range wants {
			if label == unwanted {
				t.Fatalf("lean component labels unexpectedly include %q: %#v", unwanted, leanLabels)
			}
		}
	}
}

func TestPRRenderProfileCoversComponentCatalog(t *testing.T) {
	combos := buildRenderCombos(renderProfilePR)
	requireRenderCombosCoverCatalog(t, renderProfilePR, combos)
}

func TestFullRenderProfileCoversComponentCatalog(t *testing.T) {
	combos := buildRenderCombos(renderProfileFull)
	requireRenderCombosCoverCatalog(t, renderProfileFull, combos)
}

func requireRenderCombosCoverCatalog(t *testing.T, profile string, combos []renderCombo) {
	t.Helper()
	for _, definition := range project.ComponentCatalog() {
		if !renderCombosInclude(combos, func(c project.Components) bool {
			return c.Enabled(definition.Key)
		}) {
			t.Fatalf("%s profile should include component %q at least once", profile, definition.Key)
		}
	}
}

func TestPRRenderProfileIncludesCriticalInteractions(t *testing.T) {
	combos := buildRenderCombos(renderProfilePR)
	requireRenderCombo(t, combos, "auth webapi db", func(c project.Components) bool {
		return c.Auth && c.WebAPI && c.HasDatabase()
	})
	requireRenderCombo(t, combos, "oauth auth db", func(c project.Components) bool {
		return c.OAuth && c.Auth && c.HasDatabase()
	})
	requireRenderCombo(t, combos, "scheduler jobs", func(c project.Components) bool {
		return c.Scheduler && c.Jobs
	})
	requireRenderCombo(t, combos, "metrics observability grafana", func(c project.Components) bool {
		return c.Metrics && c.Observability && c.Grafana
	})
	requireRenderCombo(t, combos, "demo app database jobs", func(c project.Components) bool {
		return c.DemoApp && c.HasDatabase() && c.Jobs
	})
}

func TestRenderCombosHaveUniqueIDs(t *testing.T) {
	for _, profile := range []string{renderProfileSmoke, renderProfilePR, renderProfileFull} {
		seen := map[string]struct{}{}
		for _, combo := range buildRenderCombos(profile) {
			if _, ok := seen[combo.id]; ok {
				t.Fatalf("%s profile contains duplicate combo id %q", profile, combo.id)
			}
			seen[combo.id] = struct{}{}
		}
	}
}

func requireRenderCombo(t *testing.T, combos []renderCombo, name string, matches func(project.Components) bool) {
	t.Helper()
	if !renderCombosInclude(combos, matches) {
		t.Fatalf("pr profile should include %s coverage", name)
	}
}

func renderCombosInclude(combos []renderCombo, matches func(project.Components) bool) bool {
	for _, combo := range combos {
		if matches(combo.components) {
			return true
		}
	}
	return false
}

func readWrittenConfig(t *testing.T, path string) project.Config {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read yaml: %v", err)
	}
	var loaded project.Config
	if err := yaml.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal yaml: %v", err)
	}
	return loaded
}
