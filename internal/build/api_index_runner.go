package build

import (
	"context"
	"os"
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

func (r *APIIndexRunner) RunQuiet() (string, error) {
	if r.runQuietFunc != nil {
		if err := r.runQuietFunc(); err != nil {
			return "", err
		}
		return "", nil
	}
	return r.runQuietWithStatus(".", "build/api_index.json", "build/api_index.diagnostics.json", "build/openapi.json")
}

func (r *APIIndexRunner) runQuietWithStatus(root string, out string, diagnostics string, openAPI string) (string, error) {
	beforeOut, beforeOutExists, err := readFileSnapshot(out)
	if err != nil {
		return "", err
	}
	beforeDiagnostics, beforeDiagnosticsExists, err := readFileSnapshot(diagnostics)
	if err != nil {
		return "", err
	}
	beforeOpenAPI, beforeOpenAPIExists, err := readFileSnapshot(openAPI)
	if err != nil {
		return "", err
	}
	if err := r.Run(root, out, diagnostics, openAPI, false); err != nil {
		return "", err
	}
	afterOut, afterOutExists, err := readFileSnapshot(out)
	if err != nil {
		return "", err
	}
	afterDiagnostics, afterDiagnosticsExists, err := readFileSnapshot(diagnostics)
	if err != nil {
		return "", err
	}
	afterOpenAPI, afterOpenAPIExists, err := readFileSnapshot(openAPI)
	if err != nil {
		return "", err
	}
	if beforeOutExists && beforeDiagnosticsExists && beforeOpenAPIExists &&
		afterOutExists && afterDiagnosticsExists && afterOpenAPIExists &&
		string(beforeOut) == string(afterOut) &&
		string(beforeDiagnostics) == string(afterDiagnostics) &&
		string(beforeOpenAPI) == string(afterOpenAPI) {
		return "no changes", nil
	}
	return "", nil
}

func readFileSnapshot(path string) ([]byte, bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return content, true, nil
}
