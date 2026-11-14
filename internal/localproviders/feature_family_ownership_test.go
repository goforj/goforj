package localproviders

import (
	"reflect"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestFeatureProviderFamilyOwnership aligns driver choices and validation with the generated managers' key ownership.
func TestFeatureProviderFamilyOwnership(t *testing.T) {
	for _, test := range []struct {
		name, components, app, appComponents, environment, key, scope string
		family                                                        project.ResourceKey
	}{
		{"queue App Feature", "cli, jobs", "queue", "cli, feature", "QUEUE_DRIVER=workerpool\nQUEUE_FEATURE_DRIVER=file\nQUEUE_FEATURE_PATH=flags.json\n", "QUEUE_FEATURE_DRIVER", "QUEUE_", project.ResourceFeature},
		{"Feature App Cache", "cli, feature", "feature", "cli, feature, cache", "FEATURE_DRIVER=memory\nFEATURE_CACHE_DRIVER=redis\nCACHE_SUPPORTED_DRIVERS=memory,redis\n", "FEATURE_CACHE_DRIVER", "FEATURE_", project.ResourceCache},
		{"database App Feature", "cli, database_sqlite", "db", "cli, feature", "DB_DRIVER=sqlite\nDB_FEATURE_DRIVER=file\nDB_FEATURE_PATH=flags.json\n", "DB_FEATURE_DRIVER", "DB_", project.ResourceFeature},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".goforj.yml", "module_name: example.org/scopes\nrender:\n  components: ["+test.components+"]\napps:\n  "+test.app+":\n    components: ["+test.appComponents+"]\n")
			put(t, root, "flags.json", `{"schema":1,"flags":{}}`)
			put(t, root, ".env", test.environment)
			session := openTest(t, root)
			definitions := resourceDefinitions(session.config, test.key)
			if len(definitions) != 1 || definitions[0].Key != test.family {
				t.Fatalf("resource definitions = %#v, want only %s", definitions, test.family)
			}
			if scopes := session.resourceScopes(test.key, definitions[0]); !reflect.DeepEqual(scopes, []string{test.scope}) {
				t.Fatalf("resource scopes = %v, want %s", scopes, test.scope)
			}
			found := false
			for _, resource := range session.Resources {
				if resource.Key == test.key {
					found = true
					if resource.Definition.Key != test.family || len(resource.Definition.Drivers) == 0 {
						t.Fatalf("invalid picker resource: %#v", resource)
					}
				}
			}
			if !found {
				t.Fatal("missing owned App resource")
			}
			if test.family == project.ResourceFeature && len(session.databaseScopes(test.key)) != 0 {
				t.Fatal("App Feature inherited a database target")
			}
			portable, err := session.Portable()
			if err != nil {
				t.Fatal(err)
			}
			if portable[test.key] != "memory" {
				t.Fatalf("portable owned driver = %q, want memory", portable[test.key])
			}
			if test.app == "queue" && portable["QUEUE_DRIVER"] != "workerpool" {
				t.Fatal("App Feature conversion changed root Queue")
			}
			if err := session.Validate(portable); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestDisabledFeatureAppRetainsLegacyProviderInventory preserves each legacy family's active named resource interpretation.
func TestDisabledFeatureAppRetainsLegacyProviderInventory(t *testing.T) {
	for _, test := range []struct{ component, app, prefix, driver string }{
		{"cache", "cache", "CACHE", "file"},
		{"database_sqlite", "db", "DB", "sqlite"},
		{"jobs", "queue", "QUEUE", "workerpool"},
		{"events", "events", "EVENTS", "inproc"},
		{"storage", "storage", "STORAGE", "local"},
		{"mail", "mail", "MAIL", "log"},
	} {
		t.Run(test.component, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".goforj.yml", "module_name: example.org/legacy\nrender:\n  components: [cli, "+test.component+"]\napps:\n  "+test.app+":\n    components: [cli]\n")
			key := test.prefix + "_FEATURE_DRIVER"
			put(t, root, ".env", key+"="+test.driver+"\n")
			session := openTest(t, root)
			found := false
			for _, resource := range session.Resources {
				if resource.Key == key {
					found = true
					if resource.Definition.Key == project.ResourceFeature {
						t.Fatal("disabled App Feature claimed a legacy resource")
					}
				}
			}
			if !found {
				t.Fatal("legacy named resource missing")
			}
			if _, err := session.Portable(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestFeatureProviderSettingsRespectOwnership keeps inherited App settings aligned with the runtime Feature overlay.
func TestFeatureProviderSettingsRespectOwnership(t *testing.T) {
	config := &project.Config{
		Render: project.RenderConfig{Components: project.Components{CLI: true, Feature: true}},
		Apps: map[string]project.AppConfig{
			"feature":         {Components: project.Components{CLI: true, Feature: true, Cache: true}},
			"billing":         {Components: project.Components{CLI: true, Feature: true}},
			"feature-feature": {Components: project.Components{CLI: true}},
		},
	}
	session := &Session{config: config}
	definition, _ := project.ResourceDefinitionByKey(project.ResourceFeature)
	for _, test := range []struct{ name, child, base, overlay, want string }{
		{"other App owns base", "CACHE", "redis", "", "memory"},
		{"explicit App override", "CACHE", "redis", "file", "file"},
		{"empty inherits", "CHECKOUT", "file", "", "file"},
		{"whitespace defaults", "CHECKOUT", "file", " \t ", "memory"},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := "FEATURE_" + test.child + "_DRIVER"
			key := "BILLING_" + base
			values := map[string]string{base: test.base, key: test.overlay}
			if got := session.resourceSetting(values, key, "DRIVER", "BILLING_", definition); got != test.want {
				t.Fatalf("effective driver = %q, want %q", got, test.want)
			}
		})
	}
	if scopes := session.resourceScopes("FEATURE_FEATURE_FEATURE_DRIVER", definition); len(scopes) != 0 {
		t.Fatalf("disabled longer App acquired active scopes: %v", scopes)
	}
}
