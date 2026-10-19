package forj

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/goforj/goforj/internal/konghelp"
	"github.com/goforj/goforj/project"
	"github.com/goforj/goforj/version"
)

// NewProjectOption describes a catalog choice without exposing TUI implementation details.
type NewProjectOption struct {
	Value          string   `json:"value"`
	Label          string   `json:"label"`
	Description    string   `json:"description"`
	Preview        string   `json:"preview,omitempty"`
	Requires       []string `json:"requires,omitempty"`
	ExclusiveGroup string   `json:"exclusive_group,omitempty"`
}

// NewProjectField describes one versioned noninteractive creation argument.
type NewProjectField struct {
	Key         string             `json:"key"`
	Label       string             `json:"label"`
	Description string             `json:"description,omitempty"`
	Kind        string             `json:"kind"`
	Step        string             `json:"step"`
	Required    bool               `json:"required,omitempty"`
	Default     any                `json:"default"`
	Options     []NewProjectOption `json:"options,omitempty"`
	Requires    []string           `json:"requires,omitempty"`
	DependsOn   string             `json:"depends_on,omitempty"`
	Role        string             `json:"role,omitempty"`
	When        map[string]string  `json:"when,omitempty"`
}

// NewProjectSchema is the machine-readable authoring contract shared with graphical clients.
type NewProjectSchema struct {
	SchemaVersion int               `json:"schema_version"`
	GoForjVersion string            `json:"goforj_version"`
	Fields        []NewProjectField `json:"fields"`
}

// projectCreationSchema derives GUI choices from the same catalogs used by the terminal wizard.
func projectCreationSchema() NewProjectSchema {
	components := []NewProjectOption{}
	defaults := []string{}
	for _, definition := range project.ProjectWizardComponentDefinitions() {
		required := []string{}
		for _, key := range definition.Requires {
			required = append(required, string(key))
		}
		components = append(components, NewProjectOption{Value: string(definition.Key), Label: definition.Label, Description: definition.Description, Requires: required, ExclusiveGroup: definition.ExclusiveGroup})
		if definition.DefaultSelected {
			defaults = append(defaults, string(definition.Key))
		}
	}
	kits := []NewProjectOption{}
	for _, definition := range project.StarterKitCatalog() {
		required := []string{}
		for _, key := range definition.Requires {
			required = append(required, string(key))
		}
		kits = append(kits, NewProjectOption{Value: string(definition.Key), Label: definition.Label, Description: definition.Description, Requires: required})
	}
	formats := []NewProjectOption{}
	for _, definition := range project.HelpFormatCatalog() {
		formats = append(formats, NewProjectOption{Value: string(definition.Key), Label: definition.Label, Description: definition.Description, Preview: konghelp.Preview(string(definition.Key))})
	}
	modes := []NewProjectOption{}
	for _, item := range makeAtlasModeItems() {
		mode := item.(AtlasModeItem)
		modes = append(modes, NewProjectOption{Value: string(mode.Mode), Label: mode.Label, Description: mode.Desc})
	}
	agents, recommended := []NewProjectOption{}, []string{}
	for _, item := range makeAtlasAgentChoices().items {
		agent := item.(AtlasAgentItem)
		agents = append(agents, NewProjectOption{Value: agent.Name, Label: agent.DisplayName})
		if agent.Selected {
			recommended = append(recommended, agent.Name)
		}
	}
	surfaces, defaultSurfaces := []NewProjectOption{}, []string{}
	for _, item := range makeAtlasSurfaceItems() {
		surface := item.(AtlasSurfaceItem)
		surfaces = append(surfaces, NewProjectOption{Value: string(surface.Surface), Label: surface.Label, Description: surface.Desc})
		if surface.Selected {
			defaultSurfaces = append(defaultSurfaces, string(surface.Surface))
		}
	}
	return NewProjectSchema{SchemaVersion: 1, GoForjVersion: version.Version, Fields: []NewProjectField{
		{Key: "name", Role: "name", Label: "Project name", Kind: "text", Step: "Project", Required: true, Default: "", Description: "A display name for your project"},
		{Key: "module", Label: "Go module", Kind: "text", Step: "Project", Required: true, Default: "", Description: "For example, github.com/yourname/yourapp"},
		{Key: "path", Role: "directory", Label: "Project directory", Kind: "path", Step: "Project", Required: true, Default: "", Description: "A new or empty directory"},
		{Key: "components", Label: "Components", Kind: "multi", Step: "Stack", Default: defaults, Options: components, Description: "Required components are included automatically"},
		{Key: "starter-kit", Label: "Starter kit", Kind: "select", Step: "Stack", Default: string(project.DefaultStarterKit()), Options: kits, DependsOn: "components", Requires: []string{string(project.ComponentWebUI)}},
		{Key: "component-library", Label: "Component library", Kind: "boolean", Step: "Stack", Default: true, DependsOn: "components", Requires: []string{string(project.ComponentWebUI)}, Description: "Include the starter kit's component examples"},
		{Key: "help-format", Label: "CLI help format", Kind: "select", Step: "Options", Default: string(project.DefaultHelpFormat()), Options: formats},
		{Key: "atlas", Label: "Atlas agent support", Kind: "select", Step: "Options", Default: "recommended", Options: modes},
		{Key: "atlas-agents", Label: "Agents", Kind: "multi", Step: "Options", Required: true, Default: recommended, Options: agents, When: map[string]string{"atlas": "custom"}, Description: "Select at least one agent"},
		{Key: "atlas-surfaces", Label: "Agent integration", Kind: "multi", Step: "Options", Required: true, Default: defaultSurfaces, Options: surfaces, When: map[string]string{"atlas": "custom"}, Description: "Select at least one integration"},
		{Key: "demo", Label: "Demo application", Kind: "boolean", Step: "Options", Default: false, Description: "Include the framework demo, using its MySQL and frontend defaults"},
	}}
}

