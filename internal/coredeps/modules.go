package coredeps

import (
	"sort"

	"github.com/goforj/goforj/project"
)

// pinnedModuleVersions keeps rendered projects on the framework versions validated together by GoForj's integration suite.
var pinnedModuleVersions = map[string]string{
	"github.com/a-h/templ":                                "v0.3.1020",
	"github.com/goforj/cache":                             "v0.4.0",
	"github.com/goforj/cache/cachecore":                   "v0.4.0",
	"github.com/goforj/cache/cachetest":                   "v0.4.0",
	"github.com/goforj/cache/driver/dynamocache":          "v0.4.0",
	"github.com/goforj/cache/driver/memcachedcache":       "v0.4.0",
	"github.com/goforj/cache/driver/mysqlcache":           "v0.4.0",
	"github.com/goforj/cache/driver/natscache":            "v0.4.0",
	"github.com/goforj/cache/driver/postgrescache":        "v0.4.0",
	"github.com/goforj/cache/driver/rediscache":           "v0.4.0",
	"github.com/goforj/cache/driver/sqlcore":              "v0.4.0",
	"github.com/goforj/cache/driver/sqlitecache":          "v0.4.0",
	"github.com/goforj/crypt":                             "v1.2.0",
	"github.com/goforj/env/v2":                            "v2.6.0",
	"github.com/goforj/events":                            "v0.2.0",
	"github.com/goforj/events/driver/gcppubsubevents":     "v0.2.0",
	"github.com/goforj/events/driver/kafkaevents":         "v0.2.0",
	"github.com/goforj/events/driver/natsevents":          "v0.2.0",
	"github.com/goforj/events/driver/natsjetstreamevents": "v0.2.0",
	"github.com/goforj/events/driver/redisevents":         "v0.2.0",
	"github.com/goforj/events/driver/snsevents":           "v0.2.0",
	"github.com/goforj/events/eventscore":                 "v0.2.0",
	"github.com/goforj/events/eventsfake":                 "v0.2.0",
	"github.com/goforj/events/eventstest":                 "v0.2.0",
	"github.com/goforj/execx":                             "v1.1.3",
	"github.com/goforj/godump":                            "v1.9.1",
	"github.com/goforj/httpx":                             "v1.1.0",
	"github.com/goforj/mail":                              "v0.3.1",
	"github.com/goforj/mail/mailses":                      "v0.3.1",
	"github.com/goforj/metrics":                           "v0.2.0",
	"github.com/goforj/null/v6":                           "v6.0.2",
	"github.com/goforj/queue":                             "v0.2.1",
	"github.com/goforj/queue/driver/mysqlqueue":           "v0.2.1",
	"github.com/goforj/queue/driver/natsqueue":            "v0.2.1",
	"github.com/goforj/queue/driver/postgresqueue":        "v0.2.1",
	"github.com/goforj/queue/driver/rabbitmqqueue":        "v0.2.1",
	"github.com/goforj/queue/driver/redisqueue":           "v0.2.1",
	"github.com/goforj/queue/driver/sqlitequeue":          "v0.2.1",
	"github.com/goforj/queue/driver/sqlqueuecore":         "v0.2.1",
	"github.com/goforj/queue/driver/sqsqueue":             "v0.2.1",
	"github.com/goforj/scheduler/v2":                      "v2.1.4",
	"github.com/goforj/storage":                           "v0.5.0",
	"github.com/goforj/storage/driver/dropboxstorage":     "v0.5.0",
	"github.com/goforj/storage/driver/ftpstorage":         "v0.5.0",
	"github.com/goforj/storage/driver/gcsstorage":         "v0.5.0",
	"github.com/goforj/storage/driver/localstorage":       "v0.5.0",
	"github.com/goforj/storage/driver/memorystorage":      "v0.5.0",
	"github.com/goforj/storage/driver/rclonestorage":      "v0.5.0",
	"github.com/goforj/storage/driver/redisstorage":       "v0.5.0",
	"github.com/goforj/storage/driver/s3storage":          "v0.5.0",
	"github.com/goforj/storage/driver/sftpstorage":        "v0.5.0",
	"github.com/goforj/storage/storagecore":               "v0.5.0",
	"github.com/goforj/storage/storagetest":               "v0.5.0",
	"github.com/goforj/web":                               "v0.6.2",
	"github.com/goforj/str":                               "v1.3.0",
	"github.com/goforj/str/v2":                            "v2.0.1",
	"github.com/goforj/wire":                              "v1.2.0",
	"github.com/nats-io/nats.go":                          "v1.50.0",
}

var rendererSyncModules = []string{
	"github.com/goforj/metrics",
	"github.com/goforj/httpx",
	"github.com/goforj/godump",
	"github.com/goforj/web",
	"github.com/goforj/str/v2",
	"github.com/goforj/scheduler/v2",
	"github.com/goforj/env/v2",
	"github.com/goforj/str",
	"github.com/goforj/null/v6",
	"github.com/goforj/wire",
}

