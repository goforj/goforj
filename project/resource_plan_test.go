package project

import (
	"reflect"
	"strings"
	"testing"
)

// TestDefaultResourcePlan locks the portable defaults and built-in transition drivers to concrete contracts.
func TestDefaultResourcePlan(t *testing.T) {
	components := Components{DatabaseMySQL: true, Mail: true, Docker: true, Jobs: true, Auth: true, Events: true}
	plan, err := DefaultResourcePlan(components)
	if err != nil {
		t.Fatalf("DefaultResourcePlan returned error: %v", err)
	}
	assertResourceSelection(t, plan, ResourceCache, "memory", []string{"memory", "redis"})
	assertResourceSelection(t, plan, ResourceQueue, "workerpool", []string{"workerpool", "redis"})
	assertResourceSelection(t, plan, ResourceEvents, "inproc", []string{"inproc", "redis"})
	assertResourceSelection(t, plan, ResourceStorage, "local", []string{"local"})
	assertResourceSelection(t, plan, ResourceMail, "smtp", []string{"log", "smtp"})
	assertResourceSelection(t, plan, ResourceDatabase, "mysql", []string{"mysql"})

	named := namedResourceSelectionsByKey(plan.GeneratedNamedSelections(components))
	for _, key := range []string{"CACHE_SESSIONS_DRIVER", "CACHE_INSPECTS_DRIVER", "CACHE_LIGHTHOUSE_DRIVER"} {
		if named[key].Active != "memory" {
			t.Fatalf("named cache %s = %q, want memory", key, named[key].Active)
		}
	}
	if named["STORAGE_PUBLIC_DRIVER"].Active != "local" {
		t.Fatalf("public storage must retain its generated local default: %#v", named)
	}
}

// TestDefaultResourcePlanOmitsDisabledCapabilities keeps dormant resources out of generation and service planning.
func TestDefaultResourcePlanOmitsDisabledCapabilities(t *testing.T) {
	plan, err := DefaultResourcePlan(Components{DatabaseSQLite: true})
	if err != nil {
		t.Fatalf("DefaultResourcePlan returned error: %v", err)
	}
	if _, ok := plan.Selection(ResourceQueue); ok {
		t.Fatal("Jobs-disabled plan contains a queue selection")
	}
	if _, ok := plan.Selection(ResourceMail); ok {
		t.Fatal("Mail-disabled plan contains a mail selection")
	}
	if _, ok := plan.Selection(ResourceEvents); ok {
		t.Fatal("Events-disabled plan contains an Events selection")
	}
	assertResourceSelection(t, plan, ResourceDatabase, "sqlite", []string{"sqlite"})
}

// TestResourcePlanNormalizationCannotResurrectDisabledEvents verifies stale transient selections are discarded by component applicability.
func TestResourcePlanNormalizationCannotResurrectDisabledEvents(t *testing.T) {
	components := Components{DatabaseSQLite: true}
	plan, err := DefaultResourcePlan(components)
	if err != nil {
		t.Fatalf("DefaultResourcePlan returned error: %v", err)
	}
	plan = plan.WithSelection(ResourceEvents, DriverSelection{Active: "redis", Supported: []string{"redis"}})
	normalized, err := plan.Normalized(components)
	if err != nil {
		t.Fatalf("Normalized rejected a stale disabled selection: %v", err)
	}
	if _, exists := normalized.Selection(ResourceEvents); exists {
		t.Fatalf("normalized Events-disabled plan retained stale Events selection: %#v", normalized)
	}
}

// TestResourcePlanNormalizationRejectsInvalidContracts proves active and built-in values cannot drift.
func TestResourcePlanNormalizationRejectsInvalidContracts(t *testing.T) {
	components := Components{DatabaseMySQL: true, Jobs: true}
	plan, err := DefaultResourcePlan(components)
	if err != nil {
		t.Fatalf("DefaultResourcePlan returned error: %v", err)
	}
	selection, _ := plan.Selection(ResourceQueue)
	selection.Active = "redis"
	selection.Supported = []string{"workerpool"}
	err = plan.WithSelection(ResourceQueue, selection).Validate(components)
	if err == nil || !strings.Contains(err.Error(), "not built into") {
		t.Fatalf("Validate error = %v, want active-subset failure", err)
	}

	selection.Supported = []string{"redis", "workerpool", "redis"}
	normalized, err := plan.WithSelection(ResourceQueue, selection).Normalized(components)
	if err != nil {
		t.Fatalf("Normalized returned error: %v", err)
	}
	assertResourceSelection(t, normalized, ResourceQueue, "redis", []string{"workerpool", "redis"})
}

