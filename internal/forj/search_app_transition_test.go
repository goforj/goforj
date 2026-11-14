package forj

import (
	"slices"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/forj/makeapp"
	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/project"
)

// TestSearchMakeAppRefreshesOwnership updates the compiled prefix inventory even when the component union is unchanged.
func TestSearchMakeAppRefreshesOwnership(t *testing.T) {
	usePrimitiveRendererRoot(t)
	config := &project.Config{ProjectName: "Search Apps", GoModuleName: "example.com/searchapps", Render: project.RenderConfig{Components: project.Components{CLI: true, Search: true}}}
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatal(err)
	}
	writePrimitiveRendererFile(t, ".env", "SEARCH_DRIVER=bleve\nSEARCH_SUPPORTED_DRIVERS=bleve\n")
	if _, err := generate.GenerateSearchFiles("."); err != nil {
		t.Fatal(err)
	}
	renderer := unitProjectRenderer(t)
	if err := renderer.RenderAppOnly(project.DefaultNamedApp("search"), makeapp.RenderOptions{Components: project.Components{CLI: true, Search: true}, SkipWire: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readPrimitiveRendererFile(t, generate.SearchManagerPath), `[]string{"SEARCH"}`) {
		t.Fatal("new App missing from compiled ownership inventory")
	}
	if strings.Contains(readPrimitiveRendererFile(t, generate.SearchAccessorsPath), "Search()") {
		t.Fatal("App root became a default index")
	}
	if !strings.Contains(strings.Join(renderer.lines, "\n"), "--search") {
		t.Fatal("App reconciliation did not report Search generation")
	}
}

// TestSearchRemoveAppRefreshesOwnership removes App prefixes and accessors without changing remaining declared resource composition.
func TestSearchRemoveAppRefreshesOwnership(t *testing.T) {
	usePrimitiveRendererRoot(t)
	app := project.DefaultNamedApp("search")
	config := &project.Config{ProjectName: "Search Removal", GoModuleName: "example.com/searchremoval", Render: project.RenderConfig{Components: project.Components{CLI: true, Search: true}}, Apps: map[string]project.AppConfig{"search": {Components: project.Components{CLI: true, Search: true}}}}
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatal(err)
	}
	writePrimitiveRendererFile(t, app.Entrypoint, "package main\n")
	writePrimitiveRendererFile(t, ".env", "SEARCH_DRIVER=bleve\nSEARCH_SHARED_PATH=shared\nSEARCH_SEARCH_ONLY_PATH=only\n")
	if _, err := generate.GenerateProjectFiles(".", generate.GenerationSelection{Search: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readPrimitiveRendererFile(t, generate.SearchAccessorsPath), "Only()") {
		t.Fatal("fixture missing App accessor")
	}
	result, err := unitProjectRenderer(t).RemoveApp(app)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(result.Updated, generate.SearchManagerPath) || !slices.Contains(result.Updated, generate.SearchAccessorsPath) {
		t.Fatal("Search ownership updates not reported")
	}
	if strings.Contains(readPrimitiveRendererFile(t, generate.SearchManagerPath), `[]string{"SEARCH"}`) {
		t.Fatal("removed App prefix retained")
	}
	accessors := readPrimitiveRendererFile(t, generate.SearchAccessorsPath)
	if strings.Contains(accessors, "Only()") || strings.Contains(accessors, "SearchOnly()") || !strings.Contains(accessors, "Shared()") {
		t.Fatalf("wrong remaining accessors: %s", accessors)
	}
	if regenerated, err := generate.GenerateProjectFiles(".", generate.GenerationSelection{Search: true}); err != nil || regenerated.ChangedFiles != 0 {
		t.Fatalf("removal left generation drift: %#v %v", regenerated, err)
	}
}

// TestSearchEnvironmentRemovalOwnership preserves exact default and sibling settings while removing the App's own keys.
func TestSearchEnvironmentRemovalOwnership(t *testing.T) {
	prefixes := []string{"SEARCH", "SEARCH_WORKER", "SEARCH_SEARCH"}
	for _, test := range []struct {
		key, removed string
		preserve     bool
	}{
		{"SEARCH_DRIVER", "SEARCH", true},
		{"SEARCH_SHARED_PATH", "SEARCH", true},
		{"SEARCH_SHARED_API_KEY", "SEARCH", true},
		{"SEARCH_BUSINESS_KEY", "SEARCH", false},
		{"SEARCH_SEARCH_PATH", "SEARCH", false},
		{"SEARCH_SEARCH_ONLY_PATH", "SEARCH", false},
		{"SEARCH_WORKER_SEARCH_PATH", "SEARCH", true},
		{"SEARCH_WORKER_SEARCH_BUSINESS_KEY", "SEARCH", false},
		{"SEARCH_SEARCH_SEARCH_PATH", "SEARCH", true},
		{"SEARCH_SEARCH_ONLY_PATH", "SEARCH_SEARCH", true},
		{"SEARCH_SEARCH_SEARCH_PATH", "SEARCH_SEARCH", false},
		{"BILLING_SEARCH_PATH", "BILLING", false},
	} {
		t.Run(test.removed+"/"+test.key, func(t *testing.T) {
			if got := preserveSearchEnvForAppRemoval(test.key, test.removed, prefixes); got != test.preserve {
				t.Fatalf("preserve=%v want=%v", got, test.preserve)
			}
		})
	}
}
