package project

import "testing"

// TestAppResourceKeyOwner keeps ownership global across resource families and independent of App capabilities.
func TestAppResourceKeyOwner(t *testing.T) {
	prefixes := []string{"FEATURE", "FEATURE_CHILD", "CACHE", ""}
	for _, definition := range ResourceCatalog() {
		for _, test := range []struct{ key, want string }{
			{definition.EnvironmentPrefix + "_DRIVER", ""},
			{"FEATURE_" + definition.EnvironmentPrefix + "_DRIVER", "FEATURE"},
			{"FEATURE_CHILD_" + definition.EnvironmentPrefix + "_REPORTS_DRIVER", "FEATURE_CHILD"},
			{"CACHE_" + definition.EnvironmentPrefix + "_ADDR", "CACHE"},
			{"FEATURE_CHILD_CUSTOM_SETTING", ""},
		} {
			if got := AppResourceKeyOwner(test.key, prefixes); got != test.want {
				t.Errorf("owner(%s) = %q, want %q", test.key, got, test.want)
			}
		}
	}
	if got := AppResourceKeyOwner("FEATURE_CACHE_DRIVER", nil); got != "" {
		t.Fatalf("unknown App owner = %q", got)
	}
}
