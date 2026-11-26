package forj

import (
	"reflect"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/forj/makeapp"
	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/project"
)

// TestSearchAppOwnershipProtectsSurvivingConsumers extends App namespace guards to Search imports, getters, and private fields.
func TestSearchAppOwnershipProtectsSurvivingConsumers(t *testing.T) {
	for _, test := range []struct{ name, path, source string }{
		{"manager", "internal/owner/service.go", "package owner\nimport \"example.com/ownership/internal/searches\"\nvar manager *searches.Manager\n"},
		{"getter", "app/wire/custom.go", "package wire\nfunc use(a *App) { _ = a.Searches }\n"},
		{"private field", "app/wire/custom.go", "package wire\nfunc use(a *App) { _ = a.search.Cache }\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, config := appResourcePreflightFixture(t)
			config.Render.Components.Search = true
			if err := writeProjectConfig(".goforj.yml", config); err != nil {
				t.Fatal(err)
			}
			writePrimitiveRendererFile(t, ".env", "SEARCH_DRIVER=bleve\nSEARCH_CACHE_DRIVER=bleve\n")
			if _, err := generate.GenerateProjectFiles(".", generate.GenerationSelectionFromComponents(config.Render.Components)); err != nil {
				t.Fatal(err)
			}
			writePrimitiveRendererFile(t, test.path, test.source)
			before := appResourcePreflightSnapshot(t, root)
			err := unitProjectRenderer(t).RenderAppOnly(project.DefaultNamedApp("search"), makeapp.RenderOptions{Components: project.Components{CLI: true, Cache: true}, SkipWire: true})
			if err == nil || !strings.Contains(err.Error(), "established search resource") {
				t.Fatalf("guard=%v", err)
			}
			if after := appResourcePreflightSnapshot(t, root); !reflect.DeepEqual(before, after) {
				t.Fatal("rejected creation mutated project")
			}
		})
	}
}

// TestSearchAppRemovalProtectsSurvivingConsumers rejects removing the last registration used by source outside the removed App.
func TestSearchAppRemovalProtectsSurvivingConsumers(t *testing.T) {
	root, config := appResourcePreflightFixture(t)
	config.Render.Components.Search = true
	config.Apps = map[string]project.AppConfig{"search": {Components: project.Components{CLI: true, Search: true}}}
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatal(err)
	}
	writePrimitiveRendererFile(t, "cmd/search/main.go", "package main\n")
	writePrimitiveRendererFile(t, ".env", "SEARCH_DRIVER=bleve\nSEARCH_SEARCH_ONLY_PATH=only\n")
	if _, err := generate.GenerateProjectFiles(".", generate.GenerationSelectionFromComponents(config.Render.Components)); err != nil {
		t.Fatal(err)
	}
	writePrimitiveRendererFile(t, "internal/owner/service.go", "package owner\nimport \"example.com/ownership/internal/searches\"\nvar manager *searches.Manager\n")
	before := appResourcePreflightSnapshot(t, root)
	_, err := unitProjectRenderer(t).RemoveApp(project.DefaultNamedApp("search"))
	if err == nil || !strings.Contains(err.Error(), "established search resource") {
		t.Fatalf("guard=%v", err)
	}
	if after := appResourcePreflightSnapshot(t, root); !reflect.DeepEqual(before, after) {
		t.Fatal("rejected removal mutated project")
	}
}
