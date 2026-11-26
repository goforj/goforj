package generate

import (
	"github.com/goforj/goforj/project"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestSearchDiscoveryExcludesOtherAppResources uses the catalog boundary to keep App settings out of default index validation.
func TestSearchDiscoveryExcludesOtherAppResources(t *testing.T) {
	root := t.TempDir()
	config := "render:\n  components: [cli, search]\napps:\n  search:\n    components: [cli, search]\n  search-worker:\n    components: [cli, search]\n"
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	for _, resource := range generationResourceEnvPrefixes() {
		if resource == "SEARCH" {
			continue
		}
		t.Run(resource, func(t *testing.T) {
			values := map[string]string{"SEARCH_SHARED_PATH": "shared", "SEARCH_SEARCH_PATH": "app-root", "SEARCH_" + resource + "_DRIVER": "other-family-driver", "SEARCH_WORKER_" + resource + "_DRIVER": "other-family-driver"}
			input := fixtureGenerationInput(root, values)
			if names := discoverSearchNames(input); !slices.Equal(names, []string{"shared"}) {
				t.Fatalf("names=%v", names)
			}
			if _, err := generateSearchFiles(input); err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(filepath.Join(root, SearchAccessorsPath))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(body), "Cache()") || strings.Contains(string(body), "Notification()") {
				t.Fatalf("cross-family accessor: %s", body)
			}
		})
	}
}

// TestSearchEnvironmentRemovalOwner limits cleanup protection to enabled managed Search settings.
func TestSearchEnvironmentRemovalOwner(t *testing.T) {
	for _, test := range []struct {
		key, owner string
		managed    bool
	}{
		{"SEARCH_DRIVER", "", true}, {"SEARCH_SUPPORTED_DRIVERS", "", true}, {"SEARCH_CATALOG_API_KEY", "", true},
		{"SEARCH_SEARCH_DRIVER", "SEARCH", true}, {"SEARCH_CHILD_SEARCH_PATH", "SEARCH_CHILD", true},
		{"SEARCH_CACHE_DRIVER", "", false}, {"SEARCH_NOTIFICATION_DRIVER", "", false}, {"SEARCH_CUSTOM_VALUE", "", false},
	} {
		t.Run(test.key, func(t *testing.T) {
			owner, managed := AppResourceEnvironmentOwner(test.key, []string{"SEARCH", "SEARCH_CHILD"}, project.Components{Search: true})
			if owner != test.owner || managed != test.managed {
				t.Fatalf("owner=%q managed=%t", owner, managed)
			}
			owner, managed = AppResourceEnvironmentOwner(test.key, []string{"SEARCH", "SEARCH_CHILD"}, project.Components{})
			if owner != "" || managed {
				t.Fatalf("disabled owner=%q managed=%t", owner, managed)
			}
		})
	}
}
