package stacks

import (
	"reflect"
	"testing"
)

// TestResourcesForRetainsProjectAndSelectedDeclarations keeps saved-only providers visible without changing the active session.
func TestResourcesForRetainsProjectAndSelectedDeclarations(t *testing.T) {
	root := fixture(t)
	put(t, root, ".env.example", "DB_EXAMPLE_DATABASE=example\n")
	put(t, root, ".env.stack.services", "DB_DRIVER=sqlite\nDB_SUPPORTED_DRIVERS=sqlite,postgres\nDB_ANALYTICS_DRIVER=postgres\nADMIN_DB_REPORTS_DRIVER=postgres\n")
	s := openTest(t, root)
	before := make([]string, 0, len(s.Resources))
	for _, resource := range s.Resources {
		before = append(before, resource.Key)
	}
	current := clone(s.Current)
	values, err := s.Load("services", false)
	if err != nil {
		t.Fatal(err)
	}
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

// TestResourcesIncludeNormalizedAppDependencies keeps the wizard and portable presets aligned with generated Auth and OAuth database support.
func TestResourcesIncludeNormalizedAppDependencies(t *testing.T) {
	for _, component := range []string{"auth", "oauth"} {
		t.Run(component, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".goforj.yml", "project_name: stacks\nmodule_name: example.org/stacks\nrender:\n  components:\n    database_sqlite: true\napps:\n  accounts:\n    components:\n      "+component+": true\n  utility:\n    components:\n      cli: true\n")
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
			if err := s.SetDriver(values, inventory["ACCOUNTS_DB_DRIVER"], "postgres"); err != nil {
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
