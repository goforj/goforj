package build

import (
	"context"
	"os"
	"path/filepath"

	"github.com/goforj/goforj/internal/apix"
	"github.com/goforj/goforj/internal/logger"
)

const noChangesStatus = "no changes"

type apiIndexPaths struct {
	root        string
	out         string
	diagnostics string
	openAPI     string
}

type fileSnapshot struct {
	content []byte
	exists  bool
}

type APIIndexRunner struct {
	logger         *logger.AppLogger
	runDefaultFunc func() error
}

func NewAPIIndexRunner(appLogger *logger.AppLogger) *APIIndexRunner {
	return &APIIndexRunner{logger: appLogger}
}

func (r *APIIndexRunner) Run(root string, out string, diagnostics string, openAPI string, emitLog bool) error {
	paths, err := resolveAPIIndexPaths(root, out, diagnostics, openAPI)
	if err != nil {
		return err
	}

	manifest, err := r.runIndex(paths)
	if err != nil {
		return err
	}

	if emitLog {
		r.logManifestSummary(manifest, paths)
	}
	return nil
}

func (r *APIIndexRunner) RunDefaultWithStatus() (string, error) {
	if r.runDefaultFunc != nil {
		if err := r.runDefaultFunc(); err != nil {
			return "", err
		}
		return "", nil
	}
	return r.runDefaultWithStatus(".", "build/api_index.json", "build/api_index.diagnostics.json", "build/openapi.json")
}

func (r *APIIndexRunner) runDefaultWithStatus(root string, out string, diagnostics string, openAPI string) (string, error) {
	paths, err := resolveAPIIndexPaths(root, out, diagnostics, openAPI)
	if err != nil {
		return "", err
	}
	before, err := readAPIIndexSnapshots(paths)
	if err != nil {
		return "", err
	}
	if _, err := r.runIndex(paths); err != nil {
		return "", err
	}
	after, err := readAPIIndexSnapshots(paths)
	if err != nil {
		return "", err
	}
	if before.equal(after) {
		return noChangesStatus, nil
	}
	return "", nil
}

func resolveAPIIndexPaths(root string, out string, diagnostics string, openAPI string) (apiIndexPaths, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return apiIndexPaths{}, err
	}
	return apiIndexPaths{
		root:        absRoot,
		out:         out,
		diagnostics: diagnostics,
		openAPI:     openAPI,
	}, nil
}

func (r *APIIndexRunner) runIndex(paths apiIndexPaths) (apix.Manifest, error) {
	return apix.Run(context.Background(), apix.IndexOptions{
		Root:            paths.root,
		OutPath:         paths.out,
		DiagnosticsPath: paths.diagnostics,
		OpenAPIPath:     paths.openAPI,
	})
}

func (r *APIIndexRunner) logManifestSummary(manifest apix.Manifest, paths apiIndexPaths) {
	r.logger.Info().
		Any("operations", len(manifest.Operations)).
		Any("schemas", len(manifest.Schemas)).
		Any("diagnostics", len(manifest.Diagnostics)).
		Any("out", paths.out).
		Any("diagnostics_out", paths.diagnostics).
		Any("openapi_out", paths.openAPI).
		Msg("API index generated")
}

type apiIndexSnapshots struct {
	out         fileSnapshot
	diagnostics fileSnapshot
	openAPI     fileSnapshot
}

func readAPIIndexSnapshots(paths apiIndexPaths) (apiIndexSnapshots, error) {
	out, err := readFileSnapshot(paths.out)
	if err != nil {
		return apiIndexSnapshots{}, err
	}
	diagnostics, err := readFileSnapshot(paths.diagnostics)
	if err != nil {
		return apiIndexSnapshots{}, err
	}
	openAPI, err := readFileSnapshot(paths.openAPI)
	if err != nil {
		return apiIndexSnapshots{}, err
	}
	return apiIndexSnapshots{
		out:         out,
		diagnostics: diagnostics,
		openAPI:     openAPI,
	}, nil
}

func (s apiIndexSnapshots) equal(other apiIndexSnapshots) bool {
	return s.out.equal(other.out) &&
		s.diagnostics.equal(other.diagnostics) &&
		s.openAPI.equal(other.openAPI)
}

func (s fileSnapshot) equal(other fileSnapshot) bool {
	if s.exists != other.exists {
		return false
	}
	if !s.exists {
		return true
	}
	return string(s.content) == string(other.content)
}

func readFileSnapshot(path string) (fileSnapshot, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileSnapshot{}, nil
		}
		return fileSnapshot{}, err
	}
	return fileSnapshot{content: content, exists: true}, nil
}
