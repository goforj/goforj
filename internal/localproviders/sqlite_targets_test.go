package localproviders

import (
	"testing"
)

// databaseResource fails close to the fixture when its database scope was not discovered.
func databaseResource(t *testing.T, s *Session, key string) Resource {
	t.Helper()
	for _, resource := range s.Resources {
		if resource.Key == key {
			return resource
		}
	}
	t.Fatalf("database resource %s was not discovered", key)
	return Resource{}
}

// TestPortableSeparatesConvertedDatabaseFromInheritedSQLiteDSN prevents a root SQLite target from overriding a converted named database's new path.
func TestPortableSeparatesConvertedDatabaseFromInheritedSQLiteDSN(t *testing.T) {
	root := fixture(t)
	put(t, root, ".env", "DB_DRIVER=sqlite\nDB_DSN=file:root.db\nDB_REPORTS_DRIVER=mysql\nDB_REPORTS_DSN=mysql-service-dsn\n")
	s := openTest(t, root)
	values, err := s.Portable()
	if err != nil {
		t.Fatal(err)
	}
	if values["DB_DSN"] != "file:root.db" || values["DB_REPORTS_DSN"] != values["DB_REPORTS_SQLITE_DATABASE"] || values["DB_REPORTS_DSN"] == "" {
		t.Fatal("converted database inherited another database's DSN")
	}
}

// TestPortablePreservesInheritedSQLiteDSN keeps an existing SQLite target when its service-configured parent is converted.
func TestPortablePreservesInheritedSQLiteDSN(t *testing.T) {
	root := fixture(t)
	put(t, root, ".env", "DB_DRIVER=mysql\nDB_DSN=file:shared.db\nDB_REPORTS_DRIVER=sqlite\nADMIN_DB_DRIVER=sqlite\n")
	s := openTest(t, root)
	values, err := s.Portable()
	if err != nil {
		t.Fatal(err)
	}
	if values["DB_DSN"] != "" || values["DB_REPORTS_DSN"] != "file:shared.db" || values["ADMIN_DB_DSN"] != "file:shared.db" {
		t.Fatal("parent conversion changed an existing inherited SQLite target")
	}
}

// TestPortableKeepsExistingSQLiteTargets preserves explicit SQLite DSNs and scoped paths while converting external siblings.
func TestPortableKeepsExistingSQLiteTargets(t *testing.T) {
	root := fixture(t)
	put(t, root, ".env", "DB_DRIVER=mysql\nDB_DATABASE=service\nDB_DSN=service-dsn\nDB_ANALYTICS_DRIVER=sqlite3\nDB_ANALYTICS_SQLITE_DATABASE=./data/analytics.db\nDB_ANALYTICS_DSN=file:analytics.db?mode=ro\nADMIN_DB_DRIVER=sqlite\nADMIN_DB_SQLITE_DATABASE=./data/admin.db\nADMIN_DB_DSN=file:admin.db?mode=ro\nCACHE_DRIVER=redis\n")
	s := openTest(t, root)
	values, err := s.Portable()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"DB_ANALYTICS_SQLITE_DATABASE", "DB_ANALYTICS_DSN", "ADMIN_DB_SQLITE_DATABASE", "ADMIN_DB_DSN"} {
		if values[key] != s.Current[key] {
			t.Errorf("portable reset %s", key)
		}
	}
	if values["DB_SQLITE_DATABASE"] != "./_data/local/db.db" || values["DB_DSN"] != "" || values["CACHE_DRIVER"] != "memory" {
		t.Fatal("external siblings did not become portable")
	}
}

// TestPortableUsesOriginalInheritance keeps an App's inherited SQLite target stable when its root provider changes earlier in the preset.
func TestPortableUsesOriginalInheritance(t *testing.T) {
	root := fixture(t)
	put(t, root, ".env", "DB_DRIVER=sqlite\nDB_DATABASE=./data/original.db\nADMIN_DB_DRIVER=mysql\nADMIN_DB_DATABASE=service\n")
	s := openTest(t, root)
	values, err := s.Portable()
	if err != nil {
		t.Fatal(err)
	}
	if values["DB_SQLITE_DATABASE"] != "./data/original.db" || values["ADMIN_DB_SQLITE_DATABASE"] != "./_data/local/admin_db.db" {
		t.Fatalf("preset changed SQLite inheritance: %v", values)
	}
}
