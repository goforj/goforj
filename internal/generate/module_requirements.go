package generate

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/goforj/goforj/internal/coredeps"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"
)

// syncGeneratedModuleRequirements seeds validated imported modules before tidy can resolve an optional driver to its latest release.
func syncGeneratedModuleRequirements(projectDir string, selection GenerationSelection) (bool, error) {
	path := filepath.Join(projectDir, "go.mod")
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read generated module requirements: %w", err)
	}
	manifest, err := modfile.Parse(path, content, nil)
	if err != nil {
		return false, fmt.Errorf("parse generated module requirements: %w", err)
	}
	if manifest.Module == nil {
		return false, fmt.Errorf("parse generated module requirements: module directive is missing")
	}
	desired, err := generatedModuleRequirements(projectDir, selection)
	if err != nil {
		return false, err
	}
	required := make(map[string]string, len(manifest.Require))
	for _, requirement := range manifest.Require {
		required[requirement.Mod.Path] = requirement.Mod.Version
	}
	changed := false
	for _, module := range desired {
		current := required[module]
		version := coredeps.MustVersionFor(module)
		if current != "" && semver.Compare(current, version) >= 0 {
			continue
		}
		replaced := false
		for _, replacement := range manifest.Replace {
			// Missing requirements let tidy select the user's replacement using Go's own version and exclusion rules.
			if replacement.Old.Path == module && (current == "" || replacement.Old.Version == "" || replacement.Old.Version == current) {
				replaced = true
				break
			}
		}
		if replaced {
			continue
		}
		if err := manifest.AddRequire(module, version); err != nil {
			return false, fmt.Errorf("pin generated module %s: %w", module, err)
		}
		changed = true
	}
	if !changed {
		return false, nil
	}
	formatted, err := manifest.Format()
	if err != nil {
		return false, fmt.Errorf("format generated module requirements: %w", err)
	}
	if _, err := writeGeneratedSource(path, formatted); err != nil {
		return false, fmt.Errorf("write generated module requirements: %w", err)
	}
	return true, nil
}

// generatedModuleRequirements limits pins to catalog modules actually imported by the selected generated artifacts.
func generatedModuleRequirements(projectDir string, selection GenerationSelection) ([]string, error) {
	packages := []struct {
		selected bool
		name     string
	}{
		{selection.Feature, "features"},
		{selection.Search, "searches"},
		{selection.Storage, "storages"},
		{selection.Cache, "caches"},
		{selection.Mail, "mail"},
		{selection.Notification, "notifications"},
		{selection.Queue, "queues"},
		{selection.Events, "events"},
		{selection.Queue || selection.Events, "inspects"},
		{selection.Database, "database"},
	}
	catalog := coredeps.AllPinnedModules()
	modules := make(map[string]bool)
	for _, pkg := range packages {
		if !pkg.selected {
			continue
		}
		names := []string{"manager_gen.go", "accessors_gen.go"}
		switch pkg.name {
		case "database":
			names = []string{"connections_gen.go"}
		case "inspects":
			names = []string{"propagation_gen.go"}
		}
		for _, name := range names {
			path := filepath.Join(projectDir, "internal", pkg.name, name)
			content, err := os.ReadFile(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("read generated imports: %w", err)
			}
			source, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ImportsOnly)
			if err != nil {
				return nil, fmt.Errorf("parse generated imports: %w", err)
			}
			for _, spec := range source.Imports {
				importPath, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					return nil, fmt.Errorf("parse generated import path: %w", err)
				}
				module := ""
				for candidate := range catalog {
					if len(candidate) > len(module) && (importPath == candidate || strings.HasPrefix(importPath, candidate+"/")) {
						module = candidate
					}
				}
				if module != "" {
					modules[module] = true
				}
			}
		}
	}
	out := make([]string, 0, len(modules))
	for module := range modules {
		out = append(out, module)
	}
	sort.Strings(out)
	return out, nil
}
