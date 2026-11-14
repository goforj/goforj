package localproviders

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/project"
)

// TestResourcesForRetainsProjectAndSelectedDeclarations keeps preview resources visible without changing the active session.
func TestResourcesForRetainsProjectAndSelectedDeclarations(t *testing.T) {
	root := fixture(t)
	put(t, root, ".env.example", "DB_EXAMPLE_DATABASE=example\n")

	s := openTest(t, root)
	before := make([]string, 0, len(s.Resources))
	for _, resource := range s.Resources {
		before = append(before, resource.Key)
	}
	current := clone(s.Current)
	values := map[string]string{"DB_ANALYTICS_DRIVER": "postgres", "ADMIN_DB_REPORTS_DRIVER": "postgres"}
	got := map[string]bool{}
	for _, resource := range s.ResourcesFor(values) {
		if got[resource.Key] {
			t.Fatalf("duplicate resource: %s", resource.Key)
		}
		got[resource.Key] = true
	}
	for _, key := range []string{"DB_DRIVER", "DB_EXAMPLE_DRIVER", "DB_ANALYTICS_DRIVER", "ADMIN_DB_REPORTS_DRIVER", "CACHE_DRIVER"} {
		if !got[key] {
			t.Errorf("missing %s", key)
		}
	}
	after := make([]string, 0, len(s.Resources))
	for _, resource := range s.Resources {
		after = append(after, resource.Key)
	}
	if !reflect.DeepEqual(after, before) || !reflect.DeepEqual(s.Current, current) {
		t.Fatal("selected inventory changed active session")
	}
}

// TestResourceInventoriesPreserveAppOnlyNames protects previews and portable conversion from widening ownership.
func TestResourceInventoriesPreserveAppOnlyNames(t *testing.T) {
	for _, definition := range project.ResourceCatalog() {
		if definition.Key != project.ResourceFeature && definition.Key != project.ResourceNotification && definition.Key != project.ResourceKey("search") {
			continue
		}
		t.Run(string(definition.Key), func(t *testing.T) {
			root := fixture(t)
			component := string(definition.Key)
			put(t, root, ".goforj.yml", "module_name: example.org/scoped\nrender:\n  components: [cli, "+component+"]\napps:\n  admin:\n    components: [cli, "+component+"]\n  reports:\n    components: [cli, "+component+"]\n")
			prefix := definition.EnvironmentPrefix + "_"
			put(t, root, ".env", prefix+"SHARED_DRIVER="+definition.DefaultDriver+"\nADMIN_"+prefix+"ALERTS_DRIVER="+definition.DefaultDriver+"\n")
			put(t, root, ".env.example", "ADMIN_"+prefix+"EXAMPLE_DRIVER="+definition.DefaultDriver+"\n")
			s := openTest(t, root)
			portable, err := s.Portable()
			if err != nil {
				t.Fatal(err)
			}
			preview := clone(portable)
			preview["ADMIN_"+prefix+"PREVIEW_DRIVER"] = definition.DefaultDriver
			inventory := map[string]bool{}
			for _, resource := range s.ResourcesFor(preview) {
				inventory[resource.Key] = true
			}
			for _, name := range []string{"ALERTS", "EXAMPLE", "PREVIEW"} {
				for _, scope := range []string{"", "REPORTS_"} {
					key := scope + prefix + name + "_DRIVER"
					if _, present := portable[key]; present || inventory[key] {
						t.Fatalf("App-only declaration widened to %s", key)
					}
				}
				if !inventory["ADMIN_"+prefix+name+"_DRIVER"] {
					t.Fatalf("owner lost %s", name)
				}
			}
			for _, scope := range []string{"", "ADMIN_", "REPORTS_"} {
				if !inventory[scope+prefix+"SHARED_DRIVER"] {
					t.Fatalf("root inheritance lost for %s", scope)
				}
			}
		})
	}
}

