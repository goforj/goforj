package generate

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"text/template"

	"github.com/goforj/env/v2"
	"github.com/goforj/str"
)

type storageAccessorTemplateData struct {
	Names []storageAccessorName
}

type storageAccessorName struct {
	Method string
	Disk   string
}

type storageConfigTemplateData struct {
	CompiledDrivers []string
	Drivers         []storageDriverSpec
	Names           []storageAccessorName
}

type storageDriverSpec struct {
	ConstName  string
	ImportPath string
	ConfigType string
	Setup      []string
	Fields     []storageConfigField
}

type storageConfigField struct {
	Name  string
	Value string
}

var storageDriverSpecs = map[string]storageDriverSpec{
	"dropbox": {
		ConstName:  "driverDropbox",
		ImportPath: "github.com/goforj/storage/driver/dropboxstorage",
		ConfigType: "dropboxstorage.Config",
		Fields: []storageConfigField{
			{Name: "Token", Value: `scope.Get("TOKEN", "")`},
			{Name: "Prefix", Value: `scope.Get("PREFIX", "")`},
		},
	},
	"ftp": {
		ConstName:  "driverFTP",
		ImportPath: "github.com/goforj/storage/driver/ftpstorage",
		ConfigType: "ftpstorage.Config",
		Fields: []storageConfigField{
			{Name: "Host", Value: `scope.Get("HOST", "")`},
			{Name: "Port", Value: `scope.GetInt("PORT", "21")`},
			{Name: "User", Value: `scope.Get("USER", "")`},
			{Name: "Password", Value: `scope.Get("PASSWORD", "")`},
			{Name: "TLS", Value: `scope.GetBool("TLS", "false")`},
			{Name: "InsecureSkipVerify", Value: `scope.GetBool("INSECURE_SKIP_VERIFY", "false")`},
			{Name: "Prefix", Value: `scope.Get("PREFIX", "")`},
		},
	},
	"gcs": {
		ConstName:  "driverGCS",
		ImportPath: "github.com/goforj/storage/driver/gcsstorage",
		ConfigType: "gcsstorage.Config",
		Fields: []storageConfigField{
			{Name: "Bucket", Value: `scope.Get("BUCKET", "")`},
			{Name: "CredentialsJSON", Value: `scope.Get("CREDENTIALS_JSON", "")`},
			{Name: "Endpoint", Value: `scope.Get("ENDPOINT", "")`},
			{Name: "Prefix", Value: `scope.Get("PREFIX", "")`},
		},
	},
	"local": {
		ConstName:  "driverLocal",
		ImportPath: "github.com/goforj/storage/driver/localstorage",
		ConfigType: "localstorage.Config",
		Setup: []string{
			`root := scope.Get("ROOT", localRoot)`,
			`if err := os.MkdirAll(root, 0o755); err != nil {`,
			`	return nil, err`,
			`}`,
		},
		Fields: []storageConfigField{
			{Name: "Root", Value: `root`},
			{Name: "Prefix", Value: `scope.Get("PREFIX", "")`},
		},
	},
	"memory": {
		ConstName:  "driverMemory",
		ImportPath: "github.com/goforj/storage/driver/memorystorage",
		ConfigType: "memorystorage.Config",
		Fields: []storageConfigField{
			{Name: "Prefix", Value: `scope.Get("PREFIX", "")`},
		},
	},
	"rclone": {
		ConstName:  "driverRclone",
		ImportPath: "github.com/goforj/storage/driver/rclonestorage",
		ConfigType: "rclonestorage.Config",
		Fields: []storageConfigField{
			{Name: "Remote", Value: `scope.Get("REMOTE", "")`},
			{Name: "Prefix", Value: `scope.Get("PREFIX", "")`},
			{Name: "RcloneConfigPath", Value: `scope.Get("RCLONE_CONFIG_PATH", "")`},
			{Name: "RcloneConfigData", Value: `scope.Get("RCLONE_CONFIG_DATA", "")`},
		},
	},
	"redis": {
		ConstName:  "driverRedis",
		ImportPath: "github.com/goforj/storage/driver/redisstorage",
		ConfigType: "redisstorage.Config",
		Setup: []string{
			`addr := str.Of(scope.Get("ADDR", "")).TrimSpace().String()`,
			`if addr == "" {`,
			`	addr = fmt.Sprintf("%s:%s", env.Get("REDIS_HOST", "redis"), env.Get("REDIS_PORT", "6379"))`,
			`}`,
		},
		Fields: []storageConfigField{
			{Name: "Addr", Value: "addr"},
			{Name: "Username", Value: `scope.Get("USERNAME", "")`},
			{Name: "Password", Value: `scope.Get("PASSWORD", env.Get("REDIS_PASSWORD", ""))`},
			{Name: "DB", Value: `scope.GetInt("DB", "0")`},
			{Name: "Prefix", Value: `scope.Get("PREFIX", "")`},
		},
	},
	"s3": {
		ConstName:  "driverS3",
		ImportPath: "github.com/goforj/storage/driver/s3storage",
		ConfigType: "s3storage.Config",
		Fields: []storageConfigField{
			{Name: "Bucket", Value: `scope.Get("BUCKET", "")`},
			{Name: "Endpoint", Value: `scope.Get("ENDPOINT", "")`},
			{Name: "Region", Value: `scope.Get("REGION", "us-east-1")`},
			{Name: "AccessKeyID", Value: `scope.Get("ACCESS_KEY_ID", "")`},
			{Name: "SecretAccessKey", Value: `scope.Get("SECRET_ACCESS_KEY", "")`},
			{Name: "UsePathStyle", Value: `scope.GetBool("USE_PATH_STYLE", "false")`},
			{Name: "UnsignedPayload", Value: `scope.GetBool("UNSIGNED_PAYLOAD", "false")`},
			{Name: "Prefix", Value: `scope.Get("PREFIX", "")`},
		},
	},
	"sftp": {
		ConstName:  "driverSFTP",
		ImportPath: "github.com/goforj/storage/driver/sftpstorage",
		ConfigType: "sftpstorage.Config",
		Fields: []storageConfigField{
			{Name: "Host", Value: `scope.Get("HOST", "")`},
			{Name: "Port", Value: `scope.GetInt("PORT", "22")`},
			{Name: "User", Value: `scope.Get("USER", "root")`},
			{Name: "Password", Value: `scope.Get("PASSWORD", "")`},
			{Name: "KeyPath", Value: `scope.Get("KEY_PATH", "")`},
			{Name: "KnownHostsPath", Value: `scope.Get("KNOWN_HOSTS_PATH", "")`},
			{Name: "InsecureIgnoreHostKey", Value: `scope.GetBool("INSECURE_IGNORE_HOST_KEY", "false")`},
			{Name: "Prefix", Value: `scope.Get("PREFIX", "")`},
		},
	},
}

