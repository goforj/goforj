package project

import (
	"slices"
	"testing"
)

// TestFeatureResourceIsOptionalAndAppScoped keeps rollout configuration out of existing project defaults.
func TestFeatureResourceIsOptionalAndAppScoped(t *testing.T) {
	if DefaultSelectedComponents().Feature {
		t.Fatal("feature was enabled by default")
	}
	disabled, err := DefaultResourcePlan(Components{})
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := disabled.Selection(ResourceFeature); exists {
		t.Fatal("disabled feature has a resource")
	}
	config := &Config{Apps: map[string]AppConfig{"billing": {Components: Components{Feature: true}}}}
	if !ProjectComponents(config).Feature || config.Render.Components.Feature {
		t.Fatal("App enablement changed default App")
	}
	plan, err := DefaultResourcePlan(ProjectComponents(config))
	if err != nil {
		t.Fatal(err)
	}
	resource, exists := plan.Selection(ResourceFeature)
	if !exists || resource.Active != "memory" || !slices.Contains(resource.Supported, "file") {
		t.Fatalf("resource = %#v", resource)
	}
}
