package forj

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/forj/makeapp"
	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/project"
)

// TestNotificationMakeAppRefreshesOwnership updates compiled App registration before wiring a new App.
func TestNotificationMakeAppRefreshesOwnership(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			usePrimitiveRendererRoot(t)
			config := &project.Config{ProjectName: "Notification Apps", GoModuleName: "example.com/notificationapps", Render: project.RenderConfig{Components: project.Components{CLI: true, Notification: true}}}
			if err := writeProjectConfig(".goforj.yml", config); err != nil {
				t.Fatal(err)
			}
			writePrimitiveRendererFile(t, ".env", "NOTIFICATION_DRIVER=log\nNOTIFICATION_SUPPORTED_DRIVERS=log,webhook\n")
			if _, err := generate.GenerateNotificationFiles("."); err != nil {
				t.Fatal(err)
			}
			renderer := unitProjectRenderer(t)
			if err := renderer.RenderAppOnly(project.DefaultNamedApp("billing"), makeapp.RenderOptions{Components: project.Components{CLI: true, Notification: enabled}, SkipWire: true}); err != nil {
				t.Fatal(err)
			}
			body := readPrimitiveRendererFile(t, generate.NotificationManagerPath)
			if strings.Contains(body, `"BILLING":`) != enabled {
				t.Fatalf("wrong App participation: %s", body)
			}
			provider := readPrimitiveRendererFile(t, "app/billing/wire/inject_managers.go")
			if strings.Contains(provider, `notifications.NewManagerForApp("BILLING")`) != enabled {
				t.Fatal("provider and ownership disagree")
			}
		})
	}
}

// TestNotificationRemoveAppRefreshesOwnership removes stale App identity and accessors while retaining remaining resources.
func TestNotificationRemoveAppRefreshesOwnership(t *testing.T) {
	usePrimitiveRendererRoot(t)
	app := project.DefaultNamedApp("billing")
	config := &project.Config{ProjectName: "Notification Removal", GoModuleName: "example.com/removal", Render: project.RenderConfig{Components: project.Components{CLI: true, Notification: true}}, Apps: map[string]project.AppConfig{"billing": {Components: project.Components{CLI: true, Notification: true}}}}
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatal(err)
	}
	writePrimitiveRendererFile(t, app.Entrypoint, "package main\n")
	writePrimitiveRendererFile(t, ".env", "NOTIFICATION_DRIVER=log\nNOTIFICATION_SHARED_DRIVER=log\nBILLING_NOTIFICATION_ONLY_DRIVER=log\n")
	if _, err := generate.GenerateProjectFiles(".", generate.GenerationSelection{Notification: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readPrimitiveRendererFile(t, generate.NotificationManagerPath), `"BILLING":`) || !strings.Contains(readPrimitiveRendererFile(t, generate.NotificationAccessorsPath), "Only()") {
		t.Fatal("fixture missing App-owned notification resources")
	}
	result, err := unitProjectRenderer(t).RemoveApp(app)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(result.Updated, generate.NotificationManagerPath) || !slices.Contains(result.Updated, generate.NotificationAccessorsPath) {
		t.Fatal("removed notification ownership not reported")
	}
	manager := readPrimitiveRendererFile(t, generate.NotificationManagerPath)
	accessors := readPrimitiveRendererFile(t, generate.NotificationAccessorsPath)
	if strings.Contains(manager, `"BILLING":`) || strings.Contains(accessors, "Only()") {
		t.Fatal("removed App retained notification ownership")
	}
	if !strings.Contains(accessors, "Shared()") || !strings.Contains(manager, `"":`) {
		t.Fatal("remaining notification resources were removed")
	}
	if regenerated, err := generate.GenerateProjectFiles(".", generate.GenerationSelection{Notification: true}); err != nil || regenerated.ChangedFiles != 0 {
		t.Fatalf("removal left regeneration drift: %#v %v", regenerated, err)
	}
}

// TestGenerateAppResourcesSelection keeps ownership reconciliation limited to the enabled App-aware resources.
func TestGenerateAppResourcesSelection(t *testing.T) {
	for _, test := range []struct {
		name                  string
		feature, notification bool
	}{
		{"disabled", false, false}, {"feature", true, false}, {"notification", false, true}, {"both", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			usePrimitiveRendererRoot(t)
			config := &project.Config{GoModuleName: "example.com/selection", Render: project.RenderConfig{Components: project.Components{CLI: true, Cache: true, Feature: test.feature, Notification: test.notification}}}
			if err := writeProjectConfig(".goforj.yml", config); err != nil {
				t.Fatal(err)
			}
			writePrimitiveRendererFile(t, ".env", "CACHE_DRIVER=memory\nFEATURE_DRIVER=memory\nNOTIFICATION_DRIVER=log\n")
			writePrimitiveRendererFile(t, generate.CacheManagerPath, "untouched cache owner\n")
			paths, err := projectRendererForTest(t, config).runGenerateAppResources()
			if err != nil {
				t.Fatal(err)
			}
			want := generate.OutputPaths(generate.GenerationSelection{Feature: test.feature, Notification: test.notification})
			if !slices.Equal(paths, want) {
				t.Fatalf("paths=%v want=%v", paths, want)
			}
			for path, enabled := range map[string]bool{generate.FeatureManagerPath: test.feature, generate.NotificationManagerPath: test.notification} {
				_, err := os.Stat(path)
				if enabled && err != nil || !enabled && !os.IsNotExist(err) {
					t.Fatalf("%s enabled=%v: %v", path, enabled, err)
				}
			}
			if readPrimitiveRendererFile(t, generate.CacheManagerPath) != "untouched cache owner\n" {
				t.Fatal("unrelated resource regenerated")
			}
		})
	}
}
