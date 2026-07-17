package backup

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/goforj/env/v2"
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
	scope := storageEnvScope(name)
	usePathStyle, err := parseEnvBool(scope.Key("USE_PATH_STYLE"))
	if err != nil {
		return ObjectStorage{}, err
	}
	cfg := s3storage.Config{
		Bucket:          scope.Get("BUCKET", ""),
		Endpoint:        scope.Get("ENDPOINT", ""),
		Region:          scope.Get("REGION", ""),
		AccessKeyID:     scope.Get("ACCESS_KEY_ID", ""),
		SecretAccessKey: scope.Get("SECRET_ACCESS_KEY", ""),
		UsePathStyle:    usePathStyle,
		Prefix:          scope.Get("PREFIX", ""),
	}
	disk, err := storage.Build(cfg)
	if err != nil {
		return ObjectStorage{}, fmt.Errorf("open S3 storage %s: %w", name, err)
	}
	return ObjectStorage{Disk: disk, Prefix: cfg.Prefix}, nil
}

// ConfiguredBackupRepository opens the configured S3 backup repository when one is enabled.
func ConfiguredBackupRepository() (BackupRepository, error) {
	backupScope := env.WithPrefix("APP_BACKUP")
	driver := strings.ToLower(strings.TrimSpace(backupScope.Get("DRIVER", "")))
	if driver == "" || driver == "local" {
		return nil, nil
	}
	if driver != "s3" && driver != "b2-s3" {
		return nil, fmt.Errorf("unsupported backup repository driver %q", driver)
	}
	s3Scope := backupScope.Child("S3")
	usePathStyle, err := parseEnvBool(s3Scope.Key("USE_PATH_STYLE"))
	if err != nil {
		return nil, err
	}
	cfg := s3storage.Config{
		Bucket:          s3Scope.Get("BUCKET", ""),
		Endpoint:        s3Scope.Get("ENDPOINT", ""),
		Region:          s3Scope.Get("REGION", ""),
		AccessKeyID:     s3Scope.Get("ACCESS_KEY_ID", ""),
		SecretAccessKey: s3Scope.Get("SECRET_ACCESS_KEY", ""),
		UsePathStyle:    usePathStyle,
		Prefix:          s3Scope.Get("PREFIX", ""),
	}
	disk, err := storage.Build(cfg)
	if err != nil {
		return nil, fmt.Errorf("open backup repository: %w", err)
	}
	return StorageRepository{Disk: disk, Prefix: cfg.Prefix}, nil
}

// storageEnvScope returns the generated storage environment scope for a disk.
func storageEnvScope(name string) env.Scope {
	name = strings.TrimSpace(name)
	if name == "" || name == "default" {
		return env.WithPrefix("STORAGE")
	}
	child := strings.ToUpper(strings.NewReplacer("-", "_", " ", "_", ".", "_").Replace(name))
	return env.WithPrefix("STORAGE").Child(child)
}

// parseEnvBool keeps omitted storage flags optional while rejecting misspelled boolean values before opening infrastructure.
func parseEnvBool(key string) (bool, error) {
	value := strings.TrimSpace(env.Get(key, ""))
	if value == "" {
		return false, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("invalid %s value %q: %w", key, value, err)
	}
	return parsed, nil
}
