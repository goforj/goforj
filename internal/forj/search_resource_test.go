package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestSearchResourceTemplateScopes verifies code, dependencies, schemas, and backend settings follow App participation.
func TestSearchResourceTemplateScopes(t *testing.T) {
	workspace := currentProjectRenderWorkspace(t)
	for _, enabled := range []bool{false, true} {
		config := &project.Config{ProjectName: "Search", GoModuleName: "example.com/search", Render: project.RenderConfig{Components: project.Components{CLI: true}}, Apps: map[string]project.AppConfig{"worker": {Components: project.Components{CLI: true, Search: enabled}}}}
		for _, app := range []project.App{project.DefaultApp(), project.DefaultNamedApp("worker")} {
			data := workspace.templateDataForApp(config, app)
			data.Resources.SearchDriver = "bleve"
			data.Resources.SearchSupportedDrivers = "bleve"
			want := enabled && app.Name == "worker"
			source := renderSharedTemplate(t, "wire/inject_managers.go.tmpl", data)
			if strings.Contains(source, "provideSearchManager") != want {
				t.Fatalf("App %s Search provider participation mismatch", app.Name)
			}
			assembly := renderSharedTemplate(t, "wire/app.go.tmpl", data)
			if strings.Contains(assembly, "Searches() *searches.Manager") != want {
				t.Fatalf("App %s Search accessor participation mismatch", app.Name)
			}
			renderer := projectRendererForTest(t, config)
			found := false
			for _, mapping := range renderer.appOwnedMappings(app) {
				if mapping.dest == filepath.Join(app.AppDir, "search.go") {
					found = true
				}
			}
			if found != want {
				t.Fatalf("App %s schema mapping mismatch", app.Name)
			}
		}
		data := workspace.templateDataForApp(config, project.DefaultApp())
		data.Resources.SearchDriver = "bleve"
		data.Resources.SearchSupportedDrivers = "bleve"
		environment := renderSharedTemplate(t, ".env.tmpl", data)
		if strings.Contains(environment, "SEARCH_DRIVER=") != enabled {
			t.Fatal("disabled component emitted Search environment")
		}
		if enabled && !strings.Contains(environment, "WORKER_SEARCH_PATH=storage/search/worker") {
			t.Fatal("named App did not get independent physical path")
		}
		overlay := renderSharedTemplate(t, "internal/cmd/env_defaults.go.tmpl", data)
		if strings.Contains(overlay, `strings.CutPrefix(key, "SEARCH_")`) != enabled {
			t.Fatal("Search projection protection does not follow project component union")
		}
	}
}

// TestSearchRemovalRequiresExplicitOwnerCleanup protects schemas from silent removal and stale compilation.
func TestSearchRemovalRequiresExplicitOwnerCleanup(t *testing.T) {
	root := usePrimitiveRendererRoot(t)
	renderer := unitProjectRenderer(t)
	renderer.config = &project.Config{GoModuleName: "example.com/search", Render: project.RenderConfig{Components: project.Components{CLI: true}}}
	if err := renderer.validateSearchRenderTransition(project.Components{}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "internal/searches"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := renderer.validateSearchRenderTransition(project.Components{}); err == nil {
		t.Fatal("disabled Search silently retained generated resources")
	}
	if err := os.Remove(filepath.Join(root, "internal/searches")); err != nil {
		t.Fatal(err)
	}
	writePrimitiveRendererFile(t, "app/search.go", "package app\n")
	if err := renderer.validateSearchRenderTransition(project.Components{Search: true}); err == nil {
		t.Fatal("disabled App silently retained its schema owner")
	}
}
