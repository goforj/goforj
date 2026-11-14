package localproviders

import "testing"

// TestDisabledFeatureAppInventoryDoesNotLeak keeps stale App sources and drivers out of default and sibling inventories.
func TestDisabledFeatureAppInventoryDoesNotLeak(t *testing.T) {
	for _, test := range []struct{ name, assignment string }{
		{"source only", "FEATURE_FEATURE_PATH=stale-flags.json\n"},
		{"driver only", "FEATURE_FEATURE_DRIVER=file\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".goforj.yml", "module_name: example.org/disabled\nrender:\n  components: [cli, feature]\napps:\n  feature:\n    components: [cli]\n  reports:\n    components: [cli, feature]\n")
			put(t, root, ".env", "FEATURE_DRIVER=memory\nFEATURE_SHARED_DRIVER=memory\n"+test.assignment)
			session := openTest(t, root)
			for _, resource := range session.Resources {
				for _, key := range []string{"FEATURE_FEATURE_DRIVER", "FEATURE_FEATURE_SHARED_DRIVER", "REPORTS_FEATURE_FEATURE_DRIVER"} {
					if resource.Key == key {
						t.Errorf("stale disabled App declaration added %s", key)
					}
				}
			}
			preview := clone(session.Current)
			preview["FEATURE_FEATURE_PREVIEW_PATH"] = "preview.json"
			for _, resource := range session.ResourcesFor(preview) {
				for _, key := range []string{"FEATURE_FEATURE_PREVIEW_DRIVER", "REPORTS_FEATURE_FEATURE_PREVIEW_DRIVER"} {
					if resource.Key == key {
						t.Errorf("disabled App preview leaked into %s", key)
					}
				}
			}
			portable, err := session.Portable()
			if err != nil {
				t.Fatal(err)
			}
			if portable["REPORTS_FEATURE_SHARED_DRIVER"] != "memory" {
				t.Fatal("portable lost inherited root Feature name")
			}
			if _, exists := portable["REPORTS_FEATURE_FEATURE_DRIVER"]; exists {
				t.Fatal("portable invented sibling Feature driver")
			}
			if test.name == "source only" {
				if _, exists := portable["FEATURE_FEATURE_DRIVER"]; exists {
					t.Fatal("portable invented disabled App Feature driver")
				}
			} else if portable["FEATURE_FEATURE_DRIVER"] != "file" {
				t.Fatal("portable changed retained disabled App driver")
			}
		})
	}
}

// TestDisabledLongerFeatureAppKeepsOwnership prevents a participating shorter App from claiming a disabled sibling's values.
func TestDisabledLongerFeatureAppKeepsOwnership(t *testing.T) {
	for _, test := range []struct{ name, assignment string }{
		{"source only", "FEATURE_FEATURE_FEATURE_PATH=stale-flags.json\n"},
		{"driver only", "FEATURE_FEATURE_FEATURE_DRIVER=file\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".goforj.yml", "module_name: example.org/disabled\nrender:\n  components: [cli, feature]\napps:\n  feature:\n    components: [cli, feature]\n  feature-feature:\n    components: [cli]\n  reports:\n    components: [cli, feature]\n")
			put(t, root, ".env", "FEATURE_DRIVER=memory\n"+test.assignment)
			session := openTest(t, root)
			for _, resource := range session.Resources {
				for _, key := range []string{"FEATURE_FEATURE_FEATURE_DRIVER", "REPORTS_FEATURE_FEATURE_FEATURE_DRIVER", "FEATURE_FEATURE_FEATURE_FEATURE_DRIVER"} {
					if resource.Key == key {
						t.Errorf("disabled longer App declaration added %s", key)
					}
				}
			}
			portable, err := session.Portable()
			if err != nil {
				t.Fatal(err)
			}
			if test.name == "source only" {
				if _, exists := portable["FEATURE_FEATURE_FEATURE_DRIVER"]; exists {
					t.Fatal("portable invented disabled longer App driver")
				}
			} else if portable["FEATURE_FEATURE_FEATURE_DRIVER"] != "file" {
				t.Fatal("portable changed disabled longer App driver")
			}
		})
	}
}
