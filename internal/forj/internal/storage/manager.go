package storage

import (
	"fmt"

	"github.com/goforj/env/v2"
	goforjstorage "github.com/goforj/storage"
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
	inner *goforjstorage.Manager
}

func NewManager() (*Manager, error) {
	cfg, err := LoadConfigFromEnv()
	if err != nil {
		return nil, err
	}
	inner, err := goforjstorage.New(cfg)
	if err != nil {
		return nil, err
	}
	return &Manager{inner: inner}, nil
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

func (m *Manager) Default() goforjstorage.Storage {
	return m.inner.Default()
}

func (m *Manager) Disk(name goforjstorage.DiskName) (goforjstorage.Storage, error) {
	if name == "" || name == defaultDiskName {
		return m.inner.Default(), nil
	}
	return m.inner.Disk(name)
}

func (m *Manager) mustDisk(name string) goforjstorage.Storage {
	disk, err := m.Disk(goforjstorage.DiskName(name))
	if err != nil {
		panic(fmt.Sprintf("storage: required disk %q is not configured: %v", name, err))
	}
	return disk
}

func discoverDiskNames() []string {
	names := env.WithPrefix("STORAGE").ChildNames(storageRootKeys)
	for i := range names {
		names[i] = str.Of(names[i]).TrimSpace().ToLower().String()
	}
	return names
}
