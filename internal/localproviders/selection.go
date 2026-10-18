package localproviders

import (
	"fmt"
	"slices"
	"strings"

	"github.com/goforj/goforj/internal/devservices"
	"github.com/goforj/goforj/project"
)

// resourceSetting follows named-to-root inheritance and App overlays for one resource family.
func resourceSetting(values map[string]string, key, suffix, app string, definition project.ResourceDefinition) string {
	if definition.Key == project.ResourceDatabase {
		_, value := databaseSettingInScope(values, key, suffix, app)
		if suffix == "DRIVER" && value == "" {
			return definition.DefaultDriver
		}
		return value
	}
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

// Drivers plans explicit selections while preserving unselected resources and the original restore point.
func (s *Session) Drivers(selections map[string]string) (map[string]string, error) {
	original := s.Current
	if s.HasPrevious() {
		var err error
		original, err = s.Previous()
		if err != nil {
			return nil, err
		}
	}
	values := clone(s.Current)
	ordered := slices.Clone(s.Resources)
	slices.SortStableFunc(ordered, func(a, b Resource) int { return s.portableOrder(a) - s.portableOrder(b) })
	excluded := map[string]bool{}
	known := map[string]bool{}
	for _, resource := range ordered {
		known[resource.Key] = true
		driver, selected := selections[resource.Key]
		if !selected {
			excluded[resource.Key] = true
			continue
		}
		driver = project.CanonicalResourceDriver(resource.Definition.Key, driver)
		if err := s.setDriver(values, resource, driver, s.Current); err != nil {
			return nil, err
		}
		if s.databaseResource(resource.Key) && driver != databaseDriver(s.Current, resource.Key, s.databaseScope(resource.Key)) {
			prefix := strings.TrimSuffix(resource.Key, "DRIVER")
			// A SQLite DSN cannot be reused as a service DSN. Recover the original connection only for its original driver.
			if driver != "sqlite" {
				values[prefix+"DSN"] = ""
			}
			if driver == databaseDriver(original, resource.Key, s.databaseScope(resource.Key)) {
				for key := range changedKeys(s.state.Previous, s.state.Applied) {
					if key == prefix+"DSN" || key == prefix+"SQLITE_DATABASE" {
						if value, present := original[key]; present {
							values[key] = value
						} else {
							delete(values, key)
						}
					}
				}
				if err := s.recoverDatabaseDSN(values, original, resource.Key, driver); err != nil {
					return nil, err
				}
			}
		}
	}
	for key := range selections {
		if !known[key] {
			return nil, fmt.Errorf("%s is not a selectable resource", key)
		}
	}
	if err := s.preserveExcluded(values, excluded); err != nil {
		return nil, err
	}
	if err := s.validateSelectedDatabaseDSNs(values, selections); err != nil {
		return nil, err
	}
	profiles := s.localComposeProfiles(values)
	before, after := s.usedProfiles(s.Current), s.usedProfiles(values)
	for _, profile := range keys(after) {
		if !before[profile] && !slices.Contains(strings.Split(profiles, ","), profile) {
			if profiles != "" {
				profiles += ","
			}
			profiles += profile
		}
	}
	if _, present := s.Current["COMPOSE_PROFILES"]; present || profiles != "" {
		values["COMPOSE_PROFILES"] = profiles
	}
	return values, s.Validate(values)
}

// databaseDriver compares the effective provider rather than its accepted configuration alias.
func databaseDriver(values map[string]string, key, app string) string {
	return databaseTargetInScope(values, key, app).driver
}

// recoverDatabaseDSN pins an original inherited connection when its parent remains on a different provider.
func (s *Session) recoverDatabaseDSN(values, original map[string]string, key, driver string) error {
	scopes := s.declaredDatabaseScopes(original, key)
	if len(scopes) == 0 {
		scopes = []string{s.databaseScope(key)}
	}
	desired := map[string]bool{}
	needsPin := false
	for _, app := range scopes {
		if databaseDriver(original, key, app) != driver {
			return fmt.Errorf("%s has conflicting original database drivers across App scopes; use distinct resource names before changing drivers", key)
		}
		_, dsn := databaseSettingInScope(original, key, "DSN", app)
		desired[dsn] = true
		_, current := databaseSettingInScope(values, key, "DSN", app)
		needsPin = needsPin || dsn != "" && current != dsn
	}
	if !needsPin {
		return nil
	}
	if len(desired) != 1 {
		return fmt.Errorf("%s has conflicting original database DSNs across App scopes; use distinct resource names before changing drivers", key)
	}
	for dsn := range desired {
		values[strings.TrimSuffix(key, "DRIVER")+"DSN"] = dsn
	}
	return nil
}

// validateSelectedDatabaseDSNs rejects inherited connection strings belonging to a different provider before publication.
func (s *Session) validateSelectedDatabaseDSNs(values, selections map[string]string) error {
	for _, resource := range s.Resources {
		selected, ok := selections[resource.Key]
		if !ok || !s.databaseResource(resource.Key) {
			continue
		}
		driver := project.CanonicalResourceDriver(project.ResourceDatabase, selected)
		for _, app := range s.declaredDatabaseScopes(values, resource.Key) {
			if driver == databaseDriver(s.Current, resource.Key, app) {
				continue
			}
			source, dsn := databaseSettingInScope(values, resource.Key, "DSN", app)
			if strings.TrimSpace(dsn) == "" {
				continue
			}
			owner := strings.TrimSuffix(source, "DSN") + "DRIVER"
			if driver != databaseDriver(values, owner, app) {
				return fmt.Errorf("%s would inherit a DSN for a different database driver via %s; switch its parent too, or configure its driver and dedicated DSN together in .env", resource.Key, source)
			}
		}
	}
	return nil
}

// databaseResource includes database consumers of a key shared with another resource family.
func (s *Session) databaseResource(key string) bool {
	for _, definition := range resourceDefinitions(s.config, key) {
		if definition.Key == project.ResourceDatabase {
			return true
		}
	}
	return false
}
