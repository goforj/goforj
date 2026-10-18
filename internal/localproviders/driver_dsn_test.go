package localproviders

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestDriversRejectInheritedIncompatibleDSN keeps named and App databases from receiving a parent's SQLite connection string.
func TestDriversRejectInheritedIncompatibleDSN(t *testing.T) {
	for _, tc := range []struct{ name, env, key string }{
		{"named", "DB_DRIVER=sqlite\nDB_DSN=file:root.db\nDB_REPORTS_DRIVER=sqlite\n", "DB_REPORTS_DRIVER"},
		{"app-named", "DB_DRIVER=sqlite\nADMIN_DB_DSN=file:admin.db\nADMIN_DB_REPORTS_DRIVER=sqlite\n", "ADMIN_DB_REPORTS_DRIVER"},
		{"other-service", "DB_DRIVER=mysql\nDB_DSN=synthetic-private-dsn\nDB_REPORTS_DRIVER=mysql\n", "DB_REPORTS_DRIVER"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".env", tc.env)
			s := openTest(t, root)
			driver := "mysql"
			if tc.name == "other-service" {
				driver = "postgres"
			}
			_, err := s.Drivers(map[string]string{tc.key: driver})
			if err == nil || !strings.Contains(err.Error(), "would inherit a DSN") {
				t.Fatalf("expected inherited DSN error, got %v", err)
			}
			if strings.Contains(err.Error(), "file:") || strings.Contains(err.Error(), "synthetic-private") {
				t.Fatal("diagnostic exposed a DSN")
			}
			data, err := os.ReadFile(filepath.Join(root, ".env"))
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != tc.env || openTest(t, root).HasPrevious() {
				t.Fatal("rejected selection changed project settings")
			}
		})
	}
}

// TestDriversRecoverInheritedOriginalDSN restores a child connection without switching its local parent back.
func TestDriversRecoverInheritedOriginalDSN(t *testing.T) {
	for _, tc := range []struct{ name, env, key string }{
		{"named", "DB_DRIVER=mysql\nDB_DSN=synthetic-private-dsn\nDB_REPORTS_DRIVER=mysql\n", "DB_REPORTS_DRIVER"},
		{"app-named", "DB_DRIVER=mysql\nADMIN_DB_DSN=synthetic-private-dsn\nADMIN_DB_REPORTS_DRIVER=mysql\n", "ADMIN_DB_REPORTS_DRIVER"},
		{"app-root", "DB_DRIVER=mysql\nDB_DSN=synthetic-private-dsn\nADMIN_DB_DRIVER=mysql\n", "ADMIN_DB_DRIVER"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".env", tc.env)
			s := openTest(t, root)
			original := clone(s.Current)
			values, err := s.Portable()
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Apply(values); err != nil {
				t.Fatal(err)
			}
			s = openTest(t, root)
			values, err = s.Drivers(map[string]string{tc.key: "mysql"})
			if err != nil {
				t.Fatal(err)
			}
			if s.databaseValue(values, tc.key, "DSN") != "synthetic-private-dsn" {
				t.Fatal("original effective service DSN was not recovered")
			}
			if values["DB_DRIVER"] != "sqlite" {
				t.Fatal("switching child changed root provider")
			}
			if err := s.Apply(values); err != nil {
				t.Fatal(err)
			}
			s = openTest(t, root)
			if err := s.Restore(); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(openTest(t, root).Current, original) {
				t.Fatal("original settings were not restored")
			}
		})
	}
}

