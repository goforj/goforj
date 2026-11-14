package generate

import (
	"strings"

	"github.com/goforj/goforj/project"
)

// generationKnownAppEnvPrefixes includes every known App so disabled Apps cannot become default resource names.
func generationKnownAppEnvPrefixes(input generationInput) []string {
	prefixes := input.configuredAppEnvPrefixes()
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

// SharedResourceEnvironmentSetting recognizes legacy root settings consumed by remaining Apps after Feature ownership is resolved.
func SharedResourceEnvironmentSetting(key string, components project.Components) bool {
	for _, definition := range project.ResourceCatalog() {
		prefix := definition.EnvironmentPrefix + "_"
		if definition.Key == project.ResourceFeature || !definition.AppliesTo(components) || !strings.HasPrefix(key, prefix) {
			continue
		}
		if key == definition.EnvironmentKey("SUPPORTED_DRIVERS") {
			return true
		}
		for _, resource := range generationEnvironmentResources() {
			if resource.prefix == definition.EnvironmentPrefix {
				_, recognized := splitScopedEnvKey(strings.TrimPrefix(key, prefix), resource.rootKeys)
				return recognized
			}
		}
	}
	return false
}

// legacyResourceGenerationInput preserves configless prefix boundaries and excludes settings consumed by enabled App Feature managers.
func legacyResourceGenerationInput(input generationInput, resourcePrefix string) generationInput {
	if resourcePrefix == "FEATURE" {
		return input
	}
	if _, err := input.projectConfig(); err != nil {
		// Legacy projects used Feature words as App names before Feature became a resource marker.
		resources := make([]generationEnvironmentResource, 0, len(generationEnvironmentResources())-1)
		for _, resource := range generationEnvironmentResources() {
			if resource.prefix != "FEATURE" {
				resources = append(resources, resource)
			}
		}
		input.appPrefixes = sortStrings(generationAppEnvPrefixSetForResources(input.projectDir, input.environment, resources))
		return input
	}
	enabled := makeSet(generationAppEnvPrefixesForResource(input, "FEATURE")...)
	prefixes := generationKnownAppEnvPrefixes(input)
	values := make(map[string]string, len(input.environment.values))
	for _, entry := range input.environment.Entries() {
		owner := appResourceEnvironmentOwner(entry.key, prefixes)
		if _, ok := enabled[owner]; owner != "" && ok && strings.HasPrefix(entry.key, owner+"_FEATURE_") {
			continue
		}
		values[entry.key] = entry.value
	}
	input.environment = generationEnvironment{values: values}
	return input
}
