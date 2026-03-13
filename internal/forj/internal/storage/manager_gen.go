package storage

import (
	"fmt"
	"path/filepath"

	"github.com/goforj/env/v2"
	goforjstorage "github.com/goforj/storage"
	"github.com/goforj/storage/driver/localstorage"
	"github.com/goforj/str"
)

const defaultDiskName goforjstorage.DiskName = "default"

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
	defaultDisk goforjstorage.Storage
}

func NewManager() (*Manager, error) {
	return newManagerFromEnv()
}

func (m *Manager) Default() goforjstorage.Storage {
	return m.defaultDisk
}

func LoadConfigFromEnv() (goforjstorage.Config, error) {
	storageScope := env.WithPrefix("STORAGE")
	disks, err := loadDisksFromEnv(storageScope)
	if err != nil {
		return goforjstorage.Config{}, err
	}
	return goforjstorage.Config{
		Default: defaultDiskName,
		Disks:   disks,
	}, nil
}

func loadDisksFromEnv(storageScope env.Scope) (map[goforjstorage.DiskName]goforjstorage.DriverConfig, error) {
	disks := map[goforjstorage.DiskName]goforjstorage.DriverConfig{}

	defaultCfg, err := buildDiskConfig(defaultDiskName, storageScope)
	if err != nil {
		return nil, err
	}
	disks[defaultDiskName] = defaultCfg

	for _, child := range storageScope.ChildNames(storageRootKeys) {
		name := goforjstorage.DiskName(str.Of(child).TrimSpace().ToLower().String())
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
	inner, err := goforjstorage.New(cfg)
	if err != nil {
		return nil, err
	}
	return &Manager{defaultDisk: inner.Default()}, nil
}

func buildDiskConfig(name goforjstorage.DiskName, scope env.Scope) (goforjstorage.DriverConfig, error) {
	driver := str.Of(scope.Get("DRIVER", driverLocal)).TrimSpace().ToLower().String()
	if driver == "" {
		driver = driverLocal
	}

	localRoot := filepath.Join("storage", "app", "private")
	if name != defaultDiskName {
		localRoot = filepath.Join("storage", "app", string(name))
	}

	switch driver {
	case driverLocal:
		return localstorage.Config{
			Root:   scope.Get("ROOT", localRoot),
			Prefix: scope.Get("PREFIX", ""),
		}, nil
	default:
		return nil, fmt.Errorf("storage: unsupported driver %q", driver)
	}
}
