// Package resourceenv_test checks resource inventories against the generator's configuration contract.
package resourceenv_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/internal/resourceenv"
	"github.com/goforj/goforj/project"
	"github.com/joho/godotenv"
)

// TestFeatureSettingConsumerInventory matches generation for evaluators declared without a driver override.
func TestFeatureSettingConsumerInventory(t *testing.T) {
	for _, setting := range []struct{ key, value string }{
		{"DOCUMENT", `{"schema":1,"flags":{}}`},
		{"PATH", "storage/flags/checkout.json"},
		{"REFRESH_INTERVAL", "30s"},
		{"INITIALIZATION_TIMEOUT", "5s"},
	} {
		for _, scope := range []string{"default", "billing", "reports", "unknown"} {
			t.Run(scope+"/"+setting.key, func(t *testing.T) {
				prefix := ""
				if scope != "default" {
					prefix = map[string]string{"billing": "BILLING_", "reports": "REPORTS_", "unknown": "UNKNOWN_"}[scope]
				}
				values := map[string]string{prefix + "FEATURE_CHECKOUT_" + setting.key: setting.value}
				want := map[string]string{"feature": "memory", "billing:feature": "memory"}
				var names []string
				if scope == "default" || scope == "billing" {
					names = []string{"checkout"}
					want["billing:feature:checkout"] = "memory"
				}
				if scope == "default" {
					want["feature:checkout"] = "memory"
				}
				assertFeatureSettingInventory(t, values, names, want)
			})
		}
	}
}

// TestFeatureSettingConsumerInventoryRejectsPhantoms keeps root settings and unknown suffixes out of named inventory.
func TestFeatureSettingConsumerInventoryRejectsPhantoms(t *testing.T) {
	assertFeatureSettingInventory(t, map[string]string{
		"FEATURE_DOCUMENT":                   `{"schema":1,"flags":{}}`,
		"FEATURE_PATH":                       "storage/flags/default.json",
		"FEATURE_REFRESH_INTERVAL":           "30s",
		"FEATURE_INITIALIZATION_TIMEOUT":     "5s",
		"FEATURE_CHECKOUT_CUSTOM_SETTING":    "retained",
		"BILLING_FEATURE_CHECKOUT_UNRELATED": "retained",
	}, nil, map[string]string{"feature": "memory", "billing:feature": "memory"})
}

// TestFeatureSettingConsumerInventoryKeepsDriverOverrides retains the effective driver when declaration keys overlap.
func TestFeatureSettingConsumerInventoryKeepsDriverOverrides(t *testing.T) {
	assertFeatureSettingInventory(t, map[string]string{
		"FEATURE_CHECKOUT_PATH":                     "storage/flags/checkout.json",
		"FEATURE_CHECKOUT_DRIVER":                   "file",
		"BILLING_FEATURE_CHECKOUT_DOCUMENT":         `{"schema":1,"flags":{}}`,
		"BILLING_FEATURE_CHECKOUT_DRIVER":           "memory",
		"BILLING_FEATURE_CHECKOUT_REFRESH_INTERVAL": "30s",
	}, []string{"checkout"}, map[string]string{
		"feature": "memory", "feature:checkout": "file",
		"billing:feature": "memory", "billing:feature:checkout": "memory",
	})
}

// assertFeatureSettingInventory compares the generator's configured evaluator names with describe's effective consumers.
func assertFeatureSettingInventory(t *testing.T, values map[string]string, names []string, want map[string]string) {
	t.Helper()
	root := t.TempDir()
	config := &project.Config{
		Render: project.RenderConfig{Components: project.Components{CLI: true, Feature: true}},
		Apps: map[string]project.AppConfig{
			"billing": {Components: project.Components{CLI: true, Feature: true}},
			"reports": {Components: project.Components{CLI: true}},
		},
	}
	configuration := "render:\n  components: [cli, feature]\napps:\n  billing:\n    components: [cli, feature]\n  reports:\n    components: [cli]\n"
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte(configuration), 0600); err != nil {
		t.Fatal(err)
	}
	gotNames := generate.ResourceNames(root, values)[project.ResourceFeature]
	if len(gotNames) != len(names) || (len(names) > 0 && !reflect.DeepEqual(gotNames, names)) {
		t.Fatalf("generated evaluator names = %v, want %v", gotNames, names)
	}
	source, err := godotenv.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	components := project.ProjectComponents(config)
	plan, err := project.DefaultResourcePlan(components)
	if err != nil {
		t.Fatal(err)
	}
	consumers, err := resourceenv.ResolveConsumers([]byte(source), plan, components, config)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, consumer := range consumers {
		got[consumer.Consumer] = consumer.Driver
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("effective consumers = %#v, want %#v", got, want)
	}
}
