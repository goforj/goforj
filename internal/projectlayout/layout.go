package projectlayout

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goforj/goforj/project"
)

// Discovery is an immutable inventory of Apps proven by conventional filesystem ownership markers.
type Discovery struct {
	namedApps []project.App
}

// NormalizeApp fills omitted paths from GoForj's conventional App layout while preserving explicit overrides.
func NormalizeApp(app project.App) project.App {
	if strings.TrimSpace(app.Name) == "" {
		return project.DefaultApp()
	}
	defaults := project.DefaultNamedApp(app.Name)
	if app.Entrypoint == "" {
		app.Entrypoint = defaults.Entrypoint
	}
	if app.AppDir == "" {
		app.AppDir = defaults.AppDir
	}
	if app.WireDir == "" {
		app.WireDir = defaults.WireDir
	}
	return app
}

// Entrypoint roots an App's main package so callers can target another project without changing process state.
func Entrypoint(root string, app project.App) string {
	return rootedPath(root, NormalizeApp(app).Entrypoint)
}

// CommandDir derives the command package from the canonical entrypoint instead of duplicating that relationship.
func CommandDir(root string, app project.App) string {
	return filepath.Dir(Entrypoint(root, app))
}

// AppDir roots App-owned composition while retaining any explicit layout override.
func AppDir(root string, app project.App) string {
	return rootedPath(root, NormalizeApp(app).AppDir)
}

// WireDir roots App-owned dependency injection while retaining any explicit layout override.
func WireDir(root string, app project.App) string {
	return rootedPath(root, NormalizeApp(app).WireDir)
}

// FrontendDir keeps editable frontend sources beside the command package that embeds their output.
func FrontendDir(root string, app project.App) string {
	return filepath.Join(CommandDir(root, app), "frontend")
}

// FrontendDistIndex centralizes the placeholder required for App binaries to compile before a frontend build exists.
func FrontendDistIndex(root string, app project.App) string {
	return filepath.Join(FrontendDir(root, app), "dist", "index.html")
}

// RuntimeExecutable preserves shell-executable spelling while allowing commands to target an explicit project root.
func RuntimeExecutable(root string, app project.App) string {
	app = NormalizeApp(app)
	return explicitRuntimePath(root, app.Name)
}

// RuntimeBinary provides the filesystem form used when inspecting or removing a built App artifact.
func RuntimeBinary(root string, app project.App) string {
	app = NormalizeApp(app)
	return rootedPath(root, filepath.Join("bin", app.Name))
}

// RuntimeReadyStamp keeps App restarts behind the publication marker written only after a successful build.
func RuntimeReadyStamp(root string, app project.App) string {
	app = NormalizeApp(app)
	return explicitRuntimePath(root, "."+app.Name+".ready")
}

// Discover reports filesystem failures while retaining Apps proven by readable markers, so compatibility callers can safely use the partial inventory.
func Discover(root string) (Discovery, error) {
	names, err := discoverConventionalAppNames(root)
	apps := make([]project.App, 0, len(names))
	for name := range names {
		if name == project.DefaultAppName || !project.IsSafeAppName(name) || project.IsReservedAppName(name) {
			continue
		}
		apps = append(apps, project.DefaultNamedApp(name))
	}
	sort.Slice(apps, func(i int, j int) bool {
		return apps[i].Name < apps[j].Name
	})
	return Discovery{namedApps: apps}, err
}

// NamedApps returns a copy so one caller cannot reorder the shared discovery result for another.
func (d Discovery) NamedApps() []project.App {
	return append([]project.App(nil), d.namedApps...)
}

// ConventionalApps keeps the default App first while making filesystem discovery deterministic.
func (d Discovery) ConventionalApps() []project.App {
	apps := []project.App{project.DefaultApp()}
	return append(apps, d.namedApps...)
}

// RuntimeApps includes configured and pending Apps before their files exist while preserving stable runtime indexes.
func (d Discovery) RuntimeApps(config *project.Config, pending ...project.App) []project.App {
	candidates := d.ConventionalApps()
	if config != nil {
		configuredNames := make([]string, 0, len(config.Apps))
		for name := range config.Apps {
			configuredNames = append(configuredNames, name)
		}
		sort.Strings(configuredNames)
		for _, name := range configuredNames {
			candidates = append(candidates, project.DefaultNamedApp(name))
		}
	}
	candidates = append(candidates, pending...)
	return orderedRuntimeApps(candidates)
}

