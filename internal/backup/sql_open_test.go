package backup

import "testing"

// TestSQLConnectionDetailsBuildsUTCMySQLDSN verifies portable operations share the framework's UTC contract.
func TestSQLConnectionDetailsBuildsUTCMySQLDSN(t *testing.T) {
	driver, dsn, err := sqlConnectionDetails(Connection{
		Driver:   "mysql",
		Database: "app",
		Host:     "database",
		Port:     "3307",
		Username: "user",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("sql connection details: %v", err)
	}
	if driver != "mysql" {
		t.Fatalf("driver = %q, want mysql", driver)
	}
	want := "user:secret@tcp(database:3307)/app?parseTime=true&loc=UTC&time_zone=%27%2B00%3A00%27"
	if dsn != want {
		t.Fatalf("DSN = %q, want %q", dsn, want)
	}
}

// TestSQLConnectionDetailsPreservesCustomMySQLDSN verifies timezone overrides remain an explicit caller choice.
func TestSQLConnectionDetailsPreservesCustomMySQLDSN(t *testing.T) {
	want := "user:secret@tcp(database:3307)/app?parseTime=true&loc=America%2FChicago&time_zone=%27-05%3A00%27"
	driver, dsn, err := sqlConnectionDetails(Connection{Driver: "mysql", DSN: want})
	if err != nil {
		t.Fatalf("sql connection details: %v", err)
	}
	if driver != "mysql" {
		t.Fatalf("driver = %q, want mysql", driver)
	}
	if dsn != want {
		t.Fatalf("custom DSN = %q, want %q", dsn, want)
	}
}

// TestSQLConnectionDetailsBuildsUTCPostgresDSN verifies portable operations establish UTC sessions.
func TestSQLConnectionDetailsBuildsUTCPostgresDSN(t *testing.T) {
	driver, dsn, err := sqlConnectionDetails(Connection{
		Driver:   "postgres",
		Database: "app",
		Host:     "database",
		Port:     "5433",
		Username: "user",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("sql connection details: %v", err)
	}
	if driver != "pgx" {
		t.Fatalf("driver = %q, want pgx", driver)
	}
	want := "postgres://user:secret@database:5433/app?sslmode=disable&timezone=UTC"
	if dsn != want {
		t.Fatalf("DSN = %q, want %q", dsn, want)
	}
}
