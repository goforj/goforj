package rendercheck

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
	"github.com/goforj/goforj/version"
	"gopkg.in/yaml.v3"
)

var legacyComponentMappingKeys = []project.ComponentKey{
	project.ComponentCLI,
	project.ComponentDemoApp,
	project.ComponentMail,
	project.ComponentAuth,
	project.ComponentOAuth,
	project.ComponentWebAPI,
	project.ComponentWebUI,
	project.ComponentMetrics,
	project.ComponentObservability,
	project.ComponentGrafana,
	project.ComponentDocker,
	project.ComponentDatabaseMySQL,
	project.ComponentDatabasePostgres,
	project.ComponentDatabaseSQLite,
	project.ComponentScheduler,
	project.ComponentJobs,
}

// writeRenderComboConfig selects the historical writer only for the migration sentinel.
func writeRenderComboConfig(path string, config project.Config, legacy bool) error {
	if !legacy {
		return testkit.WriteProjectConfig(path, config)
	}
	return writeLegacyRenderComboConfig(path, config)
}

// writeLegacyRenderComboConfig writes the pre-sequence shape that a real project upgrade must consume.
func writeLegacyRenderComboConfig(path string, config project.Config) error {
	data, err := encodeLegacyRenderComboConfig(config)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// encodeLegacyRenderComboConfig keeps the full production config shape while replacing only component selections.
func encodeLegacyRenderComboConfig(config project.Config) ([]byte, error) {
	config.Render.StarterKit = project.NormalizeStarterKit(config.Render.StarterKit)
	if strings.TrimSpace(config.Render.GoForjVersion) == "" {
		config.Render.GoForjVersion = version.Semver()
	}
	canonical, err := yaml.Marshal(&config)
	if err != nil {
		return nil, fmt.Errorf("encode canonical render config: %w", err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(canonical, &document); err != nil {
		return nil, fmt.Errorf("decode canonical render config node: %w", err)
	}
	root, err := renderConfigDocumentRoot(&document)
	if err != nil {
		return nil, err
	}
	render := renderConfigMappingValue(root, "render")
	if err := replaceLegacyComponentMapping(render, config.Render.Components); err != nil {
		return nil, fmt.Errorf("replace default App components: %w", err)
	}
	apps := renderConfigMappingValue(root, "apps")
	for name, app := range config.Apps {
		appNode := renderConfigMappingValue(apps, name)
		if err := replaceLegacyComponentMapping(appNode, app.Components); err != nil {
			return nil, fmt.Errorf("replace App %s components: %w", name, err)
		}
	}
	encoded, err := yaml.Marshal(&document)
	if err != nil {
		return nil, fmt.Errorf("encode legacy render config: %w", err)
	}
	return encoded, nil
}

// renderConfigDocumentRoot rejects malformed fixture output before a real render can hide the writer failure.
func renderConfigDocumentRoot(document *yaml.Node) (*yaml.Node, error) {
	if document == nil || document.Kind != yaml.DocumentNode || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("render config must encode as one YAML mapping document")
	}
	return document.Content[0], nil
}

// replaceLegacyComponentMapping models old configs that predate optional Cache, Events, and Storage selections.
func replaceLegacyComponentMapping(scope *yaml.Node, components project.Components) error {
	componentNode := renderConfigMappingValue(scope, "components")
	if componentNode == nil {
		return fmt.Errorf("components mapping is missing")
	}
	*componentNode = *legacyComponentMappingNode(components)
	return nil
}

// legacyComponentMappingNode emits every historical toggle so false selections are exercised as faithfully as true ones.
func legacyComponentMappingNode(components project.Components) *yaml.Node {
	mapping := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, key := range legacyComponentMappingKeys {
		mapping.Content = append(mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: string(key)},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(components.Enabled(key))},
		)
	}
	return mapping
}

// renderConfigMappingValue returns one child without decoding away the historical YAML shape under test.
func renderConfigMappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1]
		}
	}
	return nil
}
