package coredeps

import "sort"

var pinnedModuleVersions = map[string]string{
	"github.com/goforj/cache":                         "v0.2.1",
	"github.com/goforj/cache/cachecore":               "v0.2.1",
	"github.com/goforj/cache/cachetest":               "v0.2.1",
	"github.com/goforj/cache/driver/dynamocache":      "v0.2.1",
	"github.com/goforj/cache/driver/memcachedcache":   "v0.2.1",
	"github.com/goforj/cache/driver/mysqlcache":       "v0.2.1",
	"github.com/goforj/cache/driver/natscache":        "v0.2.1",
	"github.com/goforj/cache/driver/postgrescache":    "v0.2.1",
	"github.com/goforj/cache/driver/rediscache":       "v0.2.1",
	"github.com/goforj/cache/driver/sqlcore":          "v0.2.1",
	"github.com/goforj/cache/driver/sqlitecache":      "v0.2.1",
	"github.com/goforj/env/v2":                        "v2.3.0",
	"github.com/goforj/events":                        "v0.1.2",
	"github.com/goforj/events/eventscore":             "v0.1.2",
	"github.com/goforj/httpx":                         "v1.1.0",
	"github.com/goforj/mail":                          "v0.1.0",
	"github.com/goforj/metrics":                       "v0.1.0",
	"github.com/goforj/queue":                         "v0.1.16",
	"github.com/goforj/queue/driver/mysqlqueue":       "v0.1.16",
	"github.com/goforj/queue/driver/natsqueue":        "v0.1.16",
	"github.com/goforj/queue/driver/postgresqueue":    "v0.1.16",
	"github.com/goforj/queue/driver/rabbitmqqueue":    "v0.1.16",
	"github.com/goforj/queue/driver/redisqueue":       "v0.1.16",
	"github.com/goforj/queue/driver/sqlitequeue":      "v0.1.16",
	"github.com/goforj/queue/driver/sqlqueuecore":     "v0.1.16",
	"github.com/goforj/queue/driver/sqsqueue":         "v0.1.16",
	"github.com/goforj/scheduler/v2":                  "v2.1.3",
	"github.com/goforj/storage":                       "v0.4.5",
	"github.com/goforj/storage/driver/dropboxstorage": "v0.4.5",
	"github.com/goforj/storage/driver/ftpstorage":     "v0.4.5",
	"github.com/goforj/storage/driver/gcsstorage":     "v0.4.5",
	"github.com/goforj/storage/driver/localstorage":   "v0.4.5",
	"github.com/goforj/storage/driver/memorystorage":  "v0.4.5",
	"github.com/goforj/storage/driver/rclonestorage":  "v0.4.5",
	"github.com/goforj/storage/driver/redisstorage":   "v0.4.5",
	"github.com/goforj/storage/driver/s3storage":      "v0.4.5",
	"github.com/goforj/storage/driver/sftpstorage":    "v0.4.5",
	"github.com/goforj/storage/storagecore":           "v0.4.5",
	"github.com/goforj/web":                           "v0.3.0",
	"github.com/goforj/str":                           "v1.2.0",
	"github.com/nats-io/nats.go":                      "v1.50.0",
}

var rendererSyncModules = []string{
	"github.com/goforj/cache",
	"github.com/goforj/cache/cachecore",
	"github.com/goforj/cache/driver/rediscache",
	"github.com/goforj/storage",
	"github.com/goforj/storage/storagecore",
	"github.com/goforj/storage/driver/localstorage",
	"github.com/goforj/queue",
	"github.com/goforj/queue/driver/mysqlqueue",
	"github.com/goforj/queue/driver/natsqueue",
	"github.com/goforj/queue/driver/postgresqueue",
	"github.com/goforj/queue/driver/rabbitmqqueue",
	"github.com/goforj/queue/driver/redisqueue",
	"github.com/goforj/queue/driver/sqlitequeue",
	"github.com/goforj/queue/driver/sqlqueuecore",
	"github.com/goforj/queue/driver/sqsqueue",
	"github.com/goforj/events",
	"github.com/goforj/events/eventscore",
	"github.com/goforj/metrics",
	"github.com/goforj/httpx",
	"github.com/goforj/web",
	"github.com/goforj/scheduler/v2",
	"github.com/goforj/env/v2",
}

func VersionFor(module string) (string, bool) {
	version, ok := pinnedModuleVersions[module]
	return version, ok
}

func MustVersionFor(module string) string {
	version, ok := VersionFor(module)
	if !ok {
		panic("missing pinned version for " + module)
	}
	return version
}

func SyncCoreLibraries() []string {
	out := make([]string, 0, len(rendererSyncModules))
	for _, module := range rendererSyncModules {
		out = append(out, module+"@"+MustVersionFor(module))
	}
	return out
}

func AllPinnedModules() map[string]string {
	out := make(map[string]string, len(pinnedModuleVersions))
	for module, version := range pinnedModuleVersions {
		out[module] = version
	}
	return out
}

func KnownModules() []string {
	out := make([]string, 0, len(pinnedModuleVersions))
	for module := range pinnedModuleVersions {
		out = append(out, module)
	}
	sort.Strings(out)
	return out
}
