package build

import (
	"context"
	"os"
	"path/filepath"
	"reflect"

	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
	"github.com/goforj/web/webindex"
)

const noChangesStatus = "no changes"

type apiIndexPaths struct {
	root             string
	out              string
	diagnostics      string
	openAPI          string
	routeComposition string
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
	paths, err := resolveAPIIndexPaths(root, out, diagnostics, openAPI, "")
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
	paths := defaultAPIIndexPaths(activeAppTarget())
	paths.routeComposition = existingRouteCompositionPath(paths.routeComposition)
	return r.runDefaultWithStatus(".", paths.out, paths.diagnostics, paths.openAPI, paths.routeComposition)
}

func (r *APIIndexRunner) runDefaultWithStatus(root string, out string, diagnostics string, openAPI string, routeComposition string) (string, error) {
	paths, err := resolveAPIIndexPaths(root, out, diagnostics, openAPI, routeComposition)
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

func resolveAPIIndexPaths(root string, out string, diagnostics string, openAPI string, routeComposition string) (apiIndexPaths, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return apiIndexPaths{}, err
	}
	return apiIndexPaths{
		root:             absRoot,
		out:              out,
		diagnostics:      diagnostics,
		openAPI:          openAPI,
		routeComposition: routeComposition,
	}, nil
}

func (r *APIIndexRunner) runIndex(paths apiIndexPaths) (webindex.Manifest, error) {
	options := webindex.IndexOptions{
		Root:            paths.root,
		OutPath:         paths.out,
		DiagnosticsPath: paths.diagnostics,
		OpenAPIPath:     paths.openAPI,
	}
	applyRouteCompositionPath(&options, paths.routeComposition)
	return webindex.Run(context.Background(), options)
}

func defaultAPIIndexPaths(target project.AppTarget) apiIndexPaths {
	if target.Name == "" || target.Name == project.DefaultAppTargetName {
		return apiIndexPaths{
			out:              "build/api_index.json",
			diagnostics:      "build/api_index.diagnostics.json",
			openAPI:          "build/openapi.json",
			routeComposition: filepath.Join("app", "routes.go"),
		}
	}
	buildDir := filepath.Join("build", target.Name)
	return apiIndexPaths{
		out:              filepath.Join(buildDir, "api_index.json"),
		diagnostics:      filepath.Join(buildDir, "api_index.diagnostics.json"),
		openAPI:          filepath.Join(buildDir, "openapi.json"),
		routeComposition: filepath.Join(target.AppDir, "routes.go"),
	}
}

func applyRouteCompositionPath(options *webindex.IndexOptions, routeComposition string) {
	routeComposition = filepath.ToSlash(filepath.Clean(routeComposition))
	if routeComposition == "." || routeComposition == "" {
		return
	}
	field := reflect.ValueOf(options).Elem().FieldByName("RouteCompositionPath")
	if field.IsValid() && field.CanSet() && field.Kind() == reflect.String {
		field.SetString(routeComposition)
	}
}

func existingRouteCompositionPath(routeComposition string) string {
	if routeComposition == "" {
		return ""
	}
	if _, err := os.Stat(routeComposition); err != nil {
		return ""
	}
	return routeComposition
}

func (r *APIIndexRunner) logManifestSummary(manifest webindex.Manifest, paths apiIndexPaths) {
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
