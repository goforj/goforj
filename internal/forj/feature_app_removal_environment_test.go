package forj

import (
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/project"
)

// TestFeatureRemoveResourceNamedAppPreservesEnvironment protects root resources and sibling Apps while cleaning the removed App.
func TestFeatureRemoveResourceNamedAppPreservesEnvironment(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(map[bool]string{false: "conventional", true: "configured"}[configured], func(t *testing.T) {
			usePrimitiveRendererRoot(t)
			config := &project.Config{ProjectName: "Feature Removal", GoModuleName: "example.com/removal", Render: project.RenderConfig{Components: project.Components{CLI: true, Feature: true, Cache: true}}}
			if configured {
				config.Apps = map[string]project.AppConfig{"feature": {Components: project.Components{CLI: true, Feature: true, Cache: true}}, "feature-child": {Components: project.Components{CLI: true, Feature: true, Cache: true}}}
			}
			if err := writeProjectConfig(".goforj.yml", config); err != nil {
				t.Fatal(err)
			}
			app := project.DefaultNamedApp("feature")
			writePrimitiveRendererFile(t, app.Entrypoint, "package main\n")
			writePrimitiveRendererFile(t, project.DefaultNamedApp("feature-child").Entrypoint, "package main\n")
			contents := "CACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory,redis\nFEATURE_DRIVER=memory\nFEATURE_DOCUMENT={\"schema\":1,\"flags\":{}}\nFEATURE_SUPPORTED_DRIVERS=memory,file\nFEATURE_CHILD_DRIVER=memory\nFEATURE_CHILD_DOCUMENT={\"schema\":1,\"flags\":{}}\nFEATURE_CHILD_FEATURE_DRIVER=memory\nFEATURE_CHILD_FEATURE_NESTED_DRIVER=memory\nFEATURE_CHILD_API_HTTP_PORT=8181\nFEATURE_CHILD_CUSTOM_VALUE=keep\nFEATURE_CHILD_CACHE_DRIVER=redis\n# Feature\nFEATURE_CACHE_DRIVER=redis\nFEATURE_FEATURE_DRIVER=memory\nFEATURE_FEATURE_REMOVED_DRIVER=memory\nFEATURE_CUSTOM_SETTING=remove\nUNRELATED_SETTING=keep\n"
			for _, path := range []string{".env", ".env.host", ".env.example"} {
				writePrimitiveRendererFile(t, path, contents)
			}
			if _, err := generate.GenerateProjectFiles(".", generate.GenerationSelection{Feature: true}); err != nil {
				t.Fatal(err)
			}
			if _, err := unitProjectRenderer(t).RemoveApp(app); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{".env", ".env.host", ".env.example"} {
				body := readPrimitiveRendererFile(t, path)
				for _, key := range []string{"FEATURE_DRIVER=memory", "FEATURE_DOCUMENT=", "FEATURE_SUPPORTED_DRIVERS=", "FEATURE_CHILD_DRIVER=memory", "FEATURE_CHILD_DOCUMENT=", "FEATURE_CHILD_FEATURE_DRIVER=memory", "FEATURE_CHILD_FEATURE_NESTED_DRIVER=memory", "FEATURE_CHILD_API_HTTP_PORT=8181", "FEATURE_CHILD_CUSTOM_VALUE=keep", "FEATURE_CHILD_CACHE_DRIVER=redis", "UNRELATED_SETTING=keep"} {
					if !strings.Contains(body, key) {
						t.Fatalf("%s lost %s: %s", path, key, body)
					}
				}
				for _, key := range []string{"FEATURE_FEATURE_DRIVER=", "FEATURE_FEATURE_REMOVED_DRIVER=", "FEATURE_CUSTOM_SETTING=", "FEATURE_CACHE_DRIVER=", "# Feature\n"} {
					if strings.Contains(body, key) {
						t.Fatalf("%s retained %s: %s", path, key, body)
					}
				}
			}
			accessors := readPrimitiveRendererFile(t, generate.FeatureAccessorsPath)
			if !strings.Contains(accessors, "Child()") || !strings.Contains(accessors, "Nested()") || strings.Contains(accessors, "Removed()") {
				t.Fatalf("remaining accessors: %s", accessors)
			}
			if result, err := generate.GenerateProjectFiles(".", generate.GenerationSelection{Feature: true}); err != nil || result.ChangedFiles != 0 {
				t.Fatalf("regeneration: %#v %v", result, err)
			}
		})
	}
}

// TestRemoveAppEnvironmentWithoutPreservationRetainsLegacyCleanup keeps direct cleanup callers and disabled components predictable.
func TestRemoveAppEnvironmentWithoutPreservationRetainsLegacyCleanup(t *testing.T) {
	for _, withPredicate := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "disabled component"}[withPredicate], func(t *testing.T) {
			usePrimitiveRendererRoot(t)
			writePrimitiveRendererFile(t, ".env", "# Feature\nFEATURE_DRIVER=memory\nFEATURE_CHILD_FEATURE_DRIVER=memory\nFEATURE_CUSTOM_SETTING=remove\nOTHER=keep\n")
			workspace := currentProjectRenderWorkspace(t)
			var predicates []func(string) bool
			if withPredicate {
				predicates = append(predicates, func(key string) bool {
					owner, managed := generate.AppResourceEnvironmentOwner(key, []string{"FEATURE", "FEATURE_CHILD"}, project.Components{})
					return managed && owner != "FEATURE"
				})
			}
			if changed, err := workspace.removeAppEnvDefaults(".env", "feature", predicates...); err != nil || !changed {
				t.Fatalf("cleanup: %t %v", changed, err)
			}
			if body := readPrimitiveRendererFile(t, ".env"); body != "OTHER=keep\n" {
				t.Fatalf("cleanup = %q", body)
			}
		})
	}
}

// TestRemoveAppPreservesLongerAppPrefixesWithoutFeature protects sibling settings independently of managed resource support.
func TestRemoveAppPreservesLongerAppPrefixesWithoutFeature(t *testing.T) {
	usePrimitiveRendererRoot(t)
	config := &project.Config{ProjectName: "App Removal", GoModuleName: "example.com/removal", Render: project.RenderConfig{Components: project.Components{CLI: true}}}
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatal(err)
	}
	app := project.DefaultNamedApp("feature")
	writePrimitiveRendererFile(t, app.Entrypoint, "package main\n")
	writePrimitiveRendererFile(t, project.DefaultNamedApp("feature-child").Entrypoint, "package main\n")
	for _, path := range []string{".env", ".env.host", ".env.example"} {
		writePrimitiveRendererFile(t, path, "# Feature\nFEATURE_DRIVER=memory\nFEATURE_CUSTOM_VALUE=remove\nFEATURE_CHILD_API_HTTP_PORT=8181\nFEATURE_CHILD_CUSTOM_VALUE=keep\nOTHER=keep\n")
	}
	if _, err := unitProjectRenderer(t).RemoveApp(app); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".env", ".env.host", ".env.example"} {
		if body := readPrimitiveRendererFile(t, path); body != "FEATURE_CHILD_API_HTTP_PORT=8181\nFEATURE_CHILD_CUSTOM_VALUE=keep\nOTHER=keep\n" {
			t.Fatalf("%s cleanup = %q", path, body)
		}
	}
}
