package generate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestResourceNamesByAppUsesProspectiveConfig preserves on-disk configuration while inventorying configured, conventional, and Dev Apps.
func TestResourceNamesByAppUsesProspectiveConfig(t *testing.T) {
	root := featureInventoryProject(t, "apps:\n  disabled:\n    components: [cli]\n", "conventional")
	path := filepath.Join(root, ".goforj.yml")
	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	config := &project.Config{
		Render: project.RenderConfig{Components: project.Components{CLI: true, Feature: true, Cache: true}},
		Apps: map[string]project.AppConfig{
			"billing":         {Components: project.Components{CLI: true, Feature: true, Cache: true}},
			"billing-reports": {Components: project.Components{CLI: true, Feature: true}},
			"cache-billing":   {Components: project.Components{CLI: true, Feature: true}},
			"feature-feature": {Components: project.Components{CLI: true, Feature: true}},
			"disabled":        {Components: project.Components{CLI: true}},
		},
		Dev: project.DevConfig{Apps: map[string]project.DevApp{"dev-only": {}}},
	}
	values := map[string]string{
		"FEATURE_SHARED_DRIVER":                  "memory",
		"BILLING_FEATURE_CHECKOUT_DRIVER":        "memory",
		"BILLING_REPORTS_FEATURE_AUDIT_PATH":     "flags.json",
		"CACHE_BILLING_FEATURE_PAYMENT_DRIVER":   "memory",
		"FEATURE_FEATURE_FEATURE_RELEASE_DRIVER": "memory",
		"DISABLED_FEATURE_HIDDEN_DRIVER":         "memory",
		"MISSING_FEATURE_STALE_DRIVER":           "memory",
		"CACHE_SESSIONS_DRIVER":                  "memory",
		"BILLING_CACHE_CART_DRIVER":              "memory",
	}
	before, err := json.Marshal(struct {
		Config *project.Config
		Values map[string]string
	}{config, values})
	if err != nil {
		t.Fatal(err)
	}
	names := ResourceNamesByApp(root, values, config)
	wantFeature := map[string][]string{
		"": {"shared"}, "BILLING": {"checkout", "shared"}, "BILLING_REPORTS": {"audit", "shared"},
		"CACHE_BILLING": {"payment", "shared"}, "FEATURE_FEATURE": {"release", "shared"},
		"CONVENTIONAL": {"shared"}, "DEV_ONLY": {"shared"},
	}
	if !reflect.DeepEqual(names[project.ResourceFeature], wantFeature) {
		t.Fatalf("feature scopes=%#v want=%#v", names[project.ResourceFeature], wantFeature)
	}
	wantCache := map[string][]string{"": {"cart", "sessions"}, "BILLING": {"cart", "sessions"}, "CONVENTIONAL": {"cart", "sessions"}, "DEV_ONLY": {"cart", "sessions"}}
	if !reflect.DeepEqual(names[project.ResourceCache], wantCache) {
		t.Fatalf("cache scopes=%#v want=%#v", names[project.ResourceCache], wantCache)
	}
	after, err := json.Marshal(struct {
		Config *project.Config
		Values map[string]string
	}{config, values})
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("inventory mutated prospective input")
	}
	actual, err := os.ReadFile(path)
	if err != nil || string(actual) != string(disk) {
		t.Fatalf("disk config changed: %v", err)
	}
	if err := os.WriteFile(path, []byte("invalid: ["), 0600); err != nil {
		t.Fatal(err)
	}
	if got := ResourceNamesByApp(root, values, config); !reflect.DeepEqual(got, names) {
		t.Fatal("prospective inventory reloaded invalid disk config")
	}
}

// TestResourceNamesByAppDetectsLostFeatureRegistration distinguishes per-App registrations from the global accessor union.
func TestResourceNamesByAppDetectsLostFeatureRegistration(t *testing.T) {
	root := t.TempDir()
	config := &project.Config{Render: project.RenderConfig{Components: project.Components{CLI: true, Feature: true}}, Apps: map[string]project.AppConfig{
		"billing": {Components: project.Components{CLI: true, Feature: true}},
		"reports": {Components: project.Components{CLI: true, Feature: true}},
	}}
	values := map[string]string{"BILLING_FEATURE_CHECKOUT_DRIVER": "memory", "REPORTS_FEATURE_CHECKOUT_DRIVER": "memory"}
	before := ResourceNamesByApp(root, values, config)
	delete(values, "BILLING_FEATURE_CHECKOUT_DRIVER")
	after := ResourceNamesByApp(root, values, config)
	if !slices.Equal(before[project.ResourceFeature]["BILLING"], []string{"checkout"}) || len(after[project.ResourceFeature]["BILLING"]) != 0 || !slices.Equal(after[project.ResourceFeature]["REPORTS"], []string{"checkout"}) {
		t.Fatalf("registration transition: before=%#v after=%#v", before, after)
	}
	input := generationInput{projectDir: root, environment: generationEnvironment{values: values}, config: config}
	input.appPrefixes = sortStrings(input.configuredAppEnvPrefixes())
	if !slices.Equal(discoverFeatureNames(input), []string{"checkout"}) {
		t.Fatal("global accessor unexpectedly lost")
	}
}