// TestResourcePlanNormalizationRejectsUnknownKeys prevents misspelled transient state from disappearing during normalization.
func TestResourcePlanNormalizationRejectsUnknownKeys(t *testing.T) {
	components := Components{DatabaseMySQL: true}
	plan, err := DefaultResourcePlan(components)
	if err != nil {
		t.Fatalf("DefaultResourcePlan returned error: %v", err)
	}
	withUnknownResource := plan.Clone()
	withUnknownResource.Selections[ResourceKey("cahce")] = DriverSelection{Active: "memory", Supported: []string{"memory"}}
	if err := withUnknownResource.Validate(components); err == nil || !strings.Contains(err.Error(), "unknown resource") {
		t.Fatalf("unknown resource validation error = %v", err)
	}
	withUnknownNamed := plan.Clone()
	withUnknownNamed.NamedSelections["CACHE_SESSOIN_DRIVER"] = "memory"
	if err := withUnknownNamed.Validate(components); err == nil || !strings.Contains(err.Error(), "unknown generated named resource") {
		t.Fatalf("unknown named resource validation error = %v", err)
	}
}

// TestResourcePlanNormalizationCanonicalizesDatabaseAliases preserves generator-compatible owner values at the catalog boundary.
func TestResourcePlanNormalizationCanonicalizesDatabaseAliases(t *testing.T) {
	components := Components{DatabasePostgres: true}
	plan, err := DefaultResourcePlan(components)
	if err != nil {
		t.Fatalf("DefaultResourcePlan returned error: %v", err)
	}
	database, _ := plan.Selection(ResourceDatabase)
	database.Active = "postgresql"
	database.Supported = []string{"sqlite3", "postgresql", "postgres"}
	normalized, err := plan.WithSelection(ResourceDatabase, database).Normalized(components)
	if err != nil {
		t.Fatalf("Normalized rejected database aliases: %v", err)
	}
	assertResourceSelection(t, normalized, ResourceDatabase, "postgres", []string{"sqlite", "postgres"})
}

// TestResourcePlanNamedRequirementsLockRootSupport protects generated named-resource portability.
func TestResourcePlanNamedRequirementsLockRootSupport(t *testing.T) {
	components := Components{DatabaseMySQL: true, Auth: true}
	plan, err := DefaultResourcePlan(components)
	if err != nil {
		t.Fatalf("DefaultResourcePlan returned error: %v", err)
	}
	plan = plan.WithNamedSelection("CACHE_SESSIONS_DRIVER", "redis")
	cache, _ := plan.Selection(ResourceCache)
	cache.Supported = []string{"memory"}
	err = plan.WithSelection(ResourceCache, cache).Validate(components)
	if err == nil || !strings.Contains(err.Error(), "Auth sessions") {
		t.Fatalf("Validate error = %v, want named-resource owner", err)
	}

	storage, _ := plan.Selection(ResourceStorage)
	storage.Active = "s3"
	storage.Supported = []string{"s3"}
	err = plan.WithSelection(ResourceStorage, storage).Validate(components)
	if err == nil || !strings.Contains(err.Error(), "Public storage") {
		t.Fatalf("Validate error = %v, want public-storage owner", err)
	}
}

// TestDefaultResourcePlanPreservesDemoDatabaseContract keeps the current starter migration compatibility explicit.
func TestDefaultResourcePlanPreservesDemoDatabaseContract(t *testing.T) {
	components := Components{DemoApp: true, DatabaseMySQL: true}
	plan, err := DefaultResourcePlan(components)
	if err != nil {
		t.Fatalf("DefaultResourcePlan returned error: %v", err)
	}
	assertResourceSelection(t, plan, ResourceDatabase, "mysql", []string{"sqlite", "mysql"})
}

// TestResourcePlanNormalizationRequiresDemoSQLiteFallback keeps Advanced edits from narrowing the Demo database build contract.
func TestResourcePlanNormalizationRequiresDemoSQLiteFallback(t *testing.T) {
	components := Components{DemoApp: true, DatabaseMySQL: true}
	plan, err := DefaultResourcePlan(components)
	if err != nil {
		t.Fatalf("DefaultResourcePlan returned error: %v", err)
	}
	database, _ := plan.Selection(ResourceDatabase)
	database.Supported = []string{"mysql"}
	if err := plan.WithSelection(ResourceDatabase, database).Validate(components); err == nil || !strings.Contains(err.Error(), "SQLite database fallback") {
		t.Fatalf("Validate error = %v, want required Demo SQLite fallback", err)
	}
}

// assertResourceSelection compares one plan entry without exposing map aliasing to each test.
func assertResourceSelection(t *testing.T, plan ResourcePlan, key ResourceKey, active string, supported []string) {
	t.Helper()
	selection, ok := plan.Selection(key)
	if !ok {
		t.Fatalf("resource %s missing", key)
	}
	if selection.Active != active || !reflect.DeepEqual(selection.Supported, supported) {
		t.Fatalf("resource %s = %#v, want active %q supported %#v", key, selection, active, supported)
	}
}

// namedResourceSelectionsByKey indexes generated named selections by their environment assignment.
func namedResourceSelectionsByKey(selections []GeneratedNamedResourceSelection) map[string]GeneratedNamedResourceSelection {
	indexed := make(map[string]GeneratedNamedResourceSelection, len(selections))
	for _, selection := range selections {
		indexed[selection.EnvironmentKey] = selection
	}
	return indexed
}