// TestPortableRegenerationKeepsAppOnlyResourcesScoped checks portable regeneration and generated resource ownership twice.
func TestPortableRegenerationKeepsAppOnlyResourcesScoped(t *testing.T) {
	for _, test := range []struct {
		component, prefix, path string
		generate                func(string) (int, error)
	}{
		{"feature", "FEATURE", generate.FeatureManagerPath, generate.GenerateFeatureFiles},
		{"notification", "NOTIFICATION", generate.NotificationManagerPath, generate.GenerateNotificationFiles},
		{"search", "SEARCH", generate.SearchManagerPath, generate.GenerateSearchFiles},
	} {
		t.Run(test.component, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".goforj.yml", "module_name: example.org/scoped\nrender:\n  components: [cli, "+test.component+"]\napps:\n  admin:\n    components: [cli, "+test.component+"]\n  reports:\n    components: [cli, "+test.component+"]\n")
			driver := "memory"
			if test.component == "notification" {
				driver = "log"
			} else if test.component == "search" {
				driver = "bleve"
			}
			put(t, root, ".env", "ADMIN_"+test.prefix+"_ALERTS_DRIVER="+driver+"\n")
			s := openTest(t, root)
			portable, err := s.Portable()
			if err != nil {
				t.Fatal(err)
			}
			for key, value := range portable {
				t.Setenv(key, value)
			}
			if _, err := test.generate(root); err != nil {
				t.Fatal(err)
			}
			if changed, err := test.generate(root); err != nil || changed != 0 {
				t.Fatalf("regeneration: %d, %v", changed, err)
			}
			source, err := os.ReadFile(filepath.Join(root, test.path))
			if err != nil {
				t.Fatal(err)
			}
			if test.component == "search" {
				accessors, err := os.ReadFile(filepath.Join(root, generate.SearchAccessorsPath))
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(source), "func NewManagerForApp(") || !strings.Contains(string(accessors), "func (manager *Manager) Alerts()") {
					t.Fatal("search lost explicit App ownership or its declared accessor")
				}
				return
			}
			for _, fragment := range []string{`"": {}`, `"REPORTS": {}`, `"ADMIN": {"ALERTS"}`} {
				if !strings.Contains(strings.ReplaceAll(strings.Join(strings.Fields(string(source)), ""), ",", ""), strings.Join(strings.Fields(fragment), "")) {
					t.Fatalf("missing scoped map entry %s", fragment)
				}
			}
		})
	}
}

// TestResourcesIncludeNormalizedAppDependencies keeps the wizard and portable presets aligned with generated Auth and OAuth database support.
func TestResourcesIncludeNormalizedAppDependencies(t *testing.T) {
	for _, component := range []string{"auth", "oauth"} {
		t.Run(component, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".goforj.yml", "project_name: local\nmodule_name: example.org/local\nrender:\n  components:\n    database_sqlite: true\napps:\n  accounts:\n    components:\n      "+component+": true\n  utility:\n    components:\n      cli: true\n")
			put(t, root, ".env", "DB_DRIVER=sqlite\nDB_REPORTS_DATABASE=reports.db\n")
			s := openTest(t, root)
			inventory := map[string]Resource{}
			for _, resource := range s.Resources {
				inventory[resource.Key] = resource
			}
			for _, key := range []string{"ACCOUNTS_DB_DRIVER", "ACCOUNTS_DB_REPORTS_DRIVER", "ACCOUNTS_CACHE_DRIVER", "ACCOUNTS_MAIL_DRIVER"} {
				if _, found := inventory[key]; !found {
					t.Fatalf("normalized App resource missing: %s", key)
				}
			}
			if _, found := inventory["UTILITY_DB_DRIVER"]; found {
				t.Fatal("database added to an App without database dependencies")
			}
			values := clone(s.Current)
			if err := s.selectDriver(values, inventory["ACCOUNTS_DB_DRIVER"], "postgres"); err != nil {
				t.Fatal(err)
			}
			if values["ACCOUNTS_DB_DRIVER"] != "postgres" || values["DB_DRIVER"] != "sqlite" {
				t.Fatal("App driver edit changed the root selection")
			}
			portable, err := s.Portable()
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"ACCOUNTS_DB_DRIVER", "ACCOUNTS_DB_REPORTS_DRIVER"} {
				if portable[key] != "sqlite" {
					t.Fatalf("portable preset omitted %s", key)
				}
			}
		})
	}
}
