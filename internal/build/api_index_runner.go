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
	appName          string
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
	target := activeApp()
	paths := defaultAPIIndexPaths(target)
	routeComposition, err := existingRouteCompositionPath(target, paths.routeComposition)
	if err != nil {
		return "", err
	}
	paths.routeComposition = routeComposition
	if paths.routeComposition == "" {
		return apiIndexStatus(paths.appName, "no route composition"), nil
	}
	paths.root = "."
	return r.runDefaultWithStatus(paths)
}

func (r *APIIndexRunner) runDefaultWithStatus(paths apiIndexPaths) (string, error) {
	paths, err := resolveAPIIndexPaths(paths.root, paths.out, paths.diagnostics, paths.openAPI, paths.routeComposition, paths.appName)
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
		return apiIndexStatus(paths.appName, noChangesStatus), nil
	}
	return apiIndexStatus(paths.appName, ""), nil
}

func resolveAPIIndexPaths(root string, out string, diagnostics string, openAPI string, routeComposition string, appName string) (apiIndexPaths, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return apiIndexPaths{}, err
	}
	if appName == "" {
		appName = project.DefaultAppName
	}
	return apiIndexPaths{
		root:             absRoot,
		appName:          appName,
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
		SkipDir: func(_ string, name string) bool {
			return shouldSkipAPIIndexSourceDir(name)
		},
	}
	return webindex.Run(context.Background(), options)
}

func shouldSkipAPIIndexSourceDir(name string) bool {
	switch name {
	case "_data", "bin":
		return true
	default:
		return false
	}
}

func defaultAPIIndexPaths(target project.App) apiIndexPaths {
	if target.Name == "" || target.Name == project.DefaultAppName {
		return apiIndexPaths{
			appName:          project.DefaultAppName,
			out:              "build/api_index.json",
			diagnostics:      "build/api_index.diagnostics.json",
			openAPI:          "build/openapi.json",
			routeComposition: filepath.Join("app", "routes.go"),
		}
	}
	buildDir := filepath.Join("build", target.Name)
	return apiIndexPaths{
		appName:          target.Name,
		out:              filepath.Join(buildDir, "api_index.json"),
		diagnostics:      filepath.Join(buildDir, "api_index.diagnostics.json"),
		openAPI:          filepath.Join(buildDir, "openapi.json"),
		routeComposition: filepath.Join(target.AppDir, "routes.go"),
	}
}

// apiIndexStatus keeps timing output explicit about which App produced OpenAPI artifacts.
func apiIndexStatus(appName string, status string) string {
	appName = strings.TrimSpace(appName)
	if appName == "" {
		appName = project.DefaultAppName
	}
	status = strings.TrimSpace(status)
	if status == "" {
		return "app " + appName
	}
	return "app " + appName + ", " + status
}

func existingRouteCompositionPath(target project.App, routeComposition string) (string, error) {
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
		Str("app", paths.appName).
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
