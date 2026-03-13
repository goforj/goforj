package cache

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	goforjcache "github.com/goforj/cache"
	"github.com/goforj/cache/cachecore"
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
	defaultStore *goforjcache.Cache
}

func NewManager() (*Manager, error) {
	return newManagerFromEnv(env.WithPrefix("CACHE"))
}

func (m *Manager) Default() *goforjcache.Cache {
	return m.defaultStore
}

func newManagerFromEnv(cacheScope env.Scope) (*Manager, error) {
	defaultStore, err := buildStore(defaultCacheName, cacheScope)
	if err != nil {
		return nil, err
	}
	return &Manager{defaultStore: defaultStore}, nil
}

func buildStore(name string, scope env.Scope) (*goforjcache.Cache, error) {
	driver := str.Of(scope.Get("DRIVER", driverMemory)).TrimSpace().ToLower().String()
	if driver == "" {
		driver = driverMemory
	}

	baseConfig := cachecore.BaseConfig{
		DefaultTTL:    cacheDefaultTTL(scope),
		Prefix:        cachePrefix(scope),
		Compression:   cacheCompression(scope),
		MaxValueBytes: scope.GetInt("MAX_VALUE_BYTES", "0"),
		EncryptionKey: cacheEncryptionKey(scope),
	}

	switch driver {
	case driverNull:
		store := goforjcache.NewNullStoreWithConfig(context.Background(), goforjcache.StoreConfig{
			BaseConfig: baseConfig,
		})
		return goforjcache.NewCacheWithTTL(store, baseConfig.DefaultTTL), nil
	case driverFile:
		store := goforjcache.NewFileStoreWithConfig(context.Background(), goforjcache.StoreConfig{
			BaseConfig: baseConfig,
			FileDir:    cacheFileDir(name, scope),
		})
		return goforjcache.NewCacheWithTTL(store, baseConfig.DefaultTTL), nil
	case driverMemory:
		store := goforjcache.NewMemoryStoreWithConfig(context.Background(), goforjcache.StoreConfig{
			BaseConfig:            baseConfig,
			MemoryCleanupInterval: cacheMemoryCleanupInterval(scope),
		})
		return goforjcache.NewCacheWithTTL(store, baseConfig.DefaultTTL), nil
	default:
		return nil, fmt.Errorf("cache: unsupported driver %q", driver)
	}
}

func cacheDefaultTTL(scope env.Scope) time.Duration {
	seconds := scope.GetInt("DEFAULT_TTL_SECONDS", "300")
	if seconds <= 0 {
		seconds = 300
	}
	return time.Duration(seconds) * time.Second
}

func cacheMemoryCleanupInterval(scope env.Scope) time.Duration {
	seconds := scope.GetInt("MEMORY_CLEANUP_SECONDS", "600")
	if seconds <= 0 {
		seconds = 600
	}
	return time.Duration(seconds) * time.Second
}

func cachePrefix(scope env.Scope) string {
	value := scope.Get("PREFIX", "app")
	if value == "" {
		return "app"
	}
	return value
}

func cacheFileDir(name string, scope env.Scope) string {
	value := strings.TrimSpace(scope.Get("FILE_DIR", ""))
	if value != "" {
		return value
	}
	base := filepath.Join(os.TempDir(), "cache-file")
	if name == "" || name == defaultCacheName {
		return base
	}
	return filepath.Join(base, name)
}

func cacheCompression(scope env.Scope) cachecore.CompressionCodec {
	switch strings.ToLower(strings.TrimSpace(scope.Get("COMPRESSION", "none"))) {
	case "", "none":
		return cachecore.CompressionNone
	case "gzip":
		return cachecore.CompressionGzip
	case "snappy":
		return cachecore.CompressionSnappy
	default:
		return cachecore.CompressionNone
	}
}

func cacheEncryptionKey(scope env.Scope) []byte {
	value := strings.TrimSpace(scope.Get("ENCRYPTION_KEY", ""))
	if value == "" {
		return nil
	}
	return []byte(value)
}
