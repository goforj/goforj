package stacks

import (
	"fmt"
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

// SetValue validates generic editor input and preserves the provider picker's driver transition rules.
func (s *Session) SetValue(values map[string]string, key, value string) error {
	candidate := clone(values)
	definitions := resourceDefinitions(s.config, key)
	if strings.HasSuffix(key, "_DRIVER") && len(definitions) > 0 {
		base := resourceKey(s.config, key)
		definition := definitions[0]
		for _, current := range definitions {
			if strings.HasPrefix(base, current.EnvironmentPrefix+"_") {
				definition = current
			}
		}
		resource := Resource{Key: key, SupportedKey: definition.EnvironmentKey("SUPPORTED_DRIVERS"), Label: key, Definition: definition}
		if strings.TrimSpace(value) == "" {
			candidate[key] = value
			if err := s.restoreDriverInheritance(candidate, values, key); err != nil {
				return err
			}
		} else if err := s.SetDriver(candidate, resource, value); err != nil {
			return err
		}
	} else {
		candidate[key] = value
	}
	if err := s.Validate(candidate); err != nil {
		return err
	}
	for key := range values {
		delete(values, key)
	}
	for key, value := range candidate {
		values[key] = value
	}
	return nil
}

// restoreDriverInheritance preserves database targets when clearing an override selects a different effective provider.
func (s *Session) restoreDriverInheritance(values, previous map[string]string, key string) error {
	for _, definition := range resourceDefinitions(s.config, key) {
		if definition.Key != project.ResourceDatabase {
			continue
		}
		driver := ""
		for _, app := range s.databaseScopes(key) {
			current := databaseTargetInScope(values, key, app).driver
			if driver != "" && driver != current {
				return fmt.Errorf("%s inherits conflicting database drivers across App scopes; use distinct App or resource names before clearing the driver", key)
			}
			driver = current
		}
		if err := s.prepareSQLiteTarget(values, previous, key, driver); err != nil {
			return err
		}
	}
	s.retainDriverSupport(values, key, previous[key], "")
	return nil
}
