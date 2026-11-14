package generate

import (
	"github.com/goforj/goforj/project"
	"strings"
)

// ResourceNames exposes the generator's named-resource discovery to configuration tooling.
func ResourceNames(root string, values map[string]string) map[project.ResourceKey][]string {
	environment := generationEnvironment{values: values}
	input := generationInput{projectDir: root, environment: environment, appPrefixes: newGenerationEnvironmentFilter(root, environment).sortedAppPrefixes()}
	return map[project.ResourceKey][]string{
		project.ResourceFeature:  discoverFeatureNames(input),
		project.ResourceDatabase: discoverDBConnectionNames(input),
		project.ResourceCache:    discoverCacheStoreNames(input),
		project.ResourceQueue:    discoverQueueNames(input),
		project.ResourceEvents:   discoverEventNames(input),
		project.ResourceStorage:  discoverStorageDiskNames(input),
		project.ResourceMail:     discoverMailNames(input),
	}
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
