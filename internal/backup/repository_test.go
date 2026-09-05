package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goforj/storage"
	"github.com/goforj/storage/driver/localstorage"
)

func TestStorageRepositoryRoundTripAndListing(t *testing.T) {
	disk, err := storage.Build(localstorage.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "databases"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "manifest.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "databases", "default.dump"), []byte("dump"), 0o644); err != nil {
		t.Fatal(err)
	}
	repository := StorageRepository{Disk: disk, Prefix: "backups"}
	if err := repository.Upload(context.Background(), "backup-1", source); err != nil {
		t.Fatal(err)
	}
	names, err := repository.List(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{"backup-1"}) {
		t.Fatalf("repository names = %#v", names)
	}
	destination := t.TempDir()
	if err := repository.Download(context.Background(), "backup-1", destination); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(destination, "databases", "default.dump"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "dump" {
		t.Fatalf("downloaded artifact = %q", data)
	}
	info, err := os.Stat(filepath.Join(destination, "databases", "default.dump"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("downloaded artifact mode = %o, want 600", got)
	}
	if err := repository.Delete(context.Background(), "backup-1"); err != nil {
		t.Fatal(err)
	}
	names, err = repository.List(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Fatalf("repository names after delete = %#v", names)
	}
}

// TestStorageRepositoryDownloadContainsDestinationSymlinks verifies remote object names cannot write through an escaping local link.
func TestStorageRepositoryDownloadContainsDestinationSymlinks(t *testing.T) {
	disk, err := storage.Build(localstorage.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := disk.Put("backups/backup-1/linked/escaped.txt", []byte("blocked")); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	destination := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(destination, "linked")); err != nil {
		t.Skipf("create destination symlink: %v", err)
	}
	repository := StorageRepository{Disk: disk, Prefix: "backups"}
	if err := repository.Download(context.Background(), "backup-1", destination); err == nil {
		t.Fatal("expected destination symlink rejection")
	}
	if _, err := os.Stat(filepath.Join(outside, "escaped.txt")); !os.IsNotExist(err) {
		t.Fatalf("outside file stat error = %v, want not found", err)
	}
}

// TestStorageRepositoryDownloadRejectsOversizedMetadata verifies size limits apply before object contents are allocated.
func TestStorageRepositoryDownloadRejectsOversizedMetadata(t *testing.T) {
	disk, err := storage.Build(localstorage.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	limited := repositorySizeStorage{Storage: disk, entries: []storage.Entry{{Path: "backups/backup-1/large.dump", Size: repositoryDownloadMaxBytes + 1}}}
	repository := StorageRepository{Disk: limited, Prefix: "backups"}
	if err := repository.Download(context.Background(), "backup-1", t.TempDir()); err == nil {
		t.Fatal("expected oversized repository rejection")
	}
}

// TestStorageRepositoryDownloadMetadataLimits verifies every remote metadata bound before downloads begin.
func TestStorageRepositoryDownloadMetadataLimits(t *testing.T) {
	disk, err := storage.Build(localstorage.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	tooMany := make([]storage.Entry, repositoryDownloadMaxEntries+1)
	for i := range tooMany {
		tooMany[i] = storage.Entry{Path: filepath.ToSlash(filepath.Join("backups", "backup-1", fmt.Sprintf("%d.dump", i)))}
	}
	tests := []struct {
		name    string
		entries []storage.Entry
		want    string
	}{
		{name: "negative size", entries: []storage.Entry{{Path: "backups/backup-1/negative.dump", Size: -1}}, want: "has negative size -1"},
		{name: "cumulative size", entries: []storage.Entry{{Path: "backups/backup-1/one.dump", Size: repositoryDownloadMaxBytes}, {Path: "backups/backup-1/two.dump", Size: 1}}, want: "exceeds 68719476736 bytes"},
		{name: "file count", entries: tooMany, want: "exceeds 100000 files"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := StorageRepository{Disk: repositorySizeStorage{Storage: disk, entries: test.entries}, Prefix: "backups"}
			err := repository.Download(context.Background(), "backup-1", t.TempDir())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("download error = %v, want %q", err, test.want)
			}
		})
	}
}

// TestStorageRepositoryDownloadRejectsChangedObjectSize verifies metadata cannot understate downloaded memory and disk use.
func TestStorageRepositoryDownloadRejectsChangedObjectSize(t *testing.T) {
	disk, err := storage.Build(localstorage.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	path := "backups/backup-1/changing.dump"
	repository := StorageRepository{Disk: repositorySizeStorage{
		Storage: disk,
		entries: []storage.Entry{{Path: path, Size: 1}},
		data:    map[string][]byte{path: []byte("changed")},
	}, Prefix: "backups"}
	err = repository.Download(context.Background(), "backup-1", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "object size changed from 1 to 7 bytes") {
		t.Fatalf("download error = %v, want changed object size", err)
	}
}

// TestStorageRepositoryDownloadRejectsEscapingListings verifies invalid remote keys are rejected before any object is fetched.
func TestStorageRepositoryDownloadRejectsEscapingListings(t *testing.T) {
	disk, err := storage.Build(localstorage.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{
		"/backups/backup-1/absolute.dump",
		`backups/backup-1\escaped.dump`,
		"backups/backup-1/../victim.dump",
		"backups/another-backup/victim.dump",
	}
	for _, objectPath := range paths {
		t.Run(objectPath, func(t *testing.T) {
			fetched := []string{}
			repository := StorageRepository{Disk: repositorySizeStorage{
				Storage: disk,
				entries: []storage.Entry{{Path: objectPath, Size: 1}},
				fetched: &fetched,
			}, Prefix: "backups"}
			if err := repository.Download(context.Background(), "backup-1", t.TempDir()); err == nil {
				t.Fatal("expected escaping repository listing rejection")
			}
			if len(fetched) != 0 {
				t.Fatalf("fetched objects = %#v, want none", fetched)
			}
		})
	}
}

// TestStorageRepositoryListRejectsEscapingListings verifies invalid remote keys cannot create misleading backup names.
func TestStorageRepositoryListRejectsEscapingListings(t *testing.T) {
	disk, err := storage.Build(localstorage.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	repository := StorageRepository{Disk: repositorySizeStorage{
		Storage: disk,
		entries: []storage.Entry{{Path: "backups/../victim/manifest.json", Size: 2}},
	}, Prefix: "backups"}
	if _, err := repository.List(context.Background(), ""); err == nil {
		t.Fatal("expected escaping repository listing rejection")
	}
}

// TestStorageRepositoryDeleteRejectsEscapingListings verifies untrusted object keys cannot drive deletion outside a backup prefix.
func TestStorageRepositoryDeleteRejectsEscapingListings(t *testing.T) {
	disk, err := storage.Build(localstorage.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	deleted := []string{}
	repository := StorageRepository{Disk: repositorySizeStorage{
		Storage: disk,
		entries: []storage.Entry{{Path: "backups/backup-1/../victim/manifest.json", Size: 2}},
		deleted: &deleted,
	}, Prefix: "backups"}
	if err := repository.Delete(context.Background(), "backup-1"); err == nil {
		t.Fatal("expected escaping repository listing rejection")
	}
	if len(deleted) != 0 {
		t.Fatalf("deleted objects = %#v, want none", deleted)
	}
}

// TestStorageRepositoryDeleteValidatesEveryObjectBeforeDeletion verifies a late invalid key cannot leave a partial deletion.
func TestStorageRepositoryDeleteValidatesEveryObjectBeforeDeletion(t *testing.T) {
	disk, err := storage.Build(localstorage.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	deleted := []string{}
	repository := StorageRepository{Disk: repositorySizeStorage{
		Storage: disk,
		entries: []storage.Entry{
			{Path: "backups/backup-1/manifest.json", Size: 2},
			{Path: "backups/backup-1/../victim/manifest.json", Size: 2},
		},
		deleted: &deleted,
	}, Prefix: "backups"}
	if err := repository.Delete(context.Background(), "backup-1"); err == nil {
		t.Fatal("expected escaping repository listing rejection")
	}
	if len(deleted) != 0 {
		t.Fatalf("deleted objects = %#v, want none", deleted)
	}
}

// TestStorageRepositoryDeleteRejectsInvalidObjectPaths verifies destructive calls reject every unsafe key form.
func TestStorageRepositoryDeleteRejectsInvalidObjectPaths(t *testing.T) {
	disk, err := storage.Build(localstorage.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{
		"/backups/backup-1/absolute.dump",
		`backups/backup-1\escaped.dump`,
		"backups/backup-1/../victim.dump",
		"backups/another-backup/victim.dump",
	}
	for _, objectPath := range paths {
		t.Run(objectPath, func(t *testing.T) {
			deleted := []string{}
			repository := StorageRepository{Disk: repositorySizeStorage{
				Storage: disk,
				entries: []storage.Entry{{Path: objectPath, Size: 1}},
				deleted: &deleted,
			}, Prefix: "backups"}
			if err := repository.Delete(context.Background(), "backup-1"); err == nil {
				t.Fatal("expected invalid repository path rejection")
			}
			if len(deleted) != 0 {
				t.Fatalf("deleted objects = %#v, want none", deleted)
			}
		})
	}
}

// TestStorageRepositoryRejectsUnsafeNames verifies callers cannot escape a configured repository prefix.
func TestStorageRepositoryRejectsUnsafeNames(t *testing.T) {
	disk, err := storage.Build(localstorage.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	repository := StorageRepository{Disk: disk, Prefix: "backups"}
	for _, name := range []string{"../victim", "nested/victim", `nested\victim`} {
		if err := repository.Delete(context.Background(), name); err == nil {
			t.Fatalf("delete name %q succeeded, want rejection", name)
		}
	}
}

// TestStorageRepositoryRejectsUnsafePrefixes verifies configured storage roots cannot traverse logical key boundaries.
func TestStorageRepositoryRejectsUnsafePrefixes(t *testing.T) {
	disk, err := storage.Build(localstorage.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{".", "../backups", "nested/../backups", `nested\backups`} {
		repository := StorageRepository{Disk: disk, Prefix: prefix}
		if _, err := repository.List(context.Background(), ""); err == nil {
			t.Fatalf("repository prefix %q succeeded, want rejection", prefix)
		}
	}
}

// repositorySizeStorage provides controlled metadata while preserving the rest of a storage implementation.
type repositorySizeStorage struct {
	storage.Storage
	entries []storage.Entry
	data    map[string][]byte
	deleted *[]string
	fetched *[]string
}

// WithContext preserves controlled metadata while forwarding context binding to the real storage implementation.
func (s repositorySizeStorage) WithContext(ctx context.Context) storage.Storage {
	s.Storage = s.Storage.WithContext(ctx)
	return s
}

// Walk reports controlled entries used by repository limit tests.
func (s repositorySizeStorage) Walk(_ string, fn func(storage.Entry) error) error {
	for _, entry := range s.entries {
		if err := fn(entry); err != nil {
			return err
		}
	}
	return nil
}

// Get returns controlled object data when a test needs metadata and contents to diverge.
func (s repositorySizeStorage) Get(path string) ([]byte, error) {
	if s.fetched != nil {
		*s.fetched = append(*s.fetched, path)
	}
	if data, ok := s.data[path]; ok {
		return data, nil
	}
	return s.Storage.Get(path)
}

// Delete records attempted deletions so traversal tests can prove no destructive call was made.
func (s repositorySizeStorage) Delete(path string) error {
	if s.deleted != nil {
		*s.deleted = append(*s.deleted, path)
		return nil
	}
	return s.Storage.Delete(path)
}
