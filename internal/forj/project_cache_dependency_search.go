package forj

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/goforj/str/v2"

	"github.com/goforj/goforj/internal/projectlayout"
	"github.com/goforj/goforj/project"
)

// cacheOwnerExcludedPath describes whether an App-owned directory excludes only its package or its complete subtree.
type cacheOwnerExcludedPath struct {
	path        string
	descendants bool
}

// cacheOwnerSourceInspection retains the local imports discovered while checking one owner source file.
type cacheOwnerSourceInspection struct {
	dependsOnCache    bool
	importDirectories []string
}

// cacheOwnerDependencySearch follows only project packages reachable from one App ownership root.
type cacheOwnerDependencySearch struct {
	workspace       projectRenderWorkspace
	modulePath      string
	excludedPaths   []cacheOwnerExcludedPath
	frameworkPaths  map[string]bool
	packageStates   map[string]bool
	pendingPackages []string
}

// cacheOwnerDependencySearch builds a project-local import search without invoking the Go toolchain.
func (p *ProjectRenderer) cacheOwnerDependencySearch(excludedApps []project.App, frameworkPaths map[string]bool) *cacheOwnerDependencySearch {
	return &cacheOwnerDependencySearch{
		workspace:      p.workspace,
		modulePath:     str.Of(p.config.GoModuleName).Trim().TrimSuffix("/").String(),
		excludedPaths:  cacheOwnerExcludedPaths(excludedApps),
		frameworkPaths: frameworkPaths,
		packageStates:  map[string]bool{},
	}
}

// cacheOwnerExcludedPaths keeps sibling Apps outside a named App search without hiding named Apps beneath the default app directory.
func cacheOwnerExcludedPaths(apps []project.App) []cacheOwnerExcludedPath {
	paths := make([]cacheOwnerExcludedPath, 0, len(apps)*3)
	for _, app := range apps {
		app = projectlayout.NormalizeApp(app)
		paths = append(paths,
			cacheOwnerExcludedPath{path: filepath.Clean(projectlayout.CommandDir(".", app)), descendants: true},
			cacheOwnerExcludedPath{path: filepath.Clean(projectlayout.WireDir(".", app)), descendants: true},
		)
		paths = append(paths, cacheOwnerExcludedPath{
			path:        filepath.Clean(projectlayout.AppDir(".", app)),
			descendants: app.Name != project.DefaultAppName,
		})
	}
	return paths
}

// find walks one ownership root and the project-local packages imported by its owner source.
func (s *cacheOwnerDependencySearch) find(root string) (string, error) {
	dependency, err := s.scanTree(root)
	if err != nil || dependency != "" {
		return dependency, err
	}
	for len(s.pendingPackages) > 0 {
		logicalDirectory := s.pendingPackages[0]
		s.pendingPackages = s.pendingPackages[1:]
		if visited, tracked := s.packageStates[logicalDirectory]; tracked && visited {
			continue
		}
		s.packageStates[logicalDirectory] = true
		dependency, err = s.scanPackage(logicalDirectory)
		if err != nil || dependency != "" {
			return dependency, err
		}
	}
	return "", nil
}

// scanTree checks existing owner source recursively while seeding its project-local import graph.
func (s *cacheOwnerDependencySearch) scanTree(root string) (string, error) {
	physicalRoot := s.workspace.path(root)
	if _, err := os.Stat(physicalRoot); os.IsNotExist(err) {
		return "", nil
	} else if err != nil {
		return "", s.workspace.logicalError(err)
	}
	dependency := ""
	err := filepath.WalkDir(physicalRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		logical := filepath.Clean(s.workspace.logicalLabel(path))
		if entry.IsDir() {
			if logical != "." && cacheTransitionSkippedDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			if s.directoryExcluded(logical) {
				return filepath.SkipDir
			}
			s.packageStates[logical] = true
			return nil
		}
		if filepath.Ext(entry.Name()) != ".go" || s.ownerSourceExcluded(logical) {
			return nil
		}
		inspection, err := s.inspectSource(logical, path)
		if err != nil {
			return err
		}
		if inspection.dependsOnCache {
			dependency = logical
			return fs.SkipAll
		}
		s.enqueuePackages(inspection.importDirectories)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("inspect Cache owner source: %w", s.workspace.logicalError(err))
	}
	return dependency, nil
}

