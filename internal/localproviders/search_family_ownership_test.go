package localproviders

import (
	"reflect"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestSearchProviderFamilyOwnership keeps provider choices and validation aligned with generated App ownership.
func TestSearchProviderFamilyOwnership(t *testing.T) {
	for _, test := range []struct {
		name, components, app, appComponents, environment, key, scope, portable string
		family                                                                  project.ResourceKey
	}{
		{"Cache App Search", "cli, cache", "cache", "cli, search", "CACHE_DRIVER=memory\nSEARCH_SUPPORTED_DRIVERS=bleve,meilisearch\nCACHE_SEARCH_DRIVER=meilisearch\nCACHE_SEARCH_URL=https://example.com/hook\n", "CACHE_SEARCH_DRIVER", "CACHE_", "bleve", project.ResourceSearch},
		{"Search App Cache", "cli, search", "search", "cli, search, cache", "SEARCH_DRIVER=bleve\nSEARCH_CACHE_DRIVER=redis\nCACHE_SUPPORTED_DRIVERS=memory,redis\n", "SEARCH_CACHE_DRIVER", "SEARCH_", "memory", project.ResourceCache},
		{"database App Search", "cli, database_sqlite", "db", "cli, search", "DB_DRIVER=sqlite\nSEARCH_SUPPORTED_DRIVERS=bleve,meilisearch\nDB_SEARCH_DRIVER=meilisearch\nDB_SEARCH_URL=https://example.com/hook\n", "DB_SEARCH_DRIVER", "DB_", "bleve", project.ResourceSearch},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".goforj.yml", "module_name: example.org/scopes\nrender:\n  components: ["+test.components+"]\napps:\n  "+test.app+":\n    components: ["+test.appComponents+"]\n")
			put(t, root, ".env", test.environment)
			session := openTest(t, root)
			definitions := resourceDefinitions(session.config, test.key)
			if len(definitions) != 1 || definitions[0].Key != test.family {
				t.Fatalf("definitions=%#v want=%s", definitions, test.family)
			}
			if scopes := session.resourceScopes(test.key, definitions[0]); !reflect.DeepEqual(scopes, []string{test.scope}) {
				t.Fatalf("scopes=%v", scopes)
			}
			found := false
			for _, resource := range session.Resources {
				if resource.Key == test.key {
					found = true
					if resource.Definition.Key != test.family || len(resource.Definition.Drivers) == 0 {
						t.Fatalf("picker=%#v", resource)
					}
				}
			}
			if !found {
				t.Fatal("owned App resource missing")
			}
			if test.family == project.ResourceSearch && len(session.databaseScopes(test.key)) != 0 {
				t.Fatal("Search claimed a database target")
			}
			portable, err := session.Portable()
			if err != nil {
				t.Fatal(err)
			}
			if portable[test.key] != test.portable {
				t.Fatalf("portable=%q want=%q", portable[test.key], test.portable)
			}
			if err := session.Validate(portable); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestDisabledSearchAppRetainsLegacyProviderInventory preserves active legacy named resources across all six families.
func TestDisabledSearchAppRetainsLegacyProviderInventory(t *testing.T) {
	for _, test := range []struct{ component, app, prefix, driver string }{
		{"cache", "cache", "CACHE", "memory"}, {"database_sqlite", "db", "DB", "sqlite"}, {"jobs", "queue", "QUEUE", "workerpool"},
		{"events", "events", "EVENTS", "inproc"}, {"storage", "storage", "STORAGE", "local"}, {"mail", "mail", "MAIL", "log"},
	} {
		t.Run(test.component, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".goforj.yml", "module_name: example.org/legacy\nrender:\n  components: [cli, "+test.component+"]\napps:\n  "+test.app+":\n    components: [cli]\n")
			key := test.prefix + "_SEARCH_DRIVER"
			put(t, root, ".env", key+"="+test.driver+"\n")
			session := openTest(t, root)
			found := false
			for _, resource := range session.Resources {
				if resource.Key == key {
					found = true
					if resource.Definition.Key == project.ResourceSearch {
						t.Fatal("disabled Search claimed legacy resource")
					}
				}
			}
			if !found {
				t.Fatal("legacy resource missing")
			}
			if _, err := session.Portable(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestSearchProviderSettingsRespectOwnership mirrors explicit blank driver resets and App-root defaults before named overrides.
func TestSearchProviderSettingsRespectOwnership(t *testing.T) {
	config := &project.Config{Render: project.RenderConfig{Components: project.Components{CLI: true, Search: true}}, Apps: map[string]project.AppConfig{
		"search":        {Components: project.Components{CLI: true, Search: true, Cache: true}},
		"billing":       {Components: project.Components{CLI: true, Search: true}},
		"search-search": {Components: project.Components{CLI: true}},
	}}
	session := &Session{config: config}
	definition, _ := project.ResourceDefinitionByKey(project.ResourceSearch)
	for _, test := range []struct {
		name      string
		values    map[string]string
		key, want string
	}{
		{"root inheritance", map[string]string{"SEARCH_DRIVER": "meilisearch"}, "BILLING_SEARCH_DRIVER", "meilisearch"},
		{"empty App root", map[string]string{"SEARCH_DRIVER": "meilisearch", "BILLING_SEARCH_DRIVER": ""}, "BILLING_SEARCH_DRIVER", "bleve"},
		{"whitespace App root", map[string]string{"SEARCH_DRIVER": "meilisearch", "BILLING_SEARCH_DRIVER": "   "}, "BILLING_SEARCH_DRIVER", "bleve"},
		{"named inherits App root", map[string]string{"SEARCH_DRIVER": "bleve", "BILLING_SEARCH_DRIVER": "meilisearch"}, "BILLING_SEARCH_CATALOG_DRIVER", "meilisearch"},
		{"global named overrides App root", map[string]string{"SEARCH_DRIVER": "meilisearch", "BILLING_SEARCH_DRIVER": "meilisearch", "SEARCH_CATALOG_DRIVER": "bleve"}, "BILLING_SEARCH_CATALOG_DRIVER", "bleve"},
		{"empty global named", map[string]string{"SEARCH_DRIVER": "meilisearch", "BILLING_SEARCH_DRIVER": "bleve", "SEARCH_CATALOG_DRIVER": ""}, "BILLING_SEARCH_CATALOG_DRIVER", "meilisearch"},
		{"empty App named", map[string]string{"SEARCH_CATALOG_DRIVER": "bleve", "BILLING_SEARCH_DRIVER": "meilisearch", "BILLING_SEARCH_CATALOG_DRIVER": ""}, "BILLING_SEARCH_CATALOG_DRIVER", "meilisearch"},
		{"whitespace App named", map[string]string{"SEARCH_CATALOG_DRIVER": "bleve", "BILLING_SEARCH_DRIVER": "meilisearch", "BILLING_SEARCH_CATALOG_DRIVER": "   "}, "BILLING_SEARCH_CATALOG_DRIVER", "meilisearch"},
		{"explicit App named", map[string]string{"BILLING_SEARCH_DRIVER": "meilisearch", "BILLING_SEARCH_CATALOG_DRIVER": " BlEvE "}, "BILLING_SEARCH_CATALOG_DRIVER", "bleve"},
		{"another App family", map[string]string{"SEARCH_CACHE_DRIVER": "redis"}, "BILLING_SEARCH_CACHE_DRIVER", "bleve"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := session.resourceSetting(test.values, test.key, "DRIVER", "BILLING_", definition); got != test.want {
				t.Fatalf("driver=%q want=%q", got, test.want)
			}
		})
	}
	for _, test := range []struct {
		name   string
		values map[string]string
		want   string
	}{
		{"root", map[string]string{"SEARCH_URL": "base"}, "base"},
		{"App root", map[string]string{"SEARCH_URL": "base", "BILLING_SEARCH_URL": "app"}, "app"},
		{"named before App root", map[string]string{"SEARCH_URL": "base", "BILLING_SEARCH_URL": "app", "SEARCH_CATALOG_URL": "named"}, "named"},
		{"empty App named", map[string]string{"SEARCH_CATALOG_URL": "named", "BILLING_SEARCH_CATALOG_URL": ""}, "named"},
		{"explicit App named", map[string]string{"SEARCH_CATALOG_URL": "named", "BILLING_SEARCH_CATALOG_URL": "own"}, "own"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := session.resourceSetting(test.values, "BILLING_SEARCH_CATALOG_DRIVER", "URL", "BILLING_", definition); got != test.want {
				t.Fatalf("URL=%q want=%q", got, test.want)
			}
		})
	}
	if scopes := session.resourceScopes("SEARCH_SEARCH_SEARCH_DRIVER", definition); len(scopes) != 0 {
		t.Fatalf("disabled longer App scopes=%v", scopes)
	}
}
