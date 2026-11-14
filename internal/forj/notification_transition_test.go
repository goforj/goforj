package forj

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/forj/makeapp"
	"github.com/goforj/goforj/project"
)

// TestNotificationProjectRemovalRefusesBeforeMutation preserves existing senders when a render disables their component.
func TestNotificationProjectRemovalRefusesBeforeMutation(t *testing.T) {
	usePrimitiveRendererRoot(t)
	config := &project.Config{Render: project.RenderConfig{Components: project.Components{CLI: true, Notification: true}}}
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatal(err)
	}
	writePrimitiveRendererFile(t, "internal/notifications/manager_gen.go", "package notifications\n")
	writePrimitiveRendererFile(t, ".env", "NOTIFICATION_DRIVER=log\nOWNER=keep\n")
	paths := []string{".goforj.yml", ".env", "internal/notifications/manager_gen.go"}
	before := primitiveRendererFileHashes(t, paths)
	err := unitProjectRenderer(t).Render(ComponentRenderInput{components: project.Components{CLI: true}})
	if err == nil || !strings.Contains(err.Error(), "cannot disable Notifications") {
		t.Fatalf("removal = %v", err)
	}
	if !reflect.DeepEqual(before, primitiveRendererFileHashes(t, paths)) {
		t.Fatal("rejected project removal changed owner files")
	}
}

// TestNotificationAppRemovalRefusesBeforeMutation preserves a disabled App's existing accessor and provider boundary.
func TestNotificationAppRemovalRefusesBeforeMutation(t *testing.T) {
	usePrimitiveRendererRoot(t)
	app := project.DefaultNamedApp("billing")
	config := &project.Config{Render: project.RenderConfig{Components: project.Components{CLI: true, Notification: true}}, Apps: map[string]project.AppConfig{
		app.Name: {Components: project.Components{CLI: true, Notification: true}},
	}}
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(app.WireDir, "app.go")
	writePrimitiveRendererFile(t, path, "package wire\ntype App struct{}\nfunc (app *App) Notifications() any { return nil }\n")
	paths := []string{".goforj.yml", path}
	before := primitiveRendererFileHashes(t, paths)
	err := unitProjectRenderer(t).RenderAppOnly(app, makeapp.RenderOptions{Components: project.Components{CLI: true}, SkipWire: true})
	if err == nil || !strings.Contains(err.Error(), "cannot disable Notifications for App") {
		t.Fatalf("App removal = %v", err)
	}
	if !reflect.DeepEqual(before, primitiveRendererFileHashes(t, paths)) {
		t.Fatal("rejected App removal changed owner files")
	}
}

// TestNotificationLastAppRemovalRefusesResourceAndAccessorResidue protects both supported final-owner removal boundaries.
func TestNotificationLastAppRemovalRefusesResourceAndAccessorResidue(t *testing.T) {
	for _, resource := range []bool{false, true} {
		name := "accessor only"
		if resource {
			name = "resource package"
		}
		t.Run(name, func(t *testing.T) {
			usePrimitiveRendererRoot(t)
			app := project.DefaultNamedApp("billing")
			config := &project.Config{Render: project.RenderConfig{Components: project.Components{CLI: true}}, Apps: map[string]project.AppConfig{
				app.Name: {Components: project.Components{CLI: true, Notification: true}},
			}}
			if err := writeProjectConfig(".goforj.yml", config); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(app.WireDir, "app.go")
			writePrimitiveRendererFile(t, path, "package wire\ntype App struct{}\nfunc (app *App) Notifications() any { return nil }\n")
			paths := []string{".goforj.yml", path}
			if resource {
				paths = append(paths, "internal/notifications/manager_gen.go")
				writePrimitiveRendererFile(t, paths[len(paths)-1], "package notifications\n")
			}
			before := primitiveRendererFileHashes(t, paths)
			result, err := unitProjectRenderer(t).RemoveApp(app)
			if err == nil || !strings.Contains(err.Error(), "last App using Notifications") || result.Changed() {
				t.Fatalf("last owner removal = %#v, %v", result, err)
			}
			if !reflect.DeepEqual(before, primitiveRendererFileHashes(t, paths)) {
				t.Fatal("rejected final App removal changed files")
			}
		})
	}
}

// TestNotificationAccessorPreflightDistinguishesSyntaxFromUnrelatedText rejects unreadable ownership without claiming comments or free functions.
func TestNotificationAccessorPreflightDistinguishesSyntaxFromUnrelatedText(t *testing.T) {
	usePrimitiveRendererRoot(t)
	config := &project.Config{Render: project.RenderConfig{Components: project.Components{CLI: true}}}
	renderer := projectRendererForTest(t, config)
	for _, test := range []struct {
		name, body string
		want       bool
		invalid    bool
	}{
		{name: "missing"},
		{name: "unrelated", body: "package wire\n// Notifications is just text.\nfunc Notifications() {}\n"},
		{name: "accessor", body: "package wire\ntype App struct{}\nfunc (app *App) Notifications() any { return nil }\n", want: true},
		{name: "invalid", body: "package wire\nfunc (", invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(test.name, "app.go")
			if test.body != "" {
				writePrimitiveRendererFile(t, path, test.body)
			}
			got, err := renderer.notificationAccessorExists(path)
			if got != test.want || (err != nil) != test.invalid {
				t.Fatalf("accessor = %v, %v", got, err)
			}
		})
	}
}
