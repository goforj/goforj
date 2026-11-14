package coredeps

import (
	"slices"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestFeaturePinIsComponentGated avoids adding feature dependencies to existing generated projects.
func TestFeaturePinIsComponentGated(t *testing.T) {
	if slices.Contains(SyncCoreLibraries(project.Components{}), "github.com/goforj/feature@v0.1.0") {
		t.Fatal("disabled component selected feature")
	}
	if !slices.Contains(SyncCoreLibraries(project.Components{Feature: true}), "github.com/goforj/feature@v0.1.0") || MustVersionFor("github.com/goforj/feature") != "v0.1.0" {
		t.Fatal("missing released feature pin")
	}
}
