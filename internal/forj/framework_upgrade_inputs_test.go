package forj

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// TestFrameworkInputContractIgnoresIncidentalConfiguration keeps metadata schema growth from blocking comparisons of identical render intent.
func TestFrameworkInputContractIgnoresIncidentalConfiguration(t *testing.T) {
	config := &project.Config{ProjectName: "Recipe", GoModuleName: "forj", Render: project.RenderConfig{Components: project.Components{CLI: true}}}
	before, err := frameworkInputsDigest(config)
	if err != nil {
		t.Fatal(err)
	}
	config.UpdatedAt = "different machine metadata"
	config.Dev.Pre = []project.DevTask{{Name: "owner", Cmd: "never part of the render recipe"}}
	config.Render.GoForjVersion = "unrelated-scaffold-provenance"
	config.Extra = map[string]any{"secret": "not-in-render-recipe"}
	after, err := frameworkInputsDigest(config)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("incidental project configuration changed input contract v1")
	}
	config.Render.StarterKitOptions = project.NewStarterKitOptions(false)
	explicit, err := frameworkInputsDigest(config)
	if err != nil {
		t.Fatal(err)
	}
	if explicit == before {
		t.Fatal("an explicit optional feature selection was lost")
	}
	config.Apps = map[string]project.AppConfig{"worker": {Components: project.Components{CLI: true, Jobs: true}}}
	named, err := frameworkInputsDigest(config)
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
			if !reflect.DeepEqual(selection.Components, []string{field.Tag.Get("json")}) {
				t.Fatalf("component selection lost or duplicated: %v", selection.Components)
			}
			for _, named := range []bool{false, true} {
				config := &project.Config{ProjectName: "Recipe", GoModuleName: "forj", Apps: map[string]project.AppConfig{}}
				if named {
					config.Apps["worker"] = project.AppConfig{}
				}
				before, err := frameworkInputsDigest(config)
				if err != nil {
					t.Fatal(err)
				}
				if named {
					config.Apps["worker"] = project.AppConfig{Components: components}
				} else {
					config.Render.Components = components
				}
				after, err := frameworkInputsDigest(config)
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
