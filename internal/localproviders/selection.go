package localproviders

import (
	"fmt"
	"strings"

	"github.com/goforj/goforj/internal/devservices"
	"github.com/goforj/goforj/project"
)

// resourceSetting follows named-to-root inheritance and App overlays for one resource family.
func resourceSetting(values map[string]string, key, suffix, app string, definition project.ResourceDefinition) string {
	base := strings.TrimPrefix(key, app)
	for _, prefix := range []string{strings.TrimSuffix(base, "DRIVER"), definition.EnvironmentPrefix + "_"} {
		value := values[prefix+suffix]
		if overlay, present := values[app+prefix+suffix]; app != "" && present {
			value = overlay
		}
		if strings.TrimSpace(value) != "" {
			return value
		}
		if suffix == "DRIVER" && !definition.NamedDriversInheritRoot {
			break
		}
	}
	if suffix == "DRIVER" {
		return definition.DefaultDriver
	}
	return ""
}

// resourceScopes enumerates every live interpretation of a potentially shared environment key.
func (s *Session) resourceScopes(key string, definition project.ResourceDefinition) []string {
	scopes := map[string]bool{}
	prefix := definition.EnvironmentPrefix + "_"
	if strings.HasPrefix(key, prefix) && definition.AppliesTo(project.ProjectComponents(s.config)) {
		scopes[""] = true
	}
	for name, app := range s.config.Apps {
		scope := project.AppEnvironmentPrefix(name) + "_"
		if name != project.DefaultAppName && strings.HasPrefix(key, scope+prefix) && definition.AppliesTo(project.NormalizeConfiguredAppComponents(s.config, app.Components)) {
			scopes[scope] = true
		}
	}
	return keys(scopes)
}

// preserveExcluded pins inherited providers and database targets before parent changes can affect excluded resources.
func (s *Session) preserveExcluded(values map[string]string, excluded map[string]bool) error {
	for _, resource := range s.Resources {
		if !excluded[resource.Key] {
			continue
		}
		needsPin := false
		for _, definition := range resourceDefinitions(s.config, resource.Key) {
			for _, app := range s.resourceScopes(resource.Key, definition) {
				before := resourceSetting(s.Current, resource.Key, "DRIVER", app, definition)
				after := resourceSetting(values, resource.Key, "DRIVER", app, definition)
				needsPin = needsPin || project.CanonicalResourceDriver(definition.Key, before) != project.CanonicalResourceDriver(definition.Key, after)
				if definition.Key == project.ResourceDatabase {
					needsPin = needsPin || databaseTargetInScope(s.Current, resource.Key, app) != databaseTargetInScope(values, resource.Key, app)
				}
			}
		}
		if !needsPin {
			continue
		}
		selected := ""
		for _, definition := range resourceDefinitions(s.config, resource.Key) {
			for _, app := range s.resourceScopes(resource.Key, definition) {
				before := project.CanonicalResourceDriver(definition.Key, resourceSetting(s.Current, resource.Key, "DRIVER", app, definition))
				if selected != "" && selected != before {
					return fmt.Errorf("%s has conflicting inherited drivers; exclude its parent resources as well", resource.Key)
				}
				selected = before
				after := project.CanonicalResourceDriver(definition.Key, resourceSetting(values, resource.Key, "DRIVER", app, definition))
				if before != after {
					values[resource.Key] = before
					s.retainDriverSupport(values, resource.Key, s.Current[resource.Key], before)
				}
				if definition.Key == project.ResourceDatabase {
					for _, suffix := range []string{"DSN", "SQLITE_DATABASE", "DATABASE", "HOST", "PORT", "USERNAME", "PASSWORD"} {
						before := resourceSetting(s.Current, resource.Key, suffix, app, definition)
						after := resourceSetting(values, resource.Key, suffix, app, definition)
						if before != after {
							values[strings.TrimSuffix(resource.Key, "DRIVER")+suffix] = before
						}
					}
				}
			}
		}
		for _, definition := range resourceDefinitions(s.config, resource.Key) {
			if definition.Key != project.ResourceDatabase {
				continue
			}
			for _, app := range s.resourceScopes(resource.Key, definition) {
				if databaseTargetInScope(s.Current, resource.Key, app) != databaseTargetInScope(values, resource.Key, app) {
					return fmt.Errorf("%s has conflicting inherited database targets; exclude its parent resources as well", resource.Key)
				}
			}
		}
	}
	return s.Validate(values)
}

// serviceProfiles translates catalog service identities to the Compose profiles they can consume.
func serviceProfiles(service project.ServiceKey) []string {
	var profiles []string
	name := string(service)
	if name == "mysql" || strings.HasSuffix(name, "_mysql") {
		profiles = append(profiles, "mysql")
	}
	if name == "postgres" || strings.HasSuffix(name, "_postgres") {
		profiles = append(profiles, "postgres")
	}
	for _, definition := range devservices.Catalog() {
		for _, provider := range definition.Providers {
			if provider == service {
				profiles = append(profiles, definition.Profile)
			}
		}
	}
	return profiles
}

// usedProfiles includes unchanged named resources and other Apps before deciding a service is unnecessary.
func (s *Session) usedProfiles(values map[string]string) map[string]bool {
	profiles := map[string]bool{}
	for _, resource := range s.Resources {
		for _, definition := range resourceDefinitions(s.config, resource.Key) {
			for _, app := range s.resourceScopes(resource.Key, definition) {
				driver := resourceSetting(values, resource.Key, "DRIVER", app, definition)
				selected, ok := definition.Driver(driver)
				if !ok {
					continue
				}
				for _, profile := range serviceProfiles(selected.Service) {
					profiles[profile] = true
				}
			}
		}
	}
	return profiles
}

// localComposeProfiles removes only previously consumed profiles that no remaining resource needs.
func (s *Session) localComposeProfiles(values map[string]string) string {
	original := s.Current["COMPOSE_PROFILES"]
	before, after := s.usedProfiles(s.Current), s.usedProfiles(values)
	var profiles []string
	removed := false
	for _, value := range strings.Split(original, ",") {
		profile := strings.TrimSpace(value)
		if before[profile] && !after[profile] {
			removed = true
			continue
		}
		profiles = append(profiles, value)
	}
	if !removed {
		return original
	}
	return strings.Join(profiles, ",")
}
