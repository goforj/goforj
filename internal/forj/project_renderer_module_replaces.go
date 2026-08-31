package forj

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/goforj/goforj/internal/commanddiag"
)

// moduleReplacesStateFile records only entries owned by the renderer so later runs cannot remove user-managed replacements.
const moduleReplacesStateFile = ".goforj.module_replaces.json"

// applyModuleReplaces reconciles renderer-owned replacements after project configuration has been loaded.
func (p *ProjectRenderer) applyModuleReplaces() error {
	current := normalizedModuleReplaces(p.config.Render.ModuleReplaces)
	previous, err := p.workspace.loadManagedModuleReplaces()
	if err != nil {
		return err
	}

	for _, module := range previous {
		if _, ok := current[module]; ok {
			continue
		}
		if err := p.workspace.runGoModEdit("-dropreplace", module); err != nil {
			return err
		}
	}

	modules := sortedModuleKeys(current)
	for _, module := range modules {
		if err := p.workspace.runGoModEdit("-replace", fmt.Sprintf("%s=%s", module, current[module])); err != nil {
			return err
		}
	}

	if err := p.workspace.saveManagedModuleReplaces(modules); err != nil {
		return err
	}

	p.lines = append(
		p.lines,
		renderCountsLine(
			"go mod replace",
			len(modules),
			len(previous)-countManagedStillPresent(previous, current),
			"modules",
		),
	)
	return nil
}

// normalizedModuleReplaces excludes incomplete entries before they reach go mod edit or renderer-owned state.
func normalizedModuleReplaces(values map[string]string) map[string]string {
	out := make(map[string]string)
	for module, target := range values {
		module = strings.TrimSpace(module)
		target = strings.TrimSpace(target)
		if module == "" || target == "" {
			continue
		}
		out[module] = target
	}
	return out
}

// sortedModuleKeys keeps go.mod edits and renderer state deterministic across map iteration order.
func sortedModuleKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// countManagedStillPresent separates retained entries from replacements the renderer removed during this run.
func countManagedStillPresent(previous []string, current map[string]string) int {
	count := 0
	for _, module := range previous {
		if _, ok := current[module]; ok {
			count++
		}
	}
	return count
}

// loadManagedModuleReplaces reads renderer-owned replacement state from one project workspace.
func (w projectRenderWorkspace) loadManagedModuleReplaces() ([]string, error) {
	data, err := w.readFile(moduleReplacesStateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var modules []string
	if err := json.Unmarshal(data, &modules); err != nil {
		return nil, fmt.Errorf("read %s: %w", moduleReplacesStateFile, err)
	}
	return modules, nil
}

// saveManagedModuleReplaces publishes renderer-owned replacement state inside one project workspace.
func (w projectRenderWorkspace) saveManagedModuleReplaces(modules []string) error {
	if len(modules) == 0 {
		if _, err := w.removeFileIfExists(moduleReplacesStateFile); err != nil {
			return err
		}
		return nil
	}
	data, err := json.MarshalIndent(modules, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return w.writeFile(moduleReplacesStateFile, data, 0o644)
}

// runGoModEdit applies one replacement mutation from inside a project workspace.
func (w projectRenderWorkspace) runGoModEdit(flag, value string) error {
	cmd := exec.Command("go", "mod", "edit", flag, value)
	cmd.Dir = w.path()
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		err = w.logicalError(err)
		return commanddiag.Wrap(fmt.Sprintf("go mod edit %s %s", flag, value), err, string(out))
	}
	return nil
}
