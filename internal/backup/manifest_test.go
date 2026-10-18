package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestWriteManifestTightensExistingPermissions verifies overwrites do not retain permissive legacy modes.
func TestWriteManifestTightensExistingPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix permission bits")
	}
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte("legacy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteManifest(dir, Manifest{Resources: []Resource{{ID: "db.default", Artifact: "artifact"}}}); err != nil {
		t.Fatal(err)
	}
	directoryInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := directoryInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("backup directory mode = %o, want 700", got)
	}
	fileInfo, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("manifest mode = %o, want 600", got)
	}
}

func TestManifestRoundTripAndChecksum(t *testing.T) {
	dir := t.TempDir()
	artifact := filepath.Join(dir, "databases", "default.sql")
	if err := os.MkdirAll(filepath.Dir(artifact), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifact, []byte("portable test data\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := Checksum(artifact)
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{Resources: []Resource{{
		ID: "db.default", Kind: "database", Name: "default", Driver: "sqlite",
		Strategy: "sqlite3-backup", Artifact: "databases/default.sql", Checksum: fingerprint.Checksum, Size: fingerprint.Size,
	}}}
	if err := WriteManifest(dir, manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "checksums.txt")); err != nil {
		t.Fatal(err)
	}
	got, err := ReadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || len(got.Resources) != 1 || got.Resources[0].Checksum != fingerprint.Checksum {
		t.Fatalf("unexpected manifest: %#v", got)
	}
	if err := VerifyChecksum(artifact, fingerprint.Checksum); err != nil {
		t.Fatal(err)
	}
}

func TestListAndPruneKeepNewestManifestBackups(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 3; i++ {
		dir := filepath.Join(root, fmt.Sprintf("backup-%d", i))
		if err := WriteManifest(dir, Manifest{CreatedAt: time.Unix(int64(i), 0), Resources: []Resource{{ID: "db.default", Artifact: "artifact"}}}); err != nil {
			t.Fatal(err)
		}
	}
	manifests, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) != 3 || !manifests[0].CreatedAt.Equal(time.Unix(2, 0)) {
		t.Fatalf("unexpected list: %#v", manifests)
	}
	removed, err := Prune(root, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 {
		t.Fatalf("removed %d backups, want 2", len(removed))
	}
}

func TestReadManifestRejectsArtifactPathEscape(t *testing.T) {
	dir := t.TempDir()
	if err := WriteManifest(dir, Manifest{Resources: []Resource{{ID: "db.default", Artifact: "../secret"}}}); err == nil {
		t.Fatal("expected manifest writer to reject path escape")
	}
}

func TestNativeStrategyNormalizesDrivers(t *testing.T) {
	for _, driver := range []string{"sqlite", "sqlite3", "mysql", "mariadb", "postgres", "postgresql", "pgx"} {
		if _, err := NativeStrategy(driver); err != nil {
			t.Fatalf("driver %q: %v", driver, err)
		}
	}
	if _, err := NativeStrategy("oracle"); err == nil {
		t.Fatal("expected unsupported driver error")
	}
}