// writeProjectCreationSchema emits only JSON so consumers never need to strip banners or terminal controls.
func writeProjectCreationSchema() error {
	return json.NewEncoder(os.Stdout).Encode(projectCreationSchema())
}

// noninteractiveModel validates all user choices before sharing the interactive wizard's finalization and renderer.
func (c *NewProjectCmd) noninteractiveModel() (model, error) {
	m := initialModelWithOptions(newProjectModelOptions{allowNonEmpty: c.AllowNonEmpty})
	name := strings.TrimSpace(c.Name)
	if name == "" || len([]rune(name)) > 64 || strings.ContainsFunc(name, unicode.IsControl) {
		return m, fmt.Errorf("project name must contain 1 to 64 printable characters")
	}
	if strings.TrimSpace(c.Path) == "" {
		return m, fmt.Errorf("--path is required with --no-interaction")
	}
	if err := validateNewProjectModulePath(c.Module); err != nil {
		return m, err
	}
	m.projectInput.SetValue(name)
	m.moduleInput.SetValue(c.Module)
	m.pathInput.SetValue(c.Path)
	m.config.ProjectName, m.config.GoModuleName = name, c.Module
	if c.Components != "@default" {
		m.config.Render.Components = project.Components{}
		for _, key := range strings.Split(c.Components, ",") {
			if key == "" {
				continue
			}
			if _, ok := project.ComponentDefinitionByKey(project.ComponentKey(key)); !ok {
				return m, fmt.Errorf("unknown component: %s", key)
			}
			m.config.Render.Components.SetEnabled(project.ComponentKey(key), true)
		}
	}
	groups := map[string]string{}
	for _, definition := range project.ComponentCatalog() {
		if !m.config.Render.Components.Enabled(definition.Key) || definition.ExclusiveGroup == "" {
			continue
		}
		if prior := groups[definition.ExclusiveGroup]; prior != "" {
			return m, fmt.Errorf("components %s and %s are mutually exclusive", prior, definition.Key)
		}
		groups[definition.ExclusiveGroup] = string(definition.Key)
	}
	m.config.Render.Components.ResolveDependencies()
	for _, field := range projectCreationSchema().Fields {
		value := map[string]string{"starter-kit": c.StarterKit, "help-format": c.HelpFormat, "atlas": c.Atlas}[field.Key]
		if field.Kind != "select" {
			continue
		}
		found := false
		for _, option := range field.Options {
			if option.Value == value {
				found = true
			}
		}
		if !found {
			return m, fmt.Errorf("unknown %s: %s", field.Key, value)
		}
	}
	m.config.Render.StarterKit = project.StarterKit(c.StarterKit)
	if err := project.ValidateStarterKitContract(m.config.Render.StarterKit, m.config.Render.Components); err != nil {
		return m, err
	}
	m.config.Render.HelpFormat = project.HelpFormat(c.HelpFormat)
	if !c.ComponentLibrary {
		m.config.Render.StarterKitOptions = project.NewStarterKitOptions(false)
	}
	m.atlasMode = atlasMode(c.Atlas)
	if c.Atlas != "custom" && (c.AtlasAgents != "@default" || c.AtlasSurfaces != "@default") {
		return m, fmt.Errorf("--atlas-agents and --atlas-surfaces require --atlas=custom")
	}
	if c.Atlas == "custom" {
		if c.AtlasAgents != "@default" {
			selected := strings.Split(c.AtlasAgents, ",")
			known := map[string]bool{}
			for index, item := range m.atlasAgentList.Items() {
				agent := item.(AtlasAgentItem)
				known[agent.Name] = true
				agent.Selected = containsCreationChoice(selected, agent.Name)
				m.atlasAgentList.SetItem(index, agent)
			}
			for _, name := range selected {
				if !known[name] {
					return m, fmt.Errorf("unknown atlas agent: %s", name)
				}
			}
		}
		if c.AtlasSurfaces != "@default" {
			selected := strings.Split(c.AtlasSurfaces, ",")
			known := map[string]bool{}
			for index, item := range m.atlasSurfaceList.Items() {
				surface := item.(AtlasSurfaceItem)
				known[string(surface.Surface)] = true
				surface.Selected = containsCreationChoice(selected, string(surface.Surface))
				m.atlasSurfaceList.SetItem(index, surface)
			}
			for _, name := range selected {
				if !known[name] {
					return m, fmt.Errorf("unknown atlas integration: %s", name)
				}
			}
		}
		if len(m.selectedAtlasAgents()) == 0 || !m.selectedAtlasSurfaces().any() {
			return m, fmt.Errorf("custom Atlas support requires at least one agent and one integration")
		}
	}
	if c.Demo {
		m.extrasIndex = 1
		for index, item := range m.componentList.Items() {
			component := item.(ListItem)
			component.Selected = m.config.Render.Components.Enabled(component.Key)
			m.componentList.SetItem(index, component)
		}
		m.applyExtrasSelection()
	}
	if err := m.validateBeforeConfirm(); err != nil {
		return m, err
	}
	target, err := filepath.Abs(m.projectPath())
	if err != nil {
		return m, err
	}
	m.targetPath = target
	if err := m.finalizeConfig(); err != nil {
		return m, err
	}
	m.stage = StageDone
	return m, nil
}

// containsCreationChoice keeps explicit custom selections aligned with the wizard's catalog keys.
func containsCreationChoice(choices []string, value string) bool {
	for _, choice := range choices {
		if choice == value {
			return true
		}
	}
	return false
}
