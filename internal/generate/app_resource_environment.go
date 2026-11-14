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
func AppResourceEnvironmentOwner(key string, knownAppPrefixes []string, components project.Components) (owner string, managed bool) {
	if !components.Feature {
		return "", false
	}
	owner = appResourceEnvironmentOwner(key, knownAppPrefixes)
	prefix := "FEATURE_"
	if owner != "" {
		prefix = owner + "_" + prefix
	}
	if !strings.HasPrefix(key, prefix) {
		return "", false
	}
	if _, ok := splitScopedEnvKey(strings.TrimPrefix(key, prefix), featureRootKeys); !ok {
		return "", false
	}
	return owner, true
}

// legacyResourceGenerationInput excludes settings consumed by enabled App Feature managers from legacy resource interpretation.
func legacyResourceGenerationInput(input generationInput, resourcePrefix string) generationInput {
	if resourcePrefix == "FEATURE" {
		return input
	}
	if _, err := project.LoadProjectConfigAt(input.projectDir); err != nil {
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
