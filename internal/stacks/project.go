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
		out = append(out, sharedResourceChoices(config, byKey[key]))
	}
	return out
}

// resourceKey prefers participating resource scopes so an App without cache cannot claim a root database named cache.
func resourceKey(config *project.Config, key string) string {
	longest := ""
	participating := ""
	for name, app := range config.Apps {
		if name == project.DefaultAppName {
			continue
		}
		prefix := project.AppEnvironmentPrefix(name) + "_"
		if strings.HasPrefix(key, prefix) {
			candidate := strings.TrimPrefix(key, prefix)
			if rootResourceKey(candidate) {
				if len(prefix) > len(longest) {
					longest = prefix
				}
				if len(prefix) > len(participating) && resourceAppliesTo(candidate, project.NormalizeConfiguredAppComponents(config, app.Components)) {
					participating = prefix
				}
			}
		}
	}
	if participating != "" {
		return strings.TrimPrefix(key, participating)
	}
	if resourceAppliesTo(key, project.ProjectComponents(config)) {
		return key
	}
	if longest != "" {
		return strings.TrimPrefix(key, longest)
	}
	return key
}

// resourceAppliesTo distinguishes live resource scopes while allowing definitions for future components to retain their fallback interpretation.
func resourceAppliesTo(key string, components project.Components) bool {
	for _, definition := range project.ResourceCatalog() {
		if strings.HasPrefix(key, definition.EnvironmentPrefix+"_") {
			return definition.AppliesTo(components)
		}
	}
	return false
}

// managedKey limits stack ownership to resource settings and Compose selection.
func managedKey(config *project.Config, key string) bool {
	if key == "COMPOSE_PROFILES" || key == SQLiteDSNsKey {
		return true
	}
	base := resourceKey(config, key)
	return rootResourceKey(base)
}

// rootResourceKey recognizes settings that the default App can consume even when their names also match a configured App prefix.
func rootResourceKey(key string) bool {
	for _, prefix := range []string{"DB_", "CACHE_", "QUEUE_", "EVENTS_", "STORAGE_", "MAIL_", "REDIS_"} {
		if strings.HasPrefix(key, prefix) {
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
		return fmt.Errorf("%s selects an unsupported driver; shared settings require a driver supported by every resource", resource.Key)
	}
	definitions := resourceDefinitions(s.config, resource.Key)
	if !supportsResourceDriver(definitions, driver) {
		return fmt.Errorf("%s selects an unsupported driver for a shared resource; use distinct App or resource names for independent drivers", resource.Key)
	}
	old := values[resource.Key]
	for _, definition := range definitions {
		if definition.Key == project.ResourceDatabase {
			if err := s.prepareSQLiteTarget(values, previous, resource.Key, driver); err != nil {
				return err
			}
		}
	}
	s.retainDriverSupport(values, resource.Key, old, driver)
	values[resource.Key] = driver
	return nil
}

// retainDriverSupport keeps previous selections and unchanged siblings compiled for every consumer of a shared key.
func (s *Session) retainDriverSupport(values map[string]string, key, old, driver string) {
	inventory := resources(s.Root, s.config, values)
	for _, definition := range resourceDefinitions(s.config, key) {
		supportedKey := definition.EnvironmentKey("SUPPORTED_DRIVERS")
		supported := strings.Split(values[supportedKey], ",")
		selections := []string{old, driver}
		for _, sibling := range inventory {
			for _, consumer := range resourceDefinitions(s.config, sibling.Key) {
				if consumer.Key == definition.Key {
					selections = append(selections, values[sibling.Key])
				}
			}
		}
		for _, selected := range selections {
			if _, ok := definition.Driver(selected); ok {
				supported = append(supported, selected)
			}
		}
		var normalized []string
		seen := map[string]bool{}
		for _, name := range supported {
			name = project.CanonicalResourceDriver(definition.Key, name)
			if name != "" && !seen[name] {
				normalized = append(normalized, name)
				seen[name] = true
			}
		}
		if len(normalized) > 0 {
			values[supportedKey] = strings.Join(normalized, ",")
		}
	}
}

// databaseValue follows the runtime App overlay and named-to-root fallback without reading ambient credentials.
func (s *Session) databaseValue(values map[string]string, key, suffix string) string {
	_, value := s.databaseSetting(values, key, suffix)
	return value
}

// databaseSetting retains the source key so shared SQLite paths can be published without copying private service settings.
func (s *Session) databaseSetting(values map[string]string, key, suffix string) (string, string) {
	return databaseSettingInScope(values, key, suffix, s.databaseScope(key))
}

// databaseScope keeps database inheritance independent of another resource family's display precedence.
func (s *Session) databaseScope(key string) string {
	base := resourceKey(s.config, key)
	if strings.HasPrefix(base, "DB_") {
		return strings.TrimSuffix(key, base)
	}
	for _, app := range s.databaseScopes(key) {
		return app
	}
	return ""
}

// databaseSettingInScope follows one App's exact runtime overlay so overlapping App names can retain independent inheritance.
func databaseSettingInScope(values map[string]string, key, suffix, app string) (string, string) {
	base := strings.TrimPrefix(key, app)
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

// databaseScopes includes every participating database interpretation of a key, retaining the conventional fallback for future components.
func (s *Session) databaseScopes(key string) []string {
	scopes := map[string]bool{}
	if base := resourceKey(s.config, key); strings.HasPrefix(base, "DB_") {
		scopes[strings.TrimSuffix(key, base)] = true
	}
	if strings.HasPrefix(key, "DB_") && project.ProjectComponents(s.config).HasDatabase() {
		scopes[""] = true
	}
	for name, app := range s.config.Apps {
		if name == project.DefaultAppName {
			continue
		}
		prefix := project.AppEnvironmentPrefix(name) + "_"
		if strings.HasPrefix(key, prefix+"DB_") && project.NormalizeConfiguredAppComponents(s.config, app.Components).HasDatabase() {
			scopes[prefix] = true
		}
	}
	return keys(scopes)
}

// sqlitePathInScope mirrors SQLite's named-to-root path fallback without conflating overlapping App prefixes.
func sqlitePathInScope(values map[string]string, key, app string) string {
	for _, suffix := range []string{"SQLITE_DATABASE", "DATABASE"} {
		if _, path := databaseSettingInScope(values, key, suffix, app); path != "" {
			return path
		}
	}
	base := strings.TrimPrefix(key, app)
	name := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(base, "DB_"), "_DRIVER"))
	if name == "driver" {
		name = "app"
	}
	return "./_data/sqlite/" + name + ".db"
}

