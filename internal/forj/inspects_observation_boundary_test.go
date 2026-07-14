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

	source := string(content)
	for _, forbidden := range []string{
		`"{{.GoModuleName}}/internal/caches"`,
		`"{{.GoModuleName}}/internal/storages"`,
		`"github.com/goforj/queue"`,
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("Inspects manager template imports optional primitive package %s", forbidden)
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
