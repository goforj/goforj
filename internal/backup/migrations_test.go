package backup

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestProjectMigrationFingerprintUsesOrderedUpMigrations(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "migrations"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"002_events.up.sql", "001_users.up.sql", "002_events.down.sql", "README.md"} {
		if err := os.WriteFile(filepath.Join(root, "migrations", name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	names, err := MigrationNames(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{"001_users", "002_events"}) {
		t.Fatalf("migration names = %#v", names)
	}
	fingerprint, err := ProjectMigrationFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	if fingerprint != MigrationFingerprint(names) {
		t.Fatalf("fingerprint = %q", fingerprint)
	}
}
