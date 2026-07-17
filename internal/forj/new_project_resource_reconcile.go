package forj

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goforj/str"

	"github.com/goforj/goforj/internal/resourceenv"
	"github.com/goforj/goforj/project"
)

// newProjectResourcePreparation is the single owner-reconciled resource contract handed from Path to rendering.
type newProjectResourcePreparation struct {
	plan             project.ResourcePlan
	serviceIntent    project.LocalServiceIntent
	serviceConsumers []project.EffectiveResourceConsumer
	servicePlan      project.ServicePlan
}

// prepareNewProjectTargetResources applies owner-controlled target values without changing the proposal or target files.
func prepareNewProjectTargetResources(
	targetPath string,
	proposed project.ResourcePlan,
	components project.Components,
	proposedIntent project.LocalServiceIntent,
) (newProjectResourcePreparation, error) {
	if err := components.ValidateRenderContract(); err != nil {
		return newProjectResourcePreparation{}, fmt.Errorf("target resource components: %w", err)
	}

	targetExists, err := newProjectTargetDirectoryExists(targetPath)
	if err != nil {
		return newProjectResourcePreparation{}, err
	}
	legacyQueueDriver := ""
	legacyQueueDriverSet := false
	if targetExists {
		legacyQueueDriver, legacyQueueDriverSet, err = readNewProjectLegacyQueueDriver(targetPath)
		if err != nil {
			return newProjectResourcePreparation{}, err
		}
	}

	seed, err := completeNewProjectTargetResourceSeed(proposed, components, legacyQueueDriver, legacyQueueDriverSet)
	if err != nil {
		return newProjectResourcePreparation{}, err
	}
	if !targetExists {
		return resolveNewProjectResourcePreparation(seed, components, proposedIntent, nil)
	}

	source, ownerExists, err := readNewProjectTargetOwnerEnvironment(targetPath)
	if err != nil {
		return newProjectResourcePreparation{}, err
	}
	if !ownerExists {
		return resolveNewProjectResourcePreparation(seed, components, proposedIntent, nil)
	}

	reconciled, err := resourceenv.Reconcile(source, seed, components, true)
	if err != nil {
		return newProjectResourcePreparation{}, fmt.Errorf("reconcile existing target resources: %w", err)
	}
	effective := reconciled.EffectivePlan
	effectiveIntent := resourceenv.ResolveServiceIntent(source, proposedIntent)
	targetConfig, configErr := project.LoadProjectConfigAt(targetPath)
	if configErr != nil && !os.IsNotExist(configErr) {
		return newProjectResourcePreparation{}, fmt.Errorf("read existing target Apps: %w", configErr)
	}
	consumers, err := resourceenv.ResolveConsumers(source, effective, components, targetConfig)
	if err != nil {
		return newProjectResourcePreparation{}, fmt.Errorf("discover existing target resource consumers: %w", err)
	}
	return resolveNewProjectResourcePreparation(effective, components, effectiveIntent, consumers)
}

// resolveNewProjectResourcePreparation validates service placement once and captures the complete renderer handoff.
func resolveNewProjectResourcePreparation(
	plan project.ResourcePlan,
	components project.Components,
	intent project.LocalServiceIntent,
	consumers []project.EffectiveResourceConsumer,
) (newProjectResourcePreparation, error) {
	servicePlan, err := project.ResolveServicePlanWithConsumers(plan, components, intent, consumers)
	if err != nil {
		return newProjectResourcePreparation{}, fmt.Errorf("target service plan: %w", err)
	}
	return newProjectResourcePreparation{
		plan:             plan.Clone(),
		serviceIntent:    cloneNewProjectTargetServiceIntent(intent),
		serviceConsumers: cloneEffectiveResourceConsumers(consumers),
		servicePlan:      cloneNewProjectServicePlan(servicePlan),
	}, nil
}

