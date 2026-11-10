package apiindex

import (
	"reflect"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestOutputPathsMatchesAppPublication keeps framework upgrade ownership on the API indexer's own public output contract.
func TestOutputPathsMatchesAppPublication(t *testing.T) {
	for _, test := range []struct {
		app  project.App
		want []string
	}{
		{project.DefaultApp(), []string{"build/api_index.json", "build/api_index.diagnostics.json", "build/openapi.json"}},
		{project.AppForName("billing"), []string{"build/billing/api_index.json", "build/billing/api_index.diagnostics.json", "build/billing/openapi.json"}},
	} {
		if got := OutputPaths(test.app); !reflect.DeepEqual(got, test.want) {
			t.Fatalf("API output contracts=%v, want %v", got, test.want)
		}
	}
}
