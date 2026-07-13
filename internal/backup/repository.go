package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goforj/storage"
)

// BackupRepository stores complete manifest-backed backup directories.
type BackupRepository interface {
	Upload(context.Context, string, string) error
	Download(context.Context, string, string) error
	List(context.Context, string) ([]string, error)
	Delete(context.Context, string) error
}

// StorageRepository stores backup files in a GoForj storage backend.
type StorageRepository struct {
	Disk   storage.Storage
	Prefix string
}

// Upload copies a completed local backup directory into the storage backend.
func (r StorageRepository) Upload(ctx context.Context, name string, source string) error {
	disk, prefix, err := r.bound(ctx, name)
	if err != nil {
		return err
	}
	return filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported backup repository file %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(filepath.Join(prefix, filepath.ToSlash(relative)))
		if err := disk.Put(key, data); err != nil {
			return fmt.Errorf("upload %s: %w", key, err)
		}
		return nil
	})
}

// Download copies a remote backup directory into a local destination.
func (r StorageRepository) Download(ctx context.Context, name string, destination string) error {
	disk, prefix, err := r.bound(ctx, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	return disk.Walk(prefix, func(entry storage.Entry) error {
		if entry.IsDir {
			return nil
		}
		relative := strings.TrimPrefix(entry.Path, prefix)
		relative = strings.TrimPrefix(relative, "/")
		path, err := safeRepositoryExtractPath(destination, relative)
		if err != nil {
			return err
		}
		data, err := disk.Get(entry.Path)
		if err != nil {
			return fmt.Errorf("download %s: %w", entry.Path, err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
		return nil
	})
}

// List returns manifest-backed backup names below the repository prefix.
func (r StorageRepository) List(ctx context.Context, prefix string) ([]string, error) {
	disk, root, err := r.bound(ctx, "")
	if err != nil {
		return nil, err
	}
	if prefix != "" {
		root = filepath.ToSlash(filepath.Join(root, prefix))
	}
	names := map[string]struct{}{}
	if err := disk.Walk(root, func(entry storage.Entry) error {
		if entry.IsDir || !strings.HasSuffix(entry.Path, "/manifest.json") {
			return nil
		}
		name := strings.TrimPrefix(entry.Path, root)
		name = strings.TrimPrefix(name, "/")
		name = strings.TrimSuffix(name, "/manifest.json")
		if name != "" {
			names[name] = struct{}{}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

// Delete removes all files belonging to one remote backup directory.
func (r StorageRepository) Delete(ctx context.Context, name string) error {
	disk, prefix, err := r.bound(ctx, name)
	if err != nil {
		return err
	}
	return disk.Walk(prefix, func(entry storage.Entry) error {
		if entry.IsDir {
			return nil
		}
		return disk.Delete(entry.Path)
	})
}

// bound validates repository state and returns a context-bound storage handle and path.
func (r StorageRepository) bound(ctx context.Context, name string) (storage.Storage, string, error) {
	if r.Disk == nil {
		return nil, "", fmt.Errorf("backup repository storage is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	prefix := strings.Trim(strings.TrimSpace(r.Prefix), "/")
	if name != "" {
		prefix = filepath.ToSlash(filepath.Join(prefix, name))
	}
	return r.Disk.WithContext(ctx), prefix, nil
}

// safeExtractPath prevents a repository object from escaping a local destination.
func safeRepositoryExtractPath(root string, name string) (string, error) {
	if strings.TrimSpace(name) == "" || filepath.IsAbs(name) {
		return "", fmt.Errorf("invalid repository object path %q", name)
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	path, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		return "", err
	}
	if path != root && !strings.HasPrefix(path, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("repository object path escapes destination: %s", name)
	}
	return path, nil
}
