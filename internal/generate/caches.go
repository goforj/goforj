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

// cacheAccessorTemplateData keeps named cache methods and special session handling in one render snapshot.
type cacheAccessorTemplateData struct {
	HasSessions   bool
	Names         []cacheAccessorName
	OtherNames    []cacheAccessorName
	AccessorNames []cacheAccessorName
}

// cacheAccessorName binds an environment store name to its generated Go method.
type cacheAccessorName struct {
	Method string
	Store  string
}

// cacheConfigTemplateData keeps compiled drivers and named stores aligned while rendering the manager.
type cacheConfigTemplateData struct {
	CompiledDrivers []string
	Drivers         []cacheDriverSpec
	HasNATS         bool
	HasSessions     bool
	Names           []cacheAccessorName
	OtherNames      []cacheAccessorName
	AccessorNames   []cacheAccessorName
}

// cacheDriverSpec captures the import and constructor metadata needed to emit one cache driver branch.
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

// cacheConfigField binds a driver configuration field to its generated value expression.
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

// GenerateCacheFiles writes cache accessors whose imports and manifest reflect the project-owned build contract.
func GenerateCacheFiles(projectDir string) (int, error) {
	if err := validatePrimitiveEnv(projectDir, primitiveEnvContract{
		Prefix:        "CACHE",
		DefaultDriver: "memory",
		RootKeys:      cacheRootKeys,
		CommonKeys:    cacheCommonKeys,
		DriverKeys:    cacheDriverKeys,
		ChildNames: func(scope env.Scope) []string {
			return scope.ChildNames(cacheRootKeys)
		},
		AllowInactiveRootKeys: true,
		EagerNamedResources:   true,
	}); err != nil {
		return 0, err
	}
	manager, err := renderCacheConfig(projectDir)
	if err != nil {
		return 0, err
	}
	formattedManager, err := format.Source(manager)
	if err != nil {
		return 0, fmt.Errorf("failed to format generated cache manager: %w", err)
	}
	accessors, err := renderCacheAccessors(discoverCacheStoreNames(projectDir))
	if err != nil {
		return 0, err
	}
	formattedAccessors, err := format.Source(accessors)
	if err != nil {
		return 0, fmt.Errorf("failed to format generated cache accessors: %w", err)
	}
	written := 0
	changed, err := writeGeneratedSource(filepath.Join(projectDir, "internal", "caches", "manager_gen.go"), formattedManager)
	if err != nil {
		return written, err
	}
	if changed {
		written++
	}
	changed, err = writeGeneratedSource(filepath.Join(projectDir, "internal", "caches", "accessors_gen.go"), formattedAccessors)
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

// discoverCacheStoreNames includes names declared only through a configured App overlay.
func discoverCacheStoreNames(projectDir string) []string {
	names := discoverPrimitiveChildNames(projectDir, "CACHE", cacheRootKeys)
	for i := range names {
		names[i] = str.Of(names[i]).TrimSpace().ToLower().String()
	}
	sort.Strings(names)
	return names
}

// renderCacheAccessors keeps generated methods aligned with the named stores discovered for this build.
func renderCacheAccessors(names []string) ([]byte, error) {
	data := cacheAccessorTemplateData{
		OtherNames:    make([]cacheAccessorName, 0, len(names)),
		Names:         make([]cacheAccessorName, 0, len(names)),
		AccessorNames: make([]cacheAccessorName, 0, len(names)),
	}
	for _, name := range names {
		accessor := cacheAccessorName{
			Method: str.Of(name).Pascal().String(),
			Store:  name,
		}
		data.Names = append(data.Names, accessor)
		if name == "sessions" {
			data.HasSessions = true
			continue
		}
		data.OtherNames = append(data.OtherNames, accessor)
		data.AccessorNames = append(data.AccessorNames, accessor)
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

// renderCacheConfig snapshots cache driver choices once so imports and the compiled manifest cannot diverge.
func renderCacheConfig(projectDir string) ([]byte, error) {
	names := discoverCacheStoreNames(projectDir)
	driverSet := map[string]struct{}{}
	defaultDriver := effectivePrimitiveDriver(env.Get("CACHE_DRIVER", "memory"), "memory")
	driverSet[defaultDriver] = struct{}{}
	for _, child := range names {
		driver := effectivePrimitiveDriver(env.Get("CACHE_"+str.Of(child).Snake("_").ToUpper().String()+"_DRIVER", ""), "memory")
		driverSet[driver] = struct{}{}
	}
	for _, active := range appPrefixedActiveDrivers(projectDir, "CACHE", "memory", false) {
		driverSet[active.driver] = struct{}{}
	}
	drivers, err := supportedDrivers("CACHE", cacheDriverKeys, sortStrings(driverSet))
	if err != nil {
		return nil, err
	}
	data := cacheConfigTemplateData{
		CompiledDrivers: drivers,
		Drivers:         make([]cacheDriverSpec, 0, len(drivers)),
		OtherNames:      make([]cacheAccessorName, 0, len(names)),
		Names:           make([]cacheAccessorName, 0, len(names)),
		AccessorNames:   make([]cacheAccessorName, 0, len(names)),
	}
	for _, name := range names {
		accessor := cacheAccessorName{
			Method: str.Of(name).Pascal().String(),
			Store:  name,
		}
		data.Names = append(data.Names, accessor)
		if name == "sessions" {
			data.HasSessions = true
			continue
		}
		data.OtherNames = append(data.OtherNames, accessor)
		data.AccessorNames = append(data.AccessorNames, accessor)
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
// These cache manager accessors are derived from the current .env file
// and environment variables available when generation ran.
// Named accessors are generated from CACHE_<NAME>_<KEY> environment variables.
package caches

import (
	"github.com/goforj/cache"
	"github.com/goforj/str"
)

// Default returns the default cache instance derived from CACHE_* configuration.
func (m *Manager) Default() *cache.Cache {
	return m.defaultStore
}

// Sessions returns the "sessions" cache instance when configured.
func (m *Manager) Sessions() *cache.Cache {
	return m.sessions
}

{{ range .AccessorNames }}
// {{ .Method }} returns the "{{ .Store }}" cache instance.
func (m *Manager) {{ .Method }}() *cache.Cache {
	return m.{{ .Store }}
}

{{ end }}
// Names returns the generated cache names derived from CACHE_* configuration.
func (m *Manager) Names() []string {
	names := []string{"default"}
{{- range .Names }}
	names = append(names, "{{ .Store }}")
{{- end }}
	return names
}

// Instances returns the generated cache instances derived from CACHE_* configuration.
func (m *Manager) Instances() []Instance {
	instances := []Instance{
		{Name: "default", Store: m.defaultStore, IsDefault: true},
	}
{{- range .Names }}
	instances = append(instances, Instance{Name: "{{ .Store }}", Store: m.{{ .Store }}})
{{- end }}
	return instances
}

// Named returns the generated cache instance for a configured cache name.
func (m *Manager) Named(name string) *cache.Cache {
	switch str.Of(name).TrimSpace().ToLower().String() {
	case "", "default":
		return m.defaultStore
{{- range .Names }}
	case "{{ .Store }}":
		return m.{{ .Store }}
{{- end }}
	default:
		return nil
	}
}
`

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

var compiledCacheDrivers = []string{
{{- range .CompiledDrivers }}
	"{{ . }}",
{{- end }}
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

// Manager owns the cache stores generated from the project's build contract.
type Manager struct {
	defaultStore *cache.Cache
	sessions     *cache.Cache
{{- range .OtherNames }}
	{{ .Store }} *cache.Cache
{{- end }}
	observer     Observer
}

// Instance gives tooling a uniform view of each generated cache store.
type Instance struct {
	Name      string
	Store     *cache.Cache
	IsDefault bool
}

// ReadinessCheck pairs a stable cache name with its health probe.
type ReadinessCheck struct {
	Name  string
	Check func(context.Context) error
}

// Observer decouples generated cache instrumentation from its metrics and tracing consumers.
type Observer interface {
	// OnCacheOp gives generated cache wrappers one stable hook for operation telemetry.
	OnCacheOp(ctx context.Context, event CacheOpEvent)
}

// CacheOpEvent adds the logical store name to the cache runtime's operation event.
type CacheOpEvent struct {
	Name string
	cache.CacheOpEvent
}

// ObserverFunc lets a callback participate in the generated cache observer contract.
type ObserverFunc func(ctx context.Context, event CacheOpEvent)

// OnCacheOp adapts a callback so generated managers can compose it with interface-based observers.
func (f ObserverFunc) OnCacheOp(ctx context.Context, event CacheOpEvent) {
	if f == nil {
		return
	}
	f(ctx, event)
}

// observerChain retains multiple cache observers without exposing composition to callers.
type observerChain []Observer

// OnCacheOp preserves registration order when a cache operation is fanned out to multiple observers.
func (c observerChain) OnCacheOp(ctx context.Context, event CacheOpEvent) {
	for _, observer := range c {
		if observer == nil {
			continue
		}
		observer.OnCacheOp(ctx, event)
	}
}

// NewManager builds cache instances from the environment contract captured by this generated artifact.
func NewManager() (*Manager, error) {
	return newManagerFromEnv(env.WithPrefix("CACHE"))
}

// WithObserver adds observability without replacing observers already attached by framework wiring.
func (m *Manager) WithObserver(observer Observer) *Manager {
	if observer == nil {
		return m
	}
	if m.observer == nil {
		m.observer = observer
	} else {
		switch existing := m.observer.(type) {
		case observerChain:
			m.observer = append(existing, observer)
		default:
			m.observer = observerChain{existing, observer}
		}
	}
	combined := m.observer
	m.defaultStore = m.defaultStore.WithObserver(cache.ObserverFunc(func(ctx context.Context, event cache.CacheOpEvent) {
		combined.OnCacheOp(ctx, CacheOpEvent{Name: "default", CacheOpEvent: event})
	}))
{{- range .Names }}
{{- if eq .Store "sessions" }}
	m.sessions = m.sessions.WithObserver(cache.ObserverFunc(func(ctx context.Context, event cache.CacheOpEvent) {
		combined.OnCacheOp(ctx, CacheOpEvent{Name: "sessions", CacheOpEvent: event})
	}))
{{- else }}
	m.{{ .Store }} = m.{{ .Store }}.WithObserver(cache.ObserverFunc(func(ctx context.Context, event cache.CacheOpEvent) {
		combined.OnCacheOp(ctx, CacheOpEvent{Name: "{{ .Store }}", CacheOpEvent: event})
	}))
{{- end }}
{{- end }}
	return m
}

// ReadinessChecks exposes one probe per generated cache so health reflects every configured store.
func (m *Manager) ReadinessChecks() []ReadinessCheck {
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
{{- if eq .Store "sessions" }}
				return cacheReadinessCheck(ctx, m.sessions)
{{- else }}
				return cacheReadinessCheck(ctx, m.{{ .Store }})
{{- end }}
			},
		},
{{- end }}
	}
	return checks
}

// newManagerFromEnv constructs only the named cache stores captured when this artifact was generated.
func newManagerFromEnv(cacheScope env.Scope) (*Manager, error) {
	defaultStore, err := buildStore(string(defaultCacheName), cacheScope)
	if err != nil {
		return nil, err
	}
	manager := &Manager{
		defaultStore: defaultStore,
	}

{{- range .Names }}
	store{{ .Method }}, err := buildStore("{{ .Store }}", cacheScope.Child(str.Of("{{ .Store }}").Snake("_").ToUpper().String()))
	if err != nil {
		return nil, err
	}
{{- if eq .Store "sessions" }}
	manager.sessions = store{{ .Method }}
{{- else }}
	manager.{{ .Store }} = store{{ .Method }}
{{- end }}
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
	if !cacheDriverCompiled(driver) {
		return nil, fmt.Errorf("cache: active driver %q is not built in; compiled choices: %s; run forj generate --cache after updating CACHE_SUPPORTED_DRIVERS", driver, strings.Join(compiledCacheDrivers, ", "))
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

// cacheDriverCompiled reports whether driver is selectable in this generated artifact.
func cacheDriverCompiled(driver string) bool {
	for _, compiled := range compiledCacheDrivers {
		if driver == compiled {
			return true
		}
	}
	return false
}

// cacheDefaultTTL preserves expiration when configuration is missing or non-positive.
func cacheDefaultTTL(scope env.Scope) time.Duration {
	seconds := scope.GetInt("DEFAULT_TTL_SECONDS", "300")
	if seconds <= 0 {
		seconds = 300
	}
	return time.Duration(seconds) * time.Second
}

// cacheMemoryCleanupInterval preserves periodic cleanup when configuration is missing or non-positive.
func cacheMemoryCleanupInterval(scope env.Scope) time.Duration {
	seconds := scope.GetInt("MEMORY_CLEANUP_SECONDS", "600")
	if seconds <= 0 {
		seconds = 600
	}
	return time.Duration(seconds) * time.Second
}

// cachePrefix keeps the default namespace stable while allowing an explicit application prefix.
func cachePrefix(scope env.Scope) string {
	value := scope.Get("PREFIX", "app")
	if value == "" {
		return "app"
	}
	return value
}

// cacheFileDir isolates named file stores when no directory is supplied explicitly.
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

// cacheSQLiteDSN isolates named SQLite stores when the driver uses its generated default path.
func cacheSQLiteDSN(name string) string {
	base := filepath.Join(os.TempDir(), "cache-sqlite")
	if name == "" || name == string(defaultCacheName) {
		return filepath.Join(base, "default.db")
	}
	return filepath.Join(base, name+".db")
}

// cacheAddresses normalizes comma-separated endpoints before they reach multi-server cache drivers.
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

// cacheRedisTLSConfig leaves plaintext connections untouched unless TLS is explicitly selected.
func cacheRedisTLSConfig(scope env.Scope) *tls.Config {
	if !scope.GetBool("TLS", "false") {
		return nil
	}
	return &tls.Config{
		InsecureSkipVerify: scope.GetBool("INSECURE_SKIP_VERIFY", "false"),
	}
}

// cacheCompression constrains environment values to codecs supported by the cache runtime.
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

// cacheEncryptionKey treats blank configuration as encryption disabled instead of an empty secret.
func cacheEncryptionKey(scope env.Scope) []byte {
	value := strings.TrimSpace(scope.Get("ENCRYPTION_KEY", ""))
	if value == "" {
		return nil
	}
	return []byte(value)
}

// cacheReadinessCheck adapts both wrapper and driver readiness contracts to one generated probe.
func cacheReadinessCheck(ctx context.Context, store *cache.Cache) error {
	if ready, ok := any(store).(interface{ Ready() error }); ok {
		return ready.Ready()
	}
	if ready, ok := any(store.Store()).(interface{ Ready(context.Context) error }); ok {
		return ready.Ready(ctx)
	}
	return nil
}

{{- if .HasNATS }}
// buildNATSStore reuses an existing JetStream bucket or provisions it from the generated cache contract.
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

// cacheNATSBucket gives each named cache a stable, isolated JetStream bucket.
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

// cacheNATSHistory clamps history to the range accepted by JetStream key-value buckets.
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

// cacheNATSBucketTTL preserves zero as the JetStream convention for no bucket-wide expiration.
func cacheNATSBucketTTL(scope env.Scope) time.Duration {
	seconds := scope.GetInt("BUCKET_TTL_SECONDS", "0")
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
{{- end }}`