var cacheRendererSyncModules = []string{
	"github.com/goforj/cache",
	"github.com/goforj/cache/cachecore",
	"github.com/goforj/cache/driver/dynamocache",
	"github.com/goforj/cache/driver/memcachedcache",
	"github.com/goforj/cache/driver/mysqlcache",
	"github.com/goforj/cache/driver/natscache",
	"github.com/goforj/cache/driver/postgrescache",
	"github.com/goforj/cache/driver/rediscache",
	"github.com/goforj/cache/driver/sqlcore",
	"github.com/goforj/cache/driver/sqlitecache",
}

var schedulerRendererSyncModules = []string{
	"github.com/goforj/cache",
	"github.com/goforj/cache/cachecore",
}

var eventsRendererSyncModules = []string{
	"github.com/goforj/events",
	"github.com/goforj/events/eventscore",
	"github.com/goforj/events/driver/gcppubsubevents",
	"github.com/goforj/events/driver/kafkaevents",
	"github.com/goforj/events/driver/natsevents",
	"github.com/goforj/events/driver/natsjetstreamevents",
	"github.com/goforj/events/driver/redisevents",
	"github.com/goforj/events/driver/snsevents",
}

var storageRendererSyncModules = []string{
	"github.com/goforj/storage",
	"github.com/goforj/storage/storagecore",
	"github.com/goforj/storage/driver/dropboxstorage",
	"github.com/goforj/storage/driver/ftpstorage",
	"github.com/goforj/storage/driver/gcsstorage",
	"github.com/goforj/storage/driver/localstorage",
	"github.com/goforj/storage/driver/memorystorage",
	"github.com/goforj/storage/driver/rclonestorage",
	"github.com/goforj/storage/driver/redisstorage",
	"github.com/goforj/storage/driver/s3storage",
	"github.com/goforj/storage/driver/sftpstorage",
}

var mailRendererSyncModules = []string{
	"github.com/goforj/mail",
	"github.com/goforj/mail/mailses",
}

var databaseRendererSyncModules = []string{
	"github.com/goforj/crypt",
}

var jobsRendererSyncModules = []string{
	"github.com/goforj/queue",
	"github.com/goforj/queue/driver/mysqlqueue",
	"github.com/goforj/queue/driver/natsqueue",
	"github.com/goforj/queue/driver/postgresqueue",
	"github.com/goforj/queue/driver/rabbitmqqueue",
	"github.com/goforj/queue/driver/redisqueue",
	"github.com/goforj/queue/driver/sqlitequeue",
	"github.com/goforj/queue/driver/sqlqueuecore",
	"github.com/goforj/queue/driver/sqsqueue",
}

// VersionFor returns the framework-pinned version for module when it is known.
func VersionFor(module string) (string, bool) {
	version, ok := pinnedModuleVersions[module]
	return version, ok
}

// MustVersionFor returns the framework-pinned version for module and panics when it is unknown.
func MustVersionFor(module string) string {
	version, ok := VersionFor(module)
	if !ok {
		panic("missing pinned version for " + module)
	}
	return version
}

// SyncCoreLibraries returns the pinned renderer dependencies required by the selected project capabilities.
func SyncCoreLibraries(components project.Components) []string {
	modules := append([]string(nil), rendererSyncModules...)
	if components.Cache {
		modules = append(modules, cacheRendererSyncModules...)
	}
	if components.Scheduler {
		modules = append(modules, schedulerRendererSyncModules...)
	}
	if components.Events {
		modules = append(modules, eventsRendererSyncModules...)
	}
	if components.Storage {
		modules = append(modules, storageRendererSyncModules...)
	}
	if components.Mail {
		modules = append(modules, mailRendererSyncModules...)
	}
	if components.HasDatabase() || components.DemoApp {
		modules = append(modules, databaseRendererSyncModules...)
	}
	if components.Jobs {
		modules = append(modules, jobsRendererSyncModules...)
	}
	out := make([]string, 0, len(modules))
	seen := make(map[string]struct{}, len(modules))
	for _, module := range modules {
		if _, ok := seen[module]; ok {
			continue
		}
		seen[module] = struct{}{}
		out = append(out, module+"@"+MustVersionFor(module))
	}
	return out
}

// AllPinnedModules returns a copy of the complete framework module version catalog.
func AllPinnedModules() map[string]string {
	out := make(map[string]string, len(pinnedModuleVersions))
	for module, version := range pinnedModuleVersions {
		out[module] = version
	}
	return out
}

// KnownModules returns all framework-pinned module paths in lexical order.
func KnownModules() []string {
	out := make([]string, 0, len(pinnedModuleVersions))
	for module := range pinnedModuleVersions {
		out = append(out, module)
	}
	sort.Strings(out)
	return out
}
