package stacks

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSavedSQLitePathsFollowDriverInheritance preserves explicit targets in definitions, private round trips, and App-specific binary defaults.
func TestSavedSQLitePathsFollowDriverInheritance(t *testing.T) {
	for _, tc := range []struct {
		name, app, target, bakedKey string
		values                      map[string]string
	}{
		{"implicit-root", "app", "DB_SQLITE_DATABASE", "DB_SQLITE_DATABASE", map[string]string{}},
		{"implicit-root-legacy", "app", "DB_DATABASE", "DB_DATABASE", map[string]string{}},
		{"inherited-named", "app", "DB_ANALYTICS_SQLITE_DATABASE", "DB_ANALYTICS_SQLITE_DATABASE", map[string]string{"DB_DRIVER": "sqlite"}},
		{"inherited-named-legacy", "app", "DB_ANALYTICS_DATABASE", "DB_ANALYTICS_DATABASE", map[string]string{"DB_DRIVER": "sqlite3"}},
		{"implicit-app", "admin", "ADMIN_DB_SQLITE_DATABASE", "DB_SQLITE_DATABASE", map[string]string{}},
		{"inherited-app", "admin", "ADMIN_DB_SQLITE_DATABASE", "DB_SQLITE_DATABASE", map[string]string{"DB_DRIVER": "sqlite"}},
		{"inherited-app-named", "admin", "ADMIN_DB_ANALYTICS_SQLITE_DATABASE", "DB_ANALYTICS_SQLITE_DATABASE", map[string]string{"DB_DRIVER": "mysql", "ADMIN_DB_DRIVER": "sqlite"}},
		{"inherited-base-named", "admin", "ADMIN_DB_ANALYTICS_SQLITE_DATABASE", "DB_ANALYTICS_SQLITE_DATABASE", map[string]string{"DB_DRIVER": "mysql", "DB_ANALYTICS_DRIVER": "sqlite"}},
		{"empty-app-driver", "admin", "ADMIN_DB_ANALYTICS_SQLITE_DATABASE", "DB_ANALYTICS_SQLITE_DATABASE", map[string]string{"DB_DRIVER": "mysql", "DB_ANALYTICS_DRIVER": "mysql", "ADMIN_DB_DRIVER": "sqlite", "ADMIN_DB_ANALYTICS_DRIVER": ""}},
		{"shared-path-service-root", "admin", "DB_SQLITE_DATABASE", "DB_SQLITE_DATABASE", map[string]string{"DB_DRIVER": "mysql", "ADMIN_DB_DRIVER": "sqlite"}},
		{"shared-path-service-named", "admin", "DB_ANALYTICS_SQLITE_DATABASE", "DB_ANALYTICS_SQLITE_DATABASE", map[string]string{"DB_DRIVER": "mysql", "DB_ANALYTICS_DRIVER": "mysql", "ADMIN_DB_ANALYTICS_DRIVER": "sqlite"}},
		{"name-ending-sqlite", "app", "DB_REPORTING_SQLITE_SQLITE_DATABASE", "DB_REPORTING_SQLITE_SQLITE_DATABASE", map[string]string{"DB_DRIVER": "sqlite", "DB_REPORTING_SQLITE_DRIVER": ""}},
		{"overlapping-app-prefix", "admin-tools", "ADMIN_TOOLS_DB_ANALYTICS_SQLITE_DATABASE", "DB_ANALYTICS_SQLITE_DATABASE", map[string]string{"DB_DRIVER": "mysql", "ADMIN_DB_DRIVER": "mysql", "ADMIN_TOOLS_DB_DRIVER": "sqlite"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t)
			if tc.app == "admin-tools" {
				config, err := os.ReadFile(filepath.Join(root, ".goforj.yml"))
				if err != nil {
					t.Fatal(err)
				}
				put(t, root, ".goforj.yml", string(config)+"  admin-tools:\n    components:\n      database_mysql: true\n")
			}
			const target = "./data/existing.db"
			tc.values[tc.target] = target
			put(t, root, ".env", string(encodeDocument(tc.values)))
			s := openTest(t, root)
			if err := s.Save("saved", s.Current); err != nil {
				t.Fatal(err)
			}
			defaults, err := BuildDefaults(root, "saved")
			if err != nil {
				t.Fatal(err)
			}
			if defaults[tc.target] != target {
				t.Fatalf("saved definition lost %s: %v", tc.target, defaults)
			}
			baked, err := AppDefaults(root, defaults, tc.app)
			if err != nil || baked[tc.bakedKey] != target {
				t.Fatalf("binary defaults lost target: %v, %v", baked, err)
			}
			s = openTest(t, root)
			loaded, err := s.Load("saved", true)
			if err != nil || !equalValues(loaded, tc.values) {
				t.Fatalf("save changed configuration: %v, %v", loaded, err)
			}
			if err := s.Activate("saved", loaded, false); err != nil {
				t.Fatal(err)
			}
			if !equalValues(openTest(t, root).Current, tc.values) {
				t.Fatal("activation changed configuration")
			}
		})
	}
}

