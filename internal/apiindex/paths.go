package apiindex

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goforj/goforj/project"
)

// apiIndexCacheFilename stays private because the cache is an implementation detail rather than a generated contract artifact.
const apiIndexCacheFilename = ".api_index.cache"

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

// apiIndexCachePath keeps reusable analysis state outside the project so cache writes cannot invalidate Go builds.
func apiIndexCachePath(input paths) string {
	return apiIndexCachePathWithDirectories(input, os.UserCacheDir, os.TempDir)
}

// apiIndexCachePathWithDirectories selects the user cache when available and a process-independent temporary root otherwise.
func apiIndexCachePathWithDirectories(input paths, userCacheDirectory func() (string, error), temporaryDirectory func() string) string {
	cacheRoot, err := userCacheDirectory()
	if err != nil || strings.TrimSpace(cacheRoot) == "" {
		cacheRoot = temporaryDirectory()
	}
	return apiIndexCachePathUnderRoot(input, filepath.Clean(cacheRoot))
}

// apiIndexCachePathUnderRoot isolates projects and apps without exposing workspace paths in the cache hierarchy.
func apiIndexCachePathUnderRoot(input paths, cacheRoot string) string {
	identity := canonicalAPIIndexCacheRoot(input.root) + "\x00" + normalizedAPIIndexCacheAppName(input.appName)
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
	return filepath.Join(cacheRoot, "goforj", "api-index", key, apiIndexCacheFilename)
}

// canonicalAPIIndexCacheRoot normalizes equivalent absolute, relative, and symlinked project roots to one cache identity.
func canonicalAPIIndexCacheRoot(root string) string {
	absolute, err := filepath.Abs(root)
	if err == nil {
		root = absolute
	}
	root = filepath.Clean(root)
	resolved, err := filepath.EvalSymlinks(root)
	if err == nil {
		root = resolved
	}
	return filepath.Clean(root)
}

// normalizedAPIIndexCacheAppName maps the implicit app to its conventional identity before hashing.
func normalizedAPIIndexCacheAppName(appName string) string {
	appName = strings.TrimSpace(appName)
	if appName == "" {
		return project.DefaultAppName
	}
	return appName
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
