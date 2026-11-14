package generate

import "strings"

// generationKnownAppEnvPrefixes includes every known App so disabled Apps cannot become default resource names.
func generationKnownAppEnvPrefixes(input generationInput) []string {
	prefixes := configuredGenerationAppEnvPrefixes(input.projectDir)
	for _, prefix := range input.appPrefixes {
		prefixes[prefix] = struct{}{}
	}
	return sortStrings(prefixes)
}

// appResourceEnvironmentOwner gives the longest known App prefix precedence over ambiguous default resource keys.
func appResourceEnvironmentOwner(key, resourcePrefix string, prefixes []string) string {
	owner := ""
	for _, prefix := range prefixes {
		if strings.HasPrefix(key, prefix+"_"+resourcePrefix+"_") && len(prefix) > len(owner) {
			owner = prefix
		}
	}
	return owner
}

// appScopedChildNames recognizes exact resource settings after assigning their App ownership.
func appScopedChildNames(input generationInput, resourcePrefix, appPrefix string, rootKeys []string) []string {
	values := map[string]string{}
	prefixes := generationKnownAppEnvPrefixes(input)
	for _, entry := range input.environment.Entries() {
		if appResourceEnvironmentOwner(entry.key, resourcePrefix, prefixes) == appPrefix {
			values[entry.key] = entry.value
		}
	}
	scope := resourcePrefix
	if appPrefix != "" {
		scope = appPrefix + "_" + resourcePrefix
	}
	return exactScopedChildNames(generationEnvironment{values: values}, scope, rootKeys)
}