// TestInheritedSQLitePathsKeepServiceSettingsPrivate prevents path inheritance from publishing unused database names or DSNs.
func TestInheritedSQLitePathsKeepServiceSettingsPrivate(t *testing.T) {
	root := fixture(t)
	values := map[string]string{
		"DB_DRIVER": "mysql", "DB_DATABASE": "private-service", "DB_DSN": "private-service-dsn",
		"DB_SQLITE_DATABASE": "./data/shared.db", "ADMIN_DB_DRIVER": "sqlite",
		"ADMIN_DB_DATABASE": "private-unused-name", "ADMIN_DB_DSN": "file:private.db?secret=private",
		"DB_ANALYTICS_DRIVER": "mysql", "DB_ANALYTICS_SQLITE_DATABASE": "./data/inactive.db",
		"ADMIN_DB_ANALYTICS_DRIVER": "mysql",
	}
	put(t, root, ".env", string(encodeDocument(values)))
	s := openTest(t, root)
	if err := s.Save("saved", values); err != nil {
		t.Fatal(err)
	}
	defaults, err := BuildDefaults(root, "saved")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"DB_DATABASE", "DB_DSN", "ADMIN_DB_DATABASE", "ADMIN_DB_DSN", "DB_ANALYTICS_SQLITE_DATABASE"} {
		if _, present := defaults[key]; present {
			t.Errorf("private setting %s became shareable", key)
		}
	}
	if defaults["DB_SQLITE_DATABASE"] != values["DB_SQLITE_DATABASE"] {
		t.Fatal("shared SQLite path was omitted")
	}
}

// TestBuildDefaultsAcceptsInheritedSQLitePaths validates hand-authored definitions without requiring redundant per-scope driver declarations.
func TestBuildDefaultsAcceptsInheritedSQLitePaths(t *testing.T) {
	root := fixture(t)
	values := map[string]string{"DB_DRIVER": "sqlite", "DB_ANALYTICS_SQLITE_DATABASE": "./data/analytics.db", "ADMIN_DB_SQLITE_DATABASE": "./data/admin.db"}
	put(t, root, DefinitionPath("shared"), string(encodeDocument(values)))
	if err := os.Remove(filepath.Join(root, ".env")); err != nil {
		t.Fatal(err)
	}
	defaults, err := BuildDefaults(root, "shared")
	if err != nil || !equalValues(defaults, values) {
		t.Fatalf("valid inherited definition rejected or changed: %v, %v", defaults, err)
	}
}

// TestExplicitSQLitePathsRemainShareableWithoutEnabledDatabase preserves definitions prepared before a component is enabled.
func TestExplicitSQLitePathsRemainShareableWithoutEnabledDatabase(t *testing.T) {
	root := fixture(t)
	put(t, root, ".goforj.yml", "project_name: stacks\nmodule_name: example.org/stacks\napps:\n  app:\n    components:\n      cli: true\n")
	values := map[string]string{"DB_DRIVER": "sqlite", "DB_SQLITE_DATABASE": "./data/existing.db"}
	s := openTest(t, root)
	if err := s.Save("saved", values); err != nil {
		t.Fatal(err)
	}
	defaults, err := BuildDefaults(root, "saved")
	if err != nil || !equalValues(defaults, values) {
		t.Fatalf("existing explicit definition rejected: %v, %v", defaults, err)
	}
}
