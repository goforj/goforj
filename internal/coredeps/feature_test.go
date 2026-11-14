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

// TestFeatureWatcherPinIsComponentGated keeps runtime notification support optional and aligned with the CLI.
func TestFeatureWatcherPinIsComponentGated(t *testing.T) {
	pin := "github.com/fsnotify/fsnotify@v1.10.1"
	if slices.Contains(SyncCoreLibraries(project.Components{}), pin) {
		t.Fatal("disabled component selected file watcher")
	}
	if !slices.Contains(SyncCoreLibraries(project.Components{Feature: true}), pin) || MustVersionFor("github.com/fsnotify/fsnotify") != "v1.10.1" {
		t.Fatal("missing pinned feature watcher")
	}
}
