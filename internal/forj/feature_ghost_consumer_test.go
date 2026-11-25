package forj

import (
	"testing"

	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/project"
)

// TestFeatureStaleAppOverlayDoesNotBlockDescribe keeps service discovery aligned with generation after an App is removed.
func TestFeatureStaleAppOverlayDoesNotBlockDescribe(t *testing.T) {
	usePrimitiveRendererRoot(t)
	config := &project.Config{
		ProjectName:  "Stale",
		GoModuleName: "example.com/stale",
		Render:       project.RenderConfig{Components: project.Components{CLI: true, Feature: true}},
	}
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatal(err)
	}
	writePrimitiveRendererFile(t, ".env", "FEATURE_DRIVER=memory\nMISSING_FEATURE_STALE_DRIVER=hosted\n")
	if _, err := generate.GenerateProjectFiles(".", generate.GenerationSelection{Feature: true}); err != nil {
		t.Fatalf("generate with stale App overlay: %v", err)
	}
	if _, err := projectDescribeServiceRequirements(".", config); err != nil {
		t.Fatalf("describe configuration accepted by generation: %v", err)
	}
}
