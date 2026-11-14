package forj

import (
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/project"
)

// TestNotificationRemoveResourceNamedAppPreservesEnvironment protects root resources and sibling Apps while cleaning the removed App.
func TestNotificationRemoveResourceNamedAppPreservesEnvironment(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(map[bool]string{false: "conventional", true: "configured"}[configured], func(t *testing.T) {
			usePrimitiveRendererRoot(t)
			config := &project.Config{ProjectName: "Notification Removal", GoModuleName: "example.com/removal", Render: project.RenderConfig{Components: project.Components{CLI: true, Notification: true}}}
			if configured {
				config.Apps = map[string]project.AppConfig{"notification": {Components: project.Components{CLI: true, Notification: true}}, "notification-child": {Components: project.Components{CLI: true, Notification: true}}}
			}
			if err := writeProjectConfig(".goforj.yml", config); err != nil {
				t.Fatal(err)
			}
			app := project.DefaultNamedApp("notification")
			writePrimitiveRendererFile(t, app.Entrypoint, "package main\n")
			writePrimitiveRendererFile(t, project.DefaultNamedApp("notification-child").Entrypoint, "package main\n")
			contents := "NOTIFICATION_DRIVER=log\nNOTIFICATION_WEBHOOK_URL=https://example.com/hook\nNOTIFICATION_SUPPORTED_DRIVERS=log,webhook\nNOTIFICATION_CHILD_DRIVER=log\nNOTIFICATION_CHILD_WEBHOOK_URL=https://example.com/hook\nNOTIFICATION_CHILD_NOTIFICATION_DRIVER=log\nNOTIFICATION_CHILD_NOTIFICATION_NESTED_DRIVER=log\nNOTIFICATION_CHILD_API_HTTP_PORT=8181\nNOTIFICATION_CHILD_CUSTOM_VALUE=keep\n# Notification\nNOTIFICATION_NOTIFICATION_DRIVER=log\nNOTIFICATION_NOTIFICATION_REMOVED_DRIVER=log\nNOTIFICATION_CUSTOM_SETTING=remove\nUNRELATED_SETTING=keep\n"
			for _, path := range []string{".env", ".env.host", ".env.example"} {
				writePrimitiveRendererFile(t, path, contents)
			}
			if _, err := generate.GenerateProjectFiles(".", generate.GenerationSelection{Notification: true}); err != nil {
				t.Fatal(err)
			}
			if _, err := unitProjectRenderer(t).RemoveApp(app); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{".env", ".env.host", ".env.example"} {
				body := readPrimitiveRendererFile(t, path)
				for _, key := range []string{"NOTIFICATION_DRIVER=log", "NOTIFICATION_WEBHOOK_URL=", "NOTIFICATION_SUPPORTED_DRIVERS=", "NOTIFICATION_CHILD_DRIVER=log", "NOTIFICATION_CHILD_WEBHOOK_URL=", "NOTIFICATION_CHILD_NOTIFICATION_DRIVER=log", "NOTIFICATION_CHILD_NOTIFICATION_NESTED_DRIVER=log", "NOTIFICATION_CHILD_API_HTTP_PORT=8181", "NOTIFICATION_CHILD_CUSTOM_VALUE=keep", "UNRELATED_SETTING=keep"} {
					if !strings.Contains(body, key) {
						t.Fatalf("%s lost %s: %s", path, key, body)
					}
				}
				for _, key := range []string{"NOTIFICATION_NOTIFICATION_DRIVER=", "NOTIFICATION_NOTIFICATION_REMOVED_DRIVER=", "NOTIFICATION_CUSTOM_SETTING=", "# Notification\n"} {
					if strings.Contains(body, key) {
						t.Fatalf("%s retained %s: %s", path, key, body)
					}
				}
			}
			accessors := readPrimitiveRendererFile(t, generate.NotificationAccessorsPath)
			if !strings.Contains(accessors, "Child()") || !strings.Contains(accessors, "Nested()") || strings.Contains(accessors, "Removed()") {
				t.Fatalf("remaining accessors: %s", accessors)
			}
			if result, err := generate.GenerateProjectFiles(".", generate.GenerationSelection{Notification: true}); err != nil || result.ChangedFiles != 0 {
				t.Fatalf("regeneration: %#v %v", result, err)
			}
		})
	}
}

// TestNotificationRemoveAppEnvironmentWithoutPreservationRetainsLegacyCleanup keeps direct cleanup callers and disabled components predictable.
func TestNotificationRemoveAppEnvironmentWithoutPreservationRetainsLegacyCleanup(t *testing.T) {
	for _, withPredicate := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "disabled component"}[withPredicate], func(t *testing.T) {
			usePrimitiveRendererRoot(t)
			writePrimitiveRendererFile(t, ".env", "# Notification\nNOTIFICATION_DRIVER=log\nNOTIFICATION_CHILD_NOTIFICATION_DRIVER=log\nNOTIFICATION_CUSTOM_SETTING=remove\nOTHER=keep\n")
			workspace := currentProjectRenderWorkspace(t)
			var predicates []func(string) bool
			if withPredicate {
				predicates = append(predicates, func(key string) bool {
					owner, managed := generate.AppResourceEnvironmentOwner(key, []string{"NOTIFICATION", "NOTIFICATION_CHILD"}, project.Components{})
					return managed && owner != "NOTIFICATION"
				})
			}
			if changed, err := workspace.removeAppEnvDefaults(".env", "notification", predicates...); err != nil || !changed {
				t.Fatalf("cleanup: %t %v", changed, err)
			}
			if body := readPrimitiveRendererFile(t, ".env"); body != "OTHER=keep\n" {
				t.Fatalf("cleanup = %q", body)
			}
		})
	}
}

// TestNotificationRemoveAppPreservesLongerAppPrefixesWithoutNotification protects sibling settings independently of managed resource support.
func TestNotificationRemoveAppPreservesLongerAppPrefixesWithoutNotification(t *testing.T) {
	usePrimitiveRendererRoot(t)
	config := &project.Config{ProjectName: "App Removal", GoModuleName: "example.com/removal", Render: project.RenderConfig{Components: project.Components{CLI: true}}}
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatal(err)
	}
	app := project.DefaultNamedApp("notification")
	writePrimitiveRendererFile(t, app.Entrypoint, "package main\n")
	writePrimitiveRendererFile(t, project.DefaultNamedApp("notification-child").Entrypoint, "package main\n")
	for _, path := range []string{".env", ".env.host", ".env.example"} {
		writePrimitiveRendererFile(t, path, "# Notification\nNOTIFICATION_DRIVER=log\nNOTIFICATION_CUSTOM_VALUE=remove\nNOTIFICATION_CHILD_API_HTTP_PORT=8181\nNOTIFICATION_CHILD_CUSTOM_VALUE=keep\nOTHER=keep\n")
	}
	if _, err := unitProjectRenderer(t).RemoveApp(app); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".env", ".env.host", ".env.example"} {
		if body := readPrimitiveRendererFile(t, path); body != "NOTIFICATION_CHILD_API_HTTP_PORT=8181\nNOTIFICATION_CHILD_CUSTOM_VALUE=keep\nOTHER=keep\n" {
			t.Fatalf("%s cleanup = %q", path, body)
		}
	}
}
