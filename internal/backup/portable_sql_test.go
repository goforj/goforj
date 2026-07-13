package backup

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/glebarez/go-sqlite"
)

func TestPortableSQLExportImportSQLite(t *testing.T) {
	source := openPortableSQLite(t)
	target := openPortableSQLite(t)
	ddl := `CREATE TABLE portable_users (
                id INTEGER PRIMARY KEY,
                active BOOLEAN NOT NULL,
                amount NUMERIC NOT NULL,
                payload JSON,
                blob BLOB
        )`
	if _, err := source.Exec(ddl); err != nil {
		t.Fatal(err)
	}
	if _, err := target.Exec(ddl); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Exec(`INSERT INTO portable_users (id, active, amount, payload, blob) VALUES (?, ?, ?, ?, ?)`, 1, true, "12.3400", `{"ok":true}`, []byte{0, 1, 255}); err != nil {
		t.Fatal(err)
	}

	dialect, err := NewSQLDialect("sqlite")
	if err != nil {
		t.Fatal(err)
	}
	archive, err := ExportPortable(context.Background(), source, dialect, []string{"portable_users"})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := target.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ImportPortable(context.Background(), tx, dialect, archive); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := target.QueryRow(`SELECT COUNT(*) FROM portable_users`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("row count = %d, want 1", count)
	}
}

// openPortableSQLite opens an in-memory SQLite database using database/sql directly.
func openPortableSQLite(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	return db
}