// databaseTarget identifies the connection selected before a driver edit without exposing private values in diagnostics.
type databaseTarget struct {
	driver, dsn, path, host, port, database, username string
}

// databaseTargetInScope compares effective targets so identical inherited connections can still transition together.
func databaseTargetInScope(values map[string]string, key, app string) databaseTarget {
	_, selected := databaseSettingInScope(values, key, "DRIVER", app)
	target := databaseTarget{driver: project.CanonicalResourceDriver(project.ResourceDatabase, selected)}
	if target.driver == "" {
		target.driver = "sqlite"
	}
	_, target.dsn = databaseSettingInScope(values, key, "DSN", app)
	if target.dsn != "" {
		return target
	}
	if target.driver == "sqlite" {
		target.path = sqlitePathInScope(values, key, app)
		return target
	}
	_, target.host = databaseSettingInScope(values, key, "HOST", app)
	_, target.port = databaseSettingInScope(values, key, "PORT", app)
	_, target.database = databaseSettingInScope(values, key, "DATABASE", app)
	_, target.username = databaseSettingInScope(values, key, "USERNAME", app)
	return target
}

// declaredDatabaseScopes excludes synthetic named interpretations until the project actually declares their accessors.
func (s *Session) declaredDatabaseScopes(previous map[string]string, key string) []string {
	names := append(slices.Clone(s.databaseNames), generate.ResourceNames(s.Root, previous)[project.ResourceDatabase]...)
	var scopes []string
	for _, app := range s.databaseScopes(key) {
		base := strings.TrimPrefix(key, app)
		name := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(base, "DB_"), "_DRIVER"))
		if base != "DB_DRIVER" && !slices.Contains(names, name) {
			// App overlays synthesized by the editor must not invent additional source database names during the same preset.
			continue
		}
		scopes = append(scopes, app)
	}
	return scopes
}

// preserveSQLiteScopes avoids pinning unchanged SQLite inheritance and rejects transitions that cannot preserve overlapping targets.
func (s *Session) preserveSQLiteScopes(values, previous map[string]string, key, driver string) (bool, error) {
	targets := map[databaseTarget]bool{}
	changing, unchanged := false, true
	legacyPath := false
	for _, app := range s.declaredDatabaseScopes(previous, key) {
		target := databaseTargetInScope(previous, key, app)
		targets[target] = true
		changing = changing || target.driver != driver
		unchanged = unchanged && databaseTargetInScope(values, key, app) == target
		_, dedicated := databaseSettingInScope(values, key, "SQLITE_DATABASE", app)
		_, generic := databaseSettingInScope(values, key, "DATABASE", app)
		legacyPath = legacyPath || target.driver == "sqlite" && target.dsn == "" && dedicated == "" && generic != ""
	}
	if len(targets) > 1 && (changing || !unchanged) {
		return false, fmt.Errorf("%s has conflicting database targets across App scopes; use distinct App or resource names before changing drivers", key)
	}
	// Legacy DATABASE paths still need their dedicated setting before that generic key can be reused for a service database.
	return len(targets) > 1 || len(targets) > 0 && driver == "sqlite" && !changing && unchanged && !legacyPath, nil
}