// TestDriversCanonicalAliasesKeepDSN treats accepted aliases as the same provider across root and App settings.
func TestDriversCanonicalAliasesKeepDSN(t *testing.T) {
	for _, pair := range [][2]string{{"mariadb", "mysql"}, {"postgresql", "postgres"}, {"sqlite3", "sqlite"}} {
		for _, key := range []string{"DB_DRIVER", "DB_REPORTS_DRIVER", "ADMIN_DB_DRIVER", "ADMIN_DB_REPORTS_DRIVER"} {
			t.Run(pair[0]+"/"+key, func(t *testing.T) {
				root := fixture(t)
				prefix := strings.TrimSuffix(key, "DRIVER")
				put(t, root, ".env", "DB_DRIVER="+pair[0]+"\n"+key+"="+pair[0]+"\n"+prefix+"DSN=synthetic-private-dsn\n")
				s := openTest(t, root)
				values, err := s.Drivers(map[string]string{key: pair[1]})
				if err != nil {
					t.Fatal(err)
				}
				if s.databaseValue(values, key, "DSN") != "synthetic-private-dsn" {
					t.Fatal("canonical selection changed the existing DSN")
				}
				portable, err := s.Portable()
				if err != nil {
					t.Fatal(err)
				}
				if err := s.Apply(portable); err != nil {
					t.Fatal(err)
				}
				s = openTest(t, root)
				restored, err := s.Drivers(map[string]string{key: pair[1]})
				if err != nil {
					t.Fatal(err)
				}
				if s.databaseValue(restored, key, "DSN") != "synthetic-private-dsn" {
					t.Fatal("canonical selection did not recover original alias DSN")
				}
			})
		}
	}
}

// TestRecoverDatabaseDSNRejectsAmbiguousScopes refuses to collapse distinct inherited connections into one assignment.
func TestRecoverDatabaseDSNRejectsAmbiguousScopes(t *testing.T) {
	for _, mixedDrivers := range []bool{false, true} {
		s := overlappingDatabaseSession(t, map[string]string{"DB_DRIVER": "mysql", "DB_DSN": "root-private", "FOO_DB_DRIVER": "mysql", "FOO_DB_DSN": "foo-private", "FOO_DB_DB_MAX_OPEN_CONNECTIONS": "10"})
		original := clone(s.Current)
		if mixedDrivers {
			original["FOO_DB_DRIVER"] = "postgres"
		}
		values := clone(s.Current)
		values["DB_DSN"], values["FOO_DB_DSN"] = "", ""
		err := s.recoverDatabaseDSN(values, original, "FOO_DB_DB_DRIVER", "mysql")
		if err == nil || !strings.Contains(err.Error(), "conflicting original database") {
			t.Fatalf("expected ambiguous recovery error, got %v", err)
		}
		if strings.Contains(err.Error(), "private") {
			t.Fatal("ambiguous recovery exposed credentials")
		}
	}
}

// TestDriversCanSwitchSQLiteParentAndChildTogether permits clearing an inherited DSN when every consuming provider changes.
func TestDriversCanSwitchSQLiteParentAndChildTogether(t *testing.T) {
	root := fixture(t)
	put(t, root, ".env", "DB_DRIVER=sqlite\nDB_DSN=file:root.db\nDB_REPORTS_DRIVER=sqlite\n")
	s := openTest(t, root)
	values, err := s.Drivers(map[string]string{"DB_DRIVER": "mysql", "DB_REPORTS_DRIVER": "mysql"})
	if err != nil {
		t.Fatal(err)
	}
	if s.databaseValue(values, "DB_REPORTS_DRIVER", "DSN") != "" {
		t.Fatal("service retained SQLite DSN")
	}
}

// TestDriversClearIncompatibleDSNForSharedResourceKeys validates database consumers even when the picker displays a cache resource.
func TestDriversClearIncompatibleDSNForSharedResourceKeys(t *testing.T) {
	root := fixture(t)
	put(t, root, ".goforj.yml", "project_name: local\nmodule_name: example.org/local\nrender:\n  components: [cache]\napps:\n  cache:\n    components: [database_mysql]\n")
	put(t, root, ".env", "DB_DRIVER=sqlite\nDB_DSN=file:root.db\nCACHE_DRIVER=memory\nCACHE_DB_DRIVER=sqlite\n")
	s := openTest(t, root)
	values, err := s.Drivers(map[string]string{"CACHE_DB_DRIVER": "mysql"})
	if err != nil {
		t.Fatal(err)
	}
	if s.databaseValue(values, "CACHE_DB_DRIVER", "DSN") != "" {
		t.Fatal("shared database key retained the inherited SQLite DSN")
	}
}
