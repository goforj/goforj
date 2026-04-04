package generate

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"text/template"

	"github.com/goforj/env/v2"
	"github.com/goforj/str"
)

type cacheAccessorTemplateData struct {
	Names []cacheAccessorName
}

type cacheAccessorName struct {
	Method string
	Store  string
}

type cacheConfigTemplateData struct {
	Drivers []cacheDriverSpec
	HasNATS bool
	Names   []cacheAccessorName
}

type cacheDriverSpec struct {
	ConstName    string
	Constructor  string
	ImportPath   string
	ConfigType   string
	ReturnsError bool
	NeedsContext bool
	Setup        []string
	Fields       []cacheConfigField
}

type cacheConfigField struct {
	Name  string
	Value string
}

var cacheDriverSpecs = map[string]cacheDriverSpec{
	"redis": {
		ConstName:   "driverRedis",
		Constructor: "rediscache.New",
		ImportPath:  "github.com/goforj/cache/driver/rediscache",
		ConfigType:  "rediscache.Config",
		Setup: []string{
			`addr := str.Of(scope.Get("ADDR", "")).TrimSpace().String()`,
			`if addr == "" {`,
			`	addr = fmt.Sprintf("%s:%s", env.Get("REDIS_HOST", "redis"), env.Get("REDIS_PORT", "6379"))`,
			`}`,
		},
		Fields: []cacheConfigField{
			{Name: "BaseConfig", Value: "baseConfig"},
			{Name: "Addr", Value: "addr"},
			{Name: "Username", Value: `scope.Get("USERNAME", "")`},
			{Name: "Password", Value: `scope.Get("PASSWORD", env.Get("REDIS_PASSWORD", ""))`},
			{Name: "DB", Value: `scope.GetInt("DB", env.Get("REDIS_DB", "0"))`},
			{Name: "TLSConfig", Value: "cacheRedisTLSConfig(scope)"},
		},
	},
	"memcached": {
		ConstName:   "driverMemcached",
		Constructor: "memcachedcache.New",
		ImportPath:  "github.com/goforj/cache/driver/memcachedcache",
		ConfigType:  "memcachedcache.Config",
		Fields: []cacheConfigField{
			{Name: "BaseConfig", Value: "baseConfig"},
			{Name: "Addresses", Value: "cacheAddresses(scope)"},
		},
	},
	"dynamodb": {
		ConstName:    "driverDynamo",
		Constructor:  "dynamocache.New",
		ImportPath:   "github.com/goforj/cache/driver/dynamocache",
		ConfigType:   "dynamocache.Config",
		ReturnsError: true,
		NeedsContext: true,
		Fields: []cacheConfigField{
			{Name: "BaseConfig", Value: "baseConfig"},
			{Name: "Endpoint", Value: `scope.Get("ENDPOINT", "")`},
			{Name: "Region", Value: `scope.Get("REGION", "us-east-1")`},
			{Name: "Table", Value: `scope.Get("TABLE", "cache_entries")`},
		},
	},
	"sqlite": {
		ConstName:    "driverSQLite",
		Constructor:  "sqlitecache.New",
		ImportPath:   "github.com/goforj/cache/driver/sqlitecache",
		ConfigType:   "sqlitecache.Config",
		ReturnsError: true,
		Fields: []cacheConfigField{
			{Name: "BaseConfig", Value: "baseConfig"},
			{Name: "DSN", Value: `scope.Get("DSN", cacheSQLiteDSN(name))`},
			{Name: "Table", Value: `scope.Get("TABLE", "cache_entries")`},
		},
	},
	"postgres": {
		ConstName:    "driverPostgres",
		Constructor:  "postgrescache.New",
		ImportPath:   "github.com/goforj/cache/driver/postgrescache",
		ConfigType:   "postgrescache.Config",
		ReturnsError: true,
		Fields: []cacheConfigField{
			{Name: "BaseConfig", Value: "baseConfig"},
			{Name: "DSN", Value: `scope.Get("DSN", "")`},
			{Name: "Table", Value: `scope.Get("TABLE", "cache_entries")`},
		},
	},
	"mysql": {
		ConstName:    "driverMySQL",
		Constructor:  "mysqlcache.New",
		ImportPath:   "github.com/goforj/cache/driver/mysqlcache",
		ConfigType:   "mysqlcache.Config",
		ReturnsError: true,
		Fields: []cacheConfigField{
			{Name: "BaseConfig", Value: "baseConfig"},
			{Name: "DSN", Value: `scope.Get("DSN", "")`},
			{Name: "Table", Value: `scope.Get("TABLE", "cache_entries")`},
		},
	},
}

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

