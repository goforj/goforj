package forj

import (
	"slices"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/forj/makeapp"
	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/project"
)

// TestFeatureTemplatesRespectAppParticipation preserves default and named App component boundaries.
func TestFeatureTemplatesRespectAppParticipation(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		config := &project.Config{GoModuleName: "example.com/feature", Render: project.RenderConfig{Components: project.Components{CLI: true}}, Apps: map[string]project.AppConfig{"billing": {Components: project.Components{CLI: true, Feature: enabled}}}}
		workspace := currentProjectRenderWorkspace(t)
		for _, app := range []project.App{project.DefaultApp(), project.DefaultNamedApp("billing")} {
			data := workspace.templateDataForApp(config, app)
			expected := enabled && app.Name == "billing"
			for _, path := range []string{"wire/app.go.tmpl", "wire/inject_managers.go.tmpl", "wire/inject_http.go.tmpl"} {
				source := renderSharedTemplate(t, path, data)
				assertFormattedGoTemplate(t, path, source)
				assertTemplateMarker(t, path, source, "/internal/features", expected)
				if path == "wire/inject_managers.go.tmpl" && expected && !strings.Contains(source, `features.NewManagerForApp("BILLING")`) {
					t.Fatalf("wrong App prefix: %s", source)
				}
			}
		}
		data := workspace.templateDataForApp(config, project.DefaultApp())
		source := renderSharedTemplate(t, ".env.tmpl", data)
		assertTemplateMarker(t, ".env.tmpl", source, "FEATURE_DRIVER=", enabled)
		assertTemplateMarker(t, ".env.tmpl", source, "BILLING_FEATURE_DRIVER=", enabled)
		assertTemplateMarker(t, ".env.tmpl", source, "FEATURE_DOCUMENT=", false)
		assertTemplateMarker(t, ".env.tmpl", source, "FEATURE_INITIALIZATION_TIMEOUT=", false)
		assertTemplateMarker(t, ".env.tmpl", source, "FEATURE_REFRESH_INTERVAL=", false)
		assertTemplateMarker(t, ".env.tmpl", source, "FEATURE_CHECKOUT_REFRESH_INTERVAL=", false)
	}
}

// TestFeatureRemovalRejectsExistingResources prevents an optional component change from silently abandoning providers.
func TestFeatureRemovalRejectsExistingResources(t *testing.T) {
	usePrimitiveRendererRoot(t)
	config := &project.Config{GoModuleName: "example.com/features", Render: project.RenderConfig{Components: project.Components{CLI: true}}}
	writePrimitiveRendererFile(t, "internal/features/manager_gen.go", "package features\n")
	err := projectRendererForTest(t, config).validateFeatureRenderTransition(project.ProjectComponents(config))
	if err == nil || !strings.Contains(err.Error(), "cannot disable Feature") {
		t.Fatalf("removal error = %v", err)
	}
}

// TestFeatureAppRemovalRejectsExistingAccessor preserves the owning App's explicit migration boundary.
func TestFeatureAppRemovalRejectsExistingAccessor(t *testing.T) {
	usePrimitiveRendererRoot(t)
	config := &project.Config{GoModuleName: "example.com/features", Render: project.RenderConfig{Components: project.Components{CLI: true, Feature: true}}, Apps: map[string]project.AppConfig{"billing": {Components: project.Components{CLI: true}}}}
	writePrimitiveRendererFile(t, "app/billing/wire/app.go", "package wire\ntype App struct{}\nfunc (a *App) Features() any { return nil }\n")
	err := projectRendererForTest(t, config).validateFeatureRenderTransition(project.ProjectComponents(config))
	if err == nil || !strings.Contains(err.Error(), "cannot disable Feature for App") {
		t.Fatalf("App removal error = %v", err)
	}
}

// TestFeatureMakeAppRefreshesOwnership updates compiled App registration before wiring a new App.
func TestFeatureMakeAppRefreshesOwnership(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			usePrimitiveRendererRoot(t)
			config := &project.Config{ProjectName: "Feature Apps", GoModuleName: "example.com/featureapps", Render: project.RenderConfig{Components: project.Components{CLI: true, Feature: true}}}
			if err := writeProjectConfig(".goforj.yml", config); err != nil {
				t.Fatal(err)
			}
			writePrimitiveRendererFile(t, ".env", "FEATURE_DRIVER=memory\nFEATURE_SUPPORTED_DRIVERS=memory,file\n")
			if _, err := generate.GenerateFeatureFiles("."); err != nil {
				t.Fatal(err)
			}
			renderer := unitProjectRenderer(t)
			if err := renderer.RenderAppOnly(project.DefaultNamedApp("billing"), makeapp.RenderOptions{Components: project.Components{CLI: true, Feature: enabled}, SkipWire: true}); err != nil {
				t.Fatal(err)
			}
			body := readPrimitiveRendererFile(t, generate.FeatureManagerPath)
			if strings.Contains(body, `"BILLING":`) != enabled {
				t.Fatalf("wrong App participation: %s", body)
			}
			provider := readPrimitiveRendererFile(t, "app/billing/wire/inject_managers.go")
			if strings.Contains(provider, `features.NewManagerForApp("BILLING")`) != enabled {
				t.Fatal("provider and ownership disagree")
			}
		})
	}
}

// TestFeatureRemoveAppRefreshesOwnership removes stale App identity and accessors while retaining remaining resources.
func TestFeatureRemoveAppRefreshesOwnership(t *testing.T) {
	usePrimitiveRendererRoot(t)
	app := project.DefaultNamedApp("billing")
	config := &project.Config{ProjectName: "Feature Removal", GoModuleName: "example.com/removal", Render: project.RenderConfig{Components: project.Components{CLI: true, Feature: true}}, Apps: map[string]project.AppConfig{"billing": {Components: project.Components{CLI: true, Feature: true}}}}
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatal(err)
	}
	writePrimitiveRendererFile(t, app.Entrypoint, "package main\n")
	writePrimitiveRendererFile(t, ".env", "FEATURE_DRIVER=memory\nFEATURE_SHARED_DRIVER=memory\nBILLING_FEATURE_ONLY_DRIVER=memory\n")
	if _, err := generate.GenerateProjectFiles(".", generate.GenerationSelection{Feature: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readPrimitiveRendererFile(t, generate.FeatureManagerPath), `"BILLING":`) || !strings.Contains(readPrimitiveRendererFile(t, generate.FeatureAccessorsPath), "Only()") {
		t.Fatal("fixture missing App-owned feature resources")
	}
	result, err := unitProjectRenderer(t).RemoveApp(app)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(result.Updated, generate.FeatureManagerPath) || !slices.Contains(result.Updated, generate.FeatureAccessorsPath) {
		t.Fatal("removed feature ownership not reported")
	}
	manager := readPrimitiveRendererFile(t, generate.FeatureManagerPath)
	accessors := readPrimitiveRendererFile(t, generate.FeatureAccessorsPath)
	if strings.Contains(manager, `"BILLING":`) || strings.Contains(accessors, "Only()") {
		t.Fatal("removed App retained feature ownership")
	}
	if !strings.Contains(accessors, "Shared()") || !strings.Contains(manager, `"":`) {
		t.Fatal("remaining feature resources were removed")
	}
	if regenerated, err := generate.GenerateProjectFiles(".", generate.GenerationSelection{Feature: true}); err != nil || regenerated.ChangedFiles != 0 {
		t.Fatalf("removal left regeneration drift: %#v %v", regenerated, err)
	}
}
