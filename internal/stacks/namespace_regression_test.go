package stacks

import (
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/project"
)

// TestOverlappingAppDefaultsFollowSelectedPrefix preserves every resource overlay that the selected App consumes at runtime.
func TestOverlappingAppDefaultsFollowSelectedPrefix(t *testing.T) {
	for _, definition := range project.ResourceCatalog() {
		t.Run(string(definition.Key), func(t *testing.T) {
			root := fixture(t)
			prefix := definition.EnvironmentPrefix
			longApp := "foo-" + strings.ToLower(prefix)
			put(t, root, ".goforj.yml", "project_name: stacks\nmodule_name: example.org/stacks\napps:\n  foo:\n    components: [cli]\n  "+longApp+":\n    components: [cli]\n")
			key := "FOO_" + prefix + "_" + prefix + "_DRIVER"
			values := map[string]string{prefix + "_DRIVER": definition.DefaultDriver, key: definition.DefaultDriver}
			put(t, root, DefinitionPath("saved"), string(encodeDocument(values)))
			defaults, err := BuildDefaults(root, "saved")
			if err != nil {
				t.Fatal(err)
			}
			for _, app := range []string{"foo", longApp} {
				baked, err := AppDefaults(root, defaults, app)
				if err != nil {
					t.Fatal(err)
				}
				wantKey := strings.TrimPrefix(key, project.AppEnvironmentPrefix(app)+"_")
				if baked[wantKey] != definition.DefaultDriver {
					t.Fatalf("App %s lost %s: %v", app, wantKey, baked)
				}
			}
		})
	}
}

// TestSavedOverlappingAppSQLitePaths preserves a named target through the complete save-to-binary projection.
func TestSavedOverlappingAppSQLitePaths(t *testing.T) {
	for _, explicitDriver := range []bool{false, true} {
		t.Run(map[bool]string{false: "inherited", true: "explicit"}[explicitDriver], func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".goforj.yml", "project_name: stacks\nmodule_name: example.org/stacks\nrender:\n  components: [database_sqlite]\napps:\n  foo:\n    components: [database_sqlite]\n  foo-db:\n    components: [database_sqlite]\n")
			values := map[string]string{"DB_DRIVER": "sqlite", "FOO_DB_DB_SQLITE_DATABASE": "./data/foo-named.db"}
			if explicitDriver {
				values["FOO_DB_DB_DRIVER"] = "sqlite"
			}
			put(t, root, ".env", string(encodeDocument(values)))
			s := openTest(t, root)
			if err := s.Save("saved", s.Current); err != nil {
				t.Fatal(err)
			}
			defaults, err := BuildDefaults(root, "saved")
			if err != nil {
				t.Fatal(err)
			}
			for app, key := range map[string]string{"foo": "DB_DB_SQLITE_DATABASE", "foo-db": "DB_SQLITE_DATABASE"} {
				baked, err := AppDefaults(root, defaults, app)
				if err != nil || baked[key] != values["FOO_DB_DB_SQLITE_DATABASE"] {
					t.Fatalf("App %s lost its SQLite target: %v, %v", app, baked, err)
				}
			}
		})
	}
}

// TestDatabaseAliasWithResourceNamedApp matches normal generation when a database name resembles an unused App resource.
func TestDatabaseAliasWithResourceNamedApp(t *testing.T) {
	root := fixture(t)
	put(t, root, ".goforj.yml", "project_name: stacks\nmodule_name: example.org/stacks\nrender:\n  components: [database_mysql]\napps:\n  db:\n    components: [database_mysql]\n")
	values := map[string]string{"DB_DRIVER": "mysql", "DB_SUPPORTED_DRIVERS": "mysql", "DB_CACHE_DRIVER": "mariadb", "DB_CACHE_DATABASE": "cache"}
	put(t, root, ".env", string(encodeDocument(values)))
	for key, value := range values {
		t.Setenv(key, value)
	}
	if _, err := generate.GenerateDBFiles(root); err != nil {
		t.Fatalf("normal database generation: %v", err)
	}
	s := openTest(t, root)
	for _, resource := range s.Resources {
		if resource.Key == "DB_CACHE_DRIVER" && resource.Definition.Key != project.ResourceDatabase {
			t.Fatal("database was presented as a cache")
		}
	}
	if err := s.Save("saved", s.Current); err != nil {
		t.Fatal(err)
	}
	s = openTest(t, root)
	loaded, err := s.Load("saved", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Activate("saved", loaded, false); err != nil {
		t.Fatal(err)
	}
	s = openTest(t, root)
	portable, err := s.Portable()
	if err != nil || portable["DB_CACHE_DRIVER"] != "sqlite" {
		t.Fatalf("portable changed the wrong resource type: %v, %v", portable, err)
	}
}

// TestDatabaseAliasWithOverlappingUnusedApp prevents a longer App name from claiming a resource it does not consume.
func TestDatabaseAliasWithOverlappingUnusedApp(t *testing.T) {
	root := fixture(t)
	put(t, root, ".goforj.yml", "project_name: stacks\nmodule_name: example.org/stacks\nrender:\n  components: [cli]\napps:\n  admin:\n    components: [database_mysql]\n  admin-db:\n    components: [cli]\n")
	values := map[string]string{"DB_DRIVER": "mysql", "ADMIN_DB_CACHE_DRIVER": "mariadb"}
	s := openTest(t, root)
	if base := resourceKey(s.config, "ADMIN_DB_CACHE_DRIVER"); base != "DB_CACHE_DRIVER" {
		t.Fatalf("wrong participating resource scope: %s", base)
	}
	if err := s.Validate(values); err != nil {
		t.Fatal(err)
	}
}
