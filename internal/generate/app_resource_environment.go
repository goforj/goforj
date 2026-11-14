package generate

import (
	"strings"

	"github.com/goforj/goforj/project"
)

// generationKnownAppEnvPrefixes includes every known App so disabled Apps cannot become default resource names.
func generationKnownAppEnvPrefixes(input generationInput) []string {
	prefixes := configuredGenerationAppEnvPrefixes(input.projectDir)
	for _, prefix := range input.appPrefixes {
		prefixes[prefix] = struct{}{}
	}
	return sortStrings(prefixes)
}

// appResourceEnvironmentOwner gives the longest known App prefix precedence over ambiguous default resource keys.
func appResourceEnvironmentOwner(key string, prefixes []string) string {
	return project.AppResourceKeyOwner(key, prefixes)
}

// generationResourceEnvPrefixes keeps emitted ownership rules aligned with the framework resource catalog.
func generationResourceEnvPrefixes() []string {
	definitions := project.ResourceCatalog()
	prefixes := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		prefixes = append(prefixes, definition.EnvironmentPrefix)
	}
	return prefixes
}

// appScopedChildNames recognizes exact resource settings after assigning their App ownership.
func appScopedChildNames(input generationInput, resourcePrefix, appPrefix string, rootKeys []string) []string {
	values := map[string]string{}
	prefixes := generationKnownAppEnvPrefixes(input)
	for _, entry := range input.environment.Entries() {
		if appResourceEnvironmentOwner(entry.key, prefixes) == appPrefix {
			values[entry.key] = entry.value
		}
	}
	scope := resourcePrefix
	if appPrefix != "" {
		scope = appPrefix + "_" + resourcePrefix
	}
	return exactScopedChildNames(generationEnvironment{values: values}, scope, rootKeys)
}

// AppResourceEnvironmentOwner identifies enabled managed resource settings before destructive App cleanup.
// App-owned matches take precedence over another primitive's root named resources.
func AppResourceEnvironmentOwner(key string, knownAppPrefixes []string, components project.Components) (owner string, managed bool) {
	for _, resource := range []struct {
		enabled bool
		prefix  string
		keys    []string
	}{
		{components.Feature, "FEATURE", featureRootKeys},
		{components.Notification, "NOTIFICATION", append([]string{"SUPPORTED_DRIVERS"}, notificationRootKeys...)},
		{components.Search, "SEARCH", append([]string{"SUPPORTED_DRIVERS"}, searchRootKeys...)},
	} {
		if !resource.enabled {
			continue
		}
		candidate := appResourceEnvironmentOwner(key, knownAppPrefixes)
		prefix := resource.prefix + "_"
		if candidate != "" {
			prefix = candidate + "_" + prefix
		}
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		if _, ok := splitScopedEnvKey(strings.TrimPrefix(key, prefix), resource.keys); ok && (!managed || len(candidate) > len(owner)) {
			owner, managed = candidate, true
		}
	}
	return owner, managed
}
