package backup

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goforj/storage"
)

// repositoryDownloadMaxEntries prevents a remote prefix from creating an unbounded number of local files.
const repositoryDownloadMaxEntries = 100_000

// repositoryDownloadMaxBytes bounds local disk consumption before any remote object is materialized.
const repositoryDownloadMaxBytes int64 = 64 << 30

// BackupRepository stores complete manifest-backed backup directories.
type BackupRepository interface {
	// Upload defines the upload behavior required from implementations.
	Upload(context.Context, string, string) error
	// Download defines the download behavior required from implementations.
	Download(context.Context, string, string) error
	// List defines the list behavior required from implementations.
	List(context.Context, string) ([]string, error)
	// Delete defines the delete behavior required from implementations.
	Delete(context.Context, string) error
}

// StorageRepository stores backup files in a GoForj storage backend.
type StorageRepository struct {
	Disk   storage.Storage
	Prefix string
}

// repositoryBinding keeps the context-bound disk and its resolved path together.
type repositoryBinding struct {
	disk   storage.Storage
	prefix string
}

// Upload copies a completed local backup directory into the storage backend.
func (r StorageRepository) Upload(ctx context.Context, name string, source string) error {
	binding, err := r.bound(ctx, name)
	if err != nil {
		return err
	}
	sourceRoot, err := os.OpenRoot(source)
	if err != nil {
		return fmt.Errorf("open backup repository source: %w", err)
	}
	defer sourceRoot.Close()
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
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		data, err := sourceRoot.ReadFile(relative)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(filepath.Join(binding.prefix, filepath.ToSlash(relative)))
		if err := binding.disk.Put(key, data); err != nil {
			return fmt.Errorf("upload %s: %w", key, err)
		}
		return nil
	})
}

// Download copies a remote backup directory into a local destination.
func (r StorageRepository) Download(ctx context.Context, name string, destination string) error {
	binding, err := r.bound(ctx, name)
	if err != nil {
		return err
	}
	if err := ensurePrivateDirectory(destination); err != nil {
		return err
	}
	destinationRoot, err := os.OpenRoot(destination)
	if err != nil {
		return fmt.Errorf("open backup repository destination: %w", err)
	}
	defer destinationRoot.Close()
	entries := 0
	var downloadedBytes int64
	objects := make([]storage.Entry, 0)
	if err := binding.disk.Walk(binding.prefix, func(entry storage.Entry) error {
		if entry.IsDir {
			return nil
		}
		relative, err := repositoryRelativePath(binding.prefix, entry.Path)
		if err != nil {
			return err
		}
		entries++
		if entries > repositoryDownloadMaxEntries {
			return fmt.Errorf("backup repository exceeds %d files", repositoryDownloadMaxEntries)
		}
		if entry.Size < 0 {
			return fmt.Errorf("backup repository object %s has negative size %d", entry.Path, entry.Size)
		}
		if entry.Size > repositoryDownloadMaxBytes-downloadedBytes {
			return fmt.Errorf("backup repository exceeds %d bytes", repositoryDownloadMaxBytes)
		}
		downloadedBytes += entry.Size
		if _, err := safeRepositoryExtractPath(destination, relative); err != nil {
			return err
		}
		objects = append(objects, entry)
		return nil
	}); err != nil {
		return err
	}
	for _, entry := range objects {
		relative, err := repositoryRelativePath(binding.prefix, entry.Path)
		if err != nil {
			return err
		}
		path, err := safeRepositoryExtractPath(destination, relative)
		if err != nil {
			return err
		}
		relativePath, err := filepath.Rel(destination, path)
		if err != nil {
			return err
		}
		data, err := binding.disk.Get(entry.Path)
		if err != nil {
			return fmt.Errorf("download %s: %w", entry.Path, err)
		}
		if int64(len(data)) != entry.Size {
			return fmt.Errorf("download %s: object size changed from %d to %d bytes", entry.Path, entry.Size, len(data))
		}
		if err := ensurePrivateRootDirectory(destinationRoot, filepath.Dir(relativePath)); err != nil {
			return err
		}
		if err := writePrivateRootFile(destinationRoot, relativePath, data); err != nil {
			return err
		}
	}
	return nil
}

