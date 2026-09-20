package stacks

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

// TestSQLiteDSNsFollowSavedProfiles preserves private targets across sessions, service edits, and independent saved profiles.
func TestSQLiteDSNsFollowSavedProfiles(t *testing.T) {
	for _, key := range []string{"DB_DRIVER", "DB_FOO_DRIVER", "ADMIN_DB_DRIVER", "ADMIN_DB_FOO_DRIVER"} {
		t.Run(key, func(t *testing.T) {
			root := fixture(t)
			prefix := strings.TrimSuffix(key, "DRIVER")
			for _, name := range []string{"first", "second"} {
				put(t, root, ".env", key+"=sqlite\n"+prefix+"DATABASE=stale-generic\n"+prefix+"DSN=file:"+name+".db?key=private-"+name+"\nDB_FOO_SQLITE_DRIVER=sqlite\nDB_FOO_SQLITE_DSN=file:other.db\n")
				s := openTest(t, root)
				values := clone(s.Current)
				if err := s.SetDriver(values, databaseResource(t, s, key), "mysql"); err != nil {
					t.Fatal(err)
				}
				values[prefix+"DSN"] = "mysql-service-dsn"
				if err := s.Save(name, values); err != nil {
					t.Fatal(err)
				}
				public, err := os.ReadFile(filepath.Join(root, DefinitionPath(name)))
				if err != nil || bytes.Contains(public, []byte("private-")) || bytes.Contains(public, []byte(SQLiteDSNsKey)) {
					t.Fatalf("private SQLite metadata leaked into the definition: %v", err)
				}
			}
			for _, name := range []string{"first", "second"} {
				s := openTest(t, root)
				values, err := s.Load(name, true)
				if err != nil {
					t.Fatal(err)
				}
				if err := s.Activate(name, values, false); err != nil {
					t.Fatal(err)
				}
				s = openTest(t, root)
				values = clone(s.Current)
				if err := s.SetDriver(values, databaseResource(t, s, key), "sqlite"); err != nil {
					t.Fatal(err)
				}
				want := "file:" + name + ".db?key=private-" + name
				if values[prefix+"DSN"] != want || values["DB_FOO_SQLITE_DSN"] != "file:other.db" {
					t.Fatal("lost the profile's DSN or overwrote another named connection")
				}
				if err := s.Save(name, values); err != nil {
					t.Fatal(err)
				}
				baked, err := BuildDefaults(root, name)
				if err != nil {
					t.Fatal(err)
				}
				if _, present := baked[SQLiteDSNsKey]; present || strings.Contains(string(encodeDocument(baked)), "private-") {
					t.Fatal("private DSN entered binary defaults")
				}
			}
		})
	}
}

// TestSQLiteMetadataValidationRedactsValues rejects corrupt private metadata before editing any resource.
func TestSQLiteMetadataValidationRedactsValues(t *testing.T) {
	s := openTest(t, fixture(t))
	for _, metadata := range []string{`{"DB_DRIVER":"private-secret"`, `null`, `[]`, `{"DB_DRIVER":17}`, `{"not a key_DRIVER":"private-secret"}`, `{"APP_KEY":"private-secret"}`, `{"DB_DRIVER":"private-secret\u0000"}`} {
		values := clone(s.Current)
		values[SQLiteDSNsKey] = metadata
		before := clone(values)
		if err := s.Validate(values); err == nil || strings.Contains(err.Error(), "private-secret") {
			t.Fatalf("expected redacted validation failure: %v", err)
		}
		if err := s.SetDriver(values, databaseResource(t, s, "DB_DRIVER"), "sqlite"); err == nil || !reflect.DeepEqual(values, before) {
			t.Fatal("invalid metadata permitted or partially applied a driver edit")
		}
	}
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

// TestSQLiteTargetsSurviveDriverRoundTrips covers dedicated, legacy, inherited, and runtime-default paths across every database scope.
func TestSQLiteTargetsSurviveDriverRoundTrips(t *testing.T) {
	for _, key := range []string{"DB_DRIVER", "DB_ANALYTICS_DRIVER", "ADMIN_DB_DRIVER", "ADMIN_DB_ANALYTICS_DRIVER"} {
		for _, source := range []string{"dedicated", "legacy", "inherited", "default"} {
			t.Run(key+"/"+source, func(t *testing.T) {
				root := fixture(t)
				prefix := strings.TrimSuffix(key, "DRIVER")
				content := "DB_DRIVER=sqlite\n" + key + "=sqlite\n"
				want := "./data/custom.db"
				switch source {
				case "dedicated":
					content += prefix + "SQLITE_DATABASE=" + want + "\n"
				case "legacy":
					content += prefix + "DATABASE=" + want + "\n"
				case "inherited":
					content += "DB_SQLITE_DATABASE=" + want + "\n"
				default:
					want = "./_data/sqlite/app.db"
					if strings.Contains(key, "ANALYTICS") {
						want = "./_data/sqlite/analytics.db"
					}
				}
				put(t, root, ".env", content)
				s := openTest(t, root)
				for _, resource := range s.Resources {
					if resource.Key != key {
						continue
					}
					values := clone(s.Current)
					for _, driver := range []string{"mysql", "sqlite", "postgres", "sqlite"} {
						if err := s.SetDriver(values, resource, driver); err != nil {
							t.Fatal(err)
						}
						if values[prefix+"SQLITE_DATABASE"] != want {
							t.Fatalf("%s changed target: got %q, want %q", driver, values[prefix+"SQLITE_DATABASE"], want)
						}
						if driver != "sqlite" {
							values[prefix+"DATABASE"] = "service_database"
							values[prefix+"DSN"] = "service-dsn"
						} else if values[prefix+"DSN"] != "" {
							t.Fatal("retained a service DSN after returning to SQLite")
						}
					}
				}
			})
		}
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
	if values["DB_SQLITE_DATABASE"] != "./_data/stacks/portable/db.db" || values["DB_DSN"] != "" || values["CACHE_DRIVER"] != "memory" {
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
	if values["DB_SQLITE_DATABASE"] != "./data/original.db" || values["ADMIN_DB_SQLITE_DATABASE"] != "./_data/stacks/portable/admin_db.db" {
		t.Fatalf("preset changed SQLite inheritance: %v", values)
	}
}
