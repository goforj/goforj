package projectlayout

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goforj/goforj/project"
)

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

// DiscoveredNamedApps requires conventional ownership markers so arbitrary subpackages never become runnable Apps.
func DiscoveredNamedApps(root string) []project.App {
	names := discoverConventionalAppNames(root)
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
	return apps
}

// ConventionalApps keeps the default App first while making filesystem discovery deterministic for rendering and legacy dev.
func ConventionalApps(root string) []project.App {
	apps := []project.App{project.DefaultApp()}
	return append(apps, DiscoveredNamedApps(root)...)
}

// RuntimeApps includes configured and pending Apps before their files exist while preserving stable runtime indexes.
func RuntimeApps(root string, config *project.Config, pending ...project.App) []project.App {
	candidates := ConventionalApps(root)
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

// discoverConventionalAppNames treats established cmd and app ownership markers as filesystem source of truth.
func discoverConventionalAppNames(root string) map[string]struct{} {
	names := map[string]struct{}{}
	commandRoot := rootedPath(root, "cmd")
	if entries, err := os.ReadDir(commandRoot); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if name == project.DefaultAppName || !project.IsSafeAppName(name) {
				continue
			}
			if _, err := os.Stat(filepath.Join(commandRoot, name, "main.go")); err == nil {
				names[name] = struct{}{}
			}
		}
	}

	appRoot := rootedPath(root, "app")
	if entries, err := os.ReadDir(appRoot); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if name == project.DefaultAppName || !project.IsSafeAppName(name) || project.IsReservedAppName(name) {
				continue
			}
			if hasConventionalAppFiles(filepath.Join(appRoot, name)) {
				names[name] = struct{}{}
			}
		}
	}
	return names
}

// hasConventionalAppFiles excludes arbitrary app subpackages unless they contain an App-owned composition marker.
func hasConventionalAppFiles(appDir string) bool {
	for _, path := range []string{
		filepath.Join(appDir, "wire"),
		filepath.Join(appDir, "commands.go"),
		filepath.Join(appDir, "root_cmd.go"),
		filepath.Join(appDir, "routes.go"),
		filepath.Join(appDir, "schedules.go"),
		filepath.Join(appDir, "lifecycle.go"),
	} {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
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
