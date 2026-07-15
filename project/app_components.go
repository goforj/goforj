package project

import (
	"fmt"
	"strings"
)

var appComponentKeys = []ComponentKey{
	ComponentCLI,
	ComponentMail,
	ComponentWebAPI,
	ComponentWebUI,
	ComponentAuth,
	ComponentOAuth,
	ComponentDatabaseMySQL,
	ComponentDatabasePostgres,
	ComponentDatabaseSQLite,
	ComponentScheduler,
	ComponentCache,
	ComponentEvents,
	ComponentStorage,
	ComponentJobs,
}

// AppComponentDefinitions returns catalog entries that can participate in an app graph.
func AppComponentDefinitions(_ Components) []ComponentDefinition {
	return appDefinitionsForKeys(appComponentKeys)
}

// AppWizardComponentDefinitions returns the unified App component inventory used by interactive selection.
//
// Deprecated: Use AppComponentDefinitions.
func AppWizardComponentDefinitions(available Components) []ComponentDefinition {
	return AppComponentDefinitions(available)
}

// appDefinitionsForKeys keeps app component ordering stable while ignoring catalog entries that no longer exist.
func appDefinitionsForKeys(keys []ComponentKey) []ComponentDefinition {
	definitions := make([]ComponentDefinition, 0, len(keys))
	for _, key := range keys {
		definition, ok := ComponentDefinitionByKey(key)
		if !ok {
			continue
		}
		definitions = append(definitions, definition)
	}
	return definitions
}

// AppDefaultComponents mirrors the project app-surface components while excluding project-level tooling.
func AppDefaultComponents(available Components) Components {
	var selected Components
	for _, definition := range AppComponentDefinitions(available) {
		if available.Enabled(definition.Key) {
			selected.SetEnabled(definition.Key, true)
		}
	}
	return NormalizeAppComponents(available, selected)
}

// AppComponentsFromKeys builds an app component selection from CLI or wizard keys.
func AppComponentsFromKeys(available Components, keys []ComponentKey) (Components, error) {
	var selected Components
	for _, key := range keys {
		definition, ok := ComponentDefinitionByKey(key)
		if !ok {
			return Components{}, fmt.Errorf("unknown component %q", key)
		}
		if !IsAppComponentKey(key) {
			return Components{}, fmt.Errorf("%s is project-level only and cannot be selected per app", definition.Label)
		}
		if definition.ExclusiveGroup != "" {
			clearAppExclusiveGroup(&selected, definition.ExclusiveGroup)
		}
		selected.SetEnabled(key, true)
	}
	return NormalizeAppComponents(available, selected), nil
}

// NormalizeAppComponents adds app-local dependencies that are already available at project level.
func NormalizeAppComponents(available Components, selected Components) Components {
	selected.CLI = true
	selected.DemoApp = false
	selected.Docker = false
	selected.Observability = false
	selected.Grafana = false

	if selected.WebUI {
		selected.WebAPI = true
	}
	if selected.OAuth {
		selected.Auth = true
	}
	if selected.Auth || selected.OAuth {
		selected.WebAPI = true
		if available.Mail {
			selected.Mail = true
		}
		if !selected.HasDatabase() && available.HasDatabase() {
			setAppDatabaseDriver(&selected, available.DatabaseDriver())
		}
		if !selected.HasDatabase() {
			selected.DatabaseMySQL = true
		}
	}
	if available.Metrics && (selected.WebAPI || selected.WebUI || selected.Jobs || selected.Scheduler || selected.Auth || selected.HasDatabase()) {
		selected.Metrics = true
	}
	selected.ResolveDependencies()
	selected.DemoApp = false
	selected.Docker = false
	selected.Observability = false
	selected.Grafana = false
	return selected
}

// DeselectAppComponent clears a selected app component and any selected app components that depend on it.
func DeselectAppComponent(selected *Components, key ComponentKey) {
	if selected == nil {
		return
	}
	selected.SetEnabled(key, false)
	for _, candidate := range appComponentKeys {
		if candidate == key || !selected.Enabled(candidate) || !AppComponentRequires(candidate, key) {
			continue
		}
		DeselectAppComponent(selected, candidate)
	}
}

