package rendercheck

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/goforj/goforj/project"
)

// appComponentSelection keeps map ordering out of stable component comparisons.
type appComponentSelection struct {
	name       string
	components project.Components
}

// renderedConfigSnapshot captures the exact canonical config and its user-selected component boundaries.
type renderedConfigSnapshot struct {
	configBytes []byte
	components  project.Components
	apps        []appComponentSelection
}

// validateRenderIdempotence proves repeat rendering and default generation preserve the initial canonical contract.
func (worker renderComboWorker) validateRenderIdempotence(expected *project.Config, apps []project.App) error {
	baseline, err := captureRenderedConfigSnapshot(worker.workspaceRoot, expected)
	if err != nil {
		return fmt.Errorf("capture canonical config: %w", err)
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

// captureRenderedConfigSnapshot requires the first render to finish the one-way migration before it becomes the baseline.
func captureRenderedConfigSnapshot(root string, expected *project.Config) (renderedConfigSnapshot, error) {
	configBytes, config, err := readCanonicalRenderedConfig(root)
	if err != nil {
		return renderedConfigSnapshot{}, err
	}
	snapshot := renderedConfigSelection(config)
	expectedSelection := renderedConfigSelection(expected)
	if err := snapshot.validateSelection(expectedSelection); err != nil {
		return renderedConfigSnapshot{}, fmt.Errorf("rendered selection differs from requested selection: %w", err)
	}
	snapshot.configBytes = configBytes
	return snapshot, nil
}

// validateUnchanged rejects byte churn as well as component widening that a rewritten config could otherwise conceal.
func (snapshot renderedConfigSnapshot) validateUnchanged(root string) error {
	configBytes, config, err := readCanonicalRenderedConfig(root)
	if err != nil {
		return err
	}
	if !bytes.Equal(configBytes, snapshot.configBytes) {
		return fmt.Errorf(".goforj.yml bytes changed")
	}
	return renderedConfigSelection(config).validateSelection(snapshot)
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
func renderedConfigSelection(config *project.Config) renderedConfigSnapshot {
	snapshot := renderedConfigSnapshot{}
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
func (snapshot renderedConfigSnapshot) validateSelection(expected renderedConfigSnapshot) error {
	if snapshot.components != expected.components {
		return fmt.Errorf("default App components = %#v, want %#v", snapshot.components, expected.components)
	}
	if len(snapshot.apps) != len(expected.apps) {
		return fmt.Errorf("named App selection count = %d, want %d", len(snapshot.apps), len(expected.apps))
	}
	for index := range snapshot.apps {
		if snapshot.apps[index] != expected.apps[index] {
			return fmt.Errorf("named App selection = %#v, want %#v", snapshot.apps[index], expected.apps[index])
		}
	}
	return nil
}
