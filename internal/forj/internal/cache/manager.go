package cache

import (
	"fmt"

	goforjcache "github.com/goforj/cache"
	"github.com/goforj/env/v2"
	"github.com/goforj/str"
)

const defaultCacheName = "default"

const (
	driverFile      = "file"
	driverMemory    = "memory"
	driverMemcached = "memcached"
	driverMySQL     = "mysql"
	driverNATS      = "nats"
	driverNull      = "null"
	driverDynamo    = "dynamodb"
	driverPostgres  = "postgres"
	driverRedis     = "redis"
	driverSQLite    = "sqlite"
)

var cacheRootKeys = []string{
	"DRIVER",
	"DEFAULT_TTL_SECONDS",
	"PREFIX",
	"MEMORY_CLEANUP_SECONDS",
	"FILE_DIR",
	"ADDR",
	"ADDRESSES",
	"USERNAME",
	"PASSWORD",
	"DB",
	"DSN",
	"TABLE",
	"ENDPOINT",
	"REGION",
	"TLS",
	"INSECURE_SKIP_VERIFY",
	"URL",
	"BUCKET",
	"BUCKET_TTL",
	"BUCKET_TTL_SECONDS",
	"DESCRIPTION",
	"HISTORY",
	"MAX_BYTES",
	"MAX_VALUE_SIZE",
	"REPLICAS",
	"STORAGE",
	"COMPRESSED",
	"COMPRESSION",
	"MAX_VALUE_BYTES",
	"ENCRYPTION_KEY",
}

type Manager struct {
	stores map[string]*goforjcache.Cache
}

func NewManager() (*Manager, error) {
	stores, err := loadStoresFromEnv(env.WithPrefix("CACHE"))
	if err != nil {
		return nil, err
	}
	return &Manager{stores: stores}, nil
}

func (m *Manager) Default() *goforjcache.Cache {
	store, ok := m.stores[defaultCacheName]
	if !ok {
		panic("cache: default store is not configured")
	}
	return store
}

func (m *Manager) Store(name string) (*goforjcache.Cache, error) {
	normalized := str.Of(name).TrimSpace().ToLower().String()
	if normalized == "" || normalized == defaultCacheName {
		store, ok := m.stores[defaultCacheName]
		if !ok {
			return nil, fmt.Errorf("cache: default store is not configured")
		}
		return store, nil
	}
	store, ok := m.stores[normalized]
	if !ok {
		return nil, fmt.Errorf("cache: store %q is not configured", normalized)
	}
	return store, nil
}

func (m *Manager) mustStore(name string) *goforjcache.Cache {
	store, err := m.Store(name)
	if err != nil {
		panic(fmt.Sprintf("cache: required store %q is not configured: %v", name, err))
	}
	return store
}

func discoverStoreNames() []string {
	names := env.WithPrefix("CACHE").ChildNames(cacheRootKeys)
	for i := range names {
		names[i] = str.Of(names[i]).TrimSpace().ToLower().String()
	}
	return names
}