var cacheCommonKeys = makeSet(
	"DRIVER",
	"DEFAULT_TTL_SECONDS",
	"PREFIX",
	"COMPRESSION",
	"MAX_VALUE_BYTES",
	"ENCRYPTION_KEY",
)

var cacheDriverKeys = map[string]map[string]struct{}{
	"memory":    makeSet("MEMORY_CLEANUP_SECONDS"),
	"file":      makeSet("FILE_DIR"),
	"null":      makeSet(),
	"redis":     makeSet("ADDR", "USERNAME", "PASSWORD", "DB", "TLS", "INSECURE_SKIP_VERIFY"),
	"memcached": makeSet("ADDRESSES"),
	"dynamodb":  makeSet("ENDPOINT", "REGION", "TABLE"),
	"sqlite":    makeSet("DSN", "TABLE"),
	"postgres":  makeSet("DSN", "TABLE"),
	"mysql":     makeSet("DSN", "TABLE"),
	"nats":      makeSet("URL", "BUCKET", "BUCKET_TTL", "BUCKET_TTL_SECONDS", "DESCRIPTION", "HISTORY", "MAX_BYTES", "MAX_VALUE_SIZE", "REPLICAS", "STORAGE", "COMPRESSED"),
}

func GenerateCacheFiles(projectDir string) (int, error) {
	if err := validatePrimitiveEnv(primitiveEnvContract{
		Prefix:        "CACHE",
		DefaultDriver: "memory",
		RootKeys:      cacheRootKeys,
		CommonKeys:    cacheCommonKeys,
		DriverKeys:    cacheDriverKeys,
		ChildNames: func(scope env.Scope) []string {
			return scope.ChildNames(cacheRootKeys)
		},
		AllowInactiveRootKeys: true,
	}); err != nil {
		return 0, err
	}
	manager, err := renderCacheConfig()
	if err != nil {
		return 0, err
	}
	formattedManager, err := format.Source(manager)
	if err != nil {
		return 0, fmt.Errorf("failed to format generated cache manager: %w", err)
	}
	written := 0
	changed, err := writeGeneratedSource(filepath.Join(projectDir, "internal", "caches", "manager_gen.go"), formattedManager)
	if err != nil {
		return written, err
	}
	if changed {
		written++
	}
	_ = os.Remove(filepath.Join(projectDir, "internal", "caches", "runtime.go"))
	_ = os.Remove(filepath.Join(projectDir, "internal", "caches", "manager.go"))
	_ = os.Remove(filepath.Join(projectDir, "internal", "caches", "stores_gen.go"))
	_ = os.Remove(filepath.Join(projectDir, "internal", "caches", "config_gen.go"))
	return written, nil
}

func discoverCacheStoreNames() []string {
	names := env.WithPrefix("CACHE").ChildNames(cacheRootKeys)
	for i := range names {
		names[i] = str.Of(names[i]).TrimSpace().ToLower().String()
	}
	sort.Strings(names)
	return names
}

