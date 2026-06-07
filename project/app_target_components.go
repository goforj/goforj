package project

import (
	"fmt"
	"strings"
)

var targetComponentKeys = []ComponentKey{
	ComponentWebAPI,
	ComponentWebUI,
	ComponentAuth,
	ComponentOAuth,
	ComponentDatabaseMySQL,
	ComponentDatabasePostgres,
	ComponentDatabaseSQLite,
	ComponentScheduler,
	ComponentJobs,
}

var targetWizardComponentKeys = []ComponentKey{
	ComponentWebAPI,
	ComponentWebUI,
	ComponentAuth,
	ComponentOAuth,
	ComponentDatabaseMySQL,
	ComponentDatabasePostgres,
	ComponentDatabaseSQLite,
	ComponentScheduler,
	ComponentJobs,
}

// TargetComponentDefinitions returns catalog entries that can participate in an app target graph.
func TargetComponentDefinitions(available Components) []ComponentDefinition {
	return targetDefinitionsForKeys(targetComponentKeys)
}

// TargetWizardComponentDefinitions returns target entries that belong in the interactive target wizard.
func TargetWizardComponentDefinitions(available Components) []ComponentDefinition {
	return targetDefinitionsForKeys(targetWizardComponentKeys)
}

// targetDefinitionsForKeys keeps target ordering stable while ignoring catalog entries that no longer exist.
func targetDefinitionsForKeys(keys []ComponentKey) []ComponentDefinition {
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

// TargetDefaultComponents mirrors the project app-surface components while excluding project-level tooling.
func TargetDefaultComponents(available Components) Components {
	var selected Components
	for _, definition := range TargetComponentDefinitions(available) {
		if available.Enabled(definition.Key) {
			selected.SetEnabled(definition.Key, true)
		}
	}
	return NormalizeTargetComponents(available, selected)
}

// TargetComponentsFromKeys builds a target component selection from CLI or wizard keys.
func TargetComponentsFromKeys(available Components, keys []ComponentKey) (Components, error) {
	var selected Components
	for _, key := range keys {
		definition, ok := ComponentDefinitionByKey(key)
		if !ok {
			return Components{}, fmt.Errorf("unknown component %q", key)
		}
		if !IsTargetComponentKey(key) {
			return Components{}, fmt.Errorf("%s is project-level only and cannot be selected per app target", definition.Label)
		}
		if definition.ExclusiveGroup != "" {
			clearTargetExclusiveGroup(&selected, definition.ExclusiveGroup)
		}
		selected.SetEnabled(key, true)
	}
	return NormalizeTargetComponents(available, selected), nil
}

// NormalizeTargetComponents adds target-local dependencies that are already available at project level.
func NormalizeTargetComponents(available Components, selected Components) Components {
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
			setTargetDatabaseDriver(&selected, available.DatabaseDriver())
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

// PromoteTargetComponents adds selected target capabilities to the project-level render set.
func PromoteTargetComponents(available Components, selected Components) Components {
	promoted := available
	for _, key := range targetComponentKeys {
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

// IsTargetComponentKey reports whether a component can be selected at app-target scope.
func IsTargetComponentKey(key ComponentKey) bool {
	for _, candidate := range targetComponentKeys {
		if candidate == key {
			return true
		}
	}
	return false
}

// setTargetDatabaseDriver applies the target-local database exclusivity rule in one place.
func setTargetDatabaseDriver(components *Components, driver string) {
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

// clearTargetExclusiveGroup removes visible peers before enabling a new exclusive choice.
func clearTargetExclusiveGroup(components *Components, group string) {
	for _, key := range targetComponentKeys {
		definition, ok := ComponentDefinitionByKey(key)
		if !ok || definition.ExclusiveGroup != group {
			continue
		}
		components.SetEnabled(key, false)
	}
}
