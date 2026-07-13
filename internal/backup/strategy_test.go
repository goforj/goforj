package backup

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMySQLStrategyUsesPasswordEnvironmentAndWritesArtifact(t *testing.T) {
	bin := t.TempDir()
	writeFakeTool(t, bin, "mysqldump", "printf 'dump\\n'; printf '%s\\n' \"$*\" > \"$BACKUP_TEST_ARGS\"")
	writeFakeTool(t, bin, "mysql", "cat >/dev/null; printf '%s\\n' \"$*\" > \"$BACKUP_TEST_ARGS\"")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	argsFile := filepath.Join(t.TempDir(), "args")
	t.Setenv("BACKUP_TEST_ARGS", argsFile)
	artifact := filepath.Join(t.TempDir(), "dump.sql")
	strategy := mysqlStrategy{}
	connection := Connection{Database: "app", Host: "localhost", Port: "3306", Username: "user", Password: "secret"}
	if err := strategy.Backup(context.Background(), connection, artifact); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("expected native artifact")
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(args), "secret") {
		t.Fatalf("password leaked into command arguments: %s", args)
	}
	if err := strategy.Restore(context.Background(), connection, artifact); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteStrategyUsesBackupAndRestoreCommands(t *testing.T) {
	database := filepath.Join(t.TempDir(), "app.db")
	artifact := filepath.Join(t.TempDir(), "backup.db")
	target := filepath.Join(t.TempDir(), "target.db")
	db, err := sql.Open("sqlite", database)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE native_users (id INTEGER PRIMARY KEY, email TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO native_users VALUES (1, 'native@example.com')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := (sqliteStrategy{}).Backup(context.Background(), Connection{DSN: database}, artifact); err != nil {
		t.Fatal(err)
	}
	if err := (sqliteStrategy{}).Restore(context.Background(), Connection{DSN: target}, artifact); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", target)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM native_users`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("restored SQLite rows = %d, want 1", count)
	}
}

func TestPostgresStrategyUsesCustomDumpAndRestore(t *testing.T) {
	bin := t.TempDir()
	argsFile := filepath.Join(t.TempDir(), "args")
	writeFakeTool(t, bin, "pg_dump", "printf '%s\\n' \"$*\" >> \"$BACKUP_TEST_ARGS\"; touch \"$3\"")
	writeFakeTool(t, bin, "pg_restore", "printf '%s\\n' \"$*\" >> \"$BACKUP_TEST_ARGS\"")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BACKUP_TEST_ARGS", argsFile)
	connection := Connection{Database: "app", Host: "localhost", Port: "5432", Username: "user", Password: "secret"}
	artifact := filepath.Join(t.TempDir(), "backup.dump")
	if err := (postgresStrategy{}).Backup(context.Background(), connection, artifact); err != nil {
		t.Fatal(err)
	}
	if err := (postgresStrategy{}).Restore(context.Background(), connection, artifact); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(args), "secret") {
		t.Fatalf("password leaked into command arguments: %s", args)
	}
	if _, err := os.Stat(artifact); err != nil {
		t.Fatalf("expected Postgres artifact: %v", err)
	}
}

// writeFakeTool installs a deterministic native-client substitute for tests.
func writeFakeTool(t *testing.T, dir string, name string, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	content := "#!/bin/sh\nset -eu\n" + body + "\n"
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}
