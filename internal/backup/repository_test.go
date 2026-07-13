package backup

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
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
