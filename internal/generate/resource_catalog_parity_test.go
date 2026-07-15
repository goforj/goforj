package generate

import (
	"reflect"
	"sort"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestResourceCatalogMatchesGeneratorInventories prevents the wizard from advertising a driver generation cannot build.
func TestResourceCatalogMatchesGeneratorInventories(t *testing.T) {
	tests := []struct {
		resource  project.ResourceKey
		inventory map[string]map[string]struct{}
	}{
		{resource: project.ResourceCache, inventory: cacheDriverKeys},
		{resource: project.ResourceQueue, inventory: queueDriverKeys},
		{resource: project.ResourceEvents, inventory: eventDriverKeys},
		{resource: project.ResourceStorage, inventory: storageDriverKeys},
		{resource: project.ResourceMail, inventory: mailDriverKeys},
	}
	for _, test := range tests {
		t.Run(string(test.resource), func(t *testing.T) {
			definition, ok := project.ResourceDefinitionByKey(test.resource)
			if !ok {
				t.Fatalf("resource definition %q is missing", test.resource)
			}
			catalogNames := make([]string, 0, len(definition.Drivers))
			for _, driver := range definition.Drivers {
				catalogNames = append(catalogNames, driver.Name)
			}
			generatorNames := make([]string, 0, len(test.inventory))
			for driver := range test.inventory {
				generatorNames = append(generatorNames, driver)
			}
			sort.Strings(catalogNames)
			sort.Strings(generatorNames)
			if !reflect.DeepEqual(catalogNames, generatorNames) {
				t.Fatalf("catalog drivers = %#v, generator drivers = %#v", catalogNames, generatorNames)
			}
		})
	}
}

// TestDatabaseResourceCatalogMatchesGeneratorInventory keeps the transitional database flags on supported implementations only.
func TestDatabaseResourceCatalogMatchesGeneratorInventory(t *testing.T) {
	definition, ok := project.ResourceDefinitionByKey(project.ResourceDatabase)
	if !ok {
		t.Fatal("database resource definition is missing")
	}
	drivers := make([]string, 0, len(definition.Drivers))
	for _, driver := range definition.Drivers {
		drivers = append(drivers, driver.Name)
	}
	sort.Strings(drivers)
	if want := []string{"mysql", "postgres", "sqlite"}; !reflect.DeepEqual(drivers, want) {
		t.Fatalf("database catalog drivers = %#v, want %#v", drivers, want)
	}
}
