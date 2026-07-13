package build

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/goforj/goforj/project"
)

// ActiveApp resolves the App selected by environment or convention when an operation starts.
func ActiveApp() project.App {
	appName := requestedAppName()
	if project.IsSafeAppName(appName) {
		return conventionalApp(appName)
	}
	return conventionalApp(project.DefaultAppName)
}

// requestedAppName returns an explicit app selected by the command environment.
func requestedAppName() string {
	if app := strings.TrimSpace(os.Getenv("FORJ_APP")); app != "" {
		return app
	}
	return ""
}

// conventionalApp returns the standard layout paths for an app.
func conventionalApp(name string) project.App {
	return project.DefaultNamedApp(name)
}

// appPackageFromEntrypoint converts cmd/<app>/main.go into a go command package path.
func appPackageFromEntrypoint(entrypoint string) string {
	entrypoint = filepath.ToSlash(filepath.Clean(strings.TrimSpace(entrypoint)))
	if entrypoint == "." || entrypoint == "" {
		return "."
	}
	dir := filepath.ToSlash(filepath.Dir(entrypoint))
	if dir == "." || dir == "" {
		return "."
	}
	return "./" + strings.TrimPrefix(dir, "./")
}
