package stacks

import (
	"fmt"
	"slices"
	"strings"

	"github.com/goforj/goforj/internal/envfile"
	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/project"
	"github.com/goforj/str/v2"
)

// Resource identifies an editable root or named resource within an App.
type Resource struct {
	Key, SupportedKey, Label string
	Definition               project.ResourceDefinition
}

// ResourcesFor includes resources declared by the selected values while retaining the project's active and example accessor inventory.
func (s *Session) ResourcesFor(values map[string]string) []Resource {
	byKey := map[string]Resource{}
	for _, resource := range s.Resources {
		byKey[resource.Key] = resource
	}
	for _, resource := range resources(s.Root, s.config, values) {
		byKey[resource.Key] = resource
	}
	result := make([]Resource, 0, len(byKey))
	for _, key := range keys(byKey) {
		result = append(result, byKey[key])
	}
	return result
}

// resources discovers enabled roots and concrete named overrides from project-owned configuration.
func resources(root string, config *project.Config, values map[string]string) []Resource {
	byKey := map[string]Resource{}
	components := project.ProjectComponents(config)
	names := generate.ResourceNames(root, values)
	for _, definition := range project.ResourceCatalog() {
		if !definition.AppliesTo(components) {
			continue
		}
		key := definition.EnvironmentKey("DRIVER")
		byKey[key] = Resource{Key: key, SupportedKey: definition.EnvironmentKey("SUPPORTED_DRIVERS"), Label: definition.Label, Definition: definition}
		for _, name := range names[definition.Key] {
			if name == "default" || name == "root" {
				continue
			}
			key := definition.EnvironmentPrefix + "_" + str.Of(name).Snake().ToUpper().String() + "_DRIVER"
			byKey[key] = Resource{Key: key, SupportedKey: definition.EnvironmentKey("SUPPORTED_DRIVERS"), Label: key, Definition: definition}
		}
		for app, appConfig := range config.Apps {
			if app == project.DefaultAppName || !definition.AppliesTo(project.NormalizeConfiguredAppComponents(config, appConfig.Components)) {
				continue
			}
			prefix := project.AppEnvironmentPrefix(app) + "_"
			key := prefix + definition.EnvironmentKey("DRIVER")
			byKey[key] = Resource{Key: key, SupportedKey: definition.EnvironmentKey("SUPPORTED_DRIVERS"), Label: key, Definition: definition}
			for _, name := range names[definition.Key] {
				if name == "default" || name == "root" {
					continue
				}
				key := prefix + definition.EnvironmentPrefix + "_" + str.Of(name).Snake().ToUpper().String() + "_DRIVER"
				byKey[key] = Resource{Key: key, SupportedKey: definition.EnvironmentKey("SUPPORTED_DRIVERS"), Label: key, Definition: definition}
			}
		}
		for _, named := range definition.NamedResources {
			if named.RequiredComponent != "" && !components.Enabled(named.RequiredComponent) {
				continue
			}
			byKey[named.EnvironmentKey] = Resource{Key: named.EnvironmentKey, SupportedKey: definition.EnvironmentKey("SUPPORTED_DRIVERS"), Label: named.Label, Definition: definition}
		}
	}
	for _, key := range keys(values) {
		if !strings.HasSuffix(key, "_DRIVER") {
			continue
		}
		for _, definition := range project.ResourceCatalog() {
			base := resourceKey(config, key)
			if !definition.AppliesTo(components) || !strings.HasPrefix(base, definition.EnvironmentPrefix+"_") {
				continue
			}
			byKey[key] = Resource{Key: key, SupportedKey: definition.EnvironmentKey("SUPPORTED_DRIVERS"), Label: key, Definition: definition}
		}
	}
	out := make([]Resource, 0, len(byKey))
	for _, key := range keys(byKey) {
		out = append(out, byKey[key])
	}
	return out
}

// resourceKey removes only configured App prefixes, preventing unrelated application keys from being claimed.
func resourceKey(config *project.Config, key string) string {
	longest := ""
	for name := range config.Apps {
		if name == project.DefaultAppName {
			continue
		}
		prefix := project.AppEnvironmentPrefix(name) + "_"
		if len(prefix) > len(longest) && strings.HasPrefix(key, prefix) {
			candidate := strings.TrimPrefix(key, prefix)
			for _, resource := range []string{"DB_", "CACHE_", "QUEUE_", "EVENTS_", "STORAGE_", "MAIL_", "REDIS_"} {
				if strings.HasPrefix(candidate, resource) {
					longest = prefix
					break
				}
			}
		}
	}
	if longest != "" {
		return strings.TrimPrefix(key, longest)
	}
	return key
}

// managedKey limits stack ownership to resource settings and Compose selection.
func managedKey(config *project.Config, key string) bool {
	if key == "COMPOSE_PROFILES" || key == SQLiteDSNsKey {
		return true
	}
	base := resourceKey(config, key)
	for _, prefix := range []string{"DB_", "CACHE_", "QUEUE_", "EVENTS_", "STORAGE_", "MAIL_", "REDIS_"} {
		if strings.HasPrefix(base, prefix) {
			return true
		}
	}
	return false
}

