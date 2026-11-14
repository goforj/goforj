package localproviders

import (
	"testing"

	"github.com/goforj/goforj/internal/generate"
)

// TestSearchPortableRegeneration preserves named remote settings through portable publication and restoration without backend requests.
func TestSearchPortableRegeneration(t *testing.T) {
	root := fixture(t)
	put(t, root, ".goforj.yml", "module_name: example.org/searchportable\nrender:\n  components: [cli, search]\napps:\n  admin:\n    components: [cli, search]\n")
	retained := map[string]string{
		"SEARCH_CATALOG_URL":        "https://example.com/root-search",
		"SEARCH_CATALOG_API_KEY":    "example-root-key",
		"SEARCH_CATALOG_PREFIX":     "root_",
		"ADMIN_SEARCH_ONLY_URL":     "https://example.com/admin-search",
		"ADMIN_SEARCH_ONLY_API_KEY": "example-admin-key",
		"ADMIN_SEARCH_ONLY_PREFIX":  "admin_",
	}
	contents := "SEARCH_DRIVER=bleve\nSEARCH_SUPPORTED_DRIVERS=bleve,meilisearch\nSEARCH_CATALOG_DRIVER=meilisearch\nADMIN_SEARCH_ONLY_DRIVER=meilisearch\n"
	for key, value := range retained {
		contents += key + "=" + value + "\n"
	}
	put(t, root, ".env", contents)
	session := openTest(t, root)
	portable, err := session.Portable()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"SEARCH_CATALOG_DRIVER", "ADMIN_SEARCH_ONLY_DRIVER"} {
		if portable[key] != "bleve" {
			t.Fatalf("%s=%q", key, portable[key])
		}
	}
	if err := session.Apply(portable); err != nil {
		t.Fatal(err)
	}
	for pass := 0; pass < 2; pass++ {
		result, err := generate.GenerateProjectFiles(root, generate.GenerationSelection{Search: true})
		if err != nil {
			t.Fatal(err)
		}
		if pass == 1 && result.ChangedFiles != 0 {
			t.Fatalf("portable regeneration drift: %#v", result)
		}
	}
	session = openTest(t, root)
	for key, want := range retained {
		if session.Current[key] != want {
			t.Fatalf("portable changed %s", key)
		}
	}
	restored := clone(session.Current)
	restored["SEARCH_CATALOG_DRIVER"] = "meilisearch"
	restored["ADMIN_SEARCH_ONLY_DRIVER"] = "meilisearch"
	if err := session.Apply(restored); err != nil {
		t.Fatal(err)
	}
	for pass := 0; pass < 2; pass++ {
		result, err := generate.GenerateProjectFiles(root, generate.GenerationSelection{Search: true})
		if err != nil {
			t.Fatal(err)
		}
		if pass == 1 && result.ChangedFiles != 0 {
			t.Fatalf("restored regeneration drift: %#v", result)
		}
	}
	session = openTest(t, root)
	for key, want := range retained {
		if session.Current[key] != want {
			t.Fatalf("restoration changed %s", key)
		}
	}
}