// cloneNewProjectResourcePreparation prevents wizard model copies from sharing mutable maps or slices.
func cloneNewProjectResourcePreparation(preparation newProjectResourcePreparation) newProjectResourcePreparation {
	return newProjectResourcePreparation{
		plan:             preparation.plan.Clone(),
		serviceIntent:    cloneNewProjectTargetServiceIntent(preparation.serviceIntent),
		serviceConsumers: cloneEffectiveResourceConsumers(preparation.serviceConsumers),
		servicePlan:      cloneNewProjectServicePlan(preparation.servicePlan),
	}
}

// cloneNewProjectServicePlan protects nested consumer slices when a prepared plan crosses wizard stages.
func cloneNewProjectServicePlan(servicePlan project.ServicePlan) project.ServicePlan {
	cloned := project.ServicePlan{Requirements: make([]project.ServiceRequirement, len(servicePlan.Requirements))}
	for index, requirement := range servicePlan.Requirements {
		cloned.Requirements[index] = requirement
		cloned.Requirements[index].ActiveConsumers = append([]string(nil), requirement.ActiveConsumers...)
	}
	return cloned
}

// cloneEffectiveResourceConsumers prevents owner discovery from aliasing renderer handoff state.
func cloneEffectiveResourceConsumers(consumers []project.EffectiveResourceConsumer) []project.EffectiveResourceConsumer {
	return append([]project.EffectiveResourceConsumer(nil), consumers...)
}

// newProjectTargetDirectoryExists distinguishes an untouched destination from an invalid target path.
func newProjectTargetDirectoryExists(targetPath string) (bool, error) {
	if strings.TrimSpace(targetPath) == "" {
		return false, nil
	}
	info, err := os.Stat(targetPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect new-project target: %w", err)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("new-project target %q is not a directory", targetPath)
	}
	return true, nil
}

// readNewProjectTargetOwnerEnvironment ignores .env.example because committed defaults cannot override a concrete proposal.
func readNewProjectTargetOwnerEnvironment(targetPath string) ([]byte, bool, error) {
	path := filepath.Join(targetPath, ".env")
	source, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read existing target .env: %w", err)
	}
	return source, true, nil
}

// readNewProjectLegacyQueueDriver reads load-only migration state without rewriting the existing project config.
func readNewProjectLegacyQueueDriver(targetPath string) (string, bool, error) {
	config, err := project.LoadProjectConfigAt(targetPath)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read existing target .goforj.yml: %w", err)
	}
	if !config.Render.HasLegacyQueueDriver() {
		return "", false, nil
	}
	return str.Of(config.Render.LegacyQueueDriver()).TrimSpace().ToLower().String(), true, nil
}

// completeNewProjectTargetResourceSeed uses legacy Queue state only when the explicit proposal omitted that resource.
func completeNewProjectTargetResourceSeed(
	proposed project.ResourcePlan,
	components project.Components,
	legacyQueueDriver string,
	legacyQueueDriverSet bool,
) (project.ResourcePlan, error) {
	seed := proposed.Clone()
	if components.Jobs {
		if _, queueSelected := seed.Selection(project.ResourceQueue); !queueSelected {
			fallback, err := project.DefaultResourcePlan(components)
			if err != nil {
				return project.ResourcePlan{}, fmt.Errorf("resolve target queue fallback: %w", err)
			}
			queue, _ := fallback.Selection(project.ResourceQueue)
			if legacyQueueDriverSet && legacyQueueDriver != "" {
				queue.Active = legacyQueueDriver
				queue.Supported = []string{legacyQueueDriver}
			}
			seed = seed.WithSelection(project.ResourceQueue, queue)
		}
	}
	normalized, err := seed.Normalized(components)
	if err != nil {
		return project.ResourcePlan{}, fmt.Errorf("proposed target resource plan: %w", err)
	}
	return normalized, nil
}

// cloneNewProjectTargetServiceIntent prevents owner reconciliation from mutating the wizard's proposed placement map.
func cloneNewProjectTargetServiceIntent(intent project.LocalServiceIntent) project.LocalServiceIntent {
	cloned := project.LocalServiceIntent{Modes: make(map[project.ServiceKey]project.LocalServiceMode, len(intent.Modes))}
	for key, mode := range intent.Modes {
		cloned.Modes[key] = mode
	}
	return cloned
}