// scanPackage checks one reachable Go package without widening traversal into unrelated subpackages.
func (s *cacheOwnerDependencySearch) scanPackage(logicalDirectory string) (string, error) {
	entries, err := os.ReadDir(s.workspace.path(logicalDirectory))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect Cache owner package %s: %w", logicalDirectory, s.workspace.logicalError(err))
	}
	for _, entry := range entries {
		logical := filepath.Join(logicalDirectory, entry.Name())
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || s.frameworkSourceExcluded(logical) {
			continue
		}
		inspection, err := s.inspectSource(logical, s.workspace.path(logical))
		if err != nil {
			return "", fmt.Errorf("inspect Cache owner source: %w", s.workspace.logicalError(err))
		}
		if inspection.dependsOnCache {
			return filepath.Clean(logical), nil
		}
		s.enqueuePackages(inspection.importDirectories)
	}
	return "", nil
}

// inspectSource parses one owner file once so dependency and traversal decisions share the same syntax tree.
func (s *cacheOwnerDependencySearch) inspectSource(logical string, physical string) (cacheOwnerSourceInspection, error) {
	source, err := os.ReadFile(physical)
	if err != nil {
		return cacheOwnerSourceInspection{}, err
	}
	if cacheTransitionGeneratedGoSource(source) {
		return cacheOwnerSourceInspection{}, nil
	}
	file, err := parser.ParseFile(token.NewFileSet(), logical, source, 0)
	if err != nil {
		return cacheOwnerSourceInspection{}, fmt.Errorf("parse Cache owner source %s: %w", logical, err)
	}
	inspection := cacheOwnerSourceInspection{dependsOnCache: cacheGoFileDependsOnGeneratedAPI(logical, file)}
	for _, imported := range file.Imports {
		importPath, ok := cacheImportPath(imported.Path.Value)
		if !ok {
			continue
		}
		if directory, ok := s.localImportDirectory(importPath); ok {
			inspection.importDirectories = append(inspection.importDirectories, directory)
		}
	}
	return inspection, nil
}

// cacheImportPath decodes both quoted and raw Go import literals after syntax parsing has validated the file.
func cacheImportPath(literal string) (string, bool) {
	importPath, err := strconv.Unquote(literal)
	return importPath, err == nil
}

// localImportDirectory maps imports from the configured module back to safe project-relative directories.
func (s *cacheOwnerDependencySearch) localImportDirectory(importPath string) (string, bool) {
	if s.modulePath == "" {
		return "", false
	}
	if importPath == s.modulePath {
		return ".", true
	}
	prefix := s.modulePath + "/"
	if !strings.HasPrefix(importPath, prefix) {
		return "", false
	}
	directory := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(importPath, prefix)))
	if directory == "." || filepath.IsAbs(directory) || directory == ".." || strings.HasPrefix(directory, ".."+string(filepath.Separator)) {
		return "", false
	}
	return directory, true
}

// enqueuePackages adds newly reached packages once while retaining deterministic traversal order.
func (s *cacheOwnerDependencySearch) enqueuePackages(directories []string) {
	for _, directory := range directories {
		directory = filepath.Clean(directory)
		if _, tracked := s.packageStates[directory]; tracked {
			continue
		}
		s.packageStates[directory] = false
		s.pendingPackages = append(s.pendingPackages, directory)
	}
	sort.Strings(s.pendingPackages)
}

// directoryExcluded identifies recursive App exclusions that can prune a filesystem walk safely.
func (s *cacheOwnerDependencySearch) directoryExcluded(path string) bool {
	for _, excluded := range s.excludedPaths {
		if excluded.descendants && cacheTransitionPathContains(excluded.path, path) {
			return true
		}
	}
	return false
}

// ownerSourceExcluded ignores prospective framework output and source reached only because App ownership directories overlap.
func (s *cacheOwnerDependencySearch) ownerSourceExcluded(path string) bool {
	path = filepath.Clean(path)
	if s.frameworkSourceExcluded(path) {
		return true
	}
	for _, excluded := range s.excludedPaths {
		if !excluded.descendants && filepath.Dir(path) == excluded.path {
			return true
		}
	}
	return false
}

// frameworkSourceExcluded ignores source the prospective renderer replaces before compilation.
func (s *cacheOwnerDependencySearch) frameworkSourceExcluded(path string) bool {
	return s.frameworkPaths[filepath.Clean(path)]
}

// cacheTransitionPathContains reports whether candidate is root or a descendant without string-prefix path ambiguity.
func cacheTransitionPathContains(root string, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
