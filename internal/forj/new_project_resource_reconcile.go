package forj

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goforj/goforj/project"
)

// newProjectTargetResourceReconciliation is the read-only effective state shown before a non-empty target is confirmed.
type newProjectTargetResourceReconciliation struct {
	plan             project.ResourcePlan
	serviceIntent    project.LocalServiceIntent
	serviceConsumers []project.EffectiveResourceConsumer
	hasOverrides     bool
	overrideSummary  string
}

// reconcileNewProjectTargetResources applies owner-controlled target values without changing the proposed plan or target files.
func reconcileNewProjectTargetResources(
	targetPath string,
	proposed project.ResourcePlan,
	components project.Components,
	proposedIntent project.LocalServiceIntent,
) (newProjectTargetResourceReconciliation, error) {
	if err := components.ValidateRenderContract(); err != nil {
		return newProjectTargetResourceReconciliation{}, fmt.Errorf("target resource components: %w", err)
	}

	targetExists, err := newProjectTargetDirectoryExists(targetPath)
	if err != nil {
		return newProjectTargetResourceReconciliation{}, err
	}
	legacyQueueDriver := ""
	legacyQueueDriverSet := false
	if targetExists {
		legacyQueueDriver, legacyQueueDriverSet, err = readNewProjectLegacyQueueDriver(targetPath)
		if err != nil {
			return newProjectTargetResourceReconciliation{}, err
		}
	}

	seed, legacyApplied, err := completeNewProjectTargetResourceSeed(proposed, components, legacyQueueDriver, legacyQueueDriverSet)
	if err != nil {
		return newProjectTargetResourceReconciliation{}, err
	}
	result := newProjectTargetResourceReconciliation{
		plan:          seed.Clone(),
		serviceIntent: cloneNewProjectTargetServiceIntent(proposedIntent),
	}
	if !targetExists {
		if _, err := project.ResolveServicePlanWithConsumers(result.plan, components, result.serviceIntent, result.serviceConsumers); err != nil {
			return newProjectTargetResourceReconciliation{}, fmt.Errorf("target service plan: %w", err)
		}
		return result, nil
	}

	source, sourceExists, ownerControlled, err := readNewProjectTargetEnvironment(targetPath)
	if err != nil {
		return newProjectTargetResourceReconciliation{}, err
	}
	if !sourceExists || !ownerControlled {
		if _, err := project.ResolveServicePlanWithConsumers(result.plan, components, result.serviceIntent, result.serviceConsumers); err != nil {
			return newProjectTargetResourceReconciliation{}, fmt.Errorf("target service plan: %w", err)
		}
		if legacyApplied {
			result.hasOverrides = true
			result.overrideSummary = "Queue active: " + legacyQueueDriver + " (legacy config)"
		}
		return result, nil
	}

	_, effective, _, err := reconcileResourceEnvironment(source, seed, components, true)
	if err != nil {
		return newProjectTargetResourceReconciliation{}, fmt.Errorf("reconcile existing target resources: %w", err)
	}
	effectiveIntent := localServiceIntentFromEnvironment(source, proposedIntent)
	targetConfig, configErr := project.LoadProjectConfigAt(targetPath)
	if configErr != nil && !os.IsNotExist(configErr) {
		return newProjectTargetResourceReconciliation{}, fmt.Errorf("read existing target Apps: %w", configErr)
	}
	consumers, err := effectiveResourceConsumersFromEnvironment(source, effective, components, configuredResourceAppNames(targetConfig))
	if err != nil {
		return newProjectTargetResourceReconciliation{}, fmt.Errorf("discover existing target resource consumers: %w", err)
	}
	servicePlan, err := project.ResolveServicePlanWithConsumers(effective, components, effectiveIntent, consumers)
	if err != nil {
		return newProjectTargetResourceReconciliation{}, fmt.Errorf("target service plan: %w", err)
	}

	overrides := describeNewProjectTargetResourceOverrides(seed, effective, components, proposedIntent, effectiveIntent)
	if scopedServices := describeNewProjectTargetScopedServiceOverrides(effective, components, consumers, servicePlan); scopedServices != "" {
		overrides = append(overrides, scopedServices)
	}
	if legacyApplied {
		queue, queueExists := effective.Selection(project.ResourceQueue)
		if queueExists && queue.Active == strings.ToLower(strings.TrimSpace(legacyQueueDriver)) {
			if active, activeSet := envAssignment(strings.Split(string(source), "\n"), "QUEUE_DRIVER"); !activeSet || strings.TrimSpace(active) == "" {
				overrides = append(overrides, "Queue active: "+queue.Active+" (legacy config)")
			}
		}
	}
	result.plan = effective
	result.serviceIntent = effectiveIntent
	result.serviceConsumers = cloneEffectiveResourceConsumers(consumers)
	result.hasOverrides = len(overrides) > 0
	result.overrideSummary = strings.Join(overrides, "; ")
	return result, nil
}

