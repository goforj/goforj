package backup

import (
	"context"
	"database/sql"
	"fmt"
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

// TestPortableSQLRoundTripsLargeTable verifies complete large-table export and restore.
func TestPortableSQLRoundTripsLargeTable(t *testing.T) {
	const rowCount = 20000
	ctx := context.Background()
	source := openPortableSQLite(t)
	target := openPortableSQLite(t)
	defer source.Close()
	defer target.Close()
	ddl := `CREATE TABLE large_users (id INTEGER PRIMARY KEY, value TEXT NOT NULL)`
	if _, err := source.Exec(ddl); err != nil {
		t.Fatal(err)
	}
	if _, err := target.Exec(ddl); err != nil {
		t.Fatal(err)
	}
	insertLargePortableRows(t, source, rowCount)
	dialect, err := NewSQLDialect("sqlite")
	if err != nil {
		t.Fatal(err)
	}
	archive, err := ExportPortable(ctx, source, dialect, []string{"large_users"})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(archive.Tables[0].Rows); got != rowCount {
		t.Fatalf("exported rows = %d, want %d", got, rowCount)
	}
	tx, err := target.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ImportPortable(ctx, tx, dialect, archive); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := target.QueryRow(`SELECT COUNT(*) FROM large_users`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != rowCount {
		t.Fatalf("restored rows = %d, want %d", count, rowCount)
	}
	for _, id := range []int{1, rowCount} {
		var value string
		if err := target.QueryRow(`SELECT value FROM large_users WHERE id = ?`, id).Scan(&value); err != nil {
			t.Fatal(err)
		}
		if want := fmt.Sprintf("user-%05d", id); value != want {
			t.Fatalf("row %d value = %q, want %q", id, value, want)
		}
	}
}

// insertLargePortableRows creates enough data to exercise complete large-table export and restore.
func insertLargePortableRows(t *testing.T, db *sql.DB, count int) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	stmt, err := tx.Prepare(`INSERT INTO large_users (id, value) VALUES (?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	defer stmt.Close()
	for id := 1; id <= count; id++ {
		if _, err := stmt.Exec(id, fmt.Sprintf("user-%05d", id)); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
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
