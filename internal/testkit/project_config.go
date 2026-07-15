package testkit

import (
	"os"
	"strings"

	"github.com/goforj/goforj/project"
	"github.com/goforj/goforj/version"
	"gopkg.in/yaml.v3"
)

// WriteProjectConfig writes a test project config while preserving raw component selections.
func WriteProjectConfig(path string, config project.Config) error {
	config.Render.StarterKit = project.NormalizeStarterKit(config.Render.StarterKit)
	config.Render.ComponentContractVersion = project.CurrentComponentContractVersion
	if strings.TrimSpace(config.Render.GoForjVersion) == "" {
		config.Render.GoForjVersion = version.Semver()
	}
	data, err := yaml.Marshal(&config)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
