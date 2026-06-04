package build

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/goforj/goforj/project"
)

// activeAppTarget resolves the app target selected by environment or convention.
func activeAppTarget() project.AppTarget {
	targetName := requestedAppTargetName()
	if project.IsSafeAppTargetName(targetName) {
		return conventionalAppTarget(targetName)
	}
	return conventionalAppTarget(project.DefaultAppTargetName)
}

// requestedAppTargetName returns an explicit target selected by the command environment.
func requestedAppTargetName() string {
	if target := strings.TrimSpace(os.Getenv("FORJ_APP_TARGET")); target != "" {
		return target
	}
	if target := strings.TrimSpace(os.Getenv("APP_TARGET")); target != "" {
		return target
	}
	return ""
}

// conventionalAppTarget returns the standard layout paths for an app target.
func conventionalAppTarget(name string) project.AppTarget {
	return project.DefaultNamedAppTarget(name)
}

// targetPackageFromEntrypoint converts cmd/<target>/main.go into a go command package path.
func targetPackageFromEntrypoint(entrypoint string) string {
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
