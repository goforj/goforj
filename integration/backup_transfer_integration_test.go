//go:build integration_backup

package integration_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	_ "github.com/glebarez/go-sqlite"
	_ "github.com/go-sql-driver/mysql"
	"github.com/goforj/goforj/internal/backup"
	_ "github.com/jackc/pgx/v5/stdlib"
	testcontainers "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestPortableTransferMatrix(t *testing.T) {
	if os.Getenv("GOFORJ_BACKUP_INTEGRATION") != "1" {
		t.Skip("set GOFORJ_BACKUP_INTEGRATION=1 to run database transfer integration tests")
	}
	ctx := context.Background()
	databases := map[string]*sql.DB{
		"sqlite": openBackupSQLite(t),
	}
	defer databases["sqlite"].Close()
	mysql, _, stopMySQL := startBackupMySQL(t, ctx)
	defer stopMySQL()
	databases["mysql"] = openBackupDatabase(t, "mysql", mysql)
	defer databases["mysql"].Close()
	postgres, _, stopPostgres := startBackupPostgres(t, ctx)
	defer stopPostgres()
	databases["postgres"] = openBackupDatabase(t, "postgres", postgres)
	defer databases["postgres"].Close()

	drivers := []string{"sqlite", "mysql", "postgres"}
	for _, sourceDriver := range drivers {
		for _, targetDriver := range drivers {
			t.Run(sourceDriver+"_to_"+targetDriver, func(t *testing.T) {
				source := databases[sourceDriver]
				target := databases[targetDriver]
				resetBackupFixture(t, sourceDriver, source, true)
				sourceDialect, err := backup.NewSQLDialect(sourceDriver)
				if err != nil {
					t.Fatal(err)
				}
				targetDialect, err := backup.NewSQLDialect(targetDriver)
				if err != nil {
					t.Fatal(err)
				}
				archive, err := backup.ExportPortable(ctx, source, sourceDialect, []string{"portable_users"})
				if err != nil {
					t.Fatal(err)
				}
				resetBackupFixture(t, targetDriver, target, false)
				tx, err := target.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				if err := backup.ImportPortable(ctx, tx, targetDialect, archive); err != nil {
					_ = tx.Rollback()
					t.Fatal(err)
				}
				if err := tx.Commit(); err != nil {
					t.Fatal(err)
				}
				got, err := backup.ExportPortable(ctx, target, targetDialect, []string{"portable_users"})
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(canonicalRows(archive.Tables[0].Rows), canonicalRows(got.Tables[0].Rows)) {
					t.Fatalf("transferred rows differ\nsource=%v\ntarget=%v", archive.Tables[0].Rows, got.Tables[0].Rows)
				}
				assertBackupIdentityContinues(t, targetDriver, target)
			})
		}
	}
	for _, sourceDriver := range drivers {
		for _, targetDriver := range drivers {
			t.Run("large_"+sourceDriver+"_to_"+targetDriver, func(t *testing.T) {
				source := databases[sourceDriver]
				target := databases[targetDriver]
				resetLargeBackupFixture(t, sourceDriver, source, true)
				sourceDialect, err := backup.NewSQLDialect(sourceDriver)
				if err != nil {
					t.Fatal(err)
				}
				archive, err := backup.ExportPortable(ctx, source, sourceDialect, []string{"large_users"})
				if err != nil {
					t.Fatal(err)
				}
				resetLargeBackupFixture(t, targetDriver, target, false)
				targetDialect, err := backup.NewSQLDialect(targetDriver)
				if err != nil {
					t.Fatal(err)
				}
				tx, err := target.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				if err := backup.ImportPortable(ctx, tx, targetDialect, archive); err != nil {
					_ = tx.Rollback()
					t.Fatal(err)
				}
				if err := tx.Commit(); err != nil {
					t.Fatal(err)
				}
				if got := len(archive.Tables[0].Rows); got != 5000 {
					t.Fatalf("transferred large rows = %d, want 5000", got)
				}
				assertLargeBackupRows(t, target, targetDriver)
			})
		}
	}
	chains := [][]string{{"mysql", "sqlite", "postgres"}, {"postgres", "mysql", "sqlite"}, {"sqlite", "postgres", "mysql"}}
	for _, chain := range chains {
		t.Run(strings.Join(chain, "_to_"), func(t *testing.T) {
			resetBackupFixture(t, chain[0], databases[chain[0]], true)
			resetBackupFixture(t, chain[1], databases[chain[1]], false)
			first := transferBackupArchive(t, ctx, databases[chain[0]], chain[0], databases[chain[1]], chain[1])
			resetBackupFixture(t, chain[2], databases[chain[2]], false)
			second := transferBackupArchive(t, ctx, databases[chain[1]], chain[1], databases[chain[2]], chain[2])
			if !reflect.DeepEqual(canonicalRows(first.Tables[0].Rows), canonicalRows(second.Tables[0].Rows)) {
				t.Fatalf("chained transfer changed rows\nfirst=%v\nsecond=%v", first.Tables[0].Rows, second.Tables[0].Rows)
			}
		})
	}
}