// cloneEffectiveResourceConsumers prevents target previews from aliasing renderer handoff state.
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

// readNewProjectTargetEnvironment distinguishes deployment ownership from the lower-precedence committed fallback.
func readNewProjectTargetEnvironment(targetPath string) ([]byte, bool, bool, error) {
	for _, name := range []string{".env", ".env.example"} {
		path := filepath.Join(targetPath, name)
		source, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, false, false, fmt.Errorf("read existing target %s: %w", name, err)
		}
		return source, true, name == ".env", nil
	}
	return nil, false, false, nil
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
	return strings.ToLower(strings.TrimSpace(config.Render.LegacyQueueDriver())), true, nil
}

// completeNewProjectTargetResourceSeed uses legacy queue state only when the explicit proposal omitted that resource.
func completeNewProjectTargetResourceSeed(
	proposed project.ResourcePlan,
	components project.Components,
	legacyQueueDriver string,
	legacyQueueDriverSet bool,
) (project.ResourcePlan, bool, error) {
	seed := proposed.Clone()
	legacyApplied := false
	if components.Jobs {
		if _, queueSelected := seed.Selection(project.ResourceQueue); !queueSelected {
			shape := seed.Shape
			if shape == "" {
				shape = project.ResourceShapeStandalone
			}
			fallback, err := project.ResolveResourcePlan(shape, components)
			if err != nil {
				return project.ResourcePlan{}, false, fmt.Errorf("resolve target queue fallback: %w", err)
			}
			queue, _ := fallback.Selection(project.ResourceQueue)
			if legacyQueueDriverSet && legacyQueueDriver != "" {
				queue.Active = legacyQueueDriver
				queue.Supported = []string{legacyQueueDriver}
				legacyApplied = true
			}
			seed = seed.WithSelection(project.ResourceQueue, queue)
		}
	}
	normalized, err := seed.Normalized(components)
	if err != nil {
		return project.ResourcePlan{}, false, fmt.Errorf("proposed target resource plan: %w", err)
	}
	return normalized, legacyApplied, nil
}

// describeNewProjectTargetResourceOverrides returns stable confirmation text for values that differ from the proposal.
func describeNewProjectTargetResourceOverrides(
	proposed project.ResourcePlan,
	effective project.ResourcePlan,
	components project.Components,
	proposedIntent project.LocalServiceIntent,
	effectiveIntent project.LocalServiceIntent,
) []string {
	overrides := []string{}
	for _, definition := range project.ResourceCatalog() {
		if !definition.AppliesTo(components) {
			continue
		}
		proposedSelection, proposedExists := proposed.Selection(definition.Key)
		effectiveSelection, effectiveExists := effective.Selection(definition.Key)
		if !proposedExists || !effectiveExists {
			continue
		}
		if proposedSelection.Active != effectiveSelection.Active {
			overrides = append(overrides, definition.Label+" active: "+effectiveSelection.Active)
		}
		if !newProjectTargetDriverListsEqual(proposedSelection.Supported, effectiveSelection.Supported) {
			overrides = append(overrides, definition.Label+" built in: "+strings.Join(effectiveSelection.Supported, ","))
		}
	}

	proposedNamed := map[string]project.GeneratedNamedResourceSelection{}
	for _, named := range proposed.GeneratedNamedSelections(components) {
		proposedNamed[named.EnvironmentKey] = named
	}
	for _, named := range effective.GeneratedNamedSelections(components) {
		previous, ok := proposedNamed[named.EnvironmentKey]
		if ok && previous.Active != named.Active {
			overrides = append(overrides, named.Label+" active: "+named.Active)
		}
	}

	proposedRedisMode, proposedRedisSet := proposedIntent.Mode(project.ServiceRedis)
	effectiveRedisMode, effectiveRedisSet := effectiveIntent.Mode(project.ServiceRedis)
	if proposedRedisSet != effectiveRedisSet || proposedRedisMode != effectiveRedisMode {
		overrides = append(overrides, "Redis service: "+string(effectiveRedisMode))
	}
	return overrides
}

