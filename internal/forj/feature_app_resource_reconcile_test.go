package forj

import (
	"slices"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/forj/makeapp"
	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/project"
)

// TestFeatureAppCreationReconcilesLegacyResources prevents stale managers when an App claims a former resource key.
func TestFeatureAppCreationReconcilesLegacyResources(t *testing.T) {
	usePrimitiveRendererRoot(t)
	components := project.Components{CLI: true, Feature: true, Cache: true}
	config := &project.Config{ProjectName: "Ownership", GoModuleName: "example.com/ownership", Render: project.RenderConfig{Components: components}}
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatal(err)
	}
	writePrimitiveRendererFile(t, ".env", "FEATURE_DRIVER=memory\nCACHE_DRIVER=memory\nCACHE_FEATURE_DRIVER=memory\nCACHE_FEATURE_CHECKOUT_DRIVER=memory\n")
	selection := generate.GenerationSelectionFromComponents(components)
	if _, err := generate.GenerateProjectFiles(".", selection); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readPrimitiveRendererFile(t, generate.CacheAccessorsPath), "Feature()") {
		t.Fatal("fixture is missing the previous cache resource")
	}
	app := project.DefaultNamedApp("cache")
	if err := unitProjectRenderer(t).RenderAppOnly(app, makeapp.RenderOptions{Components: project.Components{CLI: true, Feature: true}, SkipWire: true}); err != nil {
		t.Fatal(err)
	}
	accessors := readPrimitiveRendererFile(t, generate.CacheAccessorsPath)
	if strings.Contains(accessors, "Feature()") || strings.Contains(accessors, "FeatureCheckout()") {
		t.Fatal("App creation retained caches now owned by its feature manager")
	}
	if !strings.Contains(readPrimitiveRendererFile(t, generate.FeatureManagerPath), `"CACHE":`) || !strings.Contains(readPrimitiveRendererFile(t, generate.FeatureAccessorsPath), "Checkout()") {
		t.Fatal("App creation did not publish its feature resources")
	}
	if result, err := generate.GenerateProjectFiles(".", selection); err != nil || result.ChangedFiles != 0 {
		t.Fatalf("App creation left resource generation drift: %#v %v", result, err)
	}
	result, err := unitProjectRenderer(t).RemoveApp(app)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range generate.OutputPaths(selection) {
		if !slices.Contains(result.Updated, path) {
			t.Fatalf("App removal omitted reconciled resource artifact %s", path)
		}
	}
	if regenerated, err := generate.GenerateProjectFiles(".", selection); err != nil || regenerated.ChangedFiles != 0 {
		t.Fatalf("App removal left resource generation drift: %#v %v", regenerated, err)
	}
}
