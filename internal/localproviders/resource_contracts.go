package localproviders

import (
	"strings"

	"github.com/goforj/goforj/project"
)

// resourceDefinitions retains every active consumer of a shared key instead of assigning it to only one resource family.
func resourceDefinitions(config *project.Config, key string) []project.ResourceDefinition {
	var definitions []project.ResourceDefinition
	for _, definition := range project.ResourceCatalog() {
		prefix := definition.EnvironmentPrefix + "_"
		participates := strings.HasPrefix(key, prefix) && definition.AppliesTo(project.ProjectComponents(config))
		for name, app := range config.Apps {
			if name != project.DefaultAppName && strings.HasPrefix(key, project.AppEnvironmentPrefix(name)+"_"+prefix) && definition.AppliesTo(project.NormalizeConfiguredAppComponents(config, app.Components)) {
				participates = true
			}
		}
		if participates {
			definitions = append(definitions, definition)
		}
	}
	if len(definitions) == 0 {
		base := resourceKey(config, key)
		for _, definition := range project.ResourceCatalog() {
			if strings.HasPrefix(base, definition.EnvironmentPrefix+"_") {
				definitions = append(definitions, definition)
			}
		}
	}
	return definitions
}

// supportsResourceDriver keeps a single stored driver compatible with all of its active resource consumers.
func supportsResourceDriver(definitions []project.ResourceDefinition, driver string) bool {
	for _, definition := range definitions {
		if _, supported := definition.Driver(driver); !supported {
			return false
		}
	}
	return true
}

// sharedResourceChoices limits the picker to drivers that every consumer can compile.
func sharedResourceChoices(config *project.Config, resource Resource) Resource {
	definitions := resourceDefinitions(config, resource.Key)
	if len(definitions) < 2 {
		return resource
	}
	var choices []project.DriverDefinition
	for _, driver := range resource.Definition.Drivers {
		if supportsResourceDriver(definitions, driver.Name) {
			choices = append(choices, driver)
		}
	}
	resource.Definition.Drivers = choices
	if _, ok := resource.Definition.Driver(resource.Definition.DefaultDriver); !ok {
		resource.Definition.DefaultDriver = ""
		for _, driver := range choices {
			if driver.Service == "" {
				resource.Definition.DefaultDriver = driver.Name
				break
			}
		}
	}
	return resource
}