// AppComponentRequires reports app-level dependencies, including rules that are stricter than project-level catalog dependencies.
func AppComponentRequires(component ComponentKey, required ComponentKey) bool {
	definition, ok := ComponentDefinitionByKey(component)
	if ok {
		for _, catalogRequired := range definition.Requires {
			if catalogRequired == required {
				return true
			}
		}
	}
	switch component {
	case ComponentWebUI:
		return required == ComponentWebAPI
	case ComponentAuth:
		return required == ComponentWebAPI || IsAppDatabaseComponent(required)
	case ComponentOAuth:
		return required == ComponentAuth || required == ComponentWebAPI || IsAppDatabaseComponent(required)
	default:
		return false
	}
}

// IsAppDatabaseComponent reports whether a component is one of the app-local database choices.
func IsAppDatabaseComponent(key ComponentKey) bool {
	switch key {
	case ComponentDatabaseMySQL, ComponentDatabasePostgres, ComponentDatabaseSQLite:
		return true
	default:
		return false
	}
}

// ProjectComponents derives the shared render capability envelope without changing the default App selection.
func ProjectComponents(config *Config) Components {
	if config == nil {
		return Components{}
	}
	available := config.Render.Components.WithResolvedDependencies()
	envelope := available
	for _, appConfig := range config.Apps {
		selected := NormalizeConfiguredAppComponents(config, appConfig.Components)
		envelope = PromoteAppComponents(envelope, selected)
	}
	return envelope
}

// NormalizeConfiguredAppComponents resolves one App against the default App so sibling selections cannot change its implicit dependencies.
func NormalizeConfiguredAppComponents(config *Config, selected Components) Components {
	if config == nil {
		return NormalizeAppComponents(Components{}, selected)
	}
	return NormalizeAppComponents(config.Render.Components.WithResolvedDependencies(), selected)
}

// PromoteAppComponents adds selected App capabilities to an in-memory project envelope.
func PromoteAppComponents(available Components, selected Components) Components {
	promoted := available
	for _, key := range appComponentKeys {
		if selected.Enabled(key) {
			promoted.SetEnabled(key, true)
		}
	}
	if selected.Auth || selected.OAuth {
		promoted.Mail = true
	}
	if selected.Metrics {
		promoted.Metrics = true
	}
	promoted.ResolveDependencies()
	promoted.DemoApp = available.DemoApp
	promoted.Docker = available.Docker
	promoted.Observability = available.Observability
	promoted.Grafana = available.Grafana
	return promoted
}

// ParseComponentKeys parses comma-separated component names using the canonical catalog keys.
func ParseComponentKeys(raw string) ([]ComponentKey, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	keys := make([]ComponentKey, 0, len(parts))
	for _, part := range parts {
		key, err := ParseComponentKey(part)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// ParseComponentKey accepts common CLI spelling while resolving to canonical component keys.
func ParseComponentKey(raw string) (ComponentKey, error) {
	token := strings.ToLower(strings.TrimSpace(raw))
	token = strings.ReplaceAll(token, "-", "_")
	token = strings.ReplaceAll(token, " ", "_")
	for _, definition := range ComponentCatalog() {
		key := string(definition.Key)
		label := strings.ToLower(strings.ReplaceAll(definition.Label, " ", "_"))
		label = strings.ReplaceAll(label, "-", "_")
		label = strings.ReplaceAll(label, "(", "")
		label = strings.ReplaceAll(label, ")", "")
		if token == key || token == label {
			return definition.Key, nil
		}
	}
	return "", fmt.Errorf("unknown component %q", raw)
}

// IsAppComponentKey reports whether a component can be selected at app scope.
func IsAppComponentKey(key ComponentKey) bool {
	for _, candidate := range appComponentKeys {
		if candidate == key {
			return true
		}
	}
	return false
}

// setAppDatabaseDriver applies the app-local database exclusivity rule in one place.
func setAppDatabaseDriver(components *Components, driver string) {
	components.DatabaseMySQL = false
	components.DatabasePostgres = false
	components.DatabaseSQLite = false
	switch driver {
	case "mysql":
		components.DatabaseMySQL = true
	case "postgres":
		components.DatabasePostgres = true
	case "sqlite":
		components.DatabaseSQLite = true
	}
}

// clearAppExclusiveGroup removes visible peers before enabling a new exclusive choice.
func clearAppExclusiveGroup(components *Components, group string) {
	for _, key := range appComponentKeys {
		definition, ok := ComponentDefinitionByKey(key)
		if !ok || definition.ExclusiveGroup != group {
			continue
		}
		components.SetEnabled(key, false)
	}
}