// List returns manifest-backed backup names below the repository prefix.
func (r StorageRepository) List(ctx context.Context, prefix string) ([]string, error) {
	binding, err := r.bound(ctx, "")
	if err != nil {
		return nil, err
	}
	root := binding.prefix
	if prefix != "" {
		prefix, err = safeRepositoryName(prefix)
		if err != nil {
			return nil, err
		}
		root = path.Join(root, prefix)
	}
	names := map[string]struct{}{}
	if err := binding.disk.Walk(root, func(entry storage.Entry) error {
		if entry.IsDir {
			return nil
		}
		relative, err := repositoryRelativePath(root, entry.Path)
		if err != nil {
			return err
		}
		if !strings.HasSuffix(relative, "/manifest.json") {
			return nil
		}
		name, err := safeRepositoryName(strings.TrimSuffix(relative, "/manifest.json"))
		if err != nil {
			return err
		}
		names[name] = struct{}{}
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
	binding, err := r.bound(ctx, name)
	if err != nil {
		return err
	}
	objects := []string{}
	if err := binding.disk.Walk(binding.prefix, func(entry storage.Entry) error {
		if entry.IsDir {
			return nil
		}
		if _, err := repositoryRelativePath(binding.prefix, entry.Path); err != nil {
			return err
		}
		objects = append(objects, entry.Path)
		return nil
	}); err != nil {
		return err
	}
	for _, object := range objects {
		if err := binding.disk.Delete(object); err != nil {
			return err
		}
	}
	return nil
}

// bound validates repository state and returns a context-bound storage handle and path.
func (r StorageRepository) bound(ctx context.Context, name string) (repositoryBinding, error) {
	if r.Disk == nil {
		return repositoryBinding{}, fmt.Errorf("backup repository storage is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	prefix, err := safeRepositoryPrefix(r.Prefix)
	if err != nil {
		return repositoryBinding{}, err
	}
	if name != "" {
		name, err = safeRepositoryName(name)
		if err != nil {
			return repositoryBinding{}, err
		}
		prefix = path.Join(prefix, name)
	}
	return repositoryBinding{disk: r.Disk.WithContext(ctx), prefix: prefix}, nil
}

// safeRepositoryPrefix normalizes a configured logical key prefix without allowing parent traversal.
func safeRepositoryPrefix(prefix string) (string, error) {
	prefix = strings.TrimSpace(prefix)
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		return "", nil
	}
	if strings.Contains(prefix, "\\") || path.Clean(prefix) != prefix || prefix == "." || strings.HasPrefix(prefix, "../") {
		return "", fmt.Errorf("invalid backup repository prefix %q", prefix)
	}
	return prefix, nil
}

// safeRepositoryName limits backup names to one logical key segment before destructive operations.
func safeRepositoryName(name string) (string, error) {
	if name == "" || strings.TrimSpace(name) != name || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		return "", fmt.Errorf("invalid backup repository name %q", name)
	}
	return name, nil
}

// repositoryRelativePath requires storage listings to stay below the exact requested logical prefix.
func repositoryRelativePath(prefix string, entryPath string) (string, error) {
	if entryPath == "" || strings.Contains(entryPath, "\\") || path.IsAbs(entryPath) || path.Clean(entryPath) != entryPath {
		return "", fmt.Errorf("invalid backup repository object path %q", entryPath)
	}
	relative := entryPath
	if prefix != "" {
		if !strings.HasPrefix(entryPath, prefix+"/") {
			return "", fmt.Errorf("backup repository object %q escapes prefix %q", entryPath, prefix)
		}
		relative = strings.TrimPrefix(entryPath, prefix+"/")
	}
	if relative == "" || relative == "." || relative == ".." || strings.HasPrefix(relative, "../") {
		return "", fmt.Errorf("invalid backup repository object path %q", entryPath)
	}
	return relative, nil
}

// safeRepositoryExtractPath prevents a repository object from escaping a local destination.
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
