package project

import (
	"reflect"
	"strings"
	"testing"
)

// TestResolveResourcePlanNormalShapes locks the compact wizard presets to concrete driver contracts.
func TestResolveResourcePlanNormalShapes(t *testing.T) {
	components := Components{DatabaseMySQL: true, Mail: true, Docker: true, Jobs: true, Auth: true}
	tests := []struct {
		name        string
		shape       StartingResourceShape
		wantCache   string
		wantQueue   string
		wantEvents  string
		wantSession string
	}{
		{name: "standalone", shape: ResourceShapeStandalone, wantCache: "memory", wantQueue: "workerpool", wantEvents: "inproc", wantSession: "memory"},
		{name: "shared", shape: ResourceShapeSharedRedis, wantCache: "redis", wantQueue: "redis", wantEvents: "redis", wantSession: "redis"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan, err := ResolveResourcePlan(test.shape, components)
			if err != nil {
				t.Fatalf("ResolveResourcePlan returned error: %v", err)
			}
			assertResourceSelection(t, plan, ResourceCache, test.wantCache, []string{"memory", "redis"})
			assertResourceSelection(t, plan, ResourceQueue, test.wantQueue, []string{"workerpool", "redis"})
			assertResourceSelection(t, plan, ResourceEvents, test.wantEvents, []string{"inproc", "redis"})
			assertResourceSelection(t, plan, ResourceStorage, "local", []string{"local"})
			assertResourceSelection(t, plan, ResourceMail, "smtp", []string{"log", "smtp"})
			assertResourceSelection(t, plan, ResourceDatabase, "mysql", []string{"mysql"})
			named := namedResourceSelectionsByKey(plan.GeneratedNamedSelections(components))
			if named["CACHE_SESSIONS_DRIVER"].Active != test.wantSession {
				t.Fatalf("sessions driver = %q, want %q", named["CACHE_SESSIONS_DRIVER"].Active, test.wantSession)
			}
			if named["CACHE_INSPECTS_DRIVER"].Active != "memory" || named["CACHE_LIGHTHOUSE_DRIVER"].Active != "memory" {
				t.Fatalf("diagnostic named caches must remain memory: %#v", named)
			}
			if named["STORAGE_PUBLIC_DRIVER"].Active != "local" {
				t.Fatalf("public storage must retain its generated local default: %#v", named)
			}
		})
	}
}

// TestResolveResourcePlanOmitsDisabledCapabilities keeps dormant resources out of generation and service planning.
func TestResolveResourcePlanOmitsDisabledCapabilities(t *testing.T) {
	plan, err := ResolveResourcePlan(ResourceShapeSharedRedis, Components{DatabaseSQLite: true})
	if err != nil {
		t.Fatalf("ResolveResourcePlan returned error: %v", err)
	}
	if _, ok := plan.Selection(ResourceQueue); ok {
		t.Fatal("Jobs-disabled plan contains a queue selection")
	}
	if _, ok := plan.Selection(ResourceMail); ok {
		t.Fatal("Mail-disabled plan contains a mail selection")
	}
	assertResourceSelection(t, plan, ResourceDatabase, "sqlite", []string{"sqlite"})
}

// TestResourcePlanNormalizationRejectsInvalidContracts proves active and built-in values cannot drift.
func TestResourcePlanNormalizationRejectsInvalidContracts(t *testing.T) {
	components := Components{DatabaseMySQL: true, Jobs: true}
	plan, err := ResolveResourcePlan(ResourceShapeStandalone, components)
	if err != nil {
		t.Fatalf("ResolveResourcePlan returned error: %v", err)
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
	plan, err := ResolveResourcePlan(ResourceShapeStandalone, components)
	if err != nil {
		t.Fatalf("ResolveResourcePlan returned error: %v", err)
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
	plan, err := ResolveResourcePlan(ResourceShapeStandalone, components)
	if err != nil {
		t.Fatalf("ResolveResourcePlan returned error: %v", err)
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

// TestResourcePlanNamedRequirementsLockRootSupport protects generated Auth session portability.
func TestResourcePlanNamedRequirementsLockRootSupport(t *testing.T) {
	components := Components{DatabaseMySQL: true, Auth: true}
	plan, err := ResolveResourcePlan(ResourceShapeSharedRedis, components)
	if err != nil {
		t.Fatalf("ResolveResourcePlan returned error: %v", err)
	}
	cache, _ := plan.Selection(ResourceCache)
	cache.Active = "memory"
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

// TestClassifyResourcePlan distinguishes normal, support-only, independent, and active-driver edits.
func TestClassifyResourcePlan(t *testing.T) {
	components := Components{DatabaseMySQL: true, Mail: true, Docker: true, Jobs: true}
	plan, err := ResolveResourcePlan(ResourceShapeStandalone, components)
	if err != nil {
		t.Fatalf("ResolveResourcePlan returned error: %v", err)
	}
	classification := ClassifyResourcePlan(plan, components)
	if classification.Label != "Standalone resources" || classification.Custom {
		t.Fatalf("classification = %#v", classification)
	}

	cache, _ := plan.Selection(ResourceCache)
	cache.Supported = append(cache.Supported, "file")
	classification = ClassifyResourcePlan(plan.WithSelection(ResourceCache, cache), components)
	if classification.Label != "Standalone resources · custom support" {
		t.Fatalf("support classification = %#v", classification)
	}

	storage, _ := plan.Selection(ResourceStorage)
	storage.Active = "memory"
	storage.Supported = []string{"local", "memory"}
	classification = ClassifyResourcePlan(plan.WithSelection(ResourceStorage, storage), components)
	if classification.Label != "Standalone resources · customized" {
		t.Fatalf("storage classification = %#v", classification)
	}

	events, _ := plan.Selection(ResourceEvents)
	events.Active = "redis"
	classification = ClassifyResourcePlan(plan.WithSelection(ResourceEvents, events), components)
	if classification.Label != "Custom" || !classification.Custom {
		t.Fatalf("active classification = %#v", classification)
	}
}

// TestResolveResourcePlanPreservesDemoDatabaseContract keeps the current starter migration compatibility explicit.
func TestResolveResourcePlanPreservesDemoDatabaseContract(t *testing.T) {
	components := Components{DemoApp: true, DatabaseMySQL: true}
	plan, err := ResolveResourcePlan(ResourceShapeStandalone, components)
	if err != nil {
		t.Fatalf("ResolveResourcePlan returned error: %v", err)
	}
	assertResourceSelection(t, plan, ResourceDatabase, "mysql", []string{"sqlite", "mysql"})
}

// TestResourcePlanNormalizationRequiresDemoSQLiteFallback keeps Advanced edits from narrowing the Demo database build contract.
func TestResourcePlanNormalizationRequiresDemoSQLiteFallback(t *testing.T) {
	components := Components{DemoApp: true, DatabaseMySQL: true}
	plan, err := ResolveResourcePlan(ResourceShapeStandalone, components)
	if err != nil {
		t.Fatalf("ResolveResourcePlan returned error: %v", err)
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