var storageRootKeys = []string{
	"DRIVER",
	"ROOT",
	"PREFIX",
	"ADDR",
	"USERNAME",
	"PASSWORD",
	"DB",
	"HOST",
	"PORT",
	"USER",
	"TLS",
	"INSECURE_SKIP_VERIFY",
	"KEY_PATH",
	"KNOWN_HOSTS_PATH",
	"INSECURE_IGNORE_HOST_KEY",
	"BUCKET",
	"ENDPOINT",
	"REGION",
	"ACCESS_KEY_ID",
	"SECRET_ACCESS_KEY",
	"USE_PATH_STYLE",
	"UNSIGNED_PAYLOAD",
	"CREDENTIALS_JSON",
	"TOKEN",
	"REMOTE",
	"RCLONE_CONFIG_PATH",
	"RCLONE_CONFIG_DATA",
}

var storageCommonKeys = makeSet(
	"DRIVER",
	"PREFIX",
)

var storageDriverKeys = map[string]map[string]struct{}{
	"local":   makeSet("ROOT"),
	"memory":  makeSet(),
	"redis":   makeSet("ADDR", "USERNAME", "PASSWORD", "DB"),
	"ftp":     makeSet("HOST", "PORT", "USER", "PASSWORD", "TLS", "INSECURE_SKIP_VERIFY"),
	"sftp":    makeSet("HOST", "PORT", "USER", "PASSWORD", "KEY_PATH", "KNOWN_HOSTS_PATH", "INSECURE_IGNORE_HOST_KEY"),
	"s3":      makeSet("BUCKET", "ENDPOINT", "REGION", "ACCESS_KEY_ID", "SECRET_ACCESS_KEY", "USE_PATH_STYLE", "UNSIGNED_PAYLOAD"),
	"gcs":     makeSet("BUCKET", "CREDENTIALS_JSON", "ENDPOINT"),
	"dropbox": makeSet("TOKEN"),
	"rclone":  makeSet("REMOTE", "RCLONE_CONFIG_PATH", "RCLONE_CONFIG_DATA"),
}

