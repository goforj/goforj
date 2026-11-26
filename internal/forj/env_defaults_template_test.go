package forj

import (
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestEnvDefaultsHelperStartsInFixtureDirectory prevents generated package initialization from loading the project environment before the helper scenario runs.
func TestEnvDefaultsHelperStartsInFixtureDirectory(t *testing.T) {
	content, err := templatesFS.ReadFile("internal/cmd/env_defaults_test.go.tmpl")
	if err != nil {
		t.Fatalf("read env defaults test template: %v", err)
	}
	if !strings.Contains(string(content), "cmd.Dir = dir") {
		t.Fatal("env defaults helper must start in its isolated fixture directory")
	}
	if !strings.Contains(string(content), `writeTestEnvFile(t, filepath.Join(dir, ".env"), "\n")`) {
		t.Fatal("APP_ENV defaults fixture must stop ancestor .env discovery")
	}
}

// TestAppResourceOverlayPolicyUsesProjectCapabilities preserves resource scopes even when only a named App enables Feature.
func TestAppResourceOverlayPolicyUsesProjectCapabilities(t *testing.T) {
	for _, test := range []struct {
		name           string
		defaultFeature bool
		namedFeature   bool
	}{
		{name: "disabled"},
		{name: "default App", defaultFeature: true},
		{name: "named App only", namedFeature: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := &project.Config{GoModuleName: "example.com/overlay", Render: project.RenderConfig{Components: project.Components{CLI: true, Feature: test.defaultFeature}}, Apps: map[string]project.AppConfig{"billing": {Components: project.Components{CLI: true, Feature: test.namedFeature}}}}
			data := currentProjectRenderWorkspace(t).templateDataForApp(config, project.DefaultApp())
			for _, path := range []string{"internal/cmd/env_defaults.go.tmpl", "internal/cmd/env_defaults_test.go.tmpl"} {
				source := renderSharedTemplate(t, path, data)
				assertFormattedGoTemplate(t, path, source)
				marker := `strings.CutPrefix(key, "FEATURE_")`
				if strings.HasSuffix(path, "_test.go.tmpl") {
					marker = "want = values[key]"
				}
				assertTemplateMarker(t, path, source, marker, test.defaultFeature || test.namedFeature)
			}
		})
	}
}

// TestNotificationOverlayPolicyUsesProjectCapabilities includes named-only Notification ownership in the legacy overlay policy.
func TestNotificationOverlayPolicyUsesProjectCapabilities(t *testing.T) {
	for _, test := range []struct {
		name        string
		root, named bool
	}{{"disabled", false, false}, {"default", true, false}, {"named", false, true}} {
		t.Run(test.name, func(t *testing.T) {
			config := &project.Config{GoModuleName: "example.com/overlay", Render: project.RenderConfig{Components: project.Components{CLI: true, Notification: test.root}}, Apps: map[string]project.AppConfig{"billing": {Components: project.Components{CLI: true, Notification: test.named}}}}
			data := currentProjectRenderWorkspace(t).templateDataForApp(config, project.DefaultApp())
			for _, path := range []string{"internal/cmd/env_defaults.go.tmpl", "internal/cmd/env_defaults_test.go.tmpl"} {
				source := renderSharedTemplate(t, path, data)
				assertFormattedGoTemplate(t, path, source)
				marker := `strings.CutPrefix(key, "NOTIFICATION_")`
				if strings.HasSuffix(path, "_test.go.tmpl") {
					marker = `key != "NOTIFICATION_BUSINESS_KEY"`
				}
				assertTemplateMarker(t, path, source, marker, test.root || test.named)
			}
		})
	}
}
