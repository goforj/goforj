package project

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// componentYAMLKeys is intentionally independent from wizard order so persisted config stays stable when the UI is regrouped.
var componentYAMLKeys = []ComponentKey{
	ComponentCLI,
	ComponentDemoApp,
	ComponentMail,
	ComponentAuth,
	ComponentOAuth,
	ComponentWebAPI,
	ComponentWebUI,
	ComponentMetrics,
	ComponentObservability,
	ComponentGrafana,
	ComponentDocker,
	ComponentDatabaseMySQL,
	ComponentDatabasePostgres,
	ComponentDatabaseSQLite,
	ComponentScheduler,
	ComponentCache,
	ComponentEvents,
	ComponentStorage,
	ComponentJobs,
}

var retiredLegacyComponentYAMLKeys = map[ComponentKey]struct{}{
	"stress_test": {},
}

const componentYAMLInlineLimit = 120

// UnmarshalYAML accepts historical boolean mappings and canonical component-name sequences.
func (c *Components) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.MappingNode:
		seen := make(map[ComponentKey]struct{}, len(value.Content)/2)
		for index := 0; index+1 < len(value.Content); index += 2 {
			entry := value.Content[index]
			if entry.Kind != yaml.ScalarNode || entry.Tag != "!!str" {
				return fmt.Errorf("decode components: legacy mapping key %d must be a component name", index/2+1)
			}
			key := ComponentKey(entry.Value)
			if !isComponentYAMLKey(key) && !isRetiredLegacyComponentYAMLKey(key) {
				return fmt.Errorf("decode components: unknown component %q in legacy mapping; valid components: %s", key, componentYAMLKeyNames())
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("decode components: duplicate component %q in legacy mapping; define each component once", key)
			}
			seen[key] = struct{}{}
		}
		type componentFields Components
		var fields componentFields
		if err := value.Decode(&fields); err != nil {
			return fmt.Errorf("decode legacy component mapping: %w", err)
		}
		*c = Components(fields)
		return nil
	case yaml.SequenceNode:
		var decoded Components
		seen := make(map[ComponentKey]struct{}, len(value.Content))
		for index, entry := range value.Content {
			if entry.Kind != yaml.ScalarNode || entry.Tag != "!!str" {
				return fmt.Errorf("decode components: entry %d must be a component name", index+1)
			}
			key := ComponentKey(entry.Value)
			if !isComponentYAMLKey(key) {
				return fmt.Errorf("decode components: unknown component %q; valid components: %s", key, componentYAMLKeyNames())
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("decode components: duplicate component %q; list each component once", key)
			}
			seen[key] = struct{}{}
			decoded.SetEnabled(key, true)
		}
		*c = decoded
		return nil
	default:
		return fmt.Errorf("decode components: expected a legacy boolean mapping or component-name sequence")
	}
}

// MarshalYAML emits enabled component keys in a stable sequence and expands only selections that would create an overlong line.
func (c Components) MarshalYAML() (any, error) {
	names := make([]string, 0, len(componentYAMLKeys))
	for _, key := range componentYAMLKeys {
		if !c.Enabled(key) {
			continue
		}
		names = append(names, string(key))
	}
	node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: componentYAMLSequenceStyle(names)}
	for _, name := range names {
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name})
	}
	return node, nil
}

// componentYAMLSequenceStyle keeps short selections scan-friendly without creating an overlong config line.
func componentYAMLSequenceStyle(names []string) yaml.Style {
	line := "components: [" + strings.Join(names, ", ") + "]"
	if len(line) <= componentYAMLInlineLimit {
		return yaml.FlowStyle
	}
	return 0
}

// NeedsComponentMigration reports whether component YAML needs a canonical rewrite because it used a historical mapping or marker.
func (c ProjectConfig) NeedsComponentMigration() bool {
	return c.needsComponentMigration
}

// UnmarshalYAML uses the render component shape to distinguish legacy omission defaults from explicit modern selections.
func (c *ProjectConfig) UnmarshalYAML(value *yaml.Node) error {
	type projectConfigFields ProjectConfig
	var fields projectConfigFields
	if err := value.Decode(&fields); err != nil {
		return fmt.Errorf("decode project config: %w", err)
	}
	*c = ProjectConfig(fields)
	if c.Render.ComponentContractVersion < 0 || c.Render.ComponentContractVersion > CurrentComponentContractVersion {
		return fmt.Errorf("decode project config: unsupported component contract version %d; this GoForj release supports version %d", c.Render.ComponentContractVersion, CurrentComponentContractVersion)
	}
	render := yamlMappingValue(value, "render")
	renderComponents := yamlMappingValue(render, "components")
	legacyDefaults := c.Render.ComponentContractVersion < CurrentComponentContractVersion && (renderComponents == nil || renderComponents.Kind == yaml.MappingNode)
	if legacyDefaults {
		c.migrateLegacyPrimitiveComponents()
	}
	c.needsComponentMigration = legacyDefaults || yamlMappingValue(render, "component_contract") != nil || projectConfigNeedsComponentMigration(value)
	return nil
}

// migrateLegacyPrimitiveComponents preserves capabilities that every App received before they became optional components.
func (c *ProjectConfig) migrateLegacyPrimitiveComponents() {
	c.Render.ComponentContractVersion = CurrentComponentContractVersion
	c.Render.Components.Cache = true
	c.Render.Components.Events = true
	c.Render.Components.Storage = true
	for name, app := range c.Apps {
		app.Components.Cache = true
		app.Components.Events = true
		app.Components.Storage = true
		c.Apps[name] = app
	}
}

// isComponentYAMLKey reports whether a name belongs to the exact persisted component contract.
func isComponentYAMLKey(key ComponentKey) bool {
	for _, candidate := range componentYAMLKeys {
		if candidate == key {
			return true
		}
	}
	return false
}

// isRetiredLegacyComponentYAMLKey keeps generated projects loadable after a component leaves the render catalog.
func isRetiredLegacyComponentYAMLKey(key ComponentKey) bool {
	_, ok := retiredLegacyComponentYAMLKeys[key]
	return ok
}

// componentYAMLKeyNames formats the accepted sequence keys for configuration errors.
func componentYAMLKeyNames() string {
	names := make([]string, len(componentYAMLKeys))
	for index, key := range componentYAMLKeys {
		names[index] = string(key)
	}
	return strings.Join(names, ", ")
}

// projectConfigNeedsComponentMigration finds legacy component mappings at project and App scope.
func projectConfigNeedsComponentMigration(value *yaml.Node) bool {
	render := yamlMappingValue(value, "render")
	if components := yamlMappingValue(render, "components"); components != nil && components.Kind == yaml.MappingNode {
		return true
	}
	apps := yamlMappingValue(value, "apps")
	if apps == nil || apps.Kind != yaml.MappingNode {
		return false
	}
	for index := 0; index+1 < len(apps.Content); index += 2 {
		if components := yamlMappingValue(apps.Content[index+1], "components"); components != nil && components.Kind == yaml.MappingNode {
			return true
		}
	}
	return false
}

// yamlMappingValue reads raw field shapes so migration decisions do not depend on decoded component values.
func yamlMappingValue(value *yaml.Node, key string) *yaml.Node {
	if value == nil || value.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(value.Content); index += 2 {
		if value.Content[index].Value == key {
			return value.Content[index+1]
		}
	}
	return nil
}
