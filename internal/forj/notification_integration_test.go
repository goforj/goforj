//go:build integration

package forj

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

// TestRenderedNotificationResourceScopesAndDisabledSurface verifies both App ownership and absent opt-in output.
func TestRenderedNotificationResourceScopesAndDisabledSurface(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name := "disabled"
		if enabled {
			name = "enabled"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			components := project.Components{CLI: true, Notification: enabled}
			apps := map[string]project.AppConfig{}
			if enabled {
				if err := writeConventionalAppMarker(root, "reports"); err != nil {
					t.Fatal(err)
				}
				apps["reports"] = project.AppConfig{Components: project.Components{CLI: true, Notification: true}}
			}
			testkit.RenderProjectWithForj(t, root, testkit.RenderProjectRequest{Config: project.Config{
				ProjectName: "Notification Resource", GoModuleName: "example.com/notificationresource",
				Render: project.RenderConfig{Components: components}, Apps: apps,
			}})
			manager := filepath.Join(root, "internal/notifications/manager_gen.go")
			_, err := os.Stat(manager)
			if enabled && err != nil || !enabled && !os.IsNotExist(err) {
				t.Fatalf("manager presence: %v", err)
			}
			environment := readRenderedFile(t, root, ".env")
			module := readRenderedFile(t, root, "go.mod")
			wiring := readRenderedFile(t, root, "app/wire/app.go")
			for token, content := range map[string]string{"NOTIFICATION_DRIVER": environment, "github.com/goforj/notification": module, "Notifications()": wiring} {
				if strings.Contains(content, token) != enabled {
					t.Fatalf("%s presence does not match component", token)
				}
			}
			if !enabled {
				return
			}
			scopeTest := `package notifications_test

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"

    "example.com/notificationresource/internal/cmd"
    "example.com/notificationresource/internal/notifications"
    "github.com/goforj/notification"
)

// TestAppScopedNotificationManager uses the existing App overlay before constructing each immutable manager.
func TestAppScopedNotificationManager(t *testing.T) {
    calls := 0
    server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) { calls++; response.WriteHeader(http.StatusNoContent) }))
    defer server.Close()
    t.Setenv("NOTIFICATION_DRIVER", "log")
    t.Setenv("REPORTS_NOTIFICATION_DRIVER", "webhook")
    t.Setenv("REPORTS_NOTIFICATION_WEBHOOK_URL", server.URL)
    t.Setenv("REPORTS_NOTIFICATION_WEBHOOK_ALLOW_PRIVATE", "true")
    root, err := notifications.NewManager()
    if err != nil { t.Fatal(err) }
    defer root.Close()
    t.Setenv("FORJ_APP", "reports")
    t.Setenv("NOTIFICATION_WEBHOOK_URL", "")
    t.Setenv("NOTIFICATION_WEBHOOK_ALLOW_PRIVATE", "false")
    if err := cmd.ApplyAppEnvOverlay(); err != nil { t.Fatal(err) }
    reports, err := notifications.NewManager()
    if err != nil { t.Fatal(err) }
    defer reports.Close()
    message := notification.Message{Title: "Scoped alert", Body: "reports only"}
    if err := reports.Default().Send(context.Background(), message); err != nil { t.Fatal(err) }
    if err := root.Default().Send(context.Background(), message); err != nil { t.Fatal(err) }
    if calls != 1 { t.Fatalf("webhook calls = %d, want only reports", calls) }
}
`
			if err := os.WriteFile(filepath.Join(root, "internal/notifications/app_scope_test.go"), []byte(scopeTest), 0600); err != nil {
				t.Fatal(err)
			}
			setRenderedEnvValue(t, root, "NOTIFICATION_SUPPORTED_DRIVERS", "log,webhook")
			build := exec.CommandContext(t.Context(), testkit.EnsureIntegrationForjBinary(t), "build")
			build.Dir = root
			build.Env = testkit.IntegrationGoProcessEnv(t, map[string]string{"GOWORK": "off"})
			if output, err := build.CombinedOutput(); err != nil {
				t.Fatalf("build webhook manifest: %v\n%s", err, output)
			}
			if !strings.Contains(readRenderedFile(t, root, "app/reports/wire/app.go"), "Notifications()") {
				t.Fatal("named App accessor missing")
			}
			command := exec.CommandContext(t.Context(), "go", "test", "./internal/notifications", "./app/wire", "./app/reports/wire")
			command.Dir = root
			command.Env = testkit.IntegrationGoProcessEnv(t, map[string]string{"GOWORK": "off"})
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("rendered tests: %v\n%s", err, output)
			}
		})
	}
}

// TestRenderedNotificationNamedOnlyResource leaves the default App disabled when a sibling enables notifications.
func TestRenderedNotificationNamedOnlyResource(t *testing.T) {
	root := t.TempDir()
	if err := writeConventionalAppMarker(root, "reports"); err != nil {
		t.Fatal(err)
	}
	testkit.RenderProjectWithForj(t, root, testkit.RenderProjectRequest{Config: project.Config{
		ProjectName: "Named Notifications", GoModuleName: "example.com/namednotifications",
		Render: project.RenderConfig{Components: project.Components{CLI: true}},
		Apps:   map[string]project.AppConfig{"reports": {Components: project.Components{CLI: true, Notification: true}}},
	}})
	if strings.Contains(readRenderedFile(t, root, "app/wire/app.go"), "Notifications()") {
		t.Fatal("default App gained disabled resource")
	}
	if !strings.Contains(readRenderedFile(t, root, "app/reports/wire/app.go"), "Notifications()") {
		t.Fatal("named App lost enabled resource")
	}
	command := exec.CommandContext(t.Context(), "go", "test", "./app/wire", "./app/reports/wire")
	command.Dir = root
	command.Env = testkit.IntegrationGoProcessEnv(t, map[string]string{"GOWORK": "off"})
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("named-only App tests: %v\n%s", err, output)
	}
}
