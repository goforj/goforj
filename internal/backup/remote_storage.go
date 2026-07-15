package backup

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/goforj/storage"
	"github.com/goforj/storage/driver/s3storage"
)

// StorageObjectLister adapts a GoForj storage disk to the backup inventory contract.
type StorageObjectLister struct {
	Disk storage.Storage
}

// ObjectStorage identifies a configured object disk together with its scoped key prefix.
type ObjectStorage struct {
	Disk   storage.Storage
	Prefix string
}

// ListObjects lists all objects below a storage prefix.
func (l StorageObjectLister) ListObjects(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	if l.Disk == nil {
		return nil, fmt.Errorf("storage disk is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	objects := []ObjectInfo{}
	err := l.Disk.WithContext(ctx).Walk(strings.Trim(strings.TrimSpace(prefix), "/"), func(entry storage.Entry) error {
		if entry.IsDir {
			return nil
		}
		objects = append(objects, ObjectInfo{Key: entry.Path, Size: entry.Size})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk remote storage: %w", err)
	}
	return objects, nil
}

// ConfiguredObjectStorage opens a configured S3-compatible App disk.
func ConfiguredObjectStorage(name string) (ObjectStorage, error) {
	prefix := storageEnvPrefix(name)
	parseBool := func(key string) bool {
		value, _ := strconv.ParseBool(os.Getenv(prefix + key))
		return value
	}
	cfg := s3storage.Config{
		Bucket:          os.Getenv(prefix + "BUCKET"),
		Endpoint:        os.Getenv(prefix + "ENDPOINT"),
		Region:          os.Getenv(prefix + "REGION"),
		AccessKeyID:     os.Getenv(prefix + "ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv(prefix + "SECRET_ACCESS_KEY"),
		UsePathStyle:    parseBool("USE_PATH_STYLE"),
		Prefix:          os.Getenv(prefix + "PREFIX"),
	}
	disk, err := storage.Build(cfg)
	if err != nil {
		return ObjectStorage{}, fmt.Errorf("open S3 storage %s: %w", name, err)
	}
	return ObjectStorage{Disk: disk, Prefix: cfg.Prefix}, nil
}

// ConfiguredBackupRepository opens the configured S3 backup repository when one is enabled.
func ConfiguredBackupRepository() (BackupRepository, error) {
	driver := strings.ToLower(strings.TrimSpace(os.Getenv("APP_BACKUP_DRIVER")))
	if driver == "" || driver == "local" {
		return nil, nil
	}
	if driver != "s3" && driver != "b2-s3" {
		return nil, fmt.Errorf("unsupported backup repository driver %q", driver)
	}
	cfg := s3storage.Config{
		Bucket:          os.Getenv("APP_BACKUP_S3_BUCKET"),
		Endpoint:        os.Getenv("APP_BACKUP_S3_ENDPOINT"),
		Region:          os.Getenv("APP_BACKUP_S3_REGION"),
		AccessKeyID:     os.Getenv("APP_BACKUP_S3_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("APP_BACKUP_S3_SECRET_ACCESS_KEY"),
		UsePathStyle:    parseEnvBool("APP_BACKUP_S3_USE_PATH_STYLE"),
		Prefix:          os.Getenv("APP_BACKUP_S3_PREFIX"),
	}
	disk, err := storage.Build(cfg)
	if err != nil {
		return nil, fmt.Errorf("open backup repository: %w", err)
	}
	return StorageRepository{Disk: disk, Prefix: cfg.Prefix}, nil
}

// storageEnvPrefix returns the generated storage environment prefix for a disk.
func storageEnvPrefix(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || name == "default" {
		return "STORAGE_"
	}
	return "STORAGE_" + strings.ToUpper(strings.NewReplacer("-", "_", " ", "_", ".", "_").Replace(name)) + "_"
}

// parseEnvBool parses an optional boolean environment value.
func parseEnvBool(key string) bool {
	value, _ := strconv.ParseBool(os.Getenv(key))
	return value
}
