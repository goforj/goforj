package forj

import (
	"os"
	"slices"
	"strings"

	"github.com/goforj/goforj/internal/envfile"
	"github.com/goforj/goforj/internal/projectlayout"
	"github.com/goforj/goforj/internal/resourceenv"
	"github.com/goforj/goforj/project"
)

// cacheOwnerEnvironmentDependency finds owner Cache assignments that generated cleanup cannot safely claim.
func (p *ProjectRenderer) cacheOwnerEnvironmentDependency(components project.Components, disabledApps []project.App, ignoredApps []project.App) (string, string, error) {
	ignored := cacheTransitionAppNames(ignoredApps)
	runtimeApps := projectlayout.RuntimeApps(p.workspace.discoveryRoot(), p.config)
	paths, err := p.workspace.cacheOwnerEnvironmentPaths()
	if err != nil {
		return "", "", err
	}
	for _, path := range paths {
		source, err := p.workspace.readFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", "", err
		}
		generatedDefaults := path == ".env" || path == ".env.host" || path == ".env.example"
		for _, app := range disabledApps {
			if key := appCacheOwnerEnvironmentKey(source, app.Name, generatedDefaults); key != "" {
				return path, key, nil
			}
		}
		if components.Cache {
			continue
		}
		cleaned := source
		if generatedDefaults {
			cleaned, _ = resourceenv.RemoveGeneratedAssignments(source, project.Components{}, runtimeApps)
		}
		for _, line := range strings.Split(string(cleaned), "\n") {
			key, ok := envfile.ScanKey(line)
			if ok && !cacheIgnoredAppEnvironmentAssignment(key, ignored) && cacheOwnerEnvironmentAssignment(key) {
				return path, key, nil
			}
		}
	}
	return "", "", nil
}

// cacheOwnerEnvironmentPaths preserves conventional preflight precedence while adding root-only owner overlays.
// A single directory read keeps dependency trees and temporary render workspaces outside owner discovery.
func (w projectRenderWorkspace) cacheOwnerEnvironmentPaths() ([]string, error) {
	entries, err := w.readDir()
	if err != nil {
		return nil, err
	}
	discovered := make(map[string]bool, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if name != ".env" && !strings.HasPrefix(name, ".env.") {
			continue
		}
		info, err := w.stat(name)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		discovered[name] = true
	}

	preferred := []string{".env", ".env.host", ".env.example", ".env.local", ".env.staging", ".env.production", ".env.testing"}
	paths := make([]string, 0, len(discovered))
	for _, path := range preferred {
		if discovered[path] {
			paths = append(paths, path)
			delete(discovered, path)
		}
	}
	custom := make([]string, 0, len(discovered))
	for path := range discovered {
		custom = append(custom, path)
	}
	slices.Sort(custom)
	return append(paths, custom...), nil
}

// cacheIgnoredAppEnvironmentAssignment excludes resource overlays removed together with an explicitly deleted App.
func cacheIgnoredAppEnvironmentAssignment(key string, ignoredApps map[string]bool) bool {
	definitions := project.ResourceCatalog()
	for appName := range ignoredApps {
		prefix := project.AppEnvironmentPrefix(appName)
		if prefix == "" {
			continue
		}
		for _, definition := range definitions {
			if strings.HasPrefix(key, prefix+"_"+definition.EnvironmentPrefix+"_") {
				return true
			}
		}
	}
	return false
}

// cacheOwnerEnvironmentAssignment recognizes root and environment-only App Cache topology without matching unrelated cache-control settings.
func cacheOwnerEnvironmentAssignment(key string) bool {
	if strings.HasPrefix(key, "CACHE_") {
		return true
	}
	marker := strings.Index(key, "_CACHE_")
	if marker <= 0 {
		return false
	}
	suffix := key[marker+len("_CACHE_"):]
	return suffix == "DRIVER" || suffix == "SUPPORTED_DRIVERS" || suffix == "ADDR" ||
		strings.HasSuffix(suffix, "_DRIVER") || strings.HasSuffix(suffix, "_ADDR")
}

// appCacheOwnerEnvironmentKey distinguishes generated App defaults from owner-authored Cache configuration.
func appCacheOwnerEnvironmentKey(source []byte, appName string, generatedDefaults bool) string {
	prefix := project.AppEnvironmentPrefix(appName)
	if prefix == "" {
		return ""
	}
	inGeneratedSection := false
	for _, line := range strings.Split(string(source), "\n") {
		if isEnvSectionHeader(line, appName) {
			inGeneratedSection = true
			continue
		}
		if inGeneratedSection && strings.TrimSpace(line) == "" {
			inGeneratedSection = false
		}
		key, ok := envfile.ScanKey(line)
		if !ok || !strings.HasPrefix(key, prefix+"_CACHE_") {
			continue
		}
		if generatedDefaults && key == prefix+"_CACHE_DRIVER" && inGeneratedSection {
			continue
		}
		return key
	}
	return ""
}

// removeGeneratedAppCacheDriverDefault removes one stale App Cache default only when its generated section proves framework ownership.
func (w projectRenderWorkspace) removeGeneratedAppCacheDriverDefault(path string, appName string) (bool, error) {
	if err := w.rejectEnvironmentSpecialFile(path); err != nil {
		return false, err
	}
	source, err := w.readFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	updated, changed := removeGeneratedAppCacheDriverDefault(source, appName)
	if !changed {
		return false, nil
	}
	return true, w.writeFile(path, updated, 0o644)
}

// removeGeneratedAppCacheDriverDefault preserves loose owner assignments while pruning the exact default inside a generated App section.
func removeGeneratedAppCacheDriverDefault(source []byte, appName string) ([]byte, bool) {
	prefix := project.AppEnvironmentPrefix(appName)
	if prefix == "" {
		return source, false
	}
	want := prefix + "_CACHE_DRIVER"
	lines := strings.Split(string(source), "\n")
	filtered := make([]string, 0, len(lines))
	changed := false
	for index := 0; index < len(lines); {
		if !isEnvSectionHeader(lines[index], appName) {
			filtered = append(filtered, lines[index])
			index++
			continue
		}
		end := index + 1
		for end < len(lines) && strings.TrimSpace(lines[end]) != "" {
			end++
		}
		section := make([]string, 0, end-index-1)
		sectionChanged := false
		for _, line := range lines[index+1 : end] {
			key, ok := envfile.ScanKey(line)
			if ok && key == want {
				sectionChanged = true
				continue
			}
			section = append(section, line)
		}
		if !sectionChanged {
			filtered = append(filtered, lines[index:end]...)
		} else {
			changed = true
			if len(section) > 0 {
				filtered = append(filtered, lines[index])
				filtered = append(filtered, section...)
			}
		}
		index = end
	}
	if !changed {
		return source, false
	}
	return []byte(strings.Join(filtered, "\n")), true
}

// removeDisabledAppCacheDriverDefaults reconciles only additional apps whose prospective contract omits Cache.
func removeDisabledAppCacheDriverDefaults(source []byte, config *project.Config, apps []project.App) ([]byte, bool) {
	updated := source
	changed := false
	for _, app := range apps {
		if appRenderComponents(config, app).Cache {
			continue
		}
		var appChanged bool
		updated, appChanged = removeGeneratedAppCacheDriverDefault(updated, app.Name)
		changed = changed || appChanged
	}
	return updated, changed
}
