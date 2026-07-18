package devservices

import (
	"reflect"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestCatalogPublishesStableProfileProviders locks profile identity separately from infrastructure identity.
func TestCatalogPublishesStableProfileProviders(t *testing.T) {
	want := []Definition{
		{Key: KeyRedis, Label: "Redis", Profile: "redis", Provides: project.ServiceRedis, Order: 10},
		{Key: KeyRustFS, Label: "RustFS", Profile: "rustfs", Provides: project.ServiceStorageS3, Order: 20},
		{Key: KeyOpenSearch, Label: "OpenSearch", Profile: "opensearch", Order: 30},
	}
	if got := Catalog(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Catalog() = %#v, want %#v", got, want)
	}
	if want[1].Profile == string(want[1].Provides) {
		t.Fatal("RustFS profile identity collapsed into its S3 infrastructure identity")
	}
}

// TestCatalogReturnsDefensiveCopies prevents callers from mutating the process-wide service inventory.
func TestCatalogReturnsDefensiveCopies(t *testing.T) {
	definitions := Catalog()
	definitions[0].Profile = "changed"

	second := Catalog()
	if second[0].Profile != "redis" {
		t.Fatalf("Catalog() retained caller mutation: %#v", second)
	}
}

// TestDefinitionLookupsRequireExactIdentity prevents neighboring profile names from enabling built-ins.
func TestDefinitionLookupsRequireExactIdentity(t *testing.T) {
	if definition, ok := DefinitionByKey(KeyRustFS); !ok || definition.Profile != "rustfs" {
		t.Fatalf("DefinitionByKey(KeyRustFS) = %#v, %t", definition, ok)
	}
	if definition, ok := DefinitionByProfile(" rustfs "); !ok || definition.Key != KeyRustFS {
		t.Fatalf("DefinitionByProfile(rustfs) = %#v, %t", definition, ok)
	}
	if definition, ok := DefinitionByProfile("rustfs-debug"); ok {
		t.Fatalf("DefinitionByProfile(rustfs-debug) = %#v, want no match", definition)
	}
}

// TestEnabledReturnsExactProfilesInCatalogOrder keeps owner token order from changing generated presentation.
func TestEnabledReturnsExactProfilesInCatalogOrder(t *testing.T) {
	got := Enabled("opensearch,rustfs-debug,redis,rustfs,redis")
	want := []Definition{
		{Key: KeyRedis, Label: "Redis", Profile: "redis", Provides: project.ServiceRedis, Order: 10},
		{Key: KeyRustFS, Label: "RustFS", Profile: "rustfs", Provides: project.ServiceStorageS3, Order: 20},
		{Key: KeyOpenSearch, Label: "OpenSearch", Profile: "opensearch", Order: 30},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Enabled() = %#v, want %#v", got, want)
	}
}
