package forj

import (
	"testing"

	"github.com/goforj/goforj/project"
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
