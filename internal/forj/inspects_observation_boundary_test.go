package forj

import (
	"strings"
	"testing"
)

// TestInspectsTemplateOwnsPrimitiveObservationTypes prevents optional primitive packages from leaking back into Inspects.
func TestInspectsTemplateOwnsPrimitiveObservationTypes(t *testing.T) {
	content, err := templatesFS.ReadFile("internal/inspects/manager.go.tmpl")
	if err != nil {
		t.Fatalf("read Inspects manager template: %v", err)
	}
	storeContent, err := templatesFS.ReadFile("internal/inspects/store.go.tmpl")
	if err != nil {
		t.Fatalf("read Inspects store template: %v", err)
	}

	source := string(content)
	dependencySource := source + string(storeContent)
	for _, forbidden := range []string{
		`"{{.GoModuleName}}/internal/caches"`,
		`"{{.GoModuleName}}/internal/storages"`,
		`"github.com/goforj/cache"`,
		`"github.com/goforj/queue"`,
	} {
		if strings.Contains(dependencySource, forbidden) {
			t.Errorf("Inspects templates import optional primitive package %s", forbidden)
		}
	}

	for _, signature := range []string{
		"event CacheOperationInspectEvent",
		"event StorageOperationInspectEvent",
		"event QueueInspectEvent",
	} {
		if !strings.Contains(source, signature) {
			t.Errorf("Inspects manager template does not accept owned observation type %q", signature)
		}
	}
}

// TestInspectManagerWiringKeepsHistoryIndependentFromAppCache verifies only the cache manager depends on inspect observation.
func TestInspectManagerWiringKeepsHistoryIndependentFromAppCache(t *testing.T) {
	content, err := templatesFS.ReadFile("wire/inject_managers.go.tmpl")
	if err != nil {
		t.Fatalf("read manager wiring template: %v", err)
	}
	source := string(content)

	for _, expected := range []string{
		"func provideInspectManager() *inspects.Manager",
		"inspectManager *inspects.Manager",
		"observability.CacheInspectObserver(inspectManager)",
	} {
		if !strings.Contains(source, expected) {
			t.Errorf("manager wiring template does not contain %q", expected)
		}
	}
	if strings.Contains(source, "func provideInspectManager(cacheManager *caches.Manager)") {
		t.Error("inspect manager still depends on App Cache")
	}
}