func TestPortableCompatibilityFailures(t *testing.T) {
	archive := backup.PortableArchive{Version: 1, Tables: []backup.PortableTable{{
		Name:    "portable_users",
		Columns: []backup.ColumnSpec{{Name: "id", Type: "integer"}, {Name: "amount", Type: "decimal"}},
	}}}
	if err := backup.ValidateSchemaCompatibility(archive, nil); err == nil {
		t.Fatal("expected missing target table failure")
	}
	if err := backup.ValidateSchemaCompatibility(archive, []backup.PortableTable{{
		Name:    "portable_users",
		Columns: []backup.ColumnSpec{{Name: "id", Type: "integer"}, {Name: "amount", Type: "json"}},
	}}); err == nil {
		t.Fatal("expected incompatible target type failure")
	}
}

func TestNativeMySQLBackupRestore(t *testing.T) {
	if os.Getenv("GOFORJ_BACKUP_INTEGRATION") != "1" {
		t.Skip("set GOFORJ_BACKUP_INTEGRATION=1 to run database backup integration tests")
	}
	ctx := context.Background()
	dsn, connection, stop := startBackupMySQL(t, ctx)
	defer stop()
	db := openBackupDatabase(t, "mysql", dsn)
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE native_backup_users (id INT PRIMARY KEY, email VARCHAR(255) NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO native_backup_users (id, email) VALUES (1, 'native@example.com')`); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(t.TempDir(), "native.sql")
	strategy, err := backup.NativeStrategy("mysql")
	if err != nil {
		t.Fatal(err)
	}
	if err := strategy.Backup(ctx, connection, artifact); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE native_backup_users`); err != nil {
		t.Fatal(err)
	}
	if err := strategy.Restore(ctx, connection, artifact); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM native_backup_users`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("restored row count = %d, want 1", count)
	}
}

func TestNativeSQLiteBackupRestore(t *testing.T) {
	if os.Getenv("GOFORJ_BACKUP_INTEGRATION") != "1" {
		t.Skip("set GOFORJ_BACKUP_INTEGRATION=1 to run database backup integration tests")
	}
	ctx := context.Background()
	sourcePath := filepath.Join(t.TempDir(), "source.db")
	targetPath := filepath.Join(t.TempDir(), "target.db")
	db, err := sql.Open("sqlite", sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE native_backup_users (id INTEGER PRIMARY KEY, email TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO native_backup_users VALUES (1, 'native@example.com')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	strategy, err := backup.NativeStrategy("sqlite")
	if err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(t.TempDir(), "native.db")
	connection := backup.Connection{Driver: "sqlite", DSN: sourcePath}
	if err := strategy.Backup(ctx, connection, artifact); err != nil {
		t.Fatal(err)
	}
	if err := strategy.Restore(ctx, backup.Connection{Driver: "sqlite", DSN: targetPath}, artifact); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", targetPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM native_backup_users`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("restored SQLite rows = %d, want 1", count)
	}
}

