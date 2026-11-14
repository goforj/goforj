package generate

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestResourceNamesSharesGeneratorDiscovery includes implicit children and App overlays while excluding driver helper keys.
func TestResourceNamesSharesGeneratorDiscovery(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte("apps:\n  admin:\n    components: [cli, database_sqlite, cache]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	names := ResourceNames(root, map[string]string{"DB_ANALYTICS_DATABASE": "analytics", "DB_SQLITE_DATABASE": "app.db", "ADMIN_CACHE_SESSIONS_DRIVER": "redis"})
	if !slices.Contains(names[project.ResourceDatabase], "analytics") || slices.Contains(names[project.ResourceDatabase], "sqlite") {
		t.Fatalf("database names: %#v", names)
	}
	if !slices.Contains(names[project.ResourceCache], "sessions") {
		t.Fatalf("cache names: %#v", names)
	}
}

// TestResourceNamesTreatsNamedSQLitePathsAsDatabaseSettings keeps helpers out of the generated accessor inventory.
func TestResourceNamesTreatsNamedSQLitePathsAsDatabaseSettings(t *testing.T) {
	root := t.TempDir()
	names := ResourceNames(root, map[string]string{"DB_ANALYTICS_SQLITE_DATABASE": "analytics.db", "DB_REPORTING_SQLITE_DRIVER": "sqlite", "DB_REPORTING_SQLITE_SQLITE_DATABASE": "reporting.db"})
	if got := names[project.ResourceDatabase]; !slices.Equal(got, []string{"analytics", "reporting_sqlite"}) {
		t.Fatalf("database names=%v", got)
	}
}

// TestBaselineDriversReturnsIndependentCatalogValues prevents callers from changing the generator's baseline.
func TestBaselineDriversReturnsIndependentCatalogValues(t *testing.T) {
	for _, definition := range project.ResourceCatalog() {
		drivers := BaselineDrivers(definition.Key)
		if !slices.Contains(drivers, definition.DefaultDriver) {
			t.Fatalf("missing default for %s", definition.Key)
		}
		drivers[0] = "changed"
		if slices.Contains(BaselineDrivers(definition.Key), "changed") {
			t.Fatal("baseline was mutated")
		}
	}
	if got := BaselineDrivers(project.ResourceKey("unknown")); len(got) != 0 {
		t.Fatal("unknown resource has drivers")
	}
}

// TestResourceNamesInScopePreservesProjectedNames retains declarations whose projected keys also resemble known App settings.
func TestResourceNamesInScopePreservesProjectedNames(t *testing.T) {
	root := featureInventoryProject(t, "apps:\n  feature:\n    components: [cli, feature]\n  notification:\n    components: [cli, notification]\n  search:\n    components: [cli, search]\n")
	values := map[string]string{"SEARCH_CACHE_PATH": "search-index", "FEATURE_CACHE_PATH": "flags.json", "NOTIFICATION_FEATURE_WEBHOOK_URL": "https://example.com/hook", "DB_ANALYTICS_DATABASE": "analytics", "DB_SQLITE_DATABASE": "app.db", "DB_REPORTING_SQLITE_SQLITE_DATABASE": "reporting.db", "CACHE_FEATURE_DRIVER": "memory"}
	names := ResourceNamesInScope(values)
	for key, want := range map[project.ResourceKey][]string{project.ResourceFeature: {"cache"}, project.ResourceSearch: {"cache"}, project.ResourceNotification: {"feature"}, project.ResourceDatabase: {"analytics", "reporting_sqlite"}, project.ResourceCache: {"feature"}} {
		if !slices.Equal(names[key], want) {
			t.Fatalf("%s names=%v want=%v", key, names[key], want)
		}
	}
	if names := ResourceNames(root, values); len(names[project.ResourceFeature]) != 0 || len(names[project.ResourceNotification]) != 0 || len(names[project.ResourceSearch]) != 0 {
		t.Fatalf("project-wide ownership changed: %#v", names)
	}
}
