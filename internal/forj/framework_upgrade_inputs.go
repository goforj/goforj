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
	Contract       int                           `json:"contract"`
	ProjectName    string                        `json:"project_name"`
	ModuleName     string                        `json:"module_name"`
	Defaults       frameworkAppInputs            `json:"defaults"`
	Apps           map[string]frameworkAppInputs `json:"apps"`
	DiscoveredApps []string                      `json:"discovered_apps"`
}

// frameworkInputSelection preserves declared selections and optional values without materializing engine-specific defaults.
func frameworkInputSelection(components project.Components, starter project.StarterKit, options *project.StarterKitOptions, help project.HelpFormat) frameworkAppInputs {
	input := frameworkAppInputs{Components: []string{}, StarterKit: string(starter), HelpFormat: string(help)}
	// The wizard omits demo_app, but persisted projects can still render that surface.
	keys := map[project.ComponentKey]bool{project.ComponentDemoApp: true}
	for _, definition := range project.ComponentCatalog() {
		keys[definition.Key] = true
	}
	for key := range keys {
		if components.Enabled(key) {
			input.Components = append(input.Components, string(key))
		}
	}
	sort.Strings(input.Components)
	if options != nil {
		input.ComponentLibrary = options.ComponentLibrary
	}
	return input
}

// frameworkInputsDigest hashes input contract v2, keeping unrelated config, provenance, and owner settings outside cross-renderer equality.
func frameworkInputsDigest(config *frameworkRenderIntent) (string, error) {
	inputs := frameworkRenderInputs{Contract: 2, ProjectName: config.ProjectName, ModuleName: config.GoModuleName,
		Defaults: frameworkInputSelection(config.Render.Components, config.Render.StarterKit, config.Render.StarterKitOptions, config.Render.HelpFormat), Apps: map[string]frameworkAppInputs{}}
	inputs.DiscoveredApps = append([]string{}, config.DiscoveredApps...)
	sort.Strings(inputs.DiscoveredApps)
	for name, app := range config.Apps {
		inputs.Apps[name] = frameworkInputSelection(app.Components, app.StarterKit, app.StarterKitOptions, app.HelpFormat)
	}
	source, err := json.Marshal(inputs)
	if err != nil {
		return "", err
	}
	return projectupgrade.Digest(source), nil
}
