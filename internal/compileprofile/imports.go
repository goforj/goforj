package compileprofile

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// goListPackage retains only the dependency fields needed to construct import chains.
type goListPackage struct {
	ImportPath string
	Imports    []string
}

// importLoadResult keeps graph nodes and analysis roots together for breadth-first traversal.
type importLoadResult struct {
	packages map[string]goListPackage
	roots    []string
}

// AnnotateImportChains adds the shortest project-rooted import chain available for each compiled package.
func (r *Report) AnnotateImportChains(root string) {
	loaded, err := loadImportPackages(root, defaultAnalyzePatterns(root))
	if err != nil {
		return
	}
	for i := range r.entries {
		r.entries[i].importChain = importChainToTarget(loaded, r.entries[i].packageName)
	}
}

// loadImportPackages asks Go for the dependency graph and resolves each project pattern to traversal roots.
func loadImportPackages(root string, patterns []string) (importLoadResult, error) {
	args := append([]string{"list", "-deps", "-json"}, patterns...)
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOCACHE=/tmp/gocache", "GOMODCACHE=/tmp/gomodcache")

	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return importLoadResult{}, fmt.Errorf("go list failed: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return importLoadResult{}, err
	}

	dec := json.NewDecoder(strings.NewReader(string(out)))
	result := importLoadResult{
		packages: map[string]goListPackage{},
		roots:    make([]string, 0, len(patterns)),
	}
	for {
		var pkg goListPackage
		if err := dec.Decode(&pkg); err != nil {
			if err == io.EOF {
				break
			}
			return importLoadResult{}, err
		}
		if pkg.ImportPath == "" {
			continue
		}
		result.packages[pkg.ImportPath] = pkg
	}
	for _, pattern := range patterns {
		rootPackages, err := loadRootPackages(root, pattern)
		if err != nil {
			return importLoadResult{}, err
		}
		result.roots = append(result.roots, rootPackages...)
	}
	return result, nil
}

// loadRootPackages expands one analysis pattern because dependency output does not identify the requested graph roots.
func loadRootPackages(root string, pattern string) ([]string, error) {
	cmd := exec.Command("go", "list", pattern)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOCACHE=/tmp/gocache", "GOMODCACHE=/tmp/gomodcache")
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("go list failed: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, err
	}
	lines := strings.Fields(string(out))
	return lines, nil
}

// importChainToTarget uses breadth-first traversal so the report explains the shortest discovered reason a package is compiled.
func importChainToTarget(loaded importLoadResult, target string) []string {
	if len(loaded.roots) == 0 {
		return nil
	}
	if _, ok := loaded.packages[target]; !ok {
		return nil
	}

	queue := append([]string(nil), loaded.roots...)
	parents := map[string]string{}
	seen := map[string]struct{}{}
	for _, root := range loaded.roots {
		seen[root] = struct{}{}
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == target {
			break
		}
		pkg, ok := loaded.packages[current]
		if !ok {
			continue
		}
		for _, next := range pkg.Imports {
			if _, ok := loaded.packages[next]; !ok {
				continue
			}
			if _, exists := seen[next]; exists {
				continue
			}
			seen[next] = struct{}{}
			parents[next] = current
			queue = append(queue, next)
		}
	}

	if _, ok := seen[target]; !ok {
		return nil
	}
	var chain []string
	current := target
	chain = append(chain, current)
	for {
		parent, ok := parents[current]
		if !ok {
			break
		}
		chain = append(chain, parent)
		current = parent
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

// defaultAnalyzePatterns limits graph roots to conventional App code so framework tooling does not dominate explanations.
func defaultAnalyzePatterns(root string) []string {
	var resolved []string
	if dirExists(filepath.Join(root, "internal")) {
		resolved = append(resolved, "./internal/...")
	}
	if dirExists(filepath.Join(root, "app")) {
		resolved = append(resolved, "./app/...")
	}
	if dirExists(filepath.Join(root, "wire")) {
		resolved = append(resolved, "./wire")
	}
	if len(resolved) == 0 {
		return []string{"."}
	}
	return resolved
}

// dirExists treats inaccessible paths as absent because analysis patterns are optional discovery hints.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
