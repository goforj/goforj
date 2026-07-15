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
	if loaded.Render.Components.Mail || loaded.Render.Components.Cache {
		t.Fatalf("expected raw yaml to preserve unresolved dependencies, got %#v", loaded.Render.Components)
	}
	effective := loaded.Render.Components.WithResolvedDependencies()
	if !effective.Mail || !effective.Cache {
		t.Fatalf("expected effective Auth dependencies to include Mail and Cache, got %#v", effective)
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

// TestPRRenderProfileIncludesPrimitiveBoundaryAndMixedAppSentinels keeps component gates covered at both project and App projections.
func TestPRRenderProfileIncludesPrimitiveBoundaryAndMixedAppSentinels(t *testing.T) {
	combos := buildRenderCombos(renderProfilePR)
	wants := map[string]bool{
		"sentinel_primitives_all_off":                     false,
		"sentinel_primitives_all_on":                      false,
		"sentinel_cache_only":                             false,
		"sentinel_events_only":                            false,
		"sentinel_storage_only":                           false,
		"sentinel_web_metrics_grafana_without_primitives": false,
		"sentinel_named_app_events_only":                  false,
		"sentinel_named_app_cache_only":                   false,
		"sentinel_default_events_named_app_off":           false,
		"sentinel_named_app_jobs_only":                    false,
		"sentinel_default_jobs_named_app_off":             false,
	}
	for _, combo := range combos {
		if _, tracked := wants[combo.id]; tracked {
			wants[combo.id] = true
		}
		if combo.id == "sentinel_named_app_cache_only" {
			if combo.components.Cache {
				t.Fatal("named-App Cache sentinel unexpectedly enables Cache on the default App")
			}
			configured, ok := combo.apps["cache-worker"]
			if !ok || !configured.Components.Cache {
				t.Fatalf("named App Cache shape mismatch: %#v", combo.apps)
			}
			config := &project.Config{Render: project.RenderConfig{Components: combo.components}, Apps: combo.apps}
			if !project.ProjectComponents(config).Cache {
				t.Fatalf("named-App sentinel did not promote Cache into the project envelope: %#v", config)
			}
		}
		if combo.id != "sentinel_named_app_events_only" && combo.id != "sentinel_default_events_named_app_off" {
			continue
		}
		appName := "events-worker"
		wantDefaultEvents := false
		wantNamedEvents := true
		if combo.id == "sentinel_default_events_named_app_off" {
			appName = "api"
			wantDefaultEvents = true
			wantNamedEvents = false
		}
		if combo.components.Events != wantDefaultEvents {
			t.Fatalf("%s default App Events = %t, want %t", combo.id, combo.components.Events, wantDefaultEvents)
		}
		configured, ok := combo.apps[appName]
		if !ok || configured.Components.Events != wantNamedEvents {
			t.Fatalf("%s named App Events shape mismatch: %#v", combo.id, combo.apps)
		}
		config := &project.Config{Render: project.RenderConfig{Components: combo.components}, Apps: combo.apps}
		if !project.ProjectComponents(config).Events {
			t.Fatalf("named-App sentinel did not promote Events into the project envelope: %#v", config)
		}
	}
	for id, found := range wants {
		if !found {
			t.Fatalf("pr profile should include %q", id)
		}
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

// TestRenderComboAppsIncludesConfiguredAppsInStableOrder keeps multi-App compile validation deterministic.
func TestRenderComboAppsIncludesConfiguredAppsInStableOrder(t *testing.T) {
	apps, err := renderComboApps(renderCombo{apps: map[string]project.AppConfig{
		"worker":    {},
		"backstage": {},
	}})
	if err != nil {
		t.Fatalf("renderComboApps() error: %v", err)
	}
	want := []string{"app", "backstage", "worker"}
	if len(apps) != len(want) {
		t.Fatalf("render Apps = %#v, want %v", apps, want)
	}
	for index, name := range want {
		if apps[index].Name != name {
			t.Fatalf("render App %d = %q, want %q", index, apps[index].Name, name)
		}
	}
}

// TestSeedRenderComboAppsCreatesNamedEntrypointMarkers verifies clean workspaces expose configured Apps to conventional discovery.
func TestSeedRenderComboAppsCreatesNamedEntrypointMarkers(t *testing.T) {
	root := t.TempDir()
	apps := []project.App{project.DefaultApp(), project.DefaultNamedApp("events-worker")}
	if err := seedRenderComboApps(root, apps); err != nil {
		t.Fatalf("seedRenderComboApps() error: %v", err)
	}
	namedEntrypoint := filepath.Join(root, "cmd", "events-worker", "main.go")
	content, err := os.ReadFile(namedEntrypoint)
	if err != nil {
		t.Fatalf("read named App marker: %v", err)
	}
	if string(content) != "package main\n" {
		t.Fatalf("named App marker = %q", content)
	}
	if _, err := os.Stat(filepath.Join(root, "cmd", "app", "main.go")); !os.IsNotExist(err) {
		t.Fatalf("default App marker should be owned by the normal renderer: %v", err)
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
