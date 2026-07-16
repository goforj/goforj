package coredeps

import (
	"slices"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestSyncCoreLibrariesGatesCacheModules verifies every Cache module is synchronized only for Cache projects.
func TestSyncCoreLibrariesGatesCacheModules(t *testing.T) {
	assertModulesGated(t, project.Components{}, project.Components{Cache: true}, cacheRendererSyncModules)
}

// TestSyncCoreLibrariesGatesEventsModules verifies every Events runtime module is synchronized only for Events projects.
func TestSyncCoreLibrariesGatesEventsModules(t *testing.T) {
	assertModulesGated(t, project.Components{}, project.Components{Events: true}, eventsRendererSyncModules)
}

// TestSyncCoreLibrariesGatesStorageModules verifies every Storage module is synchronized only for Storage projects.
func TestSyncCoreLibrariesGatesStorageModules(t *testing.T) {
	assertModulesGated(t, project.Components{}, project.Components{Storage: true}, storageRendererSyncModules)
}

// TestSyncCoreLibrariesGatesMailModules verifies Mail and its independently versioned SES driver are synchronized only for Mail projects.
func TestSyncCoreLibrariesGatesMailModules(t *testing.T) {
	assertModulesGated(t, project.Components{}, project.Components{Mail: true}, mailRendererSyncModules)
}

// TestSyncCoreLibrariesGatesCryptModule verifies database-capable projects retain the encryption dependency used by generated model hooks.
func TestSyncCoreLibrariesGatesCryptModule(t *testing.T) {
	assertModulesGated(t, project.Components{}, project.Components{DatabaseSQLite: true}, databaseRendererSyncModules)
}

// TestSyncCoreLibrariesGatesJobsModules verifies Queue and every compiled Queue driver remain pinned but are synchronized only for Jobs projects.
func TestSyncCoreLibrariesGatesJobsModules(t *testing.T) {
	assertModulesGated(t, project.Components{}, project.Components{Jobs: true}, jobsRendererSyncModules)
}

// TestQualityReleaseVersionsArePinned prevents independently versioned modules from drifting away from the release set validated by GoForj.
func TestQualityReleaseVersionsArePinned(t *testing.T) {
	cases := []struct {
		module  string
		version string
	}{
		{module: "github.com/goforj/metrics", version: "v0.2.0"},
		{module: "github.com/goforj/cache", version: "v0.4.0"},
		{module: "github.com/goforj/cache/cachecore", version: "v0.4.0"},
		{module: "github.com/goforj/cache/cachetest", version: "v0.4.0"},
		{module: "github.com/goforj/cache/driver/dynamocache", version: "v0.4.0"},
		{module: "github.com/goforj/cache/driver/memcachedcache", version: "v0.4.0"},
		{module: "github.com/goforj/cache/driver/mysqlcache", version: "v0.4.0"},
		{module: "github.com/goforj/cache/driver/natscache", version: "v0.4.0"},
		{module: "github.com/goforj/cache/driver/postgrescache", version: "v0.4.0"},
		{module: "github.com/goforj/cache/driver/rediscache", version: "v0.4.0"},
		{module: "github.com/goforj/cache/driver/sqlcore", version: "v0.4.0"},
		{module: "github.com/goforj/cache/driver/sqlitecache", version: "v0.4.0"},
		{module: "github.com/goforj/mail", version: "v0.3.1"},
		{module: "github.com/goforj/mail/mailses", version: "v0.3.1"},
		{module: "github.com/goforj/execx", version: "v1.1.2"},
		{module: "github.com/goforj/events", version: "v0.2.0"},
		{module: "github.com/goforj/events/eventscore", version: "v0.2.0"},
		{module: "github.com/goforj/events/driver/gcppubsubevents", version: "v0.2.0"},
		{module: "github.com/goforj/events/driver/kafkaevents", version: "v0.2.0"},
		{module: "github.com/goforj/events/driver/natsevents", version: "v0.2.0"},
		{module: "github.com/goforj/events/driver/natsjetstreamevents", version: "v0.2.0"},
		{module: "github.com/goforj/events/driver/redisevents", version: "v0.2.0"},
		{module: "github.com/goforj/events/driver/snsevents", version: "v0.2.0"},
		{module: "github.com/goforj/events/eventsfake", version: "v0.2.0"},
		{module: "github.com/goforj/events/eventstest", version: "v0.2.0"},
		{module: "github.com/goforj/env/v2", version: "v2.5.0"},
		{module: "github.com/goforj/storage", version: "v0.5.0"},
		{module: "github.com/goforj/storage/storagecore", version: "v0.5.0"},
		{module: "github.com/goforj/storage/storagetest", version: "v0.5.0"},
		{module: "github.com/goforj/storage/driver/dropboxstorage", version: "v0.5.0"},
		{module: "github.com/goforj/storage/driver/ftpstorage", version: "v0.5.0"},
		{module: "github.com/goforj/storage/driver/gcsstorage", version: "v0.5.0"},
		{module: "github.com/goforj/storage/driver/localstorage", version: "v0.5.0"},
		{module: "github.com/goforj/storage/driver/memorystorage", version: "v0.5.0"},
		{module: "github.com/goforj/storage/driver/rclonestorage", version: "v0.5.0"},
		{module: "github.com/goforj/storage/driver/redisstorage", version: "v0.5.0"},
		{module: "github.com/goforj/storage/driver/s3storage", version: "v0.5.0"},
		{module: "github.com/goforj/storage/driver/sftpstorage", version: "v0.5.0"},
		{module: "github.com/goforj/crypt", version: "v1.2.0"},
	}

	for _, tc := range cases {
		t.Run(tc.module, func(t *testing.T) {
			version, ok := VersionFor(tc.module)
			if !ok {
				t.Fatalf("pinned module catalog omits %q", tc.module)
			}
			if version != tc.version {
				t.Fatalf("VersionFor(%q) = %q, want %q", tc.module, version, tc.version)
			}
		})
	}
}

// assertModulesGated verifies a capability adds exactly its ordered module set and does not leak those modules when disabled.
func assertModulesGated(t *testing.T, disabledComponents, enabledComponents project.Components, gatedModules []string) {
	t.Helper()

	disabled := SyncCoreLibraries(disabledComponents)
	enabled := SyncCoreLibraries(enabledComponents)
	want := append([]string(nil), disabled...)
	for _, module := range gatedModules {
		selection := module + "@" + MustVersionFor(module)
		if slices.Contains(disabled, selection) {
			t.Fatalf("disabled modules contain %q: %#v", selection, disabled)
		}
		want = append(want, selection)
	}
	if !slices.Equal(enabled, want) {
		t.Fatalf("enabled modules = %#v, want %#v", enabled, want)
	}
}