// describeNewProjectTargetScopedServiceOverrides exposes named and App-scoped infrastructure impact without publishing endpoint identities.
func describeNewProjectTargetScopedServiceOverrides(
	effective project.ResourcePlan,
	components project.Components,
	consumers []project.EffectiveResourceConsumer,
	servicePlan project.ServicePlan,
) string {
	standardConsumers := map[string]bool{}
	for _, definition := range project.ResourceCatalog() {
		if definition.AppliesTo(components) {
			standardConsumers[effectiveResourceConsumerName("", definition.Key, "")] = true
		}
	}
	for _, named := range effective.GeneratedNamedSelections(components) {
		standardConsumers[effectiveResourceConsumerName("", named.Resource, named.Name)] = true
	}

	scopedConsumers := map[string]bool{}
	for _, consumer := range consumers {
		name := strings.ToLower(strings.TrimSpace(consumer.Consumer))
		if name != "" && !standardConsumers[name] {
			scopedConsumers[name] = true
		}
	}

	type serviceGroup struct {
		key       project.ServiceKey
		state     project.ServiceState
		label     string
		consumers []string
	}
	groups := []serviceGroup{}
	groupIndexes := map[string]int{}
	for _, requirement := range servicePlan.Requirements {
		stateLabel := ""
		switch requirement.State {
		case project.ServiceStateActiveLocal:
			stateLabel = "local"
		case project.ServiceStateExternalRequired:
			stateLabel = "external"
		default:
			continue
		}
		matched := []string{}
		for _, consumer := range requirement.ActiveConsumers {
			consumer = strings.ToLower(strings.TrimSpace(consumer))
			if scopedConsumers[consumer] {
				matched = append(matched, consumer)
			}
		}
		if len(matched) == 0 {
			continue
		}
		groupKey := string(requirement.Key) + "\x00" + stateLabel
		groupIndex, exists := groupIndexes[groupKey]
		if !exists {
			groupIndex = len(groups)
			groupIndexes[groupKey] = groupIndex
			groups = append(groups, serviceGroup{key: requirement.Key, state: requirement.State, label: requirement.Label})
		}
		for _, consumer := range matched {
			alreadyIncluded := false
			for _, existing := range groups[groupIndex].consumers {
				if existing == consumer {
					alreadyIncluded = true
					break
				}
			}
			if !alreadyIncluded {
				groups[groupIndex].consumers = append(groups[groupIndex].consumers, consumer)
			}
		}
	}

	parts := make([]string, 0, len(groups))
	for _, group := range groups {
		label := strings.TrimSpace(group.label)
		if label == "" {
			label = string(group.key)
		}
		stateLabel := "external"
		if group.state == project.ServiceStateActiveLocal {
			stateLabel = "local"
		}
		parts = append(parts, label+" "+stateLabel+" for "+strings.Join(group.consumers, ", "))
	}
	if len(parts) == 0 {
		return ""
	}
	return "Named/App service use: " + strings.Join(parts, " · ")
}

// newProjectTargetDriverListsEqual compares normalized built-in contracts without hiding order changes.
func newProjectTargetDriverListsEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// cloneNewProjectTargetServiceIntent prevents target reconciliation from mutating the wizard's proposed placement map.
func cloneNewProjectTargetServiceIntent(intent project.LocalServiceIntent) project.LocalServiceIntent {
	cloned := project.LocalServiceIntent{Modes: make(map[project.ServiceKey]project.LocalServiceMode, len(intent.Modes))}
	for key, mode := range intent.Modes {
		cloned.Modes[key] = mode
	}
	return cloned
}