func renderCacheAccessors(names []string) ([]byte, error) {
	data := cacheAccessorTemplateData{
		Names: make([]cacheAccessorName, 0, len(names)),
	}
	for _, name := range names {
		data.Names = append(data.Names, cacheAccessorName{
			Method: str.Of(name).Pascal().String(),
			Store:  name,
		})
	}
	var b bytes.Buffer
	tmpl, err := template.New("cache-accessors").Parse(cacheAccessorsSourceTemplate)
	if err != nil {
		return nil, err
	}
	if err := tmpl.Execute(&b, data); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func renderCacheConfig() ([]byte, error) {
	names := discoverCacheStoreNames()
	driverSet := map[string]struct{}{}
	defaultDriver := str.Of(env.Get("CACHE_DRIVER", "memory")).TrimSpace().ToLower().String()
	if defaultDriver != "" {
		driverSet[defaultDriver] = struct{}{}
	}
	for _, child := range env.WithPrefix("CACHE").ChildNames(cacheRootKeys) {
		driver := str.Of(env.Get("CACHE_"+child+"_DRIVER", "")).TrimSpace().ToLower().String()
		if driver != "" {
			driverSet[driver] = struct{}{}
		}
	}
	drivers, err := supportedDrivers("CACHE", cacheDriverKeys, sortStrings(driverSet))
	if err != nil {
		return nil, err
	}
	data := cacheConfigTemplateData{
		Drivers: make([]cacheDriverSpec, 0, len(drivers)),
		Names:   make([]cacheAccessorName, 0, len(names)),
	}
	for _, name := range names {
		data.Names = append(data.Names, cacheAccessorName{
			Method: str.Of(name).Pascal().String(),
			Store:  name,
		})
	}
	for _, driver := range drivers {
		if driver == "nats" {
			data.HasNATS = true
		}
		if spec, ok := cacheDriverSpecs[driver]; ok {
			data.Drivers = append(data.Drivers, spec)
		}
	}
	var b bytes.Buffer
	tmpl, err := template.New("cache-config").Parse(cacheConfigSourceTemplate)
	if err != nil {
		return nil, err
	}
	if err := tmpl.Execute(&b, data); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

const cacheAccessorsSourceTemplate = `// Code generated by forj generate --cache. DO NOT EDIT.
// Run: forj generate --cache
//
// The default cache is exposed by Manager.Default().
// This file contains named cache accessors generated from
// CACHE_<NAME>_<KEY> environment variables.
package caches

{{- if .Names }}
import "github.com/goforj/cache"
type namedStores struct {
{{- range .Names }}
	{{ .Store }} *cache.Cache
{{- end }}
}

{{ range .Names }}
// {{ .Method }} returns the "{{ .Store }}" cache instance.
func (m *Manager) {{ .Method }}() *cache.Cache {
	return m.named.{{ .Store }}
}
{{ end }}
{{- else }}
type namedStores struct{}
{{- end }}`

const cacheConfigSourceTemplate = `// Code generated by forj generate --cache. DO NOT EDIT.
// Run: forj generate --cache
package caches

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goforj/cache"
	"github.com/goforj/cache/cachecore"
	"github.com/goforj/env/v2"
{{- range .Drivers }}
	"{{ .ImportPath }}"
{{- end }}
{{- if .HasNATS }}
	"github.com/goforj/cache/driver/natscache"
	"github.com/nats-io/nats.go"
{{- end }}
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
	defaultStore *cache.Cache
{{- range .Names }}
	{{ .Store }} *cache.Cache
{{- end }}
}

type Instance struct {
	Name      string
	Store     *cache.Cache
	IsDefault bool
}

type ReadinessCheck struct {
	Name  string
	Check func(context.Context) error
}

func NewManager() (*Manager, error) {
	return newManagerFromEnv(env.WithPrefix("CACHE"))
}

func (m *Manager) Default() *cache.Cache {
	return m.defaultStore
}

func (m *Manager) Instances() []Instance {
	if m == nil {
		return nil
	}
	instances := []Instance{
		{Name: "default", Store: m.defaultStore, IsDefault: true},
	}
{{- range .Names }}
	instances = append(instances, Instance{Name: "{{ .Store }}", Store: m.{{ .Store }}})
{{- end }}
	return instances
}

func (m *Manager) ReadinessChecks() []ReadinessCheck {
	if m == nil {
		return nil
	}
	checks := []ReadinessCheck{
		{
			Name: "cache_default",
			Check: func(ctx context.Context) error {
				return cacheReadinessCheck(ctx, m.defaultStore)
			},
		},
{{- range .Names }}
		{
			Name: "cache_{{ .Store }}",
			Check: func(ctx context.Context) error {
				return cacheReadinessCheck(ctx, m.{{ .Store }})
			},
		},
{{- end }}
	}
	return checks
}

{{- range .Names }}
func (m *Manager) {{ .Method }}() *cache.Cache {
	return m.{{ .Store }}
}

{{- end }}
func newManagerFromEnv(cacheScope env.Scope) (*Manager, error) {
	defaultStore, err := buildStore(string(defaultCacheName), cacheScope)
	if err != nil {
		return nil, err
	}
	manager := &Manager{
		defaultStore: defaultStore,
	}

{{- if .Names }}
	for _, child := range cacheScope.ChildNames(cacheRootKeys) {
		name := str.Of(child).TrimSpace().ToLower().String()
		if name == "" {
			continue
		}
		store, err := buildStore(name, cacheScope.Child(child))
		if err != nil {
			return nil, err
		}
		switch name {
{{- range .Names }}
		case "{{ .Store }}":
			manager.{{ .Store }} = store
{{- end }}
		}
	}
{{- end }}

	return manager, nil
}

// buildStore is generated from the cache stores currently defined in env.
// The supported driver cases and imports in this file are derived from
// CACHE_SUPPORTED_DRIVERS, or from active CACHE_* and CACHE_<NAME>_* values when unset.
func buildStore(name string, scope env.Scope) (*cache.Cache, error) {
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
		store := cache.NewNullStoreWithConfig(context.Background(), cache.StoreConfig{
			BaseConfig: baseConfig,
		})
		return cache.NewCacheWithTTL(store, baseConfig.DefaultTTL), nil
	case driverFile:
		store := cache.NewFileStoreWithConfig(context.Background(), cache.StoreConfig{
			BaseConfig: baseConfig,
			FileDir:    cacheFileDir(name, scope),
		})
		return cache.NewCacheWithTTL(store, baseConfig.DefaultTTL), nil
	case driverMemory:
		store := cache.NewMemoryStoreWithConfig(context.Background(), cache.StoreConfig{
			BaseConfig:            baseConfig,
			MemoryCleanupInterval: cacheMemoryCleanupInterval(scope),
		})
		return cache.NewCacheWithTTL(store, baseConfig.DefaultTTL), nil
{{- if .HasNATS }}
	case driverNATS:
		store, err := buildNATSStore(name, scope, baseConfig)
		if err != nil {
			return nil, err
		}
		return cache.NewCacheWithTTL(store, baseConfig.DefaultTTL), nil
{{- end }}
{{- range .Drivers }}
	case {{ .ConstName }}:
{{- range .Setup }}
		{{ . }}
{{- end }}
{{- if .ReturnsError }}
{{- if .NeedsContext }}
		store, err := {{ .Constructor }}(context.Background(), {{ .ConfigType }}{
{{- else }}
		store, err := {{ .Constructor }}({{ .ConfigType }}{
{{- end }}
{{- range .Fields }}
			{{ .Name }}: {{ .Value }},
{{- end }}
		})
		if err != nil {
			return nil, err
		}
{{- else }}
		store := {{ .Constructor }}({{ .ConfigType }}{
{{- range .Fields }}
			{{ .Name }}: {{ .Value }},
{{- end }}
		})
{{- end }}
		return cache.NewCacheWithTTL(store, baseConfig.DefaultTTL), nil
{{- end }}
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
	if name == "" || name == string(defaultCacheName) {
		return base
	}
	return filepath.Join(base, name)
}

func cacheSQLiteDSN(name string) string {
	base := filepath.Join(os.TempDir(), "cache-sqlite")
	if name == "" || name == string(defaultCacheName) {
		return filepath.Join(base, "default.db")
	}
	return filepath.Join(base, name+".db")
}

func cacheAddresses(scope env.Scope) []string {
	value := strings.TrimSpace(scope.Get("ADDRESSES", ""))
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		addr := strings.TrimSpace(part)
		if addr != "" {
			out = append(out, addr)
		}
	}
	return out
}

func cacheRedisTLSConfig(scope env.Scope) *tls.Config {
	if !scope.GetBool("TLS", "false") {
		return nil
	}
	return &tls.Config{
		InsecureSkipVerify: scope.GetBool("INSECURE_SKIP_VERIFY", "false"),
	}
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

func cacheReadinessCheck(ctx context.Context, store *cache.Cache) error {
	if store == nil {
		return nil
	}
	if ready, ok := any(store).(interface{ Ready() error }); ok {
		return ready.Ready()
	}
	if inner := store.Store(); inner != nil {
		if ready, ok := any(inner).(interface{ Ready(context.Context) error }); ok {
			return ready.Ready(ctx)
		}
	}
	return nil
}

{{- if .HasNATS }}
func buildNATSStore(name string, scope env.Scope, baseConfig cachecore.BaseConfig) (cachecore.Store, error) {
	url := strings.TrimSpace(scope.Get("URL", env.Get("NATS_URL", "nats://127.0.0.1:4222")))
	if url == "" {
		url = "nats://127.0.0.1:4222"
	}
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, err
	}
	js, err := nc.JetStream()
	if err != nil {
		return nil, err
	}
	bucket := strings.TrimSpace(scope.Get("BUCKET", cacheNATSBucket(name)))
	if bucket == "" {
		bucket = cacheNATSBucket(name)
	}
	kv, err := js.KeyValue(bucket)
	if err != nil {
		kvConfig := &nats.KeyValueConfig{
			Bucket:      bucket,
			Description: strings.TrimSpace(scope.Get("DESCRIPTION", "")),
			History:     cacheNATSHistory(scope),
			TTL:         cacheNATSBucketTTL(scope),
			MaxBytes:    int64(scope.GetInt("MAX_BYTES", "0")),
			Replicas:    scope.GetInt("REPLICAS", "1"),
			Compression: scope.GetBool("COMPRESSED", "false"),
		}
		if maxValueSize := scope.GetInt("MAX_VALUE_SIZE", "0"); maxValueSize > 0 {
			kvConfig.MaxValueSize = int32(maxValueSize)
		}
		switch strings.ToLower(strings.TrimSpace(scope.Get("STORAGE", ""))) {
		case "file":
			kvConfig.Storage = nats.FileStorage
		case "memory":
			kvConfig.Storage = nats.MemoryStorage
		}
		kv, err = js.CreateKeyValue(kvConfig)
		if err != nil {
			return nil, err
		}
	}
	return natscache.New(natscache.Config{
		BaseConfig: baseConfig,
		KeyValue:   kv,
		BucketTTL:  scope.GetBool("BUCKET_TTL", "false"),
	}), nil
}

func cacheNATSBucket(name string) string {
	if name == "" || name == string(defaultCacheName) {
		return "CACHE"
	}
	value := strings.TrimSpace(str.Of(name).Snake("_").ToUpper().String())
	if value == "" {
		return "CACHE"
	}
	return "CACHE_" + value
}

func cacheNATSHistory(scope env.Scope) uint8 {
	value := scope.GetInt("HISTORY", "1")
	if value < 1 {
		value = 1
	}
	if value > 64 {
		value = 64
	}
	return uint8(value)
}

func cacheNATSBucketTTL(scope env.Scope) time.Duration {
	seconds := scope.GetInt("BUCKET_TTL_SECONDS", "0")
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
{{- end }}`
