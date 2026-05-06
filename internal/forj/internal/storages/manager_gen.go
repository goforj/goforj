package storages

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/goforj/env/v2"
	"github.com/goforj/storage"
	"github.com/goforj/storage/driver/localstorage"
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
}

type ReadinessCheck struct {
	Name  string
	Check func(context.Context) error
}

func NewManager() (*Manager, error) {
	return newManagerFromEnv()
}

func (m *Manager) ReadinessChecks() []ReadinessCheck {
	if m == nil {
		return nil
	}
	return []ReadinessCheck{
		{
			Name: "storage_default",
			Check: func(ctx context.Context) error {
				return storageReadinessCheck(ctx, m.defaultDisk)
			},
		},
	}
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
	return &Manager{defaultDisk: inner.Default()}, nil
}

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
	case driverLocal:
		root := scope.Get("ROOT", localRoot)
		if err := os.MkdirAll(root, 0o755); err != nil {
			return nil, err
		}
		return localstorage.Config{
			Root:   root,
			Prefix: scope.Get("PREFIX", ""),
		}, nil
	default:
		return nil, fmt.Errorf("storage: unsupported driver %q", driver)
	}
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