func TestNativePostgresBackupRestore(t *testing.T) {
	if os.Getenv("GOFORJ_BACKUP_NATIVE_POSTGRES") != "1" {
		t.Skip("set GOFORJ_BACKUP_NATIVE_POSTGRES=1 to run isolated native Postgres recovery")
	}
	if os.Getenv("GOFORJ_BACKUP_INTEGRATION") != "1" {
		t.Skip("set GOFORJ_BACKUP_INTEGRATION=1 to run database backup integration tests")
	}
	ctx := context.Background()
	dsn, container, stop := startBackupPostgres(t, ctx)
	defer stop()
	db := openBackupDatabase(t, "postgres", dsn)
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE native_backup_users (id INTEGER PRIMARY KEY, email TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO native_backup_users VALUES (1, 'native@example.com')`); err != nil {
		t.Fatal(err)
	}
	code, output, err := container.Exec(ctx, []string{"sh", "-c", "PGPASSWORD=secret pg_dump --format=custom --file=/tmp/native.dump --username=app app && test -s /tmp/native.dump"})
	outputBytes, readErr := io.ReadAll(output)
	if err != nil || readErr != nil || code != 0 {
		t.Fatalf("pg_dump exit=%d err=%v read_err=%v output=%s", code, err, readErr, outputBytes)
	}
	if _, err := db.Exec(`DROP TABLE native_backup_users`); err != nil {
		t.Fatal(err)
	}
	code, output, err = container.Exec(ctx, []string{"sh", "-c", "PGPASSWORD=secret pg_restore --clean --if-exists --exit-on-error --no-owner --username app --dbname app /tmp/native.dump 2>&1"})
	outputBytes, readErr = io.ReadAll(output)
	if err != nil || readErr != nil || code != 0 {
		t.Fatalf("pg_restore exit=%d err=%v read_err=%v output=%s", code, err, readErr, outputBytes)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM native_backup_users`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("restored Postgres rows = %d, want 1", count)
	}
}

// transferBackupArchive moves one portable archive between SQL participants.
func transferBackupArchive(t *testing.T, ctx context.Context, source *sql.DB, sourceDriver string, target *sql.DB, targetDriver string) backup.PortableArchive {
	return transferBackupArchiveByTable(t, ctx, source, sourceDriver, target, targetDriver, "portable_users")
}

