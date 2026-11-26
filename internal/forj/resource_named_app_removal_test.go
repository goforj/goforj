package forj

import (
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/project"
)

// TestRemoveCacheNamedAppPreservesDefaultResources keeps generated caller APIs available when removal refreshes resources.
func TestRemoveCacheNamedAppPreservesDefaultResources(t *testing.T) {
	for _, test := range []struct {
		name                         string
		configured, feature, sibling bool
	}{
		{name: "conventional", feature: true},
		{name: "configured", configured: true, feature: true},
		{name: "configured without Feature", configured: true},
		{name: "sibling Cache", configured: true, feature: true, sibling: true},
		{name: "sibling Cache without removed Feature", configured: true, sibling: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			usePrimitiveRendererRoot(t)
			components := project.Components{CLI: true, Feature: true, Cache: true}
			config := &project.Config{ProjectName: "Resource Removal", GoModuleName: "example.com/removal", Render: project.RenderConfig{Components: components}}
			if test.configured {
				config.Apps = map[string]project.AppConfig{
					"cache":       {Components: project.Components{CLI: true, Feature: test.feature, Cache: true}},
					"cache-child": {Components: project.Components{CLI: true, Feature: true}},
				}
			}
			if test.sibling {
				config.Render.Components.Cache = false
				config.Apps["billing"] = project.AppConfig{Components: project.Components{CLI: true, Cache: true}}
			}
			if err := writeProjectConfig(".goforj.yml", config); err != nil {
				t.Fatal(err)
			}
			app := project.DefaultNamedApp("cache")
			writePrimitiveRendererFile(t, app.Entrypoint, "package main\n")
			writePrimitiveRendererFile(t, project.DefaultNamedApp("cache-child").Entrypoint, "package main\n")
			preserved := []string{"CACHE_DRIVER=memory", "CACHE_SUPPORTED_DRIVERS=memory,redis", "CACHE_REPORTS_DRIVER=redis", "CACHE_REPORTS_ADDR=reports.example:6379", "CACHE_CACHE_DRIVER=redis", "CACHE_CACHE_ADDR=cache.example:6379", "CACHE_CHILD_FEATURE_DRIVER=memory", "CACHE_CHILD_FEATURE_NESTED_DRIVER=memory", "CACHE_CHILD_CUSTOM_VALUE=keep"}
			featureSettings := []string{"CACHE_FEATURE_DRIVER=redis", "CACHE_FEATURE_REMOVED_DRIVER=memory"}
			removed := []string{"CACHE_CUSTOM_SETTING=remove"}
			if test.feature {
				featureSettings[0] = "CACHE_FEATURE_DRIVER=memory"
				removed = append(removed, featureSettings...)
			} else {
				preserved = append(preserved, featureSettings...)
			}
			contents := "FEATURE_DRIVER=memory\n" + strings.Join(append(append([]string{}, preserved...), removed...), "\n") + "\n"
			for _, path := range []string{".env", ".env.host", ".env.example"} {
				writePrimitiveRendererFile(t, path, contents)
			}
			selection := generate.GenerationSelectionFromComponents(project.ProjectComponents(config))
			if _, err := generate.GenerateProjectFiles(".", selection); err != nil {
				t.Fatal(err)
			}
			accessorNames := []string{"Reports()", "Cache()"}
			if !test.feature {
				accessorNames = append(accessorNames, "Feature()", "FeatureRemoved()")
			}
			for _, name := range accessorNames {
				if !strings.Contains(readPrimitiveRendererFile(t, generate.CacheAccessorsPath), name) {
					t.Fatalf("fixture lacks the default %s accessor", name)
				}
			}
			if _, err := unitProjectRenderer(t).RemoveApp(app); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{".env", ".env.host", ".env.example"} {
				body := readPrimitiveRendererFile(t, path)
				for _, setting := range preserved {
					if !strings.Contains(body, setting+"\n") {
						t.Errorf("%s lost %s", path, setting)
					}
				}
				for _, setting := range removed {
					if strings.Contains(body, setting+"\n") {
						t.Errorf("%s retained %s", path, setting)
					}
				}
			}
			for _, name := range accessorNames {
				if !strings.Contains(readPrimitiveRendererFile(t, generate.CacheAccessorsPath), name) {
					t.Fatalf("App removal deleted the default %s accessor", name)
				}
			}
			if body := readPrimitiveRendererFile(t, generate.FeatureAccessorsPath); !strings.Contains(body, "Nested()") || strings.Contains(body, "Removed()") {
				t.Fatalf("remaining Feature accessors: %s", body)
			}
			if result, err := generate.GenerateProjectFiles(".", selection); err != nil || result.ChangedFiles != 0 {
				t.Fatalf("regeneration: %#v %v", result, err)
			}
		})
	}
}

