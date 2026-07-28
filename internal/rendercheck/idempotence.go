package rendercheck

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/goforj/goforj/internal/projectlayout"
	"github.com/goforj/goforj/project"
)

const appOwnedIdempotenceMarker = "// RenderCheck owner sentinel: rerenders must preserve this App-owned file.\n"

// appComponentSelection keeps map ordering out of stable component comparisons.
type appComponentSelection struct {
	name       string
	components project.Components
}

// renderedFileSnapshot records one representative App-owned file after the sentinel adds a user edit.
type renderedFileSnapshot struct {
	path     string
	contents []byte
}

// renderIdempotenceSnapshot captures canonical config state and a representative App-owned edit surface.
type renderIdempotenceSnapshot struct {
	configBytes   []byte
	components    project.Components
	apps          []appComponentSelection
	appOwnedFiles []renderedFileSnapshot
}

// validateRenderIdempotence proves repeat rendering and default generation preserve the initial canonical contract.
func (worker renderComboWorker) validateRenderIdempotence(expected *project.Config, apps []project.App) error {
	baseline, err := captureRenderIdempotenceSnapshot(worker.workspaceRoot, expected, apps)
	if err != nil {
		return fmt.Errorf("capture render idempotence baseline: %w", err)
	}
	for _, command := range []string{"render", "generate"} {
		if err := worker.runForj(command); err != nil {
			return err
		}
		if err := baseline.validateUnchanged(worker.workspaceRoot); err != nil {
			return fmt.Errorf("forj %s changed canonical config: %w", command, err)
		}
		config, err := project.LoadProjectConfigAt(worker.workspaceRoot)
		if err != nil {
			return fmt.Errorf("load config after forj %s: %w", command, err)
		}
		if err := validateRenderedComponentContracts(worker.workspaceRoot, config, apps); err != nil {
			return fmt.Errorf("forj %s component contract: %w", command, err)
		}
	}
	return nil
}

// captureRenderIdempotenceSnapshot requires the first render to finish one-way migrations before recording the baseline.
func captureRenderIdempotenceSnapshot(root string, expected *project.Config, apps []project.App) (renderIdempotenceSnapshot, error) {
	configBytes, config, err := readCanonicalRenderedConfig(root)
	if err != nil {
		return renderIdempotenceSnapshot{}, err
	}
	snapshot := renderedConfigSelection(config)
	expectedSelection := renderedConfigSelection(expected)
	if err := snapshot.validateSelection(expectedSelection); err != nil {
		return renderIdempotenceSnapshot{}, fmt.Errorf("rendered selection differs from requested selection: %w", err)
	}
	appOwnedFiles, err := captureAppOwnedFileSnapshots(root, apps)
	if err != nil {
		return renderIdempotenceSnapshot{}, err
	}
	snapshot.configBytes = configBytes
	snapshot.appOwnedFiles = appOwnedFiles
	return snapshot, nil
}

// validateUnchanged rejects byte churn as well as component widening that a rewritten config could otherwise conceal.
func (snapshot renderIdempotenceSnapshot) validateUnchanged(root string) error {
	configBytes, config, err := readCanonicalRenderedConfig(root)
	if err != nil {
		return err
	}
	if !bytes.Equal(configBytes, snapshot.configBytes) {
		return fmt.Errorf(".goforj.yml bytes changed")
	}
	if err := renderedConfigSelection(config).validateSelection(snapshot); err != nil {
		return err
	}
	return validateRenderedFilesUnchanged(root, snapshot.appOwnedFiles)
}

