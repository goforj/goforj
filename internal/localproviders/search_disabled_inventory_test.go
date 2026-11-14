package localproviders

import "testing"

// TestDisabledSearchAppInventoryDoesNotLeak keeps stale App paths, URLs, and drivers out of default and sibling inventories.
func TestDisabledSearchAppInventoryDoesNotLeak(t *testing.T) {
	for _, test := range []struct {
		name, assignment, setting, driver string
		sourceOnly                        bool
	}{
		{"path only", "SEARCH_SEARCH_PATH=storage/stale\n", "PATH", "bleve", true},
		{"URL only", "SEARCH_SEARCH_URL=https://example.com/search\n", "URL", "meilisearch", true},
		{"Bleve driver only", "SEARCH_SEARCH_DRIVER=bleve\n", "PATH", "bleve", false},
		{"Meilisearch driver only", "SEARCH_SEARCH_DRIVER=meilisearch\n", "URL", "meilisearch", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".goforj.yml", "module_name: example.org/disabled\nrender:\n  components: [cli, search]\napps:\n  search:\n    components: [cli]\n  reports:\n    components: [cli, search]\n")
			put(t, root, ".env", "SEARCH_DRIVER=bleve\nSEARCH_SHARED_DRIVER=bleve\n"+test.assignment)
			session := openTest(t, root)
			for _, resource := range session.Resources {
				for _, key := range []string{"SEARCH_SEARCH_DRIVER", "SEARCH_SEARCH_SHARED_DRIVER", "REPORTS_SEARCH_SEARCH_DRIVER"} {
					if resource.Key == key {
						t.Errorf("disabled App declaration added %s", key)
					}
				}
			}
			preview := clone(session.Current)
			preview["SEARCH_SEARCH_PREVIEW_"+test.setting] = "preview-value"
			for _, resource := range session.ResourcesFor(preview) {
				for _, key := range []string{"SEARCH_SEARCH_PREVIEW_DRIVER", "REPORTS_SEARCH_SEARCH_PREVIEW_DRIVER"} {
					if resource.Key == key {
						t.Errorf("disabled App preview leaked into %s", key)
					}
				}
			}
			portable, err := session.Portable()
			if err != nil {
				t.Fatal(err)
			}
			if portable["REPORTS_SEARCH_SHARED_DRIVER"] != "bleve" {
				t.Fatal("portable lost inherited root Search name")
			}
			if _, exists := portable["REPORTS_SEARCH_SEARCH_DRIVER"]; exists {
				t.Fatal("portable invented sibling Search driver")
			}
			if test.sourceOnly {
				if _, exists := portable["SEARCH_SEARCH_DRIVER"]; exists {
					t.Fatal("portable invented disabled App Search driver")
				}
			} else if portable["SEARCH_SEARCH_DRIVER"] != test.driver {
				t.Fatal("portable changed retained disabled App driver")
			}
		})
	}
}

// TestDisabledLongerSearchAppKeepsOwnership prevents a participating shorter App from claiming a disabled sibling's settings.
func TestDisabledLongerSearchAppKeepsOwnership(t *testing.T) {
	for _, test := range []struct {
		name, assignment, setting, driver string
		sourceOnly                        bool
	}{
		{"path only", "SEARCH_SEARCH_SEARCH_PATH=storage/stale\n", "PATH", "bleve", true},
		{"URL only", "SEARCH_SEARCH_SEARCH_URL=https://example.com/search\n", "URL", "meilisearch", true},
		{"Bleve driver only", "SEARCH_SEARCH_SEARCH_DRIVER=bleve\n", "PATH", "bleve", false},
		{"Meilisearch driver only", "SEARCH_SEARCH_SEARCH_DRIVER=meilisearch\n", "URL", "meilisearch", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".goforj.yml", "module_name: example.org/disabled\nrender:\n  components: [cli, search]\napps:\n  search:\n    components: [cli, search]\n  search-search:\n    components: [cli]\n  reports:\n    components: [cli, search]\n")
			put(t, root, ".env", "SEARCH_DRIVER=bleve\n"+test.assignment)
			session := openTest(t, root)
			for _, resource := range session.Resources {
				for _, key := range []string{"SEARCH_SEARCH_SEARCH_DRIVER", "REPORTS_SEARCH_SEARCH_SEARCH_DRIVER", "SEARCH_SEARCH_SEARCH_SEARCH_DRIVER"} {
					if resource.Key == key {
						t.Errorf("disabled longer App declaration added %s", key)
					}
				}
			}
			preview := clone(session.Current)
			preview["SEARCH_SEARCH_SEARCH_PREVIEW_"+test.setting] = "preview-value"
			for _, resource := range session.ResourcesFor(preview) {
				for _, key := range []string{"SEARCH_SEARCH_SEARCH_PREVIEW_DRIVER", "REPORTS_SEARCH_SEARCH_SEARCH_PREVIEW_DRIVER", "SEARCH_SEARCH_SEARCH_SEARCH_PREVIEW_DRIVER"} {
					if resource.Key == key {
						t.Errorf("disabled longer App preview leaked into %s", key)
					}
				}
			}
			portable, err := session.Portable()
			if err != nil {
				t.Fatal(err)
			}
			if test.sourceOnly {
				if _, exists := portable["SEARCH_SEARCH_SEARCH_DRIVER"]; exists {
					t.Fatal("portable invented disabled longer App driver")
				}
			} else if portable["SEARCH_SEARCH_SEARCH_DRIVER"] != test.driver {
				t.Fatal("portable changed retained disabled longer App driver")
			}
		})
	}
}
