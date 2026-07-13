package backup

import (
	"context"
	"testing"

	"github.com/goforj/storage"
	"github.com/goforj/storage/driver/localstorage"
)

type fakeObjectLister struct{}

// ListObjects returns deliberately unordered objects for deterministic manifest testing.
func (fakeObjectLister) ListObjects(context.Context, string) ([]ObjectInfo, error) {
	return []ObjectInfo{{Key: "z/file", Size: 2}, {Key: "a/file", Size: 1}}, nil
}

func TestStorageObjectListerBuildsInventoryFromAStorageDisk(t *testing.T) {
	disk, err := storage.Build(localstorage.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := disk.Put("uploads/avatar.txt", []byte("avatar")); err != nil {
		t.Fatal(err)
	}
	manifest, err := BuildObjectManifest(context.Background(), StorageObjectLister{Disk: disk}, "uploads")
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Objects) != 1 || manifest.Objects[0].Key != "uploads/avatar.txt" || manifest.Objects[0].Size != 6 {
		t.Fatalf("unexpected storage inventory: %#v", manifest)
	}
}

func TestObjectManifestRoundTripIsDeterministic(t *testing.T) {
	dir := t.TempDir()
	manifest, err := WriteObjectManifest(context.Background(), fakeObjectLister{}, "uploads", dir)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Objects[0].Key != "a/file" {
		t.Fatalf("objects not sorted: %#v", manifest.Objects)
	}
	got, err := ReadObjectManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Objects) != 2 || got.Objects[1].Key != "z/file" {
		t.Fatalf("unexpected object manifest: %#v", got)
	}
}
