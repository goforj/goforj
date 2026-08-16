package atlas

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/goforj/atlas/agents"
	"github.com/goforj/atlas/config"
	"github.com/goforj/atlas/files"
	"github.com/goforj/atlas/guidelines"
	"github.com/goforj/atlas/install"
	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// ReconcileGuidanceResult records native instruction files whose managed projection changed.
type ReconcileGuidanceResult struct {
	Updated []string
	Removed []string
}

// SynchronizeAgentGuidance persists Atlas's selected guideline surface as GoForj render policy and reconciles native files.
func SynchronizeAgentGuidance(root string) (ReconcileGuidanceResult, error) {
	atlasConfig, err := config.Load(root)
	if err != nil {
		return ReconcileGuidanceResult{}, fmt.Errorf("load Atlas state: %w", err)
	}
	projectConfig, err := project.LoadProjectConfigAt(root)
	if err != nil {
		return ReconcileGuidanceResult{}, fmt.Errorf("load Project configuration: %w", err)
	}
	selection := project.AgentGuidanceNone
	if atlasConfig.Features.Guidelines {
		selection = project.AgentGuidanceBaseline
	}
	if projectConfig.Render.AgentGuidance != selection || !projectConfig.Render.HasAgentGuidance() {
		projectConfig.Render.AgentGuidance = selection
		if err := writeProjectGuidanceConfig(filepath.Join(root, ".goforj.yml"), projectConfig); err != nil {
			return ReconcileGuidanceResult{}, fmt.Errorf("persist agent guidance: %w", err)
		}
	}
	return ReconcileAgentGuidance(root, selection)
}

