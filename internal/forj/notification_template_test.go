package forj

import (
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestNotificationProviderBindsOwningAppPrefix prevents correctly scoped managers from being wired to another App's membership.
func TestNotificationProviderBindsOwningAppPrefix(t *testing.T) {
	config := &project.Config{GoModuleName: "example.com/notification", Render: project.RenderConfig{Components: project.Components{CLI: true, Notification: true}}, Apps: map[string]project.AppConfig{
		"admin": {Components: project.Components{CLI: true, Notification: true}},
	}}
	workspace := currentProjectRenderWorkspace(t)
	for _, test := range []struct {
		app    project.App
		prefix string
	}{
		{project.DefaultApp(), ""}, {project.DefaultNamedApp("admin"), "ADMIN"},
	} {
		source := renderSharedTemplate(t, "wire/inject_managers.go.tmpl", workspace.templateDataForApp(config, test.app))
		assertFormattedGoTemplate(t, "wire/inject_managers.go.tmpl", source)
		if !strings.Contains(source, `notifications.NewManagerForApp("`+test.prefix+`")`) {
			t.Fatalf("App %s lost membership prefix", test.app.Name)
		}
	}
}
