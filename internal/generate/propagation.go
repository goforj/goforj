package generate

import (
	"fmt"
	"go/format"
	"path/filepath"

	"github.com/goforj/goforj/templates"
)

// generateMessageContextPropagator supplies the shared adapter when an existing app regenerates either async manager.
func generateMessageContextPropagator(projectDir string) (bool, error) {
	source, err := templates.FS.ReadFile("internal/inspects/propagation_gen.go.tmpl")
	if err != nil {
		return false, fmt.Errorf("read message context propagator template: %w", err)
	}
	formatted, err := format.Source(source)
	if err != nil {
		return false, fmt.Errorf("format message context propagator: %w", err)
	}
	return writeGeneratedSource(filepath.Join(projectDir, MessageContextPropagatorPath), formatted)
}
