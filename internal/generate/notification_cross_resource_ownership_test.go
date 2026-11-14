package generate

import (
	"reflect"
	"testing"
)

// TestNotificationDiscoveryExcludesOtherAppResourceFamilies keeps App component settings out of default sender discovery and validation.
func TestNotificationDiscoveryExcludesOtherAppResourceFamilies(t *testing.T) {
	root := notificationInventoryProject(t, "", "notification", "notification-child")
	for _, resource := range generationDriverResourcePrefixes {
		if resource == "NOTIFICATION" {
			continue
		}
		t.Run(resource, func(t *testing.T) {
			input := fixtureGenerationInput(root, map[string]string{
				"NOTIFICATION_SHARED_DRIVER":                 "log",
				"NOTIFICATION_NOTIFICATION_DRIVER":           "log",
				"NOTIFICATION_" + resource + "_DRIVER":       "other-component-driver",
				"NOTIFICATION_CHILD_" + resource + "_DRIVER": "other-component-driver",
			})
			if names := discoverNotificationNames(input); !reflect.DeepEqual(names, []string{"shared"}) {
				t.Fatalf("names = %v", names)
			}
			if _, err := generateNotificationFiles(input); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestNotificationDiscoveryPreservesUnownedResourceWordNames retains default sender names when no App owns the same environment key.
func TestNotificationDiscoveryPreservesUnownedResourceWordNames(t *testing.T) {
	root := notificationInventoryProject(t, "")
	input := fixtureGenerationInput(root, map[string]string{"NOTIFICATION_CACHE_DRIVER": "log", "NOTIFICATION_SHARED_DRIVER": "log"})
	if names := discoverNotificationNames(input); !reflect.DeepEqual(names, []string{"cache", "shared"}) {
		t.Fatalf("names = %v", names)
	}
	if _, err := generateNotificationFiles(input); err != nil {
		t.Fatal(err)
	}
}
