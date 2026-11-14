package generate

import (
	"testing"

	"github.com/goforj/goforj/project"
)

// TestAppResourceEnvironmentOwner limits removal protection to recognized settings of enabled components.
func TestAppResourceEnvironmentOwner(t *testing.T) {
	for _, test := range []struct {
		key, owner string
		managed    bool
	}{
		{"FEATURE_DRIVER", "", true}, {"FEATURE_CHILD_DOCUMENT", "", true},
		{"FEATURE_FEATURE_DRIVER", "FEATURE", true},
		{"FEATURE_CHILD_FEATURE_DOCUMENT", "FEATURE_CHILD", true},
		{"FEATURE_CHILD_FEATURE_NAMED_REFRESH_INTERVAL", "FEATURE_CHILD", true},
		{"FEATURE_CUSTOM_SETTING", "", false}, {"FEATURE_FEATURE_CUSTOM_SETTING", "", false}, {"UNRELATED_DRIVER", "", false},
	} {
		t.Run(test.key, func(t *testing.T) {
			prefixes := []string{"FEATURE", "FEATURE_CHILD"}
			owner, managed := AppResourceEnvironmentOwner(test.key, prefixes, project.Components{Feature: true})
			if owner != test.owner || managed != test.managed {
				t.Fatalf("owner=%q managed=%t", owner, managed)
			}
			if owner, managed := AppResourceEnvironmentOwner(test.key, prefixes, project.Components{}); owner != "" || managed {
				t.Fatalf("disabled owner=%q managed=%t", owner, managed)
			}
		})
	}
}