// managedValues excludes application identity and unrelated secrets from stack snapshots.
func managedValues(config *project.Config, values map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range values {
		if managedKey(config, key) {
			out[key] = value
		}
	}
	return out
}

// SetDriver selects a driver and retains the compiled support needed to switch back later.
func (s *Session) SetDriver(values map[string]string, resource Resource, driver string) error {
	return s.setDriver(values, resource, driver, values)
}

// setDriver uses the pre-edit configuration when a preset changes several resources with shared inheritance.
func (s *Session) setDriver(values map[string]string, resource Resource, driver string, previous map[string]string) error {
	driver = project.CanonicalResourceDriver(resource.Definition.Key, driver)
	if _, ok := resource.Definition.Driver(driver); !ok {
		return fmt.Errorf("%s has no driver %q", resource.Key, driver)
	}
	old := values[resource.Key]
	if resource.Definition.Key == project.ResourceDatabase {
		if err := s.prepareSQLiteTarget(values, previous, resource.Key, driver); err != nil {
			return err
		}
	}
	supported := append(strings.Split(values[resource.SupportedKey], ","), old, driver)
	for _, sibling := range resources(s.Root, s.config, values) {
		if sibling.SupportedKey == resource.SupportedKey {
			supported = append(supported, values[sibling.Key])
		}
	}
	values[resource.Key] = driver
	var normalized []string
	seen := map[string]bool{}
	for _, name := range supported {
		name = project.CanonicalResourceDriver(resource.Definition.Key, name)
		if name == "" {
			continue
		}
		if !seen[name] {
			normalized = append(normalized, name)
			seen[name] = true
		}
	}
	values[resource.SupportedKey] = strings.Join(normalized, ",")
	return nil
}

// databaseValue follows the runtime App overlay and named-to-root fallback without reading ambient credentials.
func (s *Session) databaseValue(values map[string]string, key, suffix string) string {
	_, value := s.databaseSetting(values, key, suffix)
	return value
}

// databaseSetting retains the source key so shared SQLite paths can be published without copying private service settings.
func (s *Session) databaseSetting(values map[string]string, key, suffix string) (string, string) {
	base := resourceKey(s.config, key)
	app := strings.TrimSuffix(key, base)
	for _, prefix := range []string{strings.TrimSuffix(base, "DRIVER"), "DB_"} {
		source := prefix + suffix
		value := values[source]
		if overlay, present := values[app+prefix+suffix]; app != "" && present {
			source = app + prefix + suffix
			value = overlay
		}
		if strings.TrimSpace(value) != "" {
			return source, value
		}
	}
	return "", ""
}

// prepareSQLiteTarget retains existing SQLite files, including legacy and inherited paths, before a driver edit can change their interpretation.
func (s *Session) prepareSQLiteTarget(values, previous map[string]string, key, driver string) error {
	retained, err := sqliteDSNs(values)
	if err != nil {
		return err
	}
	prefix := strings.TrimSuffix(key, "DRIVER")
	old := project.CanonicalResourceDriver(project.ResourceDatabase, s.databaseValue(previous, key, "DRIVER"))
	if old == "" {
		old = "sqlite"
	}
	dsn := s.databaseValue(previous, key, "DSN")
	if old == "sqlite" {
		if dsn != "" {
			retained[key] = dsn
			if driver == "sqlite" {
				values[prefix+"DSN"] = dsn
			}
		} else {
			delete(retained, key)
		}
		storeSQLiteDSNs(values, retained)
	}
	if old == "sqlite" && dsn == "" && strings.TrimSpace(values[prefix+"SQLITE_DATABASE"]) == "" {
		target := s.databaseValue(previous, key, "SQLITE_DATABASE")
		if target == "" {
			target = s.databaseValue(previous, key, "DATABASE")
		}
		if target == "" && s.databaseValue(previous, key, "DSN") == "" {
			name := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(resourceKey(s.config, key), "DB_"), "_DRIVER"))
			if name == "driver" {
				name = "app"
			}
			target = "./_data/sqlite/" + name + ".db"
		}
		if target != "" {
			values[prefix+"SQLITE_DATABASE"] = target
		}
	}
	if driver == "sqlite" && old != "sqlite" {
		if retained[key] != "" {
			values[prefix+"DSN"] = retained[key]
			return nil
		}
		values[prefix+"DSN"] = ""
		if strings.TrimSpace(values[prefix+"SQLITE_DATABASE"]) == "" {
			values[prefix+"SQLITE_DATABASE"] = "./_data/stacks/portable/" + strings.ToLower(strings.TrimSuffix(prefix, "_")) + ".db"
		}
		if s.databaseValue(values, key, "DSN") != "" {
			// An inherited SQLite DSN must not override this newly selected resource's distinct path.
			values[prefix+"DSN"] = values[prefix+"SQLITE_DATABASE"]
		}
	}
	return nil
}

