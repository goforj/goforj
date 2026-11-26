package localproviders

import (
	"slices"
	"testing"

	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/project"
)

// TestLegacyAppDeclarationsPopulateEveryManager keeps provider inventories aligned with the legacy managers' shared accessor names.
func TestLegacyAppDeclarationsPopulateEveryManager(t *testing.T) {
	for _, test := range []struct{ component, prefix, suffix, value string }{
		{"database_mysql", "DB", "DATABASE", "reports"},
		{"cache", "CACHE", "DRIVER", "memory"},
		{"jobs", "QUEUE", "DRIVER", "workerpool"},
		{"events", "EVENTS", "DRIVER", "inproc"},
		{"storage", "STORAGE", "DRIVER", "local"},
		{"mail", "MAIL", "DRIVER", "log"},
	} {
		t.Run(test.component, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".goforj.yml", "module_name: example.org/legacy\nrender:\n  components: [cli, "+test.component+"]\napps:\n  admin:\n    components: [cli, "+test.component+"]\n  billing:\n    components: [cli, "+test.component+"]\n  utility:\n    components: [cli]\n")
			put(t, root, ".env", "ADMIN_"+test.prefix+"_REPORTS_"+test.suffix+"="+test.value+"\n")
			session := openTest(t, root)
			inventory := map[string]bool{}
			for _, resource := range session.Resources {
				inventory[resource.Key] = true
			}
			for _, scope := range []string{"", "ADMIN_", "BILLING_"} {
				key := scope + test.prefix + "_REPORTS_DRIVER"
				if !inventory[key] {
					t.Errorf("generated manager resource missing: %s", key)
				}
			}
			if inventory["UTILITY_"+test.prefix+"_REPORTS_DRIVER"] {
				t.Fatal("resource added to an App without the component")
			}
			if test.prefix == "STORAGE" {
				for _, scope := range []string{"", "ADMIN_", "BILLING_"} {
					if !inventory[scope+"STORAGE_PUBLIC_DRIVER"] {
						t.Errorf("catalog storage resource missing in scope %q", scope)
					}
				}
			}
		})
	}
}

// TestLegacyAppDatabaseNameRemainsSelectableAndExcluded protects implicit generated connections when their parent provider changes.
func TestLegacyAppDatabaseNameRemainsSelectableAndExcluded(t *testing.T) {
	root := fixture(t)
	put(t, root, ".goforj.yml", "module_name: example.org/legacy\nrender:\n  components: [cli, database_mysql]\napps:\n  admin:\n    components: [cli, database_mysql]\n")
	put(t, root, ".env", "DB_DRIVER=mysql\nDB_DSN=synthetic-service-dsn\nADMIN_DB_REPORTS_DATABASE=reports\n")
	session := openTest(t, root)
	if names := generate.ResourceNames(root, session.Current)[project.ResourceDatabase]; !slices.Contains(names, "reports") {
		t.Fatalf("generator no longer declares Reports: %v", names)
	}
	selected, err := session.Drivers(map[string]string{"DB_REPORTS_DRIVER": "mysql"})
	if err != nil {
		t.Fatalf("select generated root connection: %v", err)
	}
	if selected["DB_REPORTS_DRIVER"] != "mysql" {
		t.Fatal("explicit Reports selection was omitted")
	}
	before := databaseTargetInScope(session.Current, "DB_REPORTS_DRIVER", "")
	portable, err := session.PortableFor(map[string]bool{"DB_REPORTS_DRIVER": true})
	if err != nil {
		t.Fatal(err)
	}
	if portable["DB_DRIVER"] != "sqlite" || portable["DB_REPORTS_DRIVER"] != "mysql" {
		t.Fatal("portable conversion did not change the parent and pin the excluded driver")
	}
	if after := databaseTargetInScope(portable, "DB_REPORTS_DRIVER", ""); after != before {
		t.Fatalf("excluded Reports target changed: before=%+v after=%+v", before, after)
	}
}

// TestFeatureAppDeclarationsStayScoped prevents legacy global discovery from promoting Feature names into sibling managers.
func TestFeatureAppDeclarationsStayScoped(t *testing.T) {
	root := fixture(t)
	put(t, root, ".goforj.yml", "module_name: example.org/features\nrender:\n  components: [cli, feature]\napps:\n  admin:\n    components: [cli, feature]\n  billing:\n    components: [cli, feature]\n")
	put(t, root, ".env", "FEATURE_SHARED_DRIVER=memory\nADMIN_FEATURE_REPORTS_DRIVER=memory\n")
	session := openTest(t, root)
	inventory := map[string]bool{}
	for _, resource := range session.Resources {
		inventory[resource.Key] = true
	}
	for _, key := range []string{"FEATURE_SHARED_DRIVER", "ADMIN_FEATURE_SHARED_DRIVER", "BILLING_FEATURE_SHARED_DRIVER", "ADMIN_FEATURE_REPORTS_DRIVER"} {
		if !inventory[key] {
			t.Errorf("scoped Feature resource missing: %s", key)
		}
	}
	for _, key := range []string{"FEATURE_REPORTS_DRIVER", "BILLING_FEATURE_REPORTS_DRIVER"} {
		if inventory[key] {
			t.Errorf("App-only Feature name leaked: %s", key)
		}
	}
}
