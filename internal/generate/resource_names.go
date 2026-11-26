package generate

import (
	"strings"

	"github.com/goforj/goforj/project"
)

// ResourceNames exposes the generator's named-resource discovery to configuration tooling.
func ResourceNames(root string, values map[string]string) map[project.ResourceKey][]string {
	environment := generationEnvironment{values: values}
	input := generationInput{projectDir: root, environment: environment, appPrefixes: newGenerationEnvironmentFilter(root, environment).sortedAppPrefixes()}
	return resourceNames(input)
}

// resourceNames keeps all inventory callers on the generator's named-resource discovery paths.
func resourceNames(input generationInput) map[project.ResourceKey][]string {
	return map[project.ResourceKey][]string{
		project.ResourceNotification: discoverNotificationNames(input),
		project.ResourceFeature:      discoverFeatureNames(input),
		project.ResourceDatabase:     discoverDBConnectionNames(input),
		project.ResourceCache:        discoverCacheStoreNames(input),
		project.ResourceQueue:        discoverQueueNames(input),
		project.ResourceEvents:       discoverEventNames(input),
		project.ResourceStorage:      discoverStorageDiskNames(input),
		project.ResourceMail:         discoverMailNames(input),
		project.ResourceSearch:       discoverSearchNames(input),
	}
}

// ResourceNamesByApp reports prospective named registrations for participating Apps without loading configuration from disk.
// Config must be a nonnil valid prospective project configuration. The empty prefix identifies the default App.
// Omitted Apps are excluded from filesystem discovery before their files are removed.
func ResourceNamesByApp(root string, values map[string]string, config *project.Config, omittedApps ...string) map[project.ResourceKey]map[string][]string {
	environment := generationEnvironment{values: values}
	input := generationInput{projectDir: root, environment: environment, config: config}
	if len(omittedApps) > 0 {
		omitted := makeSet(omittedApps...)
		input.appNames = []string{}
		for _, name := range generationKnownAppNames(root, config) {
			if _, excluded := omitted[name]; !excluded {
				input.appNames = append(input.appNames, name)
			}
		}
	}
	input.appPrefixes = sortStrings(generationAppEnvPrefixSetWithConfigured(environment, generationEnvironmentResources(), input.configuredAppEnvPrefixes()))
	names := resourceNames(input)
	result := map[project.ResourceKey]map[string][]string{}
	for _, definition := range project.ResourceCatalog() {
		scopes := map[string][]string{}
		result[definition.Key] = scopes
		if definition.Key == project.ResourceFeature {
			// Explicit configuration makes ownership discovery independent of config loading errors.
			featureScopes, _ := generationFeatureScopes(input)
			for _, scope := range featureScopes {
				scoped := make([]string, len(scope.Names))
				for index, name := range scope.Names {
					scoped[index] = strings.ToLower(name)
				}
				scopes[scope.Prefix] = scoped
			}
			continue
		}
		if definition.Key == project.ResourceSearch {
			searchScopes, _ := generationSearchScopes(input)
			for _, scope := range searchScopes {
				scoped := make([]string, len(scope.Names))
				for index, name := range scope.Names {
					scoped[index] = strings.ToLower(name)
				}
				scopes[scope.Prefix] = scoped
			}
			continue
		}
		if definition.Key == project.ResourceNotification {
			notificationScopes, _ := generationNotificationScopes(input)
			for _, scope := range notificationScopes {
				scoped := make([]string, len(scope.Names))
				for index, name := range scope.Names {
					scoped[index] = strings.ToLower(name)
				}
				scopes[scope.Prefix] = scoped
			}
			continue
		}
		if generationComponentsSupportResource(config.Render.Components.WithResolvedDependencies(), definition.EnvironmentPrefix) {
			scopes[""] = append([]string(nil), names[definition.Key]...)
		}
		for _, prefix := range generationAppEnvPrefixesForResource(input, definition.EnvironmentPrefix) {
			scopes[prefix] = append([]string(nil), names[definition.Key]...)
		}
	}
	return result
}

// ResourceNamesInScope discovers names after configuration tooling has assigned values to one App scope.
// Project ownership must not be applied again after App prefixes have been removed.
func ResourceNamesInScope(values map[string]string) map[project.ResourceKey][]string {
	names := map[project.ResourceKey][]string{}
	environment := generationEnvironment{values: values}
	definitions := map[string]project.ResourceKey{}
	for _, definition := range project.ResourceCatalog() {
		definitions[definition.EnvironmentPrefix] = definition.Key
	}
	for _, resource := range generationEnvironmentResources() {
		key := definitions[resource.prefix]
		var scoped []string
		for _, name := range exactScopedChildNames(environment, resource.prefix, resource.rootKeys) {
			name = strings.ToLower(name)
			if key == project.ResourceDatabase && (name == "root" || dbHelperConnectionName(name)) {
				continue
			}
			scoped = append(scoped, name)
		}
		names[key] = scoped
	}
	return names
}
