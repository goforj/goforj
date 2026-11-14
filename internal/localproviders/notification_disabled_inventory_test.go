package localproviders

import "testing"

// TestDisabledNotificationAppInventoryDoesNotLeak keeps stale App sources and drivers out of default and sibling inventories.
func TestDisabledNotificationAppInventoryDoesNotLeak(t *testing.T) {
	for _, test := range []struct{ name, assignment string }{
		{"source only", "NOTIFICATION_NOTIFICATION_WEBHOOK_URL=stale-flags.json\n"},
		{"driver only", "NOTIFICATION_NOTIFICATION_DRIVER=webhook\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".goforj.yml", "module_name: example.org/disabled\nrender:\n  components: [cli, notification]\napps:\n  notification:\n    components: [cli]\n  reports:\n    components: [cli, notification]\n")
			put(t, root, ".env", "NOTIFICATION_DRIVER=log\nNOTIFICATION_SHARED_DRIVER=log\n"+test.assignment)
			session := openTest(t, root)
			for _, resource := range session.Resources {
				for _, key := range []string{"NOTIFICATION_NOTIFICATION_DRIVER", "NOTIFICATION_NOTIFICATION_SHARED_DRIVER", "REPORTS_NOTIFICATION_NOTIFICATION_DRIVER"} {
					if resource.Key == key {
						t.Errorf("stale disabled App declaration added %s", key)
					}
				}
			}
			preview := clone(session.Current)
			preview["NOTIFICATION_NOTIFICATION_PREVIEW_WEBHOOK_URL"] = "preview.json"
			for _, resource := range session.ResourcesFor(preview) {
				for _, key := range []string{"NOTIFICATION_NOTIFICATION_PREVIEW_DRIVER", "REPORTS_NOTIFICATION_NOTIFICATION_PREVIEW_DRIVER"} {
					if resource.Key == key {
						t.Errorf("disabled App preview leaked into %s", key)
					}
				}
			}
			portable, err := session.Portable()
			if err != nil {
				t.Fatal(err)
			}
			if portable["REPORTS_NOTIFICATION_SHARED_DRIVER"] != "log" {
				t.Fatal("portable lost inherited root Notification name")
			}
			if _, exists := portable["REPORTS_NOTIFICATION_NOTIFICATION_DRIVER"]; exists {
				t.Fatal("portable invented sibling Notification driver")
			}
			if test.name == "source only" {
				if _, exists := portable["NOTIFICATION_NOTIFICATION_DRIVER"]; exists {
					t.Fatal("portable invented disabled App Notification driver")
				}
			} else if portable["NOTIFICATION_NOTIFICATION_DRIVER"] != "webhook" {
				t.Fatal("portable changed retained disabled App driver")
			}
		})
	}
}

// TestDisabledLongerNotificationAppKeepsOwnership prevents a participating shorter App from claiming a disabled sibling's values.
func TestDisabledLongerNotificationAppKeepsOwnership(t *testing.T) {
	for _, test := range []struct{ name, assignment string }{
		{"source only", "NOTIFICATION_NOTIFICATION_NOTIFICATION_WEBHOOK_URL=stale-flags.json\n"},
		{"driver only", "NOTIFICATION_NOTIFICATION_NOTIFICATION_DRIVER=webhook\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".goforj.yml", "module_name: example.org/disabled\nrender:\n  components: [cli, notification]\napps:\n  notification:\n    components: [cli, notification]\n  notification-notification:\n    components: [cli]\n  reports:\n    components: [cli, notification]\n")
			put(t, root, ".env", "NOTIFICATION_DRIVER=log\n"+test.assignment)
			session := openTest(t, root)
			for _, resource := range session.Resources {
				for _, key := range []string{"NOTIFICATION_NOTIFICATION_NOTIFICATION_DRIVER", "REPORTS_NOTIFICATION_NOTIFICATION_NOTIFICATION_DRIVER", "NOTIFICATION_NOTIFICATION_NOTIFICATION_NOTIFICATION_DRIVER"} {
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
				if _, exists := portable["NOTIFICATION_NOTIFICATION_NOTIFICATION_DRIVER"]; exists {
					t.Fatal("portable invented disabled longer App driver")
				}
			} else if portable["NOTIFICATION_NOTIFICATION_NOTIFICATION_DRIVER"] != "webhook" {
				t.Fatal("portable changed disabled longer App driver")
			}
		})
	}
}
