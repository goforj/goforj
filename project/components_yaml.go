package project

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

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
	ComponentJobs,
}

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
			if !isComponentYAMLKey(key) {
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

// MarshalYAML emits enabled component keys in a stable flow sequence for concise project configuration.
func (c Components) MarshalYAML() (any, error) {
	node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
	for _, key := range componentYAMLKeys {
		if !c.Enabled(key) {
			continue
		}
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: string(key)})
	}
	return node, nil
}

// NeedsComponentMigration reports whether any component selection was loaded from the historical boolean mapping.
func (c ProjectConfig) NeedsComponentMigration() bool {
	return c.needsComponentMigration
}

// UnmarshalYAML records historical component shapes without changing Components value equality.
func (c *ProjectConfig) UnmarshalYAML(value *yaml.Node) error {
	type projectConfigFields ProjectConfig
	var fields projectConfigFields
	if err := value.Decode(&fields); err != nil {
		return fmt.Errorf("decode project config: %w", err)
	}
	*c = ProjectConfig(fields)
	c.needsComponentMigration = projectConfigNeedsComponentMigration(value)
	return nil
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