// TestRemoveCatalogNamedAppPreservesDefaultAccessors applies the same ownership rule to every legacy resource family.
func TestRemoveCatalogNamedAppPreservesDefaultAccessors(t *testing.T) {
	for _, test := range []struct {
		name, prefix, driver, accessors string
		components                      project.Components
	}{
		{name: "db", prefix: "DB", driver: "sqlite", accessors: generate.DatabaseConnectionsPath, components: project.Components{DatabaseSQLite: true}},
		{name: "queue", prefix: "QUEUE", driver: "workerpool", accessors: generate.QueueAccessorsPath, components: project.Components{Jobs: true}},
		{name: "events", prefix: "EVENTS", driver: "inproc", accessors: generate.EventsAccessorsPath, components: project.Components{Events: true}},
		{name: "storage", prefix: "STORAGE", driver: "local", accessors: generate.StorageAccessorsPath, components: project.Components{Storage: true}},
		{name: "mail", prefix: "MAIL", driver: "log", accessors: generate.MailAccessorsPath, components: project.Components{Mail: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, sibling := range []bool{false, true} {
				t.Run(map[bool]string{false: "default consumer", true: "sibling consumer"}[sibling], func(t *testing.T) {
					usePrimitiveRendererRoot(t)
					components := test.components
					components.CLI, components.Feature = true, true
					config := &project.Config{ProjectName: "Catalog Removal", GoModuleName: "example.com/removal", Render: project.RenderConfig{Components: components}, Apps: map[string]project.AppConfig{test.name: {Components: project.Components{CLI: true, Feature: true}}}}
					if sibling {
						config.Render.Components = project.Components{CLI: true, Feature: true}
						config.Apps["billing"] = project.AppConfig{Components: components}
					}
					if err := writeProjectConfig(".goforj.yml", config); err != nil {
						t.Fatal(err)
					}
					app := project.DefaultNamedApp(test.name)
					writePrimitiveRendererFile(t, "go.mod", "module example.com/removal\n\ngo 1.26\n")
					writePrimitiveRendererFile(t, "internal/runtime/runtime.go", "package runtime\n")
					writePrimitiveRendererFile(t, app.Entrypoint, "package main\n")
					preserved := []string{test.prefix + "_DRIVER=" + test.driver, test.prefix + "_SUPPORTED_DRIVERS=" + test.driver, test.prefix + "_REPORTS_DRIVER=" + test.driver}
					removed := test.prefix + "_FEATURE_DRIVER=memory"
					contents := "FEATURE_DRIVER=memory\n" + strings.Join(preserved, "\n") + "\n" + removed + "\n"
					for _, path := range []string{".env", ".env.host", ".env.example"} {
						writePrimitiveRendererFile(t, path, contents)
					}
					selection := generate.GenerationSelectionFromComponents(project.ProjectComponents(config))
					if _, err := generate.GenerateProjectFiles(".", selection); err != nil {
						t.Fatal(err)
					}
					if !strings.Contains(readPrimitiveRendererFile(t, test.accessors), "Reports()") {
						t.Fatal("fixture lacks the default Reports accessor")
					}
					if _, err := unitProjectRenderer(t).RemoveApp(app); err != nil {
						t.Fatal(err)
					}
					for _, path := range []string{".env", ".env.host", ".env.example"} {
						body := readPrimitiveRendererFile(t, path)
						for _, setting := range preserved {
							if !strings.Contains(body, setting+"\n") {
								t.Errorf("%s lost %s", path, setting)
							}
						}
						if strings.Contains(body, removed+"\n") {
							t.Errorf("%s retained removed App Feature setting", path)
						}
					}
					if !strings.Contains(readPrimitiveRendererFile(t, test.accessors), "Reports()") {
						t.Fatal("App removal deleted the default Reports accessor")
					}
					if result, err := generate.GenerateProjectFiles(".", selection); err != nil || result.ChangedFiles != 0 {
						t.Fatalf("regeneration: %#v %v", result, err)
					}
				})
			}
		})
	}
}
