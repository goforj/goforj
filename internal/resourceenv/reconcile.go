package resourceenv

import (
	"fmt"
	"strings"

	"github.com/goforj/goforj/internal/envfile"
	"github.com/goforj/goforj/project"
)

// Reconciliation keeps the rewritten owner contract and its effective plan together as one validated outcome.
type Reconciliation struct {
	Source        []byte
	EffectivePlan project.ResourcePlan
	Changed       bool
}

// Reconcile applies owner precedence and fills only missing resource initialization keys.
func Reconcile(source []byte, seed project.ResourcePlan, components project.Components, portableDefaults bool) (Reconciliation, error) {
	components = components.WithResolvedDependencies()
	source, removedGeneratedAssignments := RemoveGeneratedAssignments(source, components, nil)
	lines := strings.Split(string(source), "\n")
	effective := seed.Clone()
	changed := removedGeneratedAssignments

	for _, definition := range project.ResourceCatalog() {
		if !definition.AppliesTo(components) {
			effective = effective.WithoutSelection(definition.Key)
			continue
		}
		seedSelection, ok := seed.Selection(definition.Key)
		if !ok {
			return Reconciliation{}, fmt.Errorf("resource %s has no initialization selection", definition.Label)
		}
		activeKey := definition.EnvironmentKey("DRIVER")
		supportedKey := definition.EnvironmentKey("SUPPORTED_DRIVERS")
		active, activeSet := envfile.Lookup(lines, activeKey)
		active = project.CanonicalResourceDriver(definition.Key, active)
		if !activeSet || active == "" {
			active = seedSelection.Active
			lines = envfile.SetFinal(lines, activeKey, active)
			changed = true
		}

		supportedValue, supportedSet := envfile.Lookup(lines, supportedKey)
		supported := splitDriverList(supportedValue)
		for index := range supported {
			supported[index] = project.CanonicalResourceDriver(definition.Key, supported[index])
		}
		if supportedSet && len(supported) > 0 && !stringSliceContainsFold(supported, active) {
			return Reconciliation{}, fmt.Errorf("%s in .env excludes active %s %q; add %q before rerendering", supportedKey, activeKey, active, active)
		}
		if !supportedSet || len(supported) == 0 {
			firstPortableInitialization := (definition.Key == project.ResourceEvents || definition.Key == project.ResourceQueue) && !activeSet && !supportedSet
			if portableDefaults || firstPortableInitialization {
				supported = append([]string(nil), seedSelection.Supported...)
			} else {
				supported = []string{active}
			}
			if !stringSliceContainsFold(supported, active) {
				supported = append(supported, active)
			}
			lines = envfile.SetFinal(lines, supportedKey, strings.Join(supported, ","))
			changed = true
		}
		effective = effective.WithSelection(definition.Key, project.DriverSelection{Active: active, Supported: supported})
	}

	for _, named := range seed.GeneratedNamedSelections(components) {
		active, activeSet := envfile.Lookup(lines, named.EnvironmentKey)
		active = project.CanonicalResourceDriver(named.Resource, active)
		if !activeSet || active == "" {
			active = named.Active
			lines = envfile.SetFinal(lines, named.EnvironmentKey, active)
			changed = true
		}
		effective = effective.WithNamedSelection(named.EnvironmentKey, active)
	}

	normalized, err := effective.Normalized(components)
	if err != nil {
		return Reconciliation{}, fmt.Errorf("environment resource contract: %w", err)
	}
	if !changed {
		return Reconciliation{Source: source, EffectivePlan: normalized}, nil
	}
	updated := strings.Join(lines, "\n")
	if !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	return Reconciliation{Source: []byte(updated), EffectivePlan: normalized, Changed: true}, nil
}