// GenerateStorageFiles writes disk accessors whose selectable backends are fixed by the generation snapshot.
func GenerateStorageFiles(projectDir string) (int, error) {
	if err := validatePrimitiveEnv(projectDir, primitiveEnvContract{
		Prefix:        "STORAGE",
		DefaultDriver: "local",
		RootKeys:      storageRootKeys,
		CommonKeys:    storageCommonKeys,
		DriverKeys:    storageDriverKeys,
		ChildNames: func(scope env.Scope) []string {
			return exactScopedChildNames("STORAGE", storageRootKeys)
		},
		AllowInactiveRootKeys: true,
		EagerNamedResources:   true,
	}); err != nil {
		return 0, err
	}
	manager, err := renderStorageConfig(projectDir)
	if err != nil {
		return 0, err
	}
	formattedManager, err := format.Source(manager)
	if err != nil {
		return 0, fmt.Errorf("failed to format generated storage manager: %w", err)
	}
	accessors, err := renderStorageAccessors(discoverStorageDiskNames(projectDir))
	if err != nil {
		return 0, err
	}
	formattedAccessors, err := format.Source(accessors)
	if err != nil {
		return 0, fmt.Errorf("failed to format generated storage accessors: %w", err)
	}
	written := 0
	changed, err := writeGeneratedSource(filepath.Join(projectDir, "internal", "storages", "manager_gen.go"), formattedManager)
	if err != nil {
		return written, err
	}
	if changed {
		written++
	}
	changed, err = writeGeneratedSource(filepath.Join(projectDir, "internal", "storages", "accessors_gen.go"), formattedAccessors)
	if err != nil {
		return written, err
	}
	if changed {
		written++
	}
	_ = os.Remove(filepath.Join(projectDir, "internal", "storages", "runtime.go"))
	_ = os.Remove(filepath.Join(projectDir, "internal", "storages", "manager.go"))
	_ = os.Remove(filepath.Join(projectDir, "internal", "storages", "disks_gen.go"))
	_ = os.Remove(filepath.Join(projectDir, "internal", "storages", "config_gen.go"))
	return written, nil
}

// discoverStorageDiskNames normalizes App and resource-first scopes into generated accessor names.
func discoverStorageDiskNames(projectDir string) []string {
	names := discoverStorageChildren(projectDir)
	for i := range names {
		names[i] = str.Of(names[i]).TrimSpace().ToLower().String()
	}
	return names
}

// discoverStorageChildren includes disks declared only through a configured App overlay.
func discoverStorageChildren(projectDir string) []string {
	return discoverPrimitiveChildNames(projectDir, "STORAGE", storageRootKeys)
}

// exactScopedChildNames finds names only when their trailing key matches a complete resource key.
func exactScopedChildNames(prefix string, rootKeys []string) []string {
	prefix = strings.TrimSpace(strings.ToUpper(prefix))
	if prefix == "" {
		return nil
	}

	rootKeyParts := make(map[string][]string, len(rootKeys))
	orderedRootKeys := make([]string, 0, len(rootKeys))
	for _, key := range rootKeys {
		normalized := strings.TrimSpace(strings.ToUpper(key))
		if normalized == "" {
			continue
		}
		rootKeyParts[normalized] = strings.Split(normalized, "_")
		orderedRootKeys = append(orderedRootKeys, normalized)
	}
	sort.SliceStable(orderedRootKeys, func(left, right int) bool {
		return len(rootKeyParts[orderedRootKeys[left]]) > len(rootKeyParts[orderedRootKeys[right]])
	})

	seen := map[string]struct{}{}
	names := make([]string, 0)
	envPrefix := prefix + "_"

	for _, kv := range os.Environ() {
		key, _, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(key, envPrefix) {
			continue
		}
		suffix := strings.TrimPrefix(key, envPrefix)
		if suffix == "" {
			continue
		}
		parts := strings.Split(strings.ToUpper(suffix), "_")
		for _, root := range orderedRootKeys {
			rootParts := rootKeyParts[root]
			if len(parts) <= len(rootParts) || !slices.Equal(parts[len(parts)-len(rootParts):], rootParts) {
				continue
			}
			child := strings.Join(parts[:len(parts)-len(rootParts)], "_")
			if child == "" {
				continue
			}
			if _, exists := seen[child]; exists {
				continue
			}
			seen[child] = struct{}{}
			names = append(names, child)
			break
		}
	}

	sort.Strings(names)
	return names
}