// prepareSQLiteTarget retains existing SQLite files, including legacy and inherited paths, before a driver edit can change their interpretation.
func (s *Session) prepareSQLiteTarget(values, previous map[string]string, key, driver string) error {
	retained, err := sqliteDSNs(values)
	if err != nil {
		return err
	}
	preserve, err := s.preserveSQLiteScopes(values, previous, key, driver)
	if err != nil || preserve {
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
		app := s.databaseScope(key)
		values[prefix+"SQLITE_DATABASE"] = sqlitePathInScope(previous, key, app)
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
		if resource.Definition.DefaultDriver == "" {
			return nil, fmt.Errorf("%s has no portable driver supported by every shared resource; use distinct App or resource names", resource.Key)
		}
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
		if !strings.HasSuffix(key, "_DRIVER") && !strings.HasSuffix(key, "_SUPPORTED_DRIVERS") {
			continue
		}
		for _, definition := range resourceDefinitions(s.config, key) {
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
		for _, definition := range resourceDefinitions(s.config, resource.Key) {
			if err := validateResourceContract(values, resource.Key, definition); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateResourceContract checks compiled support for each consumer of a shared driver setting.
func validateResourceContract(values map[string]string, key string, definition project.ResourceDefinition) error {
	driver := project.CanonicalResourceDriver(definition.Key, values[key])
	if driver == "" {
		return nil
	}
	if _, ok := definition.Driver(driver); !ok {
		return fmt.Errorf("%s selects unsupported driver", key)
	}
	supportedKey := definition.EnvironmentKey("SUPPORTED_DRIVERS")
	supported := strings.TrimSpace(values[supportedKey])
	if supported == "" {
		return nil
	}
	found := slices.Contains(generate.BaselineDrivers(definition.Key), driver)
	count := 0
	for _, name := range strings.Split(supported, ",") {
		name = project.CanonicalResourceDriver(definition.Key, name)
		if name == "" {
			continue
		}
		if _, ok := definition.Driver(name); !ok {
			return fmt.Errorf("%s contains unsupported driver", supportedKey)
		}
		count++
		if name == driver {
			found = true
		}
	}
	if count == 0 {
		if definition.Key == project.ResourceDatabase {
			return fmt.Errorf("%s must include at least one driver", supportedKey)
		}
		return nil
	}
	if !found {
		return fmt.Errorf("%s must include %s for %s", supportedKey, driver, key)
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
		if strings.HasSuffix(key, "_DRIVER") && len(s.databaseScopes(key)) > 0 {
			databases[key] = true
		}
	}
	for _, resource := range resources(s.Root, s.config, values) {
		if len(s.databaseScopes(resource.Key)) > 0 {
			databases[resource.Key] = true
		}
	}
	privateNames := map[string]bool{}
	for key := range databases {
		for _, app := range s.databaseScopes(key) {
			_, selected := databaseSettingInScope(values, key, "DRIVER", app)
			driver := project.CanonicalResourceDriver(project.ResourceDatabase, selected)
			prefix := strings.TrimSuffix(key, "DRIVER")
			if driver != "" && driver != "sqlite" {
				privateNames[prefix+"DATABASE"] = true
				if source, _ := databaseSettingInScope(values, key, "DATABASE", app); source != "" {
					privateNames[source] = true
				}
				continue
			}
			source, path := databaseSettingInScope(values, key, "SQLITE_DATABASE", app)
			if path != "" {
				result[source] = path
			}
			_, dsn := databaseSettingInScope(values, key, "DSN", app)
			if path != "" || dsn != "" {
				// A generic name bypassed by an overlapping SQLite scope stays private; explicit SQLITE_DATABASE paths remain shareable.
				privateNames[prefix+"DATABASE"] = true
			}
			for _, suffix := range []string{"DATABASE", "SQLITE_DATABASE"} {
				if value, ok := values[prefix+suffix]; ok {
					result[prefix+suffix] = value
				}
			}
			if value, ok := values[prefix+"DSN"]; ok && value == "" {
				result[prefix+"DSN"] = ""
			}
		}
	}
	for key := range privateNames {
		delete(result, key)
	}
	return result
}