// RemoveGeneratedAssignments prunes generated values that no longer belong to the effective component contract.
func RemoveGeneratedAssignments(source []byte, components project.Components, runtimeApps []project.App) ([]byte, bool) {
	components = components.WithResolvedDependencies()
	changed := false
	if !components.Cache {
		var removedCacheAssignments bool
		source, removedCacheAssignments = removeDisabledCacheAssignments(source, runtimeApps)
		changed = removedCacheAssignments
	}
	var removedDiagnosticAssignments bool
	source, removedDiagnosticAssignments = removeObsoleteDiagnosticCacheAssignments(source)
	return source, changed || removedDiagnosticAssignments
}

// ResolveServiceIntent reconstructs concrete owner intent from exact Compose profile tokens.
func ResolveServiceIntent(source []byte, fallback project.LocalServiceIntent) project.LocalServiceIntent {
	lines := strings.Split(string(source), "\n")
	profiles, profilesSet := envfile.Lookup(lines, "COMPOSE_PROFILES")
	if profilesSet && exactCSVToken(profiles, string(project.ServiceRedis)) {
		return fallback.WithMode(project.ServiceRedis, project.LocalServiceModeLocal)
	}
	if _, selected := fallback.Mode(project.ServiceRedis); selected {
		return fallback.WithMode(project.ServiceRedis, project.LocalServiceModeExternal)
	}
	return fallback
}

// removeDisabledCacheAssignments removes only framework-owned Cache values after the project envelope disables Cache.
func removeDisabledCacheAssignments(source []byte, runtimeApps []project.App) ([]byte, bool) {
	keys := map[string]struct{}{
		"METRICS_CACHE_ENABLED":        {},
		"CACHE_DRIVER":                 {},
		"CACHE_SUPPORTED_DRIVERS":      {},
		"CACHE_PREFIX":                 {},
		"CACHE_DEFAULT_TTL_SECONDS":    {},
		"CACHE_MEMORY_CLEANUP_SECONDS": {},
		"CACHE_SETTINGS_DRIVER":        {},
		"CACHE_SESSIONS_DRIVER":        {},
		"CACHE_INSPECTS_DRIVER":        {},
		"CACHE_LIGHTHOUSE_DRIVER":      {},
	}
	for _, app := range runtimeApps {
		prefix := project.AppEnvironmentPrefix(app.Name)
		if prefix != "" {
			keys[prefix+"_CACHE_DRIVER"] = struct{}{}
		}
	}
	return removeAssignments(source, keys)
}

// removeObsoleteDiagnosticCacheAssignments removes retired framework-owned diagnostic stores from environment contracts.
func removeObsoleteDiagnosticCacheAssignments(source []byte) ([]byte, bool) {
	return removeAssignments(source, map[string]struct{}{
		"CACHE_INSPECTS_DRIVER":   {},
		"CACHE_LIGHTHOUSE_DRIVER": {},
	})
}

// removeAssignments preserves owner formatting while deleting generated assignments by their exact keys.
func removeAssignments(source []byte, keys map[string]struct{}) ([]byte, bool) {
	lines := strings.Split(string(source), "\n")
	filtered := make([]string, 0, len(lines))
	changed := false
	for _, line := range lines {
		key, ok := envfile.ScanKey(line)
		if ok {
			if _, remove := keys[key]; remove {
				changed = true
				continue
			}
		}
		filtered = append(filtered, line)
	}
	if !changed {
		return source, false
	}
	return []byte(strings.Join(filtered, "\n")), true
}

// splitDriverList normalizes an owner list without changing its relative order before catalog validation.
func splitDriverList(value string) []string {
	drivers := []string{}
	seen := map[string]bool{}
	for _, part := range strings.Split(value, ",") {
		driver := strings.ToLower(strings.TrimSpace(part))
		if driver == "" || seen[driver] {
			continue
		}
		seen[driver] = true
		drivers = append(drivers, driver)
	}
	return drivers
}

// stringSliceContainsFold compares driver names before the plan normalizer applies canonical casing.
func stringSliceContainsFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

// exactCSVToken reports profile membership without substring matches such as redis-debug.
func exactCSVToken(value string, want string) bool {
	for _, token := range strings.Split(value, ",") {
		if strings.TrimSpace(token) == want {
			return true
		}
	}
	return false
}
