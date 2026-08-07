package backup

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/glebarez/go-sqlite"
)

func TestPortableCreateAndRestoreCommandsSQLite(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source.db")
	targetPath := filepath.Join(t.TempDir(), "target.db")
	seedBackupSQLite(t, sourcePath)
	seedBackupSQLiteSchema(t, targetPath)
	backupRoot := t.TempDir()
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_SQLITE_DATABASE", sourcePath)
	create := &CreateCmd{Path: backupRoot, Portable: true}
	if err := create.Run(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("portable backup directories = %d, want 1", len(entries))
	}
	backupDir := filepath.Join(backupRoot, entries[0].Name())
	t.Setenv("DB_SQLITE_DATABASE", targetPath)
	restore := &RestoreCmd{From: backupDir, Portable: true, TargetDriver: "sqlite", Confirm: "restore-production"}
	if err := restore.Run(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", targetPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM portable_users`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("restored rows = %d, want 1", count)
	}
}

func TestLocalStorageBackupAndRestore(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "avatar.txt"), []byte("avatar"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	t.Setenv("STORAGE_DRIVER", "local")
	t.Setenv("STORAGE_ROOT", source)
	backupRoot := t.TempDir()
	backup, err := NewService().Create(context.Background(), backupRoot, "storage.default")
	if err != nil {
		t.Fatal(err)
	}
	if len(backup.Manifest.Resources) != 1 || backup.Manifest.Resources[0].Kind != "storage" {
		t.Fatalf("unexpected storage manifest: %#v", backup.Manifest)
	}
	t.Setenv("STORAGE_ROOT", target)
	if err := NewService().Restore(context.Background(), backup.Directory, "storage.default", "restore-production"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "avatar.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "avatar" {
		t.Fatalf("restored storage data = %q", data)
	}
}

// seedBackupSQLite centralizes seed backup sqlite behavior so callers follow the same contract.
func seedBackupSQLite(t *testing.T, path string) {
	t.Helper()
	seedBackupSQLiteSchema(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO portable_users (active, amount, payload, blob_data) VALUES (TRUE, 12.3400, '{"ok":true}', X'0001FF')`); err != nil {
		t.Fatal(err)
	}
}

// seedBackupSQLiteSchema centralizes seed backup sqlite schema behavior so callers follow the same contract.
func seedBackupSQLiteSchema(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE portable_users (id INTEGER PRIMARY KEY AUTOINCREMENT, active BOOLEAN NOT NULL, amount NUMERIC NOT NULL, payload JSON, blob_data BLOB)`); err != nil {
		t.Fatal(err)
	}
}