// DiscoveredNamedApps preserves best-effort discovery for renderer compatibility; callers that can return errors should use Discover.
func DiscoveredNamedApps(root string) []project.App {
	return bestEffortDiscovery(root).NamedApps()
}

// ConventionalApps preserves best-effort discovery for rendering and legacy dev; callers that can return errors should use Discover.
func ConventionalApps(root string) []project.App {
	return bestEffortDiscovery(root).ConventionalApps()
}

// RuntimeApps preserves best-effort discovery for renderer compatibility while still including configured and pending Apps.
func RuntimeApps(root string, config *project.Config, pending ...project.App) []project.App {
	return bestEffortDiscovery(root).RuntimeApps(config, pending...)
}

// discoverConventionalAppNames treats established cmd and app ownership markers as filesystem source of truth.
func discoverConventionalAppNames(root string) (map[string]struct{}, error) {
	names := map[string]struct{}{}
	var discoveryErr error
	recordError := func(err error) {
		if discoveryErr == nil {
			discoveryErr = err
		}
	}

	commandRoot := rootedPath(root, "cmd")
	entries, err := os.ReadDir(commandRoot)
	if err != nil && !os.IsNotExist(err) {
		recordError(fmt.Errorf("discover Apps in %s: %w", commandRoot, err))
	}
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if name == project.DefaultAppName || !project.IsSafeAppName(name) {
				continue
			}
			marker := filepath.Join(commandRoot, name, "main.go")
			if _, err := os.Stat(marker); err == nil {
				names[name] = struct{}{}
			} else if !os.IsNotExist(err) {
				recordError(fmt.Errorf("inspect App marker %s: %w", marker, err))
			}
		}
	}

	appRoot := rootedPath(root, "app")
	entries, err = os.ReadDir(appRoot)
	if err != nil && !os.IsNotExist(err) {
		recordError(fmt.Errorf("discover Apps in %s: %w", appRoot, err))
	}
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if name == project.DefaultAppName || !project.IsSafeAppName(name) || project.IsReservedAppName(name) {
				continue
			}
			hasFiles, err := hasConventionalAppFiles(filepath.Join(appRoot, name))
			if err != nil {
				recordError(err)
			}
			if hasFiles {
				names[name] = struct{}{}
			}
		}
	}
	return names, discoveryErr
}

// hasConventionalAppFiles excludes arbitrary app subpackages unless they contain an App-owned composition marker.
func hasConventionalAppFiles(appDir string) (bool, error) {
	var markerErr error
	for _, path := range []string{
		filepath.Join(appDir, "wire"),
		filepath.Join(appDir, "commands.go"),
		filepath.Join(appDir, "root_cmd.go"),
		filepath.Join(appDir, "routes.go"),
		filepath.Join(appDir, "schedules.go"),
		filepath.Join(appDir, "lifecycle.go"),
	} {
		if _, err := os.Stat(path); err == nil {
			return true, markerErr
		} else if !os.IsNotExist(err) {
			if markerErr == nil {
				markerErr = fmt.Errorf("inspect App marker %s: %w", path, err)
			}
		}
	}
	return false, markerErr
}

// bestEffortDiscovery keeps renderer-facing compatibility APIs stable until their call chains can return discovery failures.
func bestEffortDiscovery(root string) Discovery {
	discovery, _ := Discover(root)
	return discovery
}

// orderedRuntimeApps keeps the default App at index zero and lets later sources provide more specific paths.
func orderedRuntimeApps(candidates []project.App) []project.App {
	byName := make(map[string]project.App, len(candidates))
	for _, candidate := range candidates {
		candidate = NormalizeApp(candidate)
		if candidate.Name == "" || !project.IsSafeAppName(candidate.Name) || project.IsReservedAppName(candidate.Name) {
			continue
		}
		byName[candidate.Name] = candidate
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		if name != project.DefaultAppName {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	apps := make([]project.App, 0, len(byName))
	if defaultApp, exists := byName[project.DefaultAppName]; exists {
		apps = append(apps, defaultApp)
	}
	for _, name := range names {
		apps = append(apps, byName[name])
	}
	return apps
}

// rootedPath resolves project-relative conventions without rewriting an explicitly absolute App path.
func rootedPath(root string, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if root == "" || root == "." {
		return path
	}
	return filepath.Join(root, path)
}

// explicitRuntimePath preserves the executable-friendly ./ prefix used by existing generated commands.
func explicitRuntimePath(root string, name string) string {
	if root == "" || root == "." {
		return "./" + filepath.ToSlash(filepath.Join("bin", name))
	}
	return filepath.ToSlash(filepath.Join(root, "bin", name))
}
