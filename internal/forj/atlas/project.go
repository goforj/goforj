package atlas

import (
	"os"
	"path/filepath"
	"strings"

	atlasproject "github.com/goforj/atlas/project"
	"github.com/goforj/goforj/project"
	"github.com/goforj/goforj/version"
	"gopkg.in/yaml.v3"
)

// Project discovers Atlas project metadata from the current GoForj project.
func Project(root string) atlasproject.Project {
	discovered, err := atlasproject.Discover(root)
	if err != nil {
		discovered = atlasproject.Project{Root: root, Name: "goforj-project"}
	}
	discovered.GoForjVersion = version.String()

	cfg, err := loadProjectConfig(root)
	if err != nil {
		return discovered.WithDiscoveredDefaults()
	}
	discovered.Name = firstNonEmpty(cfg.ProjectName, discovered.Name)
	discovered.GoForjVersion = firstNonEmpty(cfg.Render.GoForjVersion, discovered.GoForjVersion)
	discovered.Components = componentNames(project.ProjectComponents(cfg))
	discovered.FrontendKit = string(cfg.Render.StarterKit)
	discovered.DatabaseDriver = cfg.Render.Components.DatabaseDriver()
	discovered.QueueDriver = loadAtlasEnv(root)["QUEUE_DRIVER"]
	discovered.Apps = atlasAppsFromConfig(root, cfg, discovered.Apps)

	return discovered.WithDiscoveredDefaults()
}

func loadProjectConfig(root string) (*project.Config, error) {
	content, err := os.ReadFile(filepath.Join(root, ".goforj.yml"))
	if err != nil {
		return nil, err
	}
	var cfg project.Config
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func componentNames(components project.Components) []string {
	names := []string{}
	for _, definition := range project.ComponentCatalog() {
		if components.Enabled(definition.Key) {
			names = append(names, strings.ReplaceAll(string(definition.Key), "_", "-"))
		}
	}
	return names
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