// Portable suggests local providers for every discovered resource without enabling disabled components.
func (s *Session) Portable() (map[string]string, error) {
	values := clone(s.Current)
	ordered := slices.Clone(s.Resources)
	// Parent settings settle first so converted children only pin DSNs that remain inherited in the final preset.
	slices.SortStableFunc(ordered, func(a, b Resource) int {
		return s.portableOrder(a) - s.portableOrder(b)
	})
	for _, resource := range ordered {
		if err := s.setDriver(values, resource, resource.Definition.DefaultDriver, s.Current); err != nil {
			return nil, err
		}
	}
	values["COMPOSE_PROFILES"] = ""
	return values, nil
}

// portableOrder follows root, named, and App overlay dependencies when a preset updates shared database settings.
func (s *Session) portableOrder(resource Resource) int {
	if resource.Definition.Key != project.ResourceDatabase {
		return 4
	}
	base := resourceKey(s.config, resource.Key)
	order := 0
	if base != resource.Key {
		order += 2
	}
	if base != "DB_DRIVER" {
		order++
	}
	return order
}

// Validate rejects unknown drivers and settings outside the project resource boundary.
func (s *Session) Validate(values map[string]string) error {
	if _, err := sqliteDSNs(values); err != nil {
		return err
	}
	for key := range values {
		if !envfile.IsValidKey(key) || !managedKey(s.config, key) {
			return fmt.Errorf("%s is not a stack resource setting", key)
		}
	}
	for key, value := range values {
		base := resourceKey(s.config, key)
		if !strings.HasSuffix(base, "_DRIVER") && !strings.HasSuffix(base, "_SUPPORTED_DRIVERS") {
			continue
		}
		for _, definition := range project.ResourceCatalog() {
			if !strings.HasPrefix(base, definition.EnvironmentPrefix+"_") {
				continue
			}
			for _, driver := range strings.Split(value, ",") {
				driver = strings.TrimSpace(driver)
				if driver == "" {
					continue
				}
				if _, ok := definition.Driver(strings.TrimSpace(driver)); !ok {
					return fmt.Errorf("%s selects unsupported driver", key)
				}
			}
		}
	}
	for _, resource := range resources(s.Root, s.config, values) {
		driver := project.CanonicalResourceDriver(resource.Definition.Key, values[resource.Key])
		if driver == "" {
			continue
		}
		if _, ok := resource.Definition.Driver(driver); !ok {
			return fmt.Errorf("%s selects unsupported driver %q", resource.Key, driver)
		}
		supported := strings.TrimSpace(values[resource.SupportedKey])
		if supported == "" {
			continue
		}
		found := slices.Contains(generate.BaselineDrivers(resource.Definition.Key), driver)
		count := 0
		for _, name := range strings.Split(supported, ",") {
			name = project.CanonicalResourceDriver(resource.Definition.Key, name)
			if name == "" {
				continue
			}
			if _, ok := resource.Definition.Driver(name); !ok {
				return fmt.Errorf("%s contains unsupported driver %q", resource.SupportedKey, name)
			}
			count++
			if name == driver {
				found = true
			}
		}
		if count == 0 {
			if resource.Definition.Key == project.ResourceDatabase {
				return fmt.Errorf("%s must include at least one driver", resource.SupportedKey)
			}
			continue
		}
		if !found {
			return fmt.Errorf("%s must include %s for %s", resource.SupportedKey, driver, resource.Key)
		}
	}
	return nil
}

// shareable retains explicit provider choices and SQLite paths selected through runtime inheritance while keeping service connection settings private.
func (s *Session) shareable(values map[string]string) map[string]string {
	result := map[string]string{}
	databases := map[string]bool{}
	for key, value := range values {
		if strings.HasSuffix(key, "_DRIVER") || strings.HasSuffix(key, "_SUPPORTED_DRIVERS") || key == "COMPOSE_PROFILES" {
			result[key] = value
		}
		if strings.HasSuffix(key, "_DRIVER") && strings.HasPrefix(resourceKey(s.config, key), "DB_") {
			databases[key] = true
		}
	}
	for _, resource := range resources(s.Root, s.config, values) {
		if resource.Definition.Key == project.ResourceDatabase {
			databases[resource.Key] = true
		}
	}
	for key := range databases {
		driver := project.CanonicalResourceDriver(project.ResourceDatabase, s.databaseValue(values, key, "DRIVER"))
		if driver != "" && driver != "sqlite" {
			continue
		}
		prefix := strings.TrimSuffix(key, "DRIVER")
		source, path := s.databaseSetting(values, key, "SQLITE_DATABASE")
		if path != "" {
			result[source] = path
		}
		for _, suffix := range []string{"DATABASE", "SQLITE_DATABASE"} {
			// A dedicated SQLite path leaves DATABASE available for private service connection settings.
			if suffix == "DATABASE" && path != "" {
				continue
			}
			if value, ok := values[prefix+suffix]; ok {
				result[prefix+suffix] = value
			}
		}
		if value, ok := values[prefix+"DSN"]; ok && value == "" {
			result[prefix+"DSN"] = ""
		}
	}
	return result
}
