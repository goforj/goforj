package build

import (
	"fmt"
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

// resolveDefaultAppPackage prefers the selected App and treats only absent package directories as a reason to fall back.
func resolveDefaultAppPackage(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	selected := appPackageFromEntrypoint(ActiveApp().Entrypoint)
	candidates := make([]string, 0, 2)
	if selected != "." {
		candidates = append(candidates, selected)
	}
	if selected != "./cmd/app" {
		candidates = append(candidates, "./cmd/app")
	}
	for _, candidate := range candidates {
		path := filepath.Join(root, strings.TrimPrefix(candidate, "./"))
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", fmt.Errorf("inspect App package %s: %w", path, err)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("inspect App package %s: expected a directory", path)
		}
		return candidate, nil
	}
	return ".", nil
}
