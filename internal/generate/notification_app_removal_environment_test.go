package generate

import (
	"testing"

	"github.com/goforj/goforj/project"
)

// TestNotificationAppResourceEnvironmentOwner protects only enabled managed settings and their longest App owner.
func TestNotificationAppResourceEnvironmentOwner(t *testing.T) {
	for _, test := range []struct {
		key, owner string
		managed    bool
	}{
		{"NOTIFICATION_DRIVER", "", true},
		{"NOTIFICATION_SUPPORTED_DRIVERS", "", true},
		{"NOTIFICATION_CHILD_WEBHOOK_URL", "", true},
		{"NOTIFICATION_CHILD_SUPPORTED_DRIVERS", "", true},
		{"NOTIFICATION_CHILD_NOTIFICATION_SUPPORTED_DRIVERS", "NOTIFICATION_CHILD", true},
		{"NOTIFICATION_NOTIFICATION_DRIVER", "NOTIFICATION", true},
		{"NOTIFICATION_CHILD_NOTIFICATION_WEBHOOK_URL", "NOTIFICATION_CHILD", true},
		{"NOTIFICATION_CHILD_NOTIFICATION_NAMED_WEBHOOK_TIMEOUT", "NOTIFICATION_CHILD", true},
		{"NOTIFICATION_CUSTOM_SETTING", "", false},
		{"NOTIFICATION_NOTIFICATION_CUSTOM_SETTING", "", false},
		{"UNRELATED_DRIVER", "", false},
	} {
		t.Run(test.key, func(t *testing.T) {
			prefixes := []string{"NOTIFICATION", "NOTIFICATION_CHILD"}
			for _, components := range []project.Components{{Notification: true}, {Notification: true, Feature: true}} {
				owner, managed := AppResourceEnvironmentOwner(test.key, prefixes, components)
				if owner != test.owner || managed != test.managed {
					t.Fatalf("owner=%q managed=%t", owner, managed)
				}
			}
			for _, components := range []project.Components{{}, {Feature: true}} {
				if owner, managed := AppResourceEnvironmentOwner(test.key, prefixes, components); owner != "" || managed {
					t.Fatalf("disabled owner=%q managed=%t", owner, managed)
				}
			}
		})
	}
}

// TestMixedAppResourceEnvironmentOwner gives App ownership precedence over another primitive's root named resource.
func TestMixedAppResourceEnvironmentOwner(t *testing.T) {
	prefixes := []string{"FEATURE", "NOTIFICATION", "FEATURE_NOTIFICATION"}
	for _, test := range []struct{ key, owner string }{
		{"FEATURE_NOTIFICATION_DRIVER", "FEATURE"},
		{"NOTIFICATION_FEATURE_DRIVER", "NOTIFICATION"},
		{"FEATURE_NOTIFICATION_NOTIFICATION_DRIVER", "FEATURE_NOTIFICATION"},
	} {
		owner, managed := AppResourceEnvironmentOwner(test.key, prefixes, project.Components{Feature: true, Notification: true})
		if !managed || owner != test.owner {
			t.Fatalf("%s: owner=%q managed=%t", test.key, owner, managed)
		}
	}
}
