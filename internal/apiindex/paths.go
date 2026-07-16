package apiindex

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goforj/goforj/project"
)

// paths binds source discovery and all artifacts to one active app.
type paths struct {
	root             string
	appName          string
	out              string
	diagnostics      string
	openAPI          string
	routeComposition string
}

// resolvePaths anchors source discovery while preserving caller-selected artifact paths.
func resolvePaths(input paths) (paths, error) {
	absRoot, err := filepath.Abs(input.root)
	if err != nil {
		return paths{}, err
	}
	input.root = absRoot
	if input.appName == "" {
		input.appName = project.DefaultAppName
	}
	return input, nil
}

// resolveProjectRoot rejects missing or non-directory roots before source absence can be mistaken for an intentional skip.
func resolveProjectRoot(root string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve API index project root %q: %w", root, err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return "", fmt.Errorf("inspect API index project root %q: %w", root, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("API index project root %q is not a directory", root)
	}
	return absRoot, nil
}

// defaultPaths keeps each app's contract separate while retaining legacy paths for the default app.
func defaultPaths(target project.App) paths {
	if target.Name == "" || target.Name == project.DefaultAppName {
		return paths{
			appName:          project.DefaultAppName,
			out:              "build/api_index.json",
			diagnostics:      "build/api_index.diagnostics.json",
			openAPI:          "build/openapi.json",
			routeComposition: filepath.Join("app", "routes.go"),
		}
	}
	buildDir := filepath.Join("build", target.Name)
	return paths{
		appName:          target.Name,
		out:              filepath.Join(buildDir, "api_index.json"),
		diagnostics:      filepath.Join(buildDir, "api_index.diagnostics.json"),
		openAPI:          filepath.Join(buildDir, "openapi.json"),
		routeComposition: filepath.Join(target.AppDir, "routes.go"),
	}
}

// rootDefaultPaths anchors convention-selected artifacts before any read, staging, or publication work begins.
func rootDefaultPaths(root string, defaults paths) paths {
	rootPath := func(path string) string {
		if path != "" && !filepath.IsAbs(path) {
			return filepath.Join(root, path)
		}
		return path
	}
	defaults.out = rootPath(defaults.out)
	defaults.diagnostics = rootPath(defaults.diagnostics)
	defaults.openAPI = rootPath(defaults.openAPI)
	defaults.routeComposition = rootPath(defaults.routeComposition)
	return defaults
}

// participation records whether modern config can make an intentional indexing decision.
type participation struct {
	known  bool
	webAPI bool
}

// resolveParticipation uses modern app component config when it can distinguish an intentional CLI-only app from an incomplete WebAPI app.
func resolveParticipation(root string, target project.App) (participation, error) {
	config, err := project.LoadProjectConfigAt(root)
	if err != nil {
		if os.IsNotExist(err) {
			return participation{}, nil
		}
		return participation{}, fmt.Errorf("load project config for API index: %w", err)
	}

	if target.Name != "" && target.Name != project.DefaultAppName {
		appConfig, ok := config.Apps[target.Name]
		if !ok {
			return participation{}, nil
		}
		components := project.NormalizeAppComponents(config.Render.Components, appConfig.Components)
		return participation{known: true, webAPI: components.WebAPI}, nil
	}

	if config.Render.Components == (project.Components{}) {
		return participation{}, nil
	}
	components := project.NormalizeAppComponents(config.Render.Components, config.Render.Components)
	return participation{known: true, webAPI: components.WebAPI}, nil
}

// existingRouteCompositionPath returns an existing composition file while preserving unexpected filesystem failures for actionable build errors.
func existingRouteCompositionPath(target project.App, routeComposition string) (string, error) {
	if routeComposition == "" {
		return "", nil
	}
	info, err := os.Stat(routeComposition)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("stat route composition for app %q at %q: %w", target.Name, routeComposition, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("route composition for app %q at %q must be a file", target.Name, routeComposition)
	}
	return routeComposition, nil
}
