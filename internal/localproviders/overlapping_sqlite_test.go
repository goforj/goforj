package localproviders

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// overlappingDatabaseSession models one environment key consumed by two Apps with independent parent settings.
func overlappingDatabaseSession(t *testing.T, values map[string]string) *Session {
	t.Helper()
	root := fixture(t)
	put(t, root, ".goforj.yml", "project_name: local\nmodule_name: example.org/local\nrender:\n  components: [database_mysql]\napps:\n  foo:\n    components: [database_mysql]\n  foo-db:\n    components: [database_mysql]\n")
	put(t, root, ".env", string(encodeDocument(values)))
	return openTest(t, root)
}

// overlappingDatabaseResource obtains the same ambiguous setting offered by the wizard.
func overlappingDatabaseResource(t *testing.T, s *Session) Resource {
	t.Helper()
	for _, resource := range s.Resources {
		if resource.Key == "FOO_DB_DB_DRIVER" {
			return resource
		}
	}
	t.Fatal("overlapping database was not discovered")
	return Resource{}
}

// TestOverlappingDatabaseTransitionsRejectDifferentTargets prevents a shared setting from collapsing distinct source databases during conversion.
func TestOverlappingDatabaseTransitionsRejectDifferentTargets(t *testing.T) {
	for _, tc := range []struct {
		name, next string
		values     map[string]string
	}{
		{"mixed", "sqlite", map[string]string{"DB_DRIVER": "sqlite", "DB_SQLITE_DATABASE": "./data/root.db", "FOO_DB_DRIVER": "mysql", "FOO_DB_DATABASE": "foo"}},
		{"service-databases", "sqlite", map[string]string{"DB_DRIVER": "mysql", "DB_DATABASE": "root", "FOO_DB_DRIVER": "mysql", "FOO_DB_DATABASE": "foo"}},
		{"service-hosts", "sqlite", map[string]string{"DB_DRIVER": "mysql", "DB_DATABASE": "app", "DB_HOST": "root.example", "FOO_DB_HOST": "foo.example"}},
		{"sqlite-paths", "mysql", map[string]string{"DB_DRIVER": "sqlite", "DB_SQLITE_DATABASE": "./data/root.db", "FOO_DB_SQLITE_DATABASE": "./data/foo.db"}},
		{"sqlite-dsns", "mysql", map[string]string{"DB_DRIVER": "sqlite", "DB_DSN": "file:root.db?secret=private", "FOO_DB_DSN": "file:foo.db?secret=private"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.values["FOO_DB_DB_MAX_OPEN_CONNECTIONS"] = "10"
			s := overlappingDatabaseSession(t, tc.values)
			values := clone(s.Current)
			err := s.selectDriver(values, overlappingDatabaseResource(t, s), tc.next)
			if err == nil || !strings.Contains(err.Error(), "conflicting database targets") || strings.Contains(err.Error(), "secret") {
				t.Fatalf("expected a redacted scope conflict, got %v", err)
			}
			if !equalValues(values, s.Current) {
				t.Fatal("rejected driver edit changed its inputs")
			}
			if tc.next == "sqlite" {
				if _, err := s.Portable(); err == nil {
					t.Fatal("portable collapsed different database targets")
				}
			}
			data, err := os.ReadFile(filepath.Join(s.Root, ".env"))
			if err != nil || string(data) != string(encodeDocument(tc.values)) {
				t.Fatal("rejected transition changed .env")
			}
		})
	}
}

// TestOverlappingSQLiteNoOpPreservesInheritedTargets leaves independent targets in their parent scopes when the requested driver is already active.
func TestOverlappingSQLiteNoOpPreservesInheritedTargets(t *testing.T) {
	for _, suffix := range []string{"SQLITE_DATABASE", "DSN"} {
		t.Run(suffix, func(t *testing.T) {
			values := map[string]string{"DB_DRIVER": "sqlite", "DB_" + suffix: "./data/root.db", "FOO_DB_" + suffix: "./data/foo.db", "FOO_DB_DB_MAX_OPEN_CONNECTIONS": "10"}
			s := overlappingDatabaseSession(t, values)
			portable, err := s.Portable()
			if err != nil {
				t.Fatal(err)
			}
			for _, field := range []string{"DSN", "SQLITE_DATABASE"} {
				if _, present := portable["FOO_DB_DB_"+field]; present {
					t.Fatal("no-op pinned a target shared by independent scopes")
				}
			}
			for _, scope := range []string{"FOO_", "FOO_DB_"} {
				before := databaseTargetInScope(values, "FOO_DB_DB_DRIVER", scope)
				after := databaseTargetInScope(portable, "FOO_DB_DB_DRIVER", scope)
				if before != after {
					t.Fatalf("scope %s changed targets", scope)
				}
			}

		})
	}
}

// TestOverlappingSharedServiceTargetCanBecomePortable permits multiple scopes that intentionally inherit the same source connection.
func TestOverlappingSharedServiceTargetCanBecomePortable(t *testing.T) {
	s := overlappingDatabaseSession(t, map[string]string{"DB_DRIVER": "mysql", "DB_DATABASE": "shared", "FOO_DB_DRIVER": "mariadb", "FOO_DB_DB_MAX_OPEN_CONNECTIONS": "10"})
	portable, err := s.Portable()
	if err != nil {
		t.Fatal(err)
	}
	if portable["FOO_DB_DB_DRIVER"] != "sqlite" || portable["FOO_DB_DB_SQLITE_DATABASE"] == "" {
		t.Fatal("shared service target was not converted")
	}
}

// TestOverlappingExampleDatabaseDeclarationsGuardConversions includes named accessors declared only by the generation fallback file.
func TestOverlappingExampleDatabaseDeclarationsGuardConversions(t *testing.T) {
	s := overlappingDatabaseSession(t, map[string]string{"DB_DRIVER": "mysql", "DB_DATABASE": "root", "FOO_DB_DATABASE": "foo"})
	put(t, s.Root, ".env.example", "FOO_DB_DB_MAX_OPEN_CONNECTIONS=10\n")
	s = openTest(t, s.Root)
	if _, err := s.Portable(); err == nil || !strings.Contains(err.Error(), "conflicting database targets") {
		t.Fatalf("example-only named database was not protected: %v", err)
	}
}