func renderStorageAccessors(names []string) ([]byte, error) {
	data := storageAccessorTemplateData{
		Names: make([]storageAccessorName, 0, len(names)),
	}
	for _, name := range names {
		data.Names = append(data.Names, storageAccessorName{
			Method: str.Of(name).Pascal().String(),
			Disk:   name,
		})
	}
	var b bytes.Buffer
	tmpl, err := template.New("storage-accessors").Parse(storageAccessorsSourceTemplate)
	if err != nil {
		return nil, err
	}
	if err := tmpl.Execute(&b, data); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// renderStorageConfig retains the native local backend without widening the authoritative compiled manifest.
func renderStorageConfig(projectDir string) ([]byte, error) {
	names := discoverStorageDiskNames(projectDir)
	driverSet := map[string]struct{}{}
	defaultDriver := effectivePrimitiveDriver(env.Get("STORAGE_DRIVER", "local"), "local")
	driverSet[defaultDriver] = struct{}{}
	for _, child := range discoverStorageChildren(projectDir) {
		driver := effectivePrimitiveDriver(env.Get("STORAGE_"+child+"_DRIVER", ""), "local")
		driverSet[driver] = struct{}{}
	}
	for _, appPrefix := range generationAppEnvPrefixes(projectDir) {
		resourcePrefix := appPrefix + "_STORAGE"
		for _, child := range exactScopedChildNames(resourcePrefix, storageRootKeys) {
			driver := effectiveAppPrimitiveChildDriver(resourcePrefix, primitiveEnvContract{
				Prefix:        "STORAGE",
				DefaultDriver: "local",
			}, defaultDriver, defaultDriver, child)
			driverSet[driver] = struct{}{}
		}
	}
	for _, active := range appPrefixedActiveDrivers(projectDir, "STORAGE", "local", false) {
		driverSet[active.driver] = struct{}{}
	}
	drivers, err := supportedDrivers("STORAGE", storageDriverKeys, sortStrings(driverSet))
	if err != nil {
		return nil, err
	}
	compiledDrivers := slices.Clone(drivers)
	drivers = appendMissingString(drivers, "local")
	data := storageConfigTemplateData{
		CompiledDrivers: compiledDrivers,
		Drivers:         make([]storageDriverSpec, 0, len(drivers)),
		Names:           make([]storageAccessorName, 0, len(names)),
	}
	for _, name := range names {
		data.Names = append(data.Names, storageAccessorName{
			Method: str.Of(name).Pascal().String(),
			Disk:   name,
		})
	}
	for _, driver := range drivers {
		if spec, ok := storageDriverSpecs[driver]; ok {
			data.Drivers = append(data.Drivers, spec)
		}
	}
	var b bytes.Buffer
	tmpl, err := template.New("storage-config").Parse(storageConfigSourceTemplate)
	if err != nil {
		return nil, err
	}
	if err := tmpl.Execute(&b, data); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func writeGeneratedSource(path string, content []byte) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, content) {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

const storageAccessorsSourceTemplate = `// Code generated by forj generate --storage. DO NOT EDIT.
// Run: forj generate --storage
//
// These storage manager accessors are derived from the current .env file
// and environment variables available when generation ran.
// Named accessors are generated from STORAGE_<NAME>_<KEY> environment variables.
package storages

import "github.com/goforj/storage"

// Default returns the default storage disk instance derived from STORAGE_* configuration.
func (m *Manager) Default() storage.Storage {
	return m.defaultDisk
}

{{ range .Names }}
// {{ .Method }} returns the "{{ .Disk }}" storage disk.
func (m *Manager) {{ .Method }}() storage.Storage {
	return m.{{ .Disk }}
}
{{ end }}

// Instances returns the generated storage disk instances derived from STORAGE_* configuration.
func (m *Manager) Instances() []Instance {
	instances := []Instance{
		{Name: "default", Driver: m.defaultDriver, Disk: m.defaultDisk, IsDefault: true},
	}
{{- range .Names }}
	if m.{{ .Disk }} != nil {
		instances = append(instances, Instance{Name: "{{ .Disk }}", Driver: m.{{ .Disk }}Driver, Disk: m.{{ .Disk }}})
	}
{{- end }}
	return instances
}`

const storageConfigSourceTemplate = `// Code generated by forj generate --storage. DO NOT EDIT.
// Run: forj generate --storage
package storages

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"slices"
	"strings"
	"time"

	"github.com/goforj/env/v2"
	"github.com/goforj/storage"
{{- range .Drivers }}
	"{{ .ImportPath }}"
{{- end }}
	"github.com/goforj/str"
)

const defaultDiskName storage.DiskName = "default"

const (
	driverDropbox = "dropbox"
	driverFTP     = "ftp"
	driverGCS     = "gcs"
	driverLocal   = "local"
	driverMemory  = "memory"
	driverRclone  = "rclone"
	driverRedis   = "redis"
	driverS3      = "s3"
	driverSFTP    = "sftp"
)

var compiledStorageDrivers = []string{
{{- range .CompiledDrivers }}
	"{{ . }}",
{{- end }}
}

var storageRootKeys = []string{
	"DRIVER",
	"ROOT",
	"PREFIX",
	"ADDR",
	"USERNAME",
	"PASSWORD",
	"DB",
	"HOST",
	"PORT",
	"USER",
	"TLS",
	"INSECURE_SKIP_VERIFY",
	"KEY_PATH",
	"KNOWN_HOSTS_PATH",
	"INSECURE_IGNORE_HOST_KEY",
	"BUCKET",
	"ENDPOINT",
	"REGION",
	"ACCESS_KEY_ID",
	"SECRET_ACCESS_KEY",
	"USE_PATH_STYLE",
	"UNSIGNED_PAYLOAD",
	"CREDENTIALS_JSON",
	"TOKEN",
	"REMOTE",
	"RCLONE_CONFIG_PATH",
	"RCLONE_CONFIG_DATA",
}

type Manager struct {
	defaultDisk storage.Storage
	observer Observer
	defaultDriver string
	warnings []OptionalDiskWarning
{{- range .Names }}
	{{ .Disk }} storage.Storage
	{{ .Disk }}Driver string
{{- end }}
}

type Instance struct {
	Name      string
	Driver    string
	Disk      storage.Storage
	IsDefault bool
}

type ReadinessCheck struct {
	Name  string
	Check func(context.Context) error
}

type OptionalDiskWarning struct {
	Name   string
	Driver string
	Error  string
}

type Observer interface {
	OnStorageOp(ctx context.Context, event StorageOpEvent)
}

type StorageOpEvent struct {
	Operation string
	Disk      string
	Path      string
	Driver    string
	Err       error
	Duration  time.Duration
}

type ObserverFunc func(ctx context.Context, event StorageOpEvent)

func (f ObserverFunc) OnStorageOp(ctx context.Context, event StorageOpEvent) {
	if f == nil {
		return
	}
	f(ctx, event)
}

type observerChain []Observer

func (c observerChain) OnStorageOp(ctx context.Context, event StorageOpEvent) {
	for _, observer := range c {
		if observer == nil {
			continue
		}
		observer.OnStorageOp(ctx, event)
	}
}

func NewManager() (*Manager, error) {
	return newManagerFromEnv()
}

func (m *Manager) WithObserver(observer Observer) *Manager {
	if m == nil || observer == nil {
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
	if m.defaultDisk != nil {
		m.defaultDisk = wrapObservedStorage(m.defaultDisk, "default", m.defaultDriver, combined)
	}
{{- range .Names }}
	if m.{{ .Disk }} != nil {
		m.{{ .Disk }} = wrapObservedStorage(m.{{ .Disk }}, "{{ .Disk }}", m.{{ .Disk }}Driver, combined)
	}
{{- end }}
	return m
}

func (m *Manager) Warnings() []OptionalDiskWarning {
	if m == nil || len(m.warnings) == 0 {
		return nil
	}
	out := make([]OptionalDiskWarning, len(m.warnings))
	copy(out, m.warnings)
	return out
}

func (m *Manager) ReadinessChecks() []ReadinessCheck {
	if m == nil {
		return nil
	}
	checks := []ReadinessCheck{
		{
			Name: "storage_default",
			Check: func(ctx context.Context) error {
				return storageReadinessCheck(ctx, m.defaultDisk)
			},
		},
	}
{{- range .Names }}
	if m.{{ .Disk }} != nil {
		checks = append(checks, ReadinessCheck{
			Name: "storage_{{ .Disk }}",
			Check: func(ctx context.Context) error {
				return storageReadinessCheck(ctx, m.{{ .Disk }})
			},
		})
	}
{{- end }}
	return checks
}

func LoadConfigFromEnv() (storage.Config, error) {
	storageScope := env.WithPrefix("STORAGE")
	disks, err := loadDisksFromEnv(storageScope)
	if err != nil {
		return storage.Config{}, err
	}
	return storage.Config{
		Default: defaultDiskName,
		Disks:   disks,
	}, nil
}

func loadDisksFromEnv(storageScope env.Scope) (map[storage.DiskName]storage.DriverConfig, error) {
	disks := map[storage.DiskName]storage.DriverConfig{}

	defaultCfg, err := buildDiskConfig(defaultDiskName, storageScope)
	if err != nil {
		return nil, err
	}
	disks[defaultDiskName] = defaultCfg

	for _, child := range storageChildNamesFromEnv() {
		name := storage.DiskName(str.Of(child).TrimSpace().ToLower().String())
		cfg, err := buildDiskConfig(name, storageScope.Child(child))
		if err != nil {
			return nil, err
		}
		disks[name] = cfg
	}

	return disks, nil
}

func storageChildNamesFromEnv() []string {
	rootKeyParts := make(map[string][]string, len(storageRootKeys))
	for _, key := range storageRootKeys {
		rootKeyParts[key] = strings.Split(key, "_")
	}

	seen := map[string]struct{}{}
	names := make([]string, 0)
	for _, kv := range os.Environ() {
		key, _, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(key, "STORAGE_") {
			continue
		}
		suffix := strings.TrimPrefix(key, "STORAGE_")
		if suffix == "" {
			continue
		}
		parts := strings.Split(strings.ToUpper(suffix), "_")
		for _, rootKey := range storageRootKeys {
			rootParts := rootKeyParts[rootKey]
			if len(parts) <= len(rootParts) || !slices.Equal(parts[len(parts)-len(rootParts):], rootParts) {
				continue
			}
			child := strings.Join(parts[:len(parts)-len(rootParts)], "_")
			if child == "" {
				continue
			}
			if _, exists := seen[child]; exists {
				continue
			}
			seen[child] = struct{}{}
			names = append(names, child)
			break
		}
	}
	sort.Strings(names)
	return names
}

func newManagerFromEnv() (*Manager, error) {
	storageScope := env.WithPrefix("STORAGE")
	defaultCfg, err := buildDiskConfig(defaultDiskName, storageScope)
	if err != nil {
		return nil, err
	}
	defaultDisk, err := storage.Build(defaultCfg)
	if err != nil {
		return nil, fmt.Errorf("storage: initialize disk %q: %w", defaultDiskName, err)
	}
	manager := &Manager{
		defaultDisk: defaultDisk,
		defaultDriver: storageDriverNameFromScope(storageScope),
	}
{{- range .Names }}
	disk{{ .Method }}, warning{{ .Method }}, err := optionalDiskFromScope(storageScope, storage.DiskName("{{ .Disk }}"))
	if err != nil {
		return nil, err
	}
	manager.{{ .Disk }} = disk{{ .Method }}
	if disk{{ .Method }} != nil {
		manager.{{ .Disk }}Driver = storageDriverNameFromScope(storageScope.Child(str.Of("{{ .Disk }}").Snake("_").ToUpper().String()))
	} else if warning{{ .Method }} != nil {
		manager.warnings = append(manager.warnings, *warning{{ .Method }})
	}
{{- end }}
	return manager, nil
}

func optionalDiskFromScope(storageScope env.Scope, name storage.DiskName) (storage.Storage, *OptionalDiskWarning, error) {
	childScope := storageScope.Child(str.Of(string(name)).Snake("_").ToUpper().String())
	cfg, err := buildDiskConfig(name, childScope)
	if err != nil {
		return nil, nil, err
	}
	disk, err := storage.Build(cfg)
	if err == nil {
		return disk, nil, nil
	}
	if isOptionalStorageDiskError(err) {
		driver := storageDriverNameFromScope(childScope)
		return nil, &OptionalDiskWarning{
			Name:   string(name),
			Driver: driver,
			Error:  err.Error(),
		}, nil
	}
	return nil, nil, err
}

func isOptionalStorageDiskError(err error) bool {
	if err == nil {
		return false
	}
	if strings.HasPrefix(err.Error(), "storage: disk \"") && strings.HasSuffix(err.Error(), "\" not found") {
		return true
	}
	return strings.Contains(err.Error(), "connect: connection refused") ||
		strings.Contains(err.Error(), "no such host") ||
		strings.Contains(err.Error(), "i/o timeout") ||
		strings.Contains(err.Error(), "network is unreachable")
}

func storageDriverNameFromScope(scope env.Scope) string {
	driver := str.Of(scope.Get("DRIVER", driverLocal)).TrimSpace().ToLower().String()
	if driver == "" {
		return driverLocal
	}
	return driver
}

// buildDiskConfig is generated from the storage disks currently defined in env.
// The supported driver cases and imports in this file are derived from
// STORAGE_SUPPORTED_DRIVERS, or from active STORAGE_* and STORAGE_<NAME>_* values when unset.
// buildDiskConfig rejects backends outside the artifact manifest before endpoint configuration is constructed.
func buildDiskConfig(name storage.DiskName, scope env.Scope) (storage.DriverConfig, error) {
	driver := str.Of(scope.Get("DRIVER", driverLocal)).TrimSpace().ToLower().String()
	if driver == "" {
		driver = driverLocal
	}
	if !storageDriverCompiled(driver) {
		return nil, fmt.Errorf("storage: active driver %q is not built in; compiled choices: %s; run forj generate --storage after updating STORAGE_SUPPORTED_DRIVERS", driver, strings.Join(compiledStorageDrivers, ", "))
	}

	localRoot := filepath.Join("storage", "app", "private")
	if name != defaultDiskName {
		localRoot = filepath.Join("storage", "app", string(name))
	}

	switch driver {
{{- range .Drivers }}
	case {{ .ConstName }}:
{{- range .Setup }}
		{{ . }}
{{- end }}
		return {{ .ConfigType }}{
{{- range .Fields }}
			{{ .Name }}: {{ .Value }},
{{- end }}
		}, nil
{{- end }}
	default:
		return nil, fmt.Errorf("storage: unsupported driver %q", driver)
	}
}

// storageDriverCompiled reports whether driver is selectable in this generated artifact.
func storageDriverCompiled(driver string) bool {
	for _, compiled := range compiledStorageDrivers {
		if driver == compiled {
			return true
		}
	}
	return false
}

func storageReadinessCheck(ctx context.Context, disk storage.Storage) error {
	if disk == nil {
		return nil
	}
	if ready, ok := any(disk).(interface{ Ready(context.Context) error }); ok {
		return ready.Ready(ctx)
	}
	if ready, ok := any(disk).(interface{ Ready() error }); ok {
		return ready.Ready()
	}
	_, err := disk.WithContext(ctx).List("")
	return err
}

type observedStorage struct {
	inner    storage.Storage
	name     string
	driver   string
	observer Observer
	ctx      context.Context
}

func wrapObservedStorage(inner storage.Storage, name string, driver string, observer Observer) storage.Storage {
	if inner == nil || observer == nil {
		return inner
	}
	if wrapped, ok := inner.(*observedStorage); ok {
		wrapped.name = name
		wrapped.driver = driver
		wrapped.observer = observer
		return wrapped
	}
	return &observedStorage{
		inner:    inner,
		name:     name,
		driver:   driver,
		observer: observer,
	}
}

func (s *observedStorage) observe(ctx context.Context, op string, path string, start time.Time, err error) {
	if s == nil || s.observer == nil {
		return
	}
	s.observer.OnStorageOp(ctx, StorageOpEvent{
		Operation: op,
		Disk:      s.name,
		Path:      path,
		Driver:    s.driver,
		Err:       err,
		Duration:  time.Since(start),
	})
}

func (s *observedStorage) WithContext(ctx context.Context) storage.Storage {
	clone := *s
	if ctx == nil {
		ctx = context.Background()
	}
	clone.ctx = ctx
	return &clone
}

func (s *observedStorage) context() context.Context {
	if s == nil || s.ctx == nil {
		return context.Background()
	}
	return s.ctx
}

func (s *observedStorage) Get(p string) ([]byte, error) {
	start := time.Now()
	ctx := s.context()
	body, err := s.inner.WithContext(ctx).Get(p)
	s.observe(ctx, "get", p, start, err)
	return body, err
}

func (s *observedStorage) Put(p string, contents []byte) error {
	start := time.Now()
	ctx := s.context()
	err := s.inner.WithContext(ctx).Put(p, contents)
	s.observe(ctx, "put", p, start, err)
	return err
}

func (s *observedStorage) MakeDir(p string) error {
	start := time.Now()
	ctx := s.context()
	err := s.inner.WithContext(ctx).MakeDir(p)
	s.observe(ctx, "make_dir", p, start, err)
	return err
}

func (s *observedStorage) Delete(p string) error {
	start := time.Now()
	ctx := s.context()
	err := s.inner.WithContext(ctx).Delete(p)
	s.observe(ctx, "delete", p, start, err)
	return err
}

func (s *observedStorage) Stat(p string) (storage.Entry, error) {
	start := time.Now()
	ctx := s.context()
	entry, err := s.inner.WithContext(ctx).Stat(p)
	s.observe(ctx, "stat", p, start, err)
	return entry, err
}

func (s *observedStorage) Exists(p string) (bool, error) {
	start := time.Now()
	ctx := s.context()
	exists, err := s.inner.WithContext(ctx).Exists(p)
	s.observe(ctx, "exists", p, start, err)
	return exists, err
}

func (s *observedStorage) List(p string) ([]storage.Entry, error) {
	start := time.Now()
	ctx := s.context()
	entries, err := s.inner.WithContext(ctx).List(p)
	s.observe(ctx, "list", p, start, err)
	return entries, err
}

func (s *observedStorage) Walk(p string, fn func(storage.Entry) error) error {
	start := time.Now()
	ctx := s.context()
	err := s.inner.WithContext(ctx).Walk(p, fn)
	s.observe(ctx, "walk", p, start, err)
	return err
}

func (s *observedStorage) Copy(src, dst string) error {
	start := time.Now()
	ctx := s.context()
	err := s.inner.WithContext(ctx).Copy(src, dst)
	s.observe(ctx, "copy", src+" -> "+dst, start, err)
	return err
}

func (s *observedStorage) Move(src, dst string) error {
	start := time.Now()
	ctx := s.context()
	err := s.inner.WithContext(ctx).Move(src, dst)
	s.observe(ctx, "move", src+" -> "+dst, start, err)
	return err
}

func (s *observedStorage) URL(p string) (string, error) {
	start := time.Now()
	ctx := s.context()
	url, err := s.inner.WithContext(ctx).URL(p)
	s.observe(ctx, "url", p, start, err)
	return url, err
}

func (s *observedStorage) ListPage(p string, offset, limit int) (storage.ListPageResult, error) {
	start := time.Now()
	ctx := s.context()
	paged, ok := s.inner.WithContext(ctx).(storage.PagedStorage)
	if !ok {
		err := storage.ErrUnsupported
		s.observe(ctx, "list_page", p, start, err)
		return storage.ListPageResult{}, err
	}
	result, err := paged.ListPage(p, offset, limit)
	s.observe(ctx, "list_page", p, start, err)
	return result, err
}`
