package coredeps

import (
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestSyncCoreLibrariesGatesEventsModules verifies disabled Events projects do not acquire project-wide Events dependencies.
func TestSyncCoreLibrariesGatesEventsModules(t *testing.T) {
	disabled := SyncCoreLibraries(project.Components{})
	enabled := SyncCoreLibraries(project.Components{Events: true})

	for _, module := range []string{"github.com/goforj/events@", "github.com/goforj/events/eventscore@"} {
		if containsModulePrefix(disabled, module) {
			t.Fatalf("Events-disabled modules contain %q: %#v", module, disabled)
		}
		if !containsModulePrefix(enabled, module) {
			t.Fatalf("Events-enabled modules omit %q: %#v", module, enabled)
		}
	}
}

// TestSyncCoreLibrariesGatesStorageModules verifies disabled Storage projects do not acquire project-wide Storage dependencies.
func TestSyncCoreLibrariesGatesStorageModules(t *testing.T) {
	disabled := SyncCoreLibraries(project.Components{})
	enabled := SyncCoreLibraries(project.Components{Storage: true})

	for _, module := range []string{
		"github.com/goforj/storage@",
		"github.com/goforj/storage/storagecore@",
		"github.com/goforj/storage/driver/localstorage@",
	} {
		if containsModulePrefix(disabled, module) {
			t.Fatalf("Storage-disabled modules contain %q: %#v", module, disabled)
		}
		if !containsModulePrefix(enabled, module) {
			t.Fatalf("Storage-enabled modules omit %q: %#v", module, enabled)
		}
	}
}

// containsModulePrefix reports whether a pinned module selection contains the requested module path.
func containsModulePrefix(modules []string, prefix string) bool {
	for _, module := range modules {
		if strings.HasPrefix(module, prefix) {
			return true
		}
	}
	return false
}
