package forj

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// TestFrameworkInputContractIgnoresIncidentalConfiguration keeps metadata schema growth from blocking comparisons of identical render intent.
func TestFrameworkInputContractIgnoresIncidentalConfiguration(t *testing.T) {
	config := &project.Config{ProjectName: "Recipe", GoModuleName: "forj", Render: project.RenderConfig{Components: project.Components{CLI: true}}}
	before, err := frameworkInputsDigest(&frameworkRenderIntent{Config: config})
	if err != nil {
		t.Fatal(err)
	}
	config.UpdatedAt = "different machine metadata"
	config.Dev.Pre = []project.DevTask{{Name: "owner", Cmd: "never part of the render recipe"}}
	config.Render.GoForjVersion = "unrelated-scaffold-provenance"
	config.Extra = map[string]any{"secret": "not-in-render-recipe"}
	after, err := frameworkInputsDigest(&frameworkRenderIntent{Config: config})
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("incidental project configuration changed input contract v2")
	}
	config.Render.StarterKitOptions = project.NewStarterKitOptions(false)
	explicit, err := frameworkInputsDigest(&frameworkRenderIntent{Config: config})
	if err != nil {
		t.Fatal(err)
	}
	if explicit == before {
		t.Fatal("an explicit optional feature selection was lost")
	}
	config.Apps = map[string]project.AppConfig{"worker": {Components: project.Components{CLI: true, Jobs: true}}}
	named, err := frameworkInputsDigest(&frameworkRenderIntent{Config: config})
	if err != nil {
		t.Fatal(err)
	}
	if named == explicit {
		t.Fatal("named App participation was lost")
	}
}

// TestFrameworkInputContractTracksAllComponents prevents supported persisted flags from disappearing behind wizard-only catalogs.
func TestFrameworkInputContractTracksAllComponents(t *testing.T) {
	shape := reflect.TypeFor[project.Components]()
	for index := range shape.NumField() {
		field := shape.Field(index)
		t.Run(field.Name, func(t *testing.T) {
			var components project.Components
			reflect.ValueOf(&components).Elem().Field(index).SetBool(true)
			selection := frameworkInputSelection(components, project.StarterKitNone, nil, "")
			if !reflect.DeepEqual(selection.Components, []string{strings.SplitN(field.Tag.Get("json"), ",", 2)[0]}) {
				t.Fatalf("component selection lost or duplicated: %v", selection.Components)
			}
			for _, named := range []bool{false, true} {
				config := &project.Config{ProjectName: "Recipe", GoModuleName: "forj", Apps: map[string]project.AppConfig{}}
				if named {
					config.Apps["worker"] = project.AppConfig{}
				}
				before, err := frameworkInputsDigest(&frameworkRenderIntent{Config: config})
				if err != nil {
					t.Fatal(err)
				}
				if named {
					config.Apps["worker"] = project.AppConfig{Components: components}
				} else {
					config.Render.Components = components
				}
				after, err := frameworkInputsDigest(&frameworkRenderIntent{Config: config})
				if err != nil || before == after {
					t.Fatalf("named=%v: component was omitted from recipe: %v", named, err)
				}
			}
		})
	}
}

// TestFrameworkInputRecheckDetectsDemoSelection rejects stale intent even when only a hidden wizard selection changes.
func TestFrameworkInputRecheckDetectsDemoSelection(t *testing.T) {
	root := t.TempDir()
	config := &project.Config{ProjectName: "Recipe", GoModuleName: "forj", Render: project.RenderConfig{Components: project.Components{CLI: true}}}
	var expected string
	for _, changed := range []bool{false, true} {
		config.Render.Components.DemoApp = changed
		source, err := yaml.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), source, 0o600); err != nil {
			t.Fatal(err)
		}
		if !changed {
			projected, err := frameworkUpgradeConfig(root)
			if err != nil {
				t.Fatal(err)
			}
			expected, err = frameworkInputsDigest(projected)
			if err != nil {
				t.Fatal(err)
			}
			if err := verifyFrameworkInputs(root, expected); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := verifyFrameworkInputs(root, expected); err == nil {
			t.Fatal("accepted a changed demo selection")
		}
	}
}

// TestFrameworkInputContractSelectionOrderIsStable fingerprints component keys independently of catalogue presentation order.
func TestFrameworkInputContractSelectionOrderIsStable(t *testing.T) {
	selection := frameworkInputSelection(project.Components{CLI: true, Mail: true, Cache: true}, project.StarterKitNone, nil, project.HelpFormatGuided)
	if len(selection.Components) != 3 || selection.Components[0] != "cache" || selection.Components[1] != "cli" || selection.Components[2] != "mail" {
		t.Fatalf("unstable component keys: %v", selection.Components)
	}
	if selection.ComponentLibrary != nil {
		t.Fatal("an omitted engine default was materialized in the recipe")
	}
}

// TestFrameworkInputDiscoveryTracksParticipation freezes discovery independently of explicit settings across renderer projections.
func TestFrameworkInputDiscoveryTracksParticipation(t *testing.T) {
	t.Setenv("TMPDIR", "/tmp")
	root := t.TempDir()
	config := &project.Config{ProjectName: "Discovery recipe", GoModuleName: "example.com/discovery", Render: project.RenderConfig{Components: project.Components{CLI: true}},
		Apps: map[string]project.AppConfig{"worker": {Components: project.Components{CLI: true}}}}
	if err := writeFrameworkProjection(root, &frameworkRenderIntent{Config: config}); err != nil {
		t.Fatal(err)
	}
	intent, err := frameworkUpgradeConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(intent.DiscoveredApps) != 0 || !reflect.DeepEqual(intent.Apps, config.Apps) {
		t.Fatal("explicit settings synthesized filesystem discovery")
	}
	before, err := frameworkInputsDigest(intent)
	if err != nil {
		t.Fatal(err)
	}
	writeProjectRendererTestFile(t, filepath.Join(root, "cmd/worker/main.go"), "package main\n")
	if err := verifyFrameworkInputs(root, before); err == nil {
		t.Fatal("accepted changed discovery for an explicitly configured App")
	}
	writeProjectRendererTestFile(t, filepath.Join(root, "cmd/inherited/main.go"), "package main\n")
	intent, err = frameworkUpgradeConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(intent.DiscoveredApps, []string{"inherited", "worker"}) || !reflect.DeepEqual(intent.Apps, config.Apps) {
		t.Fatal("discovery changed the explicit recipe or lost filesystem participation")
	}
	expected, err := frameworkInputsDigest(intent)
	if err != nil {
		t.Fatal(err)
	}
	peerRoot := t.TempDir()
	if err := writeFrameworkProjection(peerRoot, intent); err != nil {
		t.Fatal(err)
	}
	if err := verifyFrameworkInputs(peerRoot, expected); err != nil {
		t.Fatalf("peer projection changed the frozen recipe: %v", err)
	}
	intent.DiscoveredApps[0], intent.DiscoveredApps[1] = intent.DiscoveredApps[1], intent.DiscoveredApps[0]
	reordered, err := frameworkInputsDigest(intent)
	if err != nil || reordered != expected || !reflect.DeepEqual(intent.DiscoveredApps, []string{"worker", "inherited"}) {
		t.Fatalf("digest depended on discovery order or mutated intent: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "cmd/inherited/main.go")); err != nil {
		t.Fatal(err)
	}
	if err := verifyFrameworkInputs(root, expected); err == nil {
		t.Fatal("accepted removal of an inherited App")
	}
}
