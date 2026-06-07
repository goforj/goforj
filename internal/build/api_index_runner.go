package build

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
	"github.com/goforj/web/webindex"
)

const noChangesStatus = "no changes"

type apiIndexPaths struct {
	root             string
	appTarget        string
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
	paths, err := resolveAPIIndexPaths(root, out, diagnostics, openAPI, "", "")
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
	target := activeAppTarget()
	paths := defaultAPIIndexPaths(target)
	routeComposition, err := existingRouteCompositionPath(target, paths.routeComposition)
	if err != nil {
		return "", err
	}
	paths.routeComposition = routeComposition
	if paths.routeComposition == "" {
		return apiIndexStatus(paths.appTarget, "no route composition"), nil
	}
	paths.root = "."
	return r.runDefaultWithStatus(paths)
}

func (r *APIIndexRunner) runDefaultWithStatus(paths apiIndexPaths) (string, error) {
	paths, err := resolveAPIIndexPaths(paths.root, paths.out, paths.diagnostics, paths.openAPI, paths.routeComposition, paths.appTarget)
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
		return apiIndexStatus(paths.appTarget, noChangesStatus), nil
	}
	return apiIndexStatus(paths.appTarget, ""), nil
}

func resolveAPIIndexPaths(root string, out string, diagnostics string, openAPI string, routeComposition string, appTarget string) (apiIndexPaths, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return apiIndexPaths{}, err
	}
	if appTarget == "" {
		appTarget = project.DefaultAppTargetName
	}
	return apiIndexPaths{
		root:             absRoot,
		appTarget:        appTarget,
		out:              out,
		diagnostics:      diagnostics,
		openAPI:          openAPI,
		routeComposition: routeComposition,
	}, nil
}

func (r *APIIndexRunner) runIndex(paths apiIndexPaths) (webindex.Manifest, error) {
	options := webindex.IndexOptions{
		Root:                 paths.root,
		OutPath:              paths.out,
		DiagnosticsPath:      paths.diagnostics,
		OpenAPIPath:          paths.openAPI,
		RouteCompositionPath: paths.routeComposition,
	}
	return webindex.Run(context.Background(), options)
}

func defaultAPIIndexPaths(target project.AppTarget) apiIndexPaths {
	if target.Name == "" || target.Name == project.DefaultAppTargetName {
		return apiIndexPaths{
			appTarget:        project.DefaultAppTargetName,
			out:              "build/api_index.json",
			diagnostics:      "build/api_index.diagnostics.json",
			openAPI:          "build/openapi.json",
			routeComposition: filepath.Join("app", "routes.go"),
		}
	}
	buildDir := filepath.Join("build", target.Name)
	return apiIndexPaths{
		appTarget:        target.Name,
		out:              filepath.Join(buildDir, "api_index.json"),
		diagnostics:      filepath.Join(buildDir, "api_index.diagnostics.json"),
		openAPI:          filepath.Join(buildDir, "openapi.json"),
		routeComposition: filepath.Join(target.AppDir, "routes.go"),
	}
}

// apiIndexStatus keeps timing output explicit about which App target produced OpenAPI artifacts.
func apiIndexStatus(appTarget string, status string) string {
	appTarget = strings.TrimSpace(appTarget)
	if appTarget == "" {
		appTarget = project.DefaultAppTargetName
	}
	status = strings.TrimSpace(status)
	if status == "" {
		return "app target " + appTarget
	}
	return "app target " + appTarget + ", " + status
}

func existingRouteCompositionPath(target project.AppTarget, routeComposition string) (string, error) {
	if routeComposition == "" {
		return "", nil
	}
	if _, err := os.Stat(routeComposition); err != nil {
		return "", nil
	}
	return routeComposition, nil
}

func (r *APIIndexRunner) logManifestSummary(manifest webindex.Manifest, paths apiIndexPaths) {
	r.logger.Info().
		Str("app_target", paths.appTarget).
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