// transferBackupArchiveByTable exports and imports one named table between SQL participants.
func transferBackupArchiveByTable(t *testing.T, ctx context.Context, source *sql.DB, sourceDriver string, target *sql.DB, targetDriver string, table string) backup.PortableArchive {
	t.Helper()
	sourceDialect, err := backup.NewSQLDialect(sourceDriver)
	if err != nil {
		t.Fatal(err)
	}
	targetDialect, err := backup.NewSQLDialect(targetDriver)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := backup.ExportPortable(ctx, source, sourceDialect, []string{table})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := target.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := backup.ImportPortable(ctx, tx, targetDialect, archive); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	got, err := backup.ExportPortable(ctx, target, targetDialect, []string{table})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// resetLargeBackupFixture recreates and optionally seeds a 5,000-row table for every SQL dialect.
func resetLargeBackupFixture(t *testing.T, driver string, db *sql.DB, withData bool) {
	t.Helper()
	if _, err := db.Exec(`DROP TABLE IF EXISTS large_users`); err != nil {
		t.Fatal(err)
	}
	idType := "INTEGER PRIMARY KEY"
	if driver == "postgres" {
		idType = "BIGSERIAL PRIMARY KEY"
	}
	if driver == "mysql" {
		idType = "BIGINT AUTO_INCREMENT PRIMARY KEY"
	}
	if _, err := db.Exec(fmt.Sprintf(`CREATE TABLE large_users (id %s, value TEXT NOT NULL)`, idType)); err != nil {
		t.Fatal(err)
	}
	if !withData {
		return
	}
	placeholder := "?"
	if driver == "postgres" {
		placeholder = "$1, $2"
	} else {
		placeholder += ", ?"
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	stmt, err := tx.Prepare("INSERT INTO large_users (id, value) VALUES (" + placeholder + ")")
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	defer stmt.Close()
	for id := 1; id <= 5000; id++ {
		if _, err := stmt.Exec(id, fmt.Sprintf("large-user-%05d", id)); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

// assertLargeBackupRows verifies count and boundary values after a cross-database transfer.
func assertLargeBackupRows(t *testing.T, db *sql.DB, driver string) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM large_users`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 5000 {
		t.Fatalf("large table rows = %d, want 5000", count)
	}
	for _, id := range []int{1, 5000} {
		var value string
		query := `SELECT value FROM large_users WHERE id = ?`
		if driver == "postgres" {
			query = `SELECT value FROM large_users WHERE id = $1`
		}
		if err := db.QueryRow(query, id).Scan(&value); err != nil {
			t.Fatal(err)
		}
		if want := fmt.Sprintf("large-user-%05d", id); value != want {
			t.Fatalf("large row %d value = %q, want %q", id, value, want)
		}
	}
}

// resetBackupFixture recreates a dialect-specific compatibility table and optionally seeds one row.
func resetBackupFixture(t *testing.T, driver string, db *sql.DB, withData bool) {
	t.Helper()
	if _, err := db.Exec(`DROP TABLE IF EXISTS portable_users`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(backupFixtureDDL(driver)); err != nil {
		t.Fatal(err)
	}
	if withData {
		if _, err := db.Exec(backupFixtureInsert(driver)); err != nil {
			t.Fatal(err)
		}
	}
}

// backupFixtureDDL returns equivalent compatibility schema for one SQL dialect.
func backupFixtureDDL(driver string) string {
	jsonType, bytesType, idType := "JSON", "BLOB", "INTEGER PRIMARY KEY AUTOINCREMENT"
	if driver == "postgres" {
		jsonType, bytesType, idType = "JSONB", "BYTEA", "BIGSERIAL PRIMARY KEY"
	}
	if driver == "mysql" {
		jsonType, bytesType, idType = "JSON", "BLOB", "BIGINT AUTO_INCREMENT PRIMARY KEY"
	}
	return fmt.Sprintf(`CREATE TABLE portable_users (
				id %s,
                active BOOLEAN NOT NULL,
                amount NUMERIC(20,4) NOT NULL,
                payload %s,
				blob_data %s
		)`, idType, jsonType, bytesType)
}

// assertBackupIdentityContinues proves a restored generated key advances past imported rows.
func assertBackupIdentityContinues(t *testing.T, driver string, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(backupFixtureSecondInsert(driver)); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := db.QueryRow(`SELECT MAX(id) FROM portable_users`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if id != 2 {
		t.Fatalf("next generated identity = %d, want 2", id)
	}
}

// backupFixtureSecondInsert returns a row insert that relies on generated identity state.
func backupFixtureSecondInsert(driver string) string {
	if driver == "postgres" {
		return `INSERT INTO portable_users (active, amount, payload, blob_data) VALUES (TRUE, 7.5000, '{"ok":false}'::jsonb, decode('0a0b', 'hex'))`
	}
	return `INSERT INTO portable_users (active, amount, payload, blob_data) VALUES (TRUE, 7.5000, '{"ok":false}', X'0a0b')`
}

// backupFixtureInsert returns one seeded compatibility row for a SQL dialect.
func backupFixtureInsert(driver string) string {
	if driver == "postgres" {
		return `INSERT INTO portable_users (id, active, amount, payload, blob_data) VALUES (1, TRUE, 12.3400, '{"ok":true}'::jsonb, decode('0001ff', 'hex'))`
	}
	return `INSERT INTO portable_users (id, active, amount, payload, blob_data) VALUES (1, TRUE, 12.3400, '{"ok":true}', X'0001ff')`
}

// canonicalRows compares transferred rows without driver order or decimal scale.
func canonicalRows(rows []backup.PortableRow) []string {
	values := make([]string, 0, len(rows))
	for _, row := range rows {
		keys := make([]string, 0, len(row))
		for key := range row {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		value := ""
		for _, key := range keys {
			canonical := row[key].Value
			if row[key].Type == "decimal" {
				if number, ok := new(big.Rat).SetString(canonical); ok {
					canonical = number.RatString()
				}
			}
			value += key + "=" + row[key].Type + ":" + canonical + "\x00"
		}
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

// openBackupSQLite opens the local SQLite participant for matrix tests.
func openBackupSQLite(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "portable.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	return db
}

// openBackupDatabase opens a network database using the standard SQL driver name.
func openBackupDatabase(t *testing.T, driver string, dsn string) *sql.DB {
	t.Helper()
	sqlDriver := driver
	if driver == "postgres" {
		sqlDriver = "pgx"
	}
	db, err := sql.Open(sqlDriver, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	return db
}

// startBackupMySQL starts the MySQL matrix participant and returns connection metadata.
func startBackupMySQL(t *testing.T, ctx context.Context) (string, backup.Connection, func()) {
	t.Helper()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: testcontainers.ContainerRequest{
		Image: "mysql:8.4", ExposedPorts: []string{"3306/tcp"}, Env: map[string]string{"MYSQL_DATABASE": "app", "MYSQL_USER": "app", "MYSQL_PASSWORD": "secret", "MYSQL_ROOT_PASSWORD": "root", "TZ": "America/Los_Angeles"},
		WaitingFor: wait.ForLog("ready for connections").WithStartupTimeout(90 * time.Second),
	}, Started: true})
	if err != nil {
		t.Fatal(err)
	}
	host := containerHost(t, ctx, container, "3306/tcp")
	dsn := fmt.Sprintf("app:secret@tcp(%s)/app?parseTime=true&loc=UTC&time_zone=%%27%%2B00%%3A00%%27", host)
	waitForBackupDatabase(t, dsn, "mysql")
	hostname, port, err := net.SplitHostPort(host)
	if err != nil {
		t.Fatal(err)
	}
	return dsn, backup.Connection{Driver: "mysql", Database: "app", Host: hostname, Port: port, Username: "app", Password: "secret"}, func() { _ = container.Terminate(context.Background()) }
}

// startBackupPostgres starts the Postgres matrix participant.
func startBackupPostgres(t *testing.T, ctx context.Context) (string, testcontainers.Container, func()) {
	t.Helper()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: testcontainers.ContainerRequest{
		Image: "postgres:16-alpine", ExposedPorts: []string{"5432/tcp"}, Env: map[string]string{"POSTGRES_DB": "app", "POSTGRES_USER": "app", "POSTGRES_PASSWORD": "secret", "TZ": "America/Los_Angeles"},
		WaitingFor: wait.ForLog("database system is ready to accept connections").WithStartupTimeout(60 * time.Second),
	}, Started: true})
	if err != nil {
		t.Fatal(err)
	}
	host := containerHost(t, ctx, container, "5432/tcp")
	dsn := fmt.Sprintf("postgres://app:secret@%s/app?sslmode=disable&timezone=UTC", host)
	waitForBackupDatabase(t, dsn, "postgres")
	return dsn, container, func() { _ = container.Terminate(context.Background()) }
}

// containerHost resolves a mapped container port into a host:port endpoint.
func containerHost(t *testing.T, ctx context.Context, container testcontainers.Container, port string) string {
	t.Helper()
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mapped, err := container.MappedPort(ctx, nat.Port(port))
	if err != nil {
		t.Fatal(err)
	}
	return net.JoinHostPort(host, mapped.Port())
}

// waitForBackupDatabase waits for a standard SQL driver to accept connections.
func waitForBackupDatabase(t *testing.T, dsn string, driver string) {
	t.Helper()
	sqlDriver := driver
	if driver == "postgres" {
		sqlDriver = "pgx"
	}
	deadline := time.Now().Add(60 * time.Second)
	consecutive := 0
	for time.Now().Before(deadline) {
		db, err := sql.Open(sqlDriver, dsn)
		ready := err == nil && db.Ping() == nil
		if db != nil {
			_ = db.Close()
		}
		if ready {
			consecutive++
			if consecutive >= 3 {
				return
			}
		} else {
			consecutive = 0
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("database %s did not become ready", driver)
}
