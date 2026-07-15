package coredeps

import (
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestSyncCoreLibrariesGatesCacheModules verifies disabled Cache projects do not acquire project-wide Cache dependencies.
func TestSyncCoreLibrariesGatesCacheModules(t *testing.T) {
	disabled := SyncCoreLibraries(project.Components{})
	enabled := SyncCoreLibraries(project.Components{Cache: true})

	for _, module := range []string{
		"github.com/goforj/cache@",
		"github.com/goforj/cache/cachecore@",
		"github.com/goforj/cache/driver/rediscache@",
	} {
		if containsModulePrefix(disabled, module) {
			t.Fatalf("Cache-disabled modules contain %q: %#v", module, disabled)
		}
		if !containsModulePrefix(enabled, module) {
			t.Fatalf("Cache-enabled modules omit %q: %#v", module, enabled)
		}
	}
}

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

// TestSyncCoreLibrariesGatesJobsModules verifies Queue and every compiled Queue driver remain pinned but are synchronized only for Jobs projects.
func TestSyncCoreLibrariesGatesJobsModules(t *testing.T) {
	disabled := SyncCoreLibraries(project.Components{})
	enabled := SyncCoreLibraries(project.Components{Jobs: true})

	for _, module := range []string{
		"github.com/goforj/queue",
		"github.com/goforj/queue/driver/mysqlqueue",
		"github.com/goforj/queue/driver/natsqueue",
		"github.com/goforj/queue/driver/postgresqueue",
		"github.com/goforj/queue/driver/rabbitmqqueue",
		"github.com/goforj/queue/driver/redisqueue",
		"github.com/goforj/queue/driver/sqlitequeue",
		"github.com/goforj/queue/driver/sqlqueuecore",
		"github.com/goforj/queue/driver/sqsqueue",
	} {
		if containsModulePrefix(disabled, module+"@") {
			t.Fatalf("Jobs-disabled modules contain %q: %#v", module, disabled)
		}
		if !containsModulePrefix(enabled, module+"@") {
			t.Fatalf("Jobs-enabled modules omit %q: %#v", module, enabled)
		}
		if _, ok := VersionFor(module); !ok {
			t.Fatalf("pinned module catalog omits %q", module)
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
