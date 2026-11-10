package forj

import (
	"encoding/json"
	"sort"

	"github.com/goforj/goforj/internal/projectupgrade"
	"github.com/goforj/goforj/project"
)

// frameworkAppInputs fixes the renderer protocol's supported input shape independently of evolving project configuration structs.
type frameworkAppInputs struct {
	Components       []string `json:"components"`
	StarterKit       string   `json:"starter_kit"`
	ComponentLibrary *bool    `json:"component_library,omitempty"`
	HelpFormat       string   `json:"help_format"`
}

// frameworkRenderInputs versions the non-secret recipe rather than fingerprinting incidental configuration schema fields.
type frameworkRenderInputs struct {
	Contract    int                           `json:"contract"`
	ProjectName string                        `json:"project_name"`
	ModuleName  string                        `json:"module_name"`
	Defaults    frameworkAppInputs            `json:"defaults"`
	Apps        map[string]frameworkAppInputs `json:"apps"`
}

// frameworkInputSelection preserves declared selections and optional values without materializing engine-specific defaults.
func frameworkInputSelection(components project.Components, starter project.StarterKit, options *project.StarterKitOptions, help project.HelpFormat) frameworkAppInputs {
	input := frameworkAppInputs{Components: []string{}, StarterKit: string(starter), HelpFormat: string(help)}
	for _, definition := range project.ComponentCatalog() {
		if components.Enabled(definition.Key) {
			input.Components = append(input.Components, string(definition.Key))
		}
	}
	sort.Strings(input.Components)
	if options != nil {
		input.ComponentLibrary = options.ComponentLibrary
	}
	return input
}

// frameworkInputsDigest hashes input contract v1, keeping unrelated config, provenance, and owner settings outside cross-renderer equality.
func frameworkInputsDigest(config *project.Config) (string, error) {
	inputs := frameworkRenderInputs{Contract: 1, ProjectName: config.ProjectName, ModuleName: config.GoModuleName,
		Defaults: frameworkInputSelection(config.Render.Components, config.Render.StarterKit, config.Render.StarterKitOptions, config.Render.HelpFormat), Apps: map[string]frameworkAppInputs{}}
	for name, app := range config.Apps {
		inputs.Apps[name] = frameworkInputSelection(app.Components, app.StarterKit, app.StarterKitOptions, app.HelpFormat)
	}
	source, err := json.Marshal(inputs)
	if err != nil {
		return "", err
	}
	return projectupgrade.Digest(source), nil
}
