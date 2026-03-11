package build

import (
	"context"
	"path/filepath"

	"github.com/goforj/goforj/internal/apix"
	"github.com/goforj/goforj/internal/logger"
)

type APIIndexRunner struct {
	logger       *logger.AppLogger
	runQuietFunc func() error
}

func NewAPIIndexRunner(appLogger *logger.AppLogger) *APIIndexRunner {
	return &APIIndexRunner{logger: appLogger}
}

func (r *APIIndexRunner) Run(root string, out string, diagnostics string, openAPI string, emitLog bool) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	manifest, err := apix.Run(context.Background(), apix.IndexOptions{
		Root:            absRoot,
		OutPath:         out,
		DiagnosticsPath: diagnostics,
		OpenAPIPath:     openAPI,
	})
	if err != nil {
		return err
	}

	if emitLog {
		r.logger.Info().
			Any("operations", len(manifest.Operations)).
			Any("schemas", len(manifest.Schemas)).
			Any("diagnostics", len(manifest.Diagnostics)).
			Any("out", out).
			Any("diagnostics_out", diagnostics).
			Any("openapi_out", openAPI).
			Msg("API index generated")
	}
	return nil
}

func (r *APIIndexRunner) RunQuiet() error {
	if r.runQuietFunc != nil {
		return r.runQuietFunc()
	}
	return r.Run(".", "build/api_index.json", "build/api_index.diagnostics.json", "build/openapi.json", false)
}
