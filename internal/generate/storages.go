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

type storageAccessorTemplateData struct {
	Names []storageAccessorName
}

type storageAccessorName struct {
	Method string
	Disk   string
}

type storageConfigTemplateData struct {
	Drivers []storageDriverSpec
	Names   []storageAccessorName
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
		Fields: []storageConfigField{
			{Name: "Root", Value: `scope.Get("ROOT", localRoot)`},
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

func GenerateStorageFiles(projectDir string) (int, error) {
	if err := validatePrimitiveEnv(primitiveEnvContract{
		Prefix:        "STORAGE",
		DefaultDriver: "local",
		RootKeys:      storageRootKeys,
		CommonKeys:    storageCommonKeys,
		DriverKeys:    storageDriverKeys,
		ChildNames: func(scope env.Scope) []string {
			return scope.ChildNames(storageRootKeys)
		},
	}); err != nil {
		return 0, err
	}
	manager, err := renderStorageConfig()
	if err != nil {
		return 0, err
	}
	formattedManager, err := format.Source(manager)
	if err != nil {
		return 0, fmt.Errorf("failed to format generated storage manager: %w", err)
	}
	written := 0
	changed, err := writeGeneratedSource(filepath.Join(projectDir, "internal", "storages", "manager_gen.go"), formattedManager)
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

func discoverStorageDiskNames() []string {
	names := discoverStorageChildren()
	for i := range names {
		names[i] = str.Of(names[i]).TrimSpace().ToLower().String()
	}
	return names
}

func discoverStorageChildren() []string {
	return env.WithPrefix("STORAGE").ChildNames(storageRootKeys)
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

func renderStorageConfig() ([]byte, error) {
	names := discoverStorageDiskNames()
	driverSet := map[string]struct{}{}
	defaultDriver := str.Of(env.Get("STORAGE_DRIVER", "local")).TrimSpace().ToLower().String()
	if defaultDriver != "" {
		driverSet[defaultDriver] = struct{}{}
	}
	for _, child := range discoverStorageChildren() {
		driver := str.Of(env.Get("STORAGE_"+child+"_DRIVER", "")).TrimSpace().ToLower().String()
		if driver != "" {
			driverSet[driver] = struct{}{}
		}
	}
	drivers := make([]string, 0, len(driverSet))
	for driver := range driverSet {
		drivers = append(drivers, driver)
	}
	sort.Strings(drivers)
	data := storageConfigTemplateData{
		Drivers: make([]storageDriverSpec, 0, len(drivers)),
		Names:   make([]storageAccessorName, 0, len(names)),
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
// The default disk is exposed by Manager.Default().
// This file contains named storage disk accessors generated from
// STORAGE_<NAME>_<KEY> environment variables.
package storages

import "github.com/goforj/storage"
type namedDisks struct {
{{- range .Names }}
	{{ .Disk }} storage.Storage
{{- end }}
}

{{ range .Names }}
// {{ .Method }} returns the "{{ .Disk }}" storage disk.
func (m *Manager) {{ .Method }}() storage.Storage {
	return m.named.{{ .Disk }}
}
{{ end }}`

const storageConfigSourceTemplate = `// Code generated by forj generate --storage. DO NOT EDIT.
// Run: forj generate --storage
package storages

import (
	"fmt"
	"path/filepath"

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
{{- range .Names }}
	{{ .Disk }} storage.Storage
{{- end }}
}

func NewManager() (*Manager, error) {
	return newManagerFromEnv()
}

func (m *Manager) Default() storage.Storage {
	return m.defaultDisk
}

{{- range .Names }}
func (m *Manager) {{ .Method }}() storage.Storage {
	return m.{{ .Disk }}
}

{{- end }}
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

	for _, child := range storageScope.ChildNames(storageRootKeys) {
		name := storage.DiskName(str.Of(child).TrimSpace().ToLower().String())
		cfg, err := buildDiskConfig(name, storageScope.Child(child))
		if err != nil {
			return nil, err
		}
		disks[name] = cfg
	}

	return disks, nil
}

func newManagerFromEnv() (*Manager, error) {
	cfg, err := LoadConfigFromEnv()
	if err != nil {
		return nil, err
	}
	inner, err := storage.New(cfg)
	if err != nil {
		return nil, err
	}
	manager := &Manager{
		defaultDisk: inner.Default(),
	}
{{- range .Names }}
	disk{{ .Method }}, err := inner.Disk(storage.DiskName("{{ .Disk }}"))
	if err != nil {
		return nil, err
	}
	manager.{{ .Disk }} = disk{{ .Method }}
{{- end }}
	return manager, nil
}

// buildDiskConfig is generated from the storage disks currently defined in env.
// The supported driver cases and imports in this file are derived from
// STORAGE_* and STORAGE_<NAME>_* values at generate time.
func buildDiskConfig(name storage.DiskName, scope env.Scope) (storage.DriverConfig, error) {
	driver := str.Of(scope.Get("DRIVER", driverLocal)).TrimSpace().ToLower().String()
	if driver == "" {
		driver = driverLocal
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
}`