// InferAgentGuidance migrates only Atlas-owned legacy evidence into the durable Project setting.
func InferAgentGuidance(root string) (project.AgentGuidance, error) {
	if _, err := os.Stat(config.FilePath(root)); err == nil {
		cfg, err := config.Load(root)
		if err != nil {
			return "", fmt.Errorf("load Atlas state: %w", err)
		}
		if cfg.Features.Guidelines {
			return project.AgentGuidanceBaseline, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect Atlas state: %w", err)
	}
	for _, agent := range agents.Builtins() {
		content, err := os.ReadFile(agent.GuidelinesPath(root))
		if err == nil && strings.Contains(string(content), "<!-- "+files.DefaultMarker+":start -->") {
			return project.AgentGuidanceBaseline, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("inspect %s guidance: %w", agent.Name(), err)
		}
	}
	return project.AgentGuidanceNone, nil
}

// ReconcileAgentGuidance projects the canonical Atlas baseline only into committed native targets.
func ReconcileAgentGuidance(root string, selection project.AgentGuidance) (ReconcileGuidanceResult, error) {
	if !selection.Valid() {
		return ReconcileGuidanceResult{}, fmt.Errorf("unsupported agent guidance %q", selection)
	}
	targets, err := guidanceTargets(root)
	if err != nil {
		return ReconcileGuidanceResult{}, err
	}
	return reconcileAgentGuidanceTargets(root, selection, targets)
}

// ReconcileGuidanceIntent applies Atlas's versioned native-guidance request through GoForj's atomic writer.
func ReconcileGuidanceIntent(root string, intent install.GuidanceReconciliation) (ReconcileGuidanceResult, error) {
	if intent.Version != install.GuidanceReconciliationVersion {
		return ReconcileGuidanceResult{}, fmt.Errorf("unsupported Atlas guidance reconciliation version %d", intent.Version)
	}
	selection := project.AgentGuidanceNone
	if intent.Enabled {
		selection = project.AgentGuidanceBaseline
	}
	projectConfig, err := project.LoadProjectConfigAt(root)
	if err != nil {
		return ReconcileGuidanceResult{}, fmt.Errorf("load Project configuration: %w", err)
	}
	if projectConfig.Render.AgentGuidance != selection || !projectConfig.Render.HasAgentGuidance() {
		projectConfig.Render.AgentGuidance = selection
		if err := writeProjectGuidanceConfig(filepath.Join(root, ".goforj.yml"), projectConfig); err != nil {
			return ReconcileGuidanceResult{}, fmt.Errorf("persist agent guidance: %w", err)
		}
	}
	return reconcileAgentGuidanceTargets(root, selection, intent.Targets)
}

// reconcileAgentGuidanceTargets applies one validated target selection without allowing Atlas to write native files.
func reconcileAgentGuidanceTargets(root string, selection project.AgentGuidance, targets []string) (ReconcileGuidanceResult, error) {
	if !selection.Valid() {
		return ReconcileGuidanceResult{}, fmt.Errorf("unsupported agent guidance %q", selection)
	}
	selectedTargets := slices.Clone(targets)
	slices.Sort(selectedTargets)
	selectedTargets = slices.Compact(selectedTargets)
	for _, name := range selectedTargets {
		if _, ok := agents.ByName(name); !ok {
			return ReconcileGuidanceResult{}, fmt.Errorf("Atlas guidance selects unsupported agent %q", name)
		}
	}
	content := guidelines.Compose(Project(root))
	result := ReconcileGuidanceResult{}
	for _, agent := range agents.Builtins() {
		path := agent.GuidelinesPath(root)
		selected := selection == project.AgentGuidanceBaseline && slices.Contains(selectedTargets, agent.Name())
		changed, removed, err := reconcileGuidanceFile(path, selected, content)
		if err != nil {
			return result, fmt.Errorf("reconcile %s guidance: %w", agent.Name(), err)
		}
		if changed {
			result.Updated = append(result.Updated, path)
		}
		if removed {
			result.Removed = append(result.Removed, path)
		}
	}
	return result, nil
}

// guidanceTargets uses committed Atlas selection and a stable Codex fallback instead of host discovery.
func guidanceTargets(root string) ([]string, error) {
	if _, err := os.Stat(config.FilePath(root)); errors.Is(err, os.ErrNotExist) {
		return []string{"codex"}, nil
	} else if err != nil {
		return nil, fmt.Errorf("inspect Atlas state: %w", err)
	}
	cfg, err := config.Load(root)
	if err != nil {
		return nil, fmt.Errorf("load Atlas state: %w", err)
	}
	if len(cfg.Agents) == 0 {
		return []string{"codex"}, nil
	}
	targets := slices.Clone(cfg.Agents)
	slices.Sort(targets)
	targets = slices.Compact(targets)
	for _, name := range targets {
		if _, ok := agents.ByName(name); !ok {
			return nil, fmt.Errorf("Atlas state selects unsupported agent %q", name)
		}
	}
	return targets, nil
}

// reconcileGuidanceFile replaces one managed marker or removes only that marker when unselected.
func reconcileGuidanceFile(path string, selected bool, generated string) (bool, bool, error) {
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, false, err
	}
	current := string(existing)
	next := files.RemoveMarkerBlock(current, files.DefaultMarker)
	if selected {
		next = files.MergeMarkerBlock(current, files.DefaultMarker, generated)
	}
	if next == current {
		return false, false, nil
	}
	if strings.TrimSpace(next) == "" {
		if errors.Is(err, os.ErrNotExist) {
			return false, false, nil
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, false, err
		}
		return true, true, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, false, err
	}
	mode := fs.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return false, false, statErr
	}
	if err := writeGuidanceAtomically(path, []byte(next), mode); err != nil {
		return false, false, err
	}
	return true, false, nil
}

// writeGuidanceAtomically prevents an interrupted projection from publishing partial native instructions.
func writeGuidanceAtomically(path string, content []byte, mode fs.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode.Perm()); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

// writeProjectGuidanceConfig uses the renderer's complete-file publication boundary.
func writeProjectGuidanceConfig(path string, cfg *project.Config) error {
	var content bytes.Buffer
	encoder := yaml.NewEncoder(&content)
	encoder.SetIndent(2)
	if err := encoder.Encode(cfg); err != nil {
		return err
	}
	return writeGuidanceAtomically(path, content.Bytes(), 0o644)
}
