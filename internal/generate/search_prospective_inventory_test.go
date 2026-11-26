package generate

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestSearchProspectiveInventoryKeepsAppRegistrations distinguishes lost App registrations from surviving global accessors.
func TestSearchProspectiveInventoryKeepsAppRegistrations(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte("invalid: ["), 0600); err != nil {
		t.Fatal(err)
	}
	components := project.Components{CLI: true, Search: true}
	config := &project.Config{Render: project.RenderConfig{Components: components}, Apps: map[string]project.AppConfig{
		"billing": {Components: components}, "reports": {Components: components}, "billing-search": {Components: components}, "search-search": {Components: components}, "cache": {Components: project.Components{CLI: true}},
	}}
	values := map[string]string{"SEARCH_SHARED_PATH": "shared", "BILLING_SEARCH_CATALOG_PATH": "billing", "REPORTS_SEARCH_CATALOG_PATH": "reports", "BILLING_SEARCH_SEARCH_AUDIT_PATH": "audit", "SEARCH_SEARCH_SEARCH_RELEASE_PATH": "release", "CACHE_SEARCH_DRIVER": "memory"}
	before := ResourceNamesByApp(root, values, config)[project.ResourceSearch]
	want := map[string][]string{"": {"shared"}, "BILLING": {"catalog", "shared"}, "REPORTS": {"catalog", "shared"}, "BILLING_SEARCH": {"audit", "shared"}, "SEARCH_SEARCH": {"release", "shared"}}
	if !reflect.DeepEqual(before, want) {
		t.Fatalf("scopes=%#v want=%#v", before, want)
	}
	delete(values, "BILLING_SEARCH_CATALOG_PATH")
	after := ResourceNamesByApp(root, values, config)[project.ResourceSearch]
	if !slices.Equal(after["BILLING"], []string{"shared"}) || !slices.Equal(after["REPORTS"], []string{"catalog", "shared"}) {
		t.Fatalf("App registration loss=%#v", after)
	}
	input := generationInput{projectDir: root, config: config, environment: generationEnvironment{values: values}}
	input.appPrefixes = sortStrings(input.configuredAppEnvPrefixes())
	if !slices.Contains(discoverSearchNames(input), "catalog") {
		t.Fatal("global accessor unexpectedly disappeared")
	}
}

// TestSearchProspectiveOmissionMatchesDeletion excludes removed physical Apps without preserving their namespace as tombstones.
func TestSearchProspectiveOmissionMatchesDeletion(t *testing.T) {
	for _, name := range []string{"search", "search-search", "cache", "billing-search"} {
		t.Run(name, func(t *testing.T) {
			root := featureInventoryProject(t, "", name, "billing-search-reports")
			config := &project.Config{Render: project.RenderConfig{Components: project.Components{CLI: true, Search: true, Cache: true}}}
			values := map[string]string{"SEARCH_SHARED_PATH": "shared", "SEARCH_SEARCH_SEARCH_CATALOG_PATH": "catalog", "CACHE_SEARCH_DRIVER": "memory", "BILLING_SEARCH_SEARCH_AUDIT_PATH": "audit", "BILLING_SEARCH_REPORTS_SEARCH_RELEASE_PATH": "release"}
			projected := ResourceNamesByApp(root, values, config, name)
			if err := os.RemoveAll(filepath.Join(root, "cmd", name)); err != nil {
				t.Fatal(err)
			}
			if actual := ResourceNamesByApp(root, values, config); !reflect.DeepEqual(projected, actual) {
				t.Fatalf("projected=%#v actual=%#v", projected, actual)
			}
		})
	}
}
