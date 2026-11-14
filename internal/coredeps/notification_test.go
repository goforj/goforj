package coredeps

import (
	"testing"

	"github.com/goforj/goforj/project"
)

// TestNotificationModulesAreGated pins only enabled notification Projects without increasing Go requirements.
func TestNotificationModulesAreGated(t *testing.T) {
	assertModulesGated(t, project.Components{}, project.Components{Notification: true}, []string{"github.com/goforj/notification"})
	if MustVersionFor("github.com/goforj/notification") != "v0.1.0" {
		t.Fatal("notification release pin drifted")
	}
}
