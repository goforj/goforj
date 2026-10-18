package backup

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// TestSelectedBackupSetRoundTrip proves multiple named databases and storage can be captured without including unselected resources.
func TestSelectedBackupSetRoundTrip(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("FORJ_APP", "app")
	if err := os.MkdirAll("bin", 0700); err != nil {
		t.Fatal(err)
	}
	contract := `#!/bin/sh
printf '%s\n' '{"version":1,"app":"app","resources":[{"id":"db.default","kind":"database","name":"default","driver":"sqlite"},{"id":"db.reporting","kind":"database","name":"reporting","driver":"sqlite"},{"id":"storage.private","kind":"storage","name":"private","driver":"local"}]}'
`
	if err := os.WriteFile("bin/app", []byte(contract), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"default", "reporting"} {
		file := filepath.Join(root, name+".sqlite")
		db, err := sql.Open("sqlite", file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("CREATE TABLE evidence (value TEXT); INSERT INTO evidence VALUES ('" + name + "')"); err != nil {
			t.Fatal(err)
		}
		db.Close()
		prefix := "DB"
		if name != "default" {
			prefix = "DB_REPORTING"
		}
		t.Setenv(prefix+"_DRIVER", "sqlite")
		t.Setenv(prefix+"_DSN", file)
	}
	storage := filepath.Join("storage", "uploads")
	os.MkdirAll(filepath.Join(storage, "scope"), 0700)
	os.WriteFile(filepath.Join(storage, "scope", "note.txt"), []byte("retained upload"), 0600)
	t.Setenv("STORAGE_PRIVATE_DRIVER", "local")
	t.Setenv("STORAGE_PRIVATE_ROOT", storage)
	t.Setenv("STORAGE_PRIVATE_PREFIX", "scope")
	service := NewService()
	backup, err := service.CreateSelected(context.Background(), filepath.Join(root, "backups"), CreateOptions{Resources: []string{"db.reporting", "storage.private"}, MaxBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if backup.Manifest.Version != 2 {
		t.Fatalf("scoped backup version = %d, want 2", backup.Manifest.Version)
	}
	if len(backup.Manifest.Resources) != 2 {
		t.Fatal("selection lost or included a resource")
	}
	if _, err = service.Verify(backup.Directory); err != nil {
		t.Fatal(err)
	}
	for _, resource := range backup.Manifest.Resources {
		if resource.ID == "db.default" {
			t.Fatal("unselected database included")
		}
	}
	targetDB := filepath.Join(root, "restored.sqlite")
	targetStorage := filepath.Join("storage", "restored-uploads")
	t.Setenv("DB_REPORTING_DSN", targetDB)
	t.Setenv("STORAGE_PRIVATE_ROOT", targetStorage)
	t.Setenv("STORAGE_PRIVATE_PREFIX", "restored-scope")
	if err = service.Restore(context.Background(), backup.Directory, "", "restore-production"); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", targetDB)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var value string
	if err = db.QueryRow("SELECT value FROM evidence").Scan(&value); err != nil || value != "reporting" {
		t.Fatal("named database restore failed", err, value)
	}
	note, err := os.ReadFile(filepath.Join(targetStorage, "restored-scope", "note.txt"))
	if err != nil || string(note) != "retained upload" {
		t.Fatal("storage restore failed", err)
	}
}
