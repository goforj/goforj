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

func loadStoresFromEnv(cacheScope env.Scope) (map[string]*goforjcache.Cache, error) {
	stores := map[string]*goforjcache.Cache{}

	defaultStore, err := buildStore(defaultCacheName, cacheScope)
	if err != nil {
		return nil, err
	}
	stores[defaultCacheName] = defaultStore

	for _, child := range cacheScope.ChildNames(cacheRootKeys) {
		name := str.Of(child).TrimSpace().ToLower().String()
		if name == "" {
			continue
		}
		store, err := buildStore(name, cacheScope.Child(child))
		if err != nil {
			return nil, err
		}
		stores[name] = store
	}

	return stores, nil
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