// captureAppOwnedFileSnapshots adds a harmless owner edit to a small explicit surface before recording exact bytes.
func captureAppOwnedFileSnapshots(root string, apps []project.App) ([]renderedFileSnapshot, error) {
	snapshots := make([]renderedFileSnapshot, 0, len(apps)*3)
	for _, app := range apps {
		app = projectlayout.NormalizeApp(app)
		paths := []string{
			filepath.Join(app.AppDir, "lifecycle.go"),
			filepath.Join(app.WireDir, "inject_services_app.go"),
		}
		jobsPath := filepath.Join(app.WireDir, "inject_jobs_app.go")
		if _, err := os.Stat(filepath.Join(root, jobsPath)); err == nil {
			paths = append(paths, jobsPath)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("inspect App-owned file %s: %w", jobsPath, err)
		}

		for _, path := range paths {
			fullPath := filepath.Join(root, path)
			contents, err := os.ReadFile(fullPath)
			if err != nil {
				return nil, fmt.Errorf("read App-owned file %s: %w", path, err)
			}
			info, err := os.Stat(fullPath)
			if err != nil {
				return nil, fmt.Errorf("inspect App-owned file %s: %w", path, err)
			}
			marked := append([]byte(nil), contents...)
			if len(marked) > 0 && marked[len(marked)-1] != '\n' {
				marked = append(marked, '\n')
			}
			marked = append(marked, appOwnedIdempotenceMarker...)
			if err := os.WriteFile(fullPath, marked, info.Mode().Perm()); err != nil {
				return nil, fmt.Errorf("mark App-owned file %s: %w", path, err)
			}
			snapshots = append(snapshots, renderedFileSnapshot{path: path, contents: marked})
		}
	}
	return snapshots, nil
}

// validateRenderedFilesUnchanged reports the first owner edit that a repeated command rewrote or removed.
func validateRenderedFilesUnchanged(root string, snapshots []renderedFileSnapshot) error {
	for _, snapshot := range snapshots {
		contents, err := os.ReadFile(filepath.Join(root, snapshot.path))
		if err != nil {
			return fmt.Errorf("read App-owned file %s: %w", snapshot.path, err)
		}
		if !bytes.Equal(contents, snapshot.contents) {
			return fmt.Errorf("App-owned file %s bytes changed", snapshot.path)
		}
	}
	return nil
}

// readCanonicalRenderedConfig verifies migration metadata and mapping syntax have disappeared from persisted config.
func readCanonicalRenderedConfig(root string) ([]byte, *project.Config, error) {
	path := filepath.Join(root, ".goforj.yml")
	configBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read .goforj.yml: %w", err)
	}
	config, err := project.LoadProjectConfigAt(root)
	if err != nil {
		return nil, nil, fmt.Errorf("load .goforj.yml: %w", err)
	}
	if config.NeedsComponentMigration() {
		return nil, nil, fmt.Errorf(".goforj.yml still uses legacy component mappings")
	}
	if bytes.Contains(configBytes, []byte("component_contract:")) {
		return nil, nil, fmt.Errorf(".goforj.yml retains retired component_contract metadata")
	}
	return configBytes, config, nil
}

// renderedConfigSelection reduces a full config to the selections that rerendering is forbidden to widen.
func renderedConfigSelection(config *project.Config) renderIdempotenceSnapshot {
	snapshot := renderIdempotenceSnapshot{}
	if config == nil {
		return snapshot
	}
	snapshot.components = config.Render.Components
	names := make([]string, 0, len(config.Apps))
	for name := range config.Apps {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		snapshot.apps = append(snapshot.apps, appComponentSelection{
			name:       name,
			components: config.Apps[name].Components,
		})
	}
	return snapshot
}

// validateSelection reports the exact App scope that drifted so matrix failures remain actionable.
func (snapshot renderIdempotenceSnapshot) validateSelection(expected renderIdempotenceSnapshot) error {
	if snapshot.components != expected.components {
		return fmt.Errorf("default App components = %#v, want %#v", snapshot.components, expected.components)
	}
	if len(snapshot.apps) != len(expected.apps) {
		return fmt.Errorf("additional app selection count = %d, want %d", len(snapshot.apps), len(expected.apps))
	}
	for index := range snapshot.apps {
		if snapshot.apps[index] != expected.apps[index] {
			return fmt.Errorf("additional app selection = %#v, want %#v", snapshot.apps[index], expected.apps[index])
		}
	}
	return nil
}