// TestResourceNamesByAppLeavesLegacyDiscoveryUnchanged keeps configless resource discovery compatible after prospective inventory.
func TestResourceNamesByAppLeavesLegacyDiscoveryUnchanged(t *testing.T) {
	root := t.TempDir()
	values := map[string]string{"BILLING_CACHE_SESSIONS_DRIVER": "memory", "DB_SQLITE_DATABASE": "app.db", "FEATURE_CHECKOUT_PATH": "flags.json"}
	before := ResourceNames(root, values)
	ResourceNamesByApp(root, values, &project.Config{})
	if after := ResourceNames(root, values); !reflect.DeepEqual(before, after) {
		t.Fatalf("legacy inventory changed: before=%#v after=%#v", before, after)
	}
	if !slices.Equal(before[project.ResourceCache], []string{"sessions"}) {
		t.Fatalf("legacy cache inventory=%#v", before)
	}
}

// TestResourceNamesByAppLegacyManagersShareUnion registers each legacy manager's global names in every participating scope.
func TestResourceNamesByAppLegacyManagersShareUnion(t *testing.T) {
	root := t.TempDir()
	components := project.Components{CLI: true, Cache: true, DatabaseSQLite: true, Jobs: true, Events: true, Storage: true, Mail: true}
	config := &project.Config{Render: project.RenderConfig{Components: components}, Apps: map[string]project.AppConfig{
		"billing": {Components: components}, "reports": {Components: components}, "disabled": {Components: project.Components{CLI: true}},
	}}
	values := map[string]string{}
	for _, prefix := range []string{"CACHE", "DB", "QUEUE", "EVENTS", "STORAGE", "MAIL"} {
		values[prefix+"_PRIMARY_DRIVER"] = "unused"
		values["BILLING_"+prefix+"_SECONDARY_DRIVER"] = "unused"
	}
	names := ResourceNamesByApp(root, values, config)
	for _, key := range []project.ResourceKey{project.ResourceCache, project.ResourceDatabase, project.ResourceQueue, project.ResourceEvents, project.ResourceStorage, project.ResourceMail} {
		want := map[string][]string{"": {"primary", "secondary"}, "BILLING": {"primary", "secondary"}, "REPORTS": {"primary", "secondary"}}
		if !reflect.DeepEqual(names[key], want) {
			t.Fatalf("%s scopes=%#v want=%#v", key, names[key], want)
		}
	}
	config.Render.Components = project.Components{CLI: true}
	names = ResourceNamesByApp(root, values, config)
	for key, scopes := range names {
		if _, exists := scopes[""]; exists {
			t.Fatalf("disabled default %s scope retained: %#v", key, scopes)
		}
	}
}

// TestResourceNamesByAppOmitsRemovedPhysicalApp preserves legacy resources when a removed App would otherwise inherit Feature participation.
func TestResourceNamesByAppOmitsRemovedPhysicalApp(t *testing.T) {
	root := featureInventoryProject(t, "apps:\n  cache:\n    components: [cli, cache]\n", "cache")
	before, err := project.LoadProjectConfigAt(root)
	if err != nil {
		t.Fatal(err)
	}
	before.Render.Components.Cache = true
	before.Dev.Apps = map[string]project.DevApp{"cache": {}}
	values := map[string]string{"CACHE_FEATURE_DRIVER": "memory"}
	original := ResourceNamesByApp(root, values, before)
	candidate := *before
	candidate.Apps = nil
	candidate.Dev.Apps = nil
	projected := ResourceNamesByApp(root, values, &candidate, "cache")
	if !slices.Equal(original[project.ResourceCache][""], []string{"feature"}) || !slices.Equal(projected[project.ResourceCache][""], []string{"feature"}) {
		t.Fatalf("legacy Feature accessor lost: before=%#v after=%#v", original, projected)
	}
	if _, exists := projected[project.ResourceFeature]["CACHE"]; exists {
		t.Fatalf("removed App inherited Feature: %#v", projected)
	}
}

// TestResourceNamesByAppOmissionMatchesDeletedInventory keeps prospective namespace ownership identical to actual filesystem removal.
func TestResourceNamesByAppOmissionMatchesDeletedInventory(t *testing.T) {
	for _, name := range []string{"cache", "cache-billing", "feature-feature", "billing", "billing-feature"} {
		t.Run(name, func(t *testing.T) {
			root := featureInventoryProject(t, "", name, "billing-feature-reports")
			config, err := project.LoadProjectConfigAt(root)
			if err != nil {
				t.Fatal(err)
			}
			values := map[string]string{
				"CACHE_FEATURE_DRIVER":                          "memory",
				"CACHE_BILLING_FEATURE_PAYMENT_DRIVER":          "memory",
				"FEATURE_FEATURE_FEATURE_RELEASE_DRIVER":        "memory",
				"BILLING_FEATURE_CHECKOUT_DRIVER":               "memory",
				"BILLING_FEATURE_FEATURE_AUDIT_DRIVER":          "memory",
				"BILLING_FEATURE_REPORTS_FEATURE_SHARED_DRIVER": "memory",
			}
			projected := ResourceNamesByApp(root, values, config, name)
			if err := os.RemoveAll(filepath.Join(root, "cmd", name)); err != nil {
				t.Fatal(err)
			}
			actual := ResourceNamesByApp(root, values, config)
			if !reflect.DeepEqual(projected, actual) {
				t.Fatalf("prospective=%#v post-delete=%#v", projected, actual)
			}
		})
	}
}
