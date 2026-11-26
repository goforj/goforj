package generate

import (
	"testing"

	"github.com/goforj/goforj/project"
)

// TestSharedResourceEnvironmentSetting follows the legacy fallback contracts after App Feature ownership is resolved.
func TestSharedResourceEnvironmentSetting(t *testing.T) {
	components := project.Components{Feature: true, Notification: true, Cache: true, DatabaseSQLite: true, Jobs: true, Events: true, Storage: true, Mail: true}
	for _, definition := range project.ResourceCatalog() {
		t.Run(string(definition.Key), func(t *testing.T) {
			prefix := definition.EnvironmentPrefix
			for _, suffix := range []string{"DRIVER", "SUPPORTED_DRIVERS", "REPORTS_DRIVER", "FEATURE_DRIVER", prefix + "_DRIVER"} {
				key := prefix + "_" + suffix
				want := definition.Key != project.ResourceFeature
				if got := SharedResourceEnvironmentSetting(key, components); got != want {
					t.Errorf("default setting %s protection = %t, want %t", key, got, want)
				}
				if SharedResourceEnvironmentSetting(key, project.Components{}) {
					t.Errorf("inactive default setting %s is protected", key)
				}
			}
			for _, key := range []string{prefix + "_CUSTOM_SETTING", "UNRELATED_DRIVER"} {
				if SharedResourceEnvironmentSetting(key, components) {
					t.Errorf("non-default setting %s is protected", key)
				}
			}
		})
	}
}
