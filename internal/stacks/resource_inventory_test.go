package stacks

import (
	"reflect"
	"testing"
)

// TestResourcesForRetainsProjectAndSelectedDeclarations keeps saved-only providers visible without changing the active session.
func TestResourcesForRetainsProjectAndSelectedDeclarations(t *testing.T) {
	root := fixture(t)
	put(t, root, ".env.example", "DB_EXAMPLE_DATABASE=example\n")
	put(t, root, ".env.stack.services", "DB_DRIVER=sqlite\nDB_SUPPORTED_DRIVERS=sqlite,postgres\nDB_ANALYTICS_DRIVER=postgres\nADMIN_DB_REPORTS_DRIVER=postgres\n")
	s := openTest(t, root)
	before := make([]string, 0, len(s.Resources))
	for _, resource := range s.Resources {
		before = append(before, resource.Key)
	}
	current := clone(s.Current)
	values, err := s.Load("services", false)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, resource := range s.ResourcesFor(values) {
		if got[resource.Key] {
			t.Fatalf("duplicate resource: %s", resource.Key)
		}
		got[resource.Key] = true
	}
	for _, key := range []string{"DB_DRIVER", "DB_EXAMPLE_DRIVER", "DB_ANALYTICS_DRIVER", "ADMIN_DB_REPORTS_DRIVER", "CACHE_DRIVER"} {
		if !got[key] {
			t.Errorf("missing %s", key)
		}
	}
	after := make([]string, 0, len(s.Resources))
	for _, resource := range s.Resources {
		after = append(after, resource.Key)
	}
	if !reflect.DeepEqual(after, before) || !reflect.DeepEqual(s.Current, current) {
		t.Fatal("selected inventory changed active session")
	}
}
