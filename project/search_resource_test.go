package project

import "testing"

// TestSearchResourceIsOptIn verifies existing Projects keep their component and driver defaults.
func TestSearchResourceIsOptIn(t *testing.T) {
	if DefaultSelectedComponents().Search {
		t.Fatal("Search enabled by default")
	}
	components := Components{}
	components.SetEnabled(ComponentSearch, true)
	if !components.Enabled(ComponentSearch) {
		t.Fatal("Search component was not enabled")
	}
	plan, err := DefaultResourcePlan(components)
	if err != nil {
		t.Fatal(err)
	}
	selection, ok := plan.Selection(ResourceSearch)
	if !ok || selection.Active != "bleve" || len(selection.Supported) != 1 {
		t.Fatalf("default Search selection: %#v", selection)
	}
	selection.Active = "meilisearch"
	selection.Supported = []string{"bleve", "meilisearch"}
	plan, err = plan.WithSelection(ResourceSearch, selection).Normalized(components)
	if err != nil {
		t.Fatal(err)
	}
	services, err := ResolveServicePlan(plan, components, LocalServiceIntent{})
	if err != nil {
		t.Fatal(err)
	}
	service, ok := services.Requirement(ServiceSearchMeilisearch)
	if !ok || service.State != ServiceStateExternalRequired {
		t.Fatalf("external Search service: %#v", service)
	}
	components.SetEnabled(ComponentSearch, false)
	if components.Enabled(ComponentSearch) {
		t.Fatal("Search component was not disabled")
	}
	plan, err = DefaultResourcePlan(components)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := plan.Selection(ResourceSearch); ok {
		t.Fatal("disabled Search added a resource")
	}
}
