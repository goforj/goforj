package forj

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/goforj/console"
	"github.com/goforj/goforj/internal/forj/atlas"
	"github.com/goforj/goforj/internal/konghelp"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/projectlayout"
	"github.com/goforj/goforj/project"
	"github.com/goforj/goforj/version"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// WizardStage identifies the current step in the project creation wizard.
type WizardStage int

const (
	// StageProjectName collects the display name for the generated project.
	StageProjectName WizardStage = iota
	// StageModuleName collects the Go module path.
	StageModuleName
	// StageSelectComponents collects the project-level component selection.
	StageSelectComponents
	// StageHelpFormat collects the CLI help formatter.
	StageHelpFormat
	// StageStarterKit collects the frontend starter kit when Web UI is enabled.
	StageStarterKit
	// StageExtras collects optional project profiles that expand component selection.
	StageExtras
	// StageAtlasSupport collects optional AI agent support installation.
	StageAtlasSupport
	// StageAtlasAgents collects custom AI agent selections.
	StageAtlasAgents
	// StageAtlasSurfaces collects custom Atlas file surface selections.
	StageAtlasSurfaces
	// StageProjectPath collects the destination directory.
	StageProjectPath
	// StageConfirm shows the final project creation summary.
	StageConfirm
	// StageDone marks the wizard as ready to quit.
	StageDone
)

var (
	primaryText           = lipgloss.Color("#f5f6f7") // off-white
	mutedText             = lipgloss.Color("#8b93a1") // gray
	accentColor           = lipgloss.Color("#8C97E6") // soft blue-violet
	errorColor            = lipgloss.Color("#c97b7b") // muted red
	normalStyle           = lipgloss.NewStyle().Foreground(primaryText)
	successStyle          = lipgloss.NewStyle().Foreground(primaryText)
	titleStyle            = lipgloss.NewStyle().Foreground(primaryText).Bold(true)
	subtitleStyle         = lipgloss.NewStyle().Foreground(mutedText).Italic(true)
	helpStyle             = lipgloss.NewStyle().Foreground(mutedText)
	progressDoneMark      = lipgloss.NewStyle().Foreground(accentColor)
	progressDoneLabel     = lipgloss.NewStyle().Foreground(mutedText)
	progressCurrentStyle  = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	progressPendingStyle  = lipgloss.NewStyle().Foreground(mutedText)
	titleIndicatorStyle   = lipgloss.NewStyle().Foreground(mutedText)
	subLabelStyle         = helpStyle.Italic(true)
	errorStyle            = lipgloss.NewStyle().Foreground(errorColor)
	inputRuleStyle        = lipgloss.NewStyle().Foreground(primaryText)
	labelKeyStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
	labelSepStyle         = lipgloss.NewStyle().Foreground(mutedText)
	listNameStyle         = lipgloss.NewStyle().Foreground(primaryText)
	listNameDimStyle      = lipgloss.NewStyle().Foreground(mutedText)
	listDescStyle         = lipgloss.NewStyle().Foreground(mutedText)
	listFocusedNameStyle  = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	listFocusedDescStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#b8c0d0"))
	panelTitleDoneStyle   = lipgloss.NewStyle().Foreground(mutedText)
	panelTitleActiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#c6e5ff"))
	listOptionMutedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#636b78"))
	panelBorderStyle      = lipgloss.NewStyle().Foreground(primaryText)
	statusOKStyle         = lipgloss.NewStyle().Foreground(accentColor)
	statusErrorStyle      = lipgloss.NewStyle().Foreground(errorColor)
)

// ListItem adapts a project component definition to the Bubbles list model.
type ListItem struct {
	Key      project.ComponentKey
	Name     string
	Desc     string
	Selected bool
}

// Title satisfies the Bubbles list item contract for project component rows.
func (i ListItem) Title() string { return i.Name }

// Description satisfies the Bubbles list item contract for project component rows.
func (i ListItem) Description() string { return i.Desc }

// FilterValue satisfies the Bubbles list item contract even though filtering is disabled.
func (i ListItem) FilterValue() string { return i.Name }

// StarterKitItem adapts a starter-kit definition to the Bubbles list model.
type StarterKitItem struct {
	Key   project.StarterKit
	Label string
	Desc  string
}

// Title satisfies the Bubbles list item contract for starter-kit rows.
func (i StarterKitItem) Title() string { return i.Label }

// Description satisfies the Bubbles list item contract for starter-kit rows.
func (i StarterKitItem) Description() string { return i.Desc }

// FilterValue satisfies the Bubbles list item contract even though filtering is disabled.
func (i StarterKitItem) FilterValue() string { return i.Label }

// HelpFormatItem adapts a help formatter option to the Bubbles list model.
type HelpFormatItem struct {
	Key   project.HelpFormat
	Label string
	Desc  string
}

// Title satisfies the Bubbles list item contract for help-format rows.
func (i HelpFormatItem) Title() string { return i.Label }

// Description satisfies the Bubbles list item contract for help-format rows.
func (i HelpFormatItem) Description() string { return i.Desc }

// FilterValue satisfies the Bubbles list item contract even though filtering is disabled.
func (i HelpFormatItem) FilterValue() string { return i.Label }

type starterKitOptionRow struct {
	Label   string
	Enabled bool
}

type atlasMode string

const (
	atlasModeRecommended atlasMode = "recommended"
	atlasModeMinimal     atlasMode = "minimal"
	atlasModeCustom      atlasMode = "custom"
	atlasModeSkip        atlasMode = "skip"
)

type atlasSurface string

const (
	atlasSurfaceGuidelines atlasSurface = "guidelines"
	atlasSurfaceSkills     atlasSurface = "skills"
	atlasSurfaceMCP        atlasSurface = "mcp"
)

// AtlasModeItem adapts an Atlas support mode to the Bubbles list model.
type AtlasModeItem struct {
	Mode  atlasMode
	Label string
	Desc  string
}

// Title satisfies the Bubbles list item contract for Atlas mode rows.
func (i AtlasModeItem) Title() string { return i.Label }

// Description satisfies the Bubbles list item contract even though descriptions are manually rendered.
func (i AtlasModeItem) Description() string { return i.Desc }

// FilterValue satisfies the Bubbles list item contract even though filtering is disabled.
func (i AtlasModeItem) FilterValue() string { return i.Label }

// AtlasAgentItem adapts a supported local AI agent to the Bubbles list model.
type AtlasAgentItem struct {
	Name        string
	DisplayName string
	Detected    bool
	Selected    bool
}

// Title satisfies the Bubbles list item contract for Atlas agent rows.
func (i AtlasAgentItem) Title() string { return i.DisplayName }

// Description satisfies the Bubbles list item contract even though descriptions are manually rendered.
func (i AtlasAgentItem) Description() string {
	if i.Detected {
		return "detected"
	}
	return "available"
}

// FilterValue satisfies the Bubbles list item contract even though filtering is disabled.
func (i AtlasAgentItem) FilterValue() string { return i.DisplayName }

// AtlasSurfaceItem adapts an Atlas install surface to the Bubbles list model.
type AtlasSurfaceItem struct {
	Surface  atlasSurface
	Label    string
	Desc     string
	Selected bool
}

// Title satisfies the Bubbles list item contract for Atlas surface rows.
func (i AtlasSurfaceItem) Title() string { return i.Label }

// Description satisfies the Bubbles list item contract even though descriptions are manually rendered.
func (i AtlasSurfaceItem) Description() string { return i.Desc }

// FilterValue satisfies the Bubbles list item contract even though filtering is disabled.
func (i AtlasSurfaceItem) FilterValue() string { return i.Label }

// makeProjectComponentItems converts the shared component catalog into wizard rows.
func makeProjectComponentItems() []list.Item {
	definitions := project.ProjectWizardComponentDefinitions()
	items := make([]list.Item, 0, len(definitions))
	for _, component := range definitions {
		items = append(items, ListItem{
			Key:      component.Key,
			Name:     component.Label,
			Desc:     component.Description,
			Selected: component.DefaultSelected,
		})
	}
	return items
}

// makeStarterKitItems converts the shared starter-kit catalog into wizard rows.
func makeStarterKitItems() []list.Item {
	definitions := project.StarterKitCatalog()
	items := make([]list.Item, 0, len(definitions))
	for _, definition := range definitions {
		items = append(items, StarterKitItem{
			Key:   definition.Key,
			Label: definition.Label,
			Desc:  definition.Description,
		})
	}
	return items
}

// makeHelpFormatItems converts the shared help formatter catalog into wizard rows.
func makeHelpFormatItems() []list.Item {
	definitions := project.HelpFormatCatalog()
	items := make([]list.Item, 0, len(definitions))
	for _, definition := range definitions {
		items = append(items, HelpFormatItem{
			Key:   definition.Key,
			Label: definition.Label,
			Desc:  definition.Description,
		})
	}
	return items
}

// makeAtlasModeItems returns the high-level Atlas install choices for the wizard.
func makeAtlasModeItems() []list.Item {
	return []list.Item{
		AtlasModeItem{Mode: atlasModeRecommended, Label: "Recommended", Desc: "detected agents with guidelines, skills, and MCP"},
		AtlasModeItem{Mode: atlasModeMinimal, Label: "Minimal", Desc: "detected agents with guidelines only"},
		AtlasModeItem{Mode: atlasModeCustom, Label: "Custom", Desc: "choose agents and install surfaces"},
		AtlasModeItem{Mode: atlasModeSkip, Label: "Skip", Desc: "do not install local agent support"},
	}
}

// atlasAgentChoices keeps the wizard's detected recommendations stable through final installation.
type atlasAgentChoices struct {
	items       []list.Item
	recommended []string
}

// makeAtlasAgentChoices captures wizard rows and recommendations before project creation changes filesystem context.
func makeAtlasAgentChoices() atlasAgentChoices {
	options := atlas.AgentOptions(context.Background(), ".")
	recommendedNames := atlas.RecommendedAgents(context.Background(), ".")
	recommended := map[string]bool{}
	for _, name := range recommendedNames {
		recommended[name] = true
	}
	items := make([]list.Item, 0, len(options))
	for _, option := range options {
		items = append(items, AtlasAgentItem{
			Name:        option.Name,
			DisplayName: option.DisplayName,
			Detected:    option.Detected,
			Selected:    recommended[option.Name],
		})
	}
	return atlasAgentChoices{items: items, recommended: recommendedNames}
}

// makeAtlasSurfaceItems returns install surfaces with the recommended defaults selected.
func makeAtlasSurfaceItems() []list.Item {
	return []list.Item{
		AtlasSurfaceItem{Surface: atlasSurfaceGuidelines, Label: "Guidelines", Desc: "project guidance for local agents", Selected: true},
		AtlasSurfaceItem{Surface: atlasSurfaceSkills, Label: "Skills / prompts", Desc: "framework-specific reusable instructions", Selected: true},
		AtlasSurfaceItem{Surface: atlasSurfaceMCP, Label: "MCP server config", Desc: "read-only project tools and docs search", Selected: true},
	}
}

type model struct {
	stage                WizardStage
	projectInput         textinput.Model
	moduleInput          textinput.Model
	pathInput            textinput.Model
	componentList        list.Model
	helpFormatList       list.Model
	starterKitList       list.Model
	atlasModeList        list.Model
	atlasAgentList       list.Model
	atlasSurfaceList     list.Model
	selectedComponents   []string
	config               project.Config
	cancelled            bool
	errorMsg             string
	targetPath           string
	termWidth            int
	allowNonEmpty        bool
	extrasIndex          int
	demoAppEnabled       bool
	starterKitApplicable bool
	componentLibrary     bool
	resourcePreparation  *newProjectResourcePreparation
	atlasMode            atlasMode
	atlasRecommendations []string
}

const wizardWidth = 90

// newProjectModelOptions carries non-interactive command flags into wizard validation.
type newProjectModelOptions struct {
	allowNonEmpty bool
}

// components returns the one mutable capability set shared by every wizard stage.
func (m *model) components() *project.Components {
	return &m.config.Render.Components
}

// finalizeConfig derives development tasks only after the effective resource and service plans are valid.
func (m *model) finalizeConfig() error {
	preparation, err := m.selectedResourcePreparation()
	if err != nil {
		return fmt.Errorf("resolve App resources: %w", err)
	}

	m.config.UpdatedAt = time.Now().Format("2006-01-02 15:04:05 MST")
	m.config.Render.GoForjVersion = version.Semver()
	components := m.components()
	m.config.Render.StarterKit = project.NormalizeStarterKit(m.config.Render.StarterKit)
	if !components.WebUI || components.DemoApp {
		m.config.Render.StarterKit = project.StarterKitNone
		m.config.Render.StarterKitOptions = nil
	}
	m.config.Render.HelpFormat = project.NormalizeHelpFormat(m.config.Render.HelpFormat)
	m.config.Render.AgentGuidance = m.selectedAgentGuidance()
	// Reset slices before populating.
	m.config.Dev = project.DevConfig{
		Pre:               []project.DevTask{},
		Apps:              map[string]project.DevApp{},
		SoundOnWatchError: true,
		AutoMigrate:       components.HasDatabase(),
		DownOnExit:        true,
		WirePaths:         []string{project.DefaultApp().WireDir},
	}
	serviceTasks := planNewProjectServiceTasks(preparation.plan, preparation.servicePlan, *components)
	m.config.Dev.Pre = append(m.config.Dev.Pre, serviceTasks.Pre...)
	m.config.Dev.Down = append(m.config.Dev.Down, serviceTasks.Down...)

	if components.WebUI && project.StarterKitUsesNPM(m.config.Render.StarterKit) {
		m.config.Dev.Pre = append(m.config.Dev.Pre, generatedDevFrontendInstallTask(project.DefaultApp()))
	}

	if components.HasRuntime() {
		m.config.Dev.Apps = map[string]project.DevApp{
			project.DefaultAppName: generatedDevAppConfig(&m.config, project.DefaultApp(), ""),
		}
	}

	if components.WebUI && !project.StarterKitUsesNPM(m.config.Render.StarterKit) && packageJSONHasNpmDev(m.targetPath) {
		m.config.Dev.Watches = append(m.config.Dev.Watches, project.DevWatch{
			Name:  "NPM",
			Watch: frontendNPMWatch(projectlayout.FrontendDir(".", project.DefaultApp())),
			Exec:  "npm run dev",
		})
	}
	return nil
}

// selectedAgentGuidance maps wizard surfaces onto the durable render contract independently from optional Atlas features.
func (m model) selectedAgentGuidance() project.AgentGuidance {
	if m.selectedAtlasSurfaces().guidelines {
		return project.AgentGuidanceBaseline
	}
	return project.AgentGuidanceNone
}

// frontendNPMWatch returns compatibility filters for an existing frontend whose
// build lifecycle is not owned by a known GoForj starter kit.
func frontendNPMWatch(frontendDir string) string {
	return "-cd ./" + filepath.ToSlash(frontendDir) + " -xdir _data -xdir node_modules -xdir dist"
}

// initialModelWithOptions builds wizard state for command-line flags that need to affect validation.
func initialModelWithOptions(options newProjectModelOptions) model {
	ti := styledTextInput()
	ti.Placeholder = "My Awesome App"
	ti.Focus()
	ti.CharLimit = 64

	pi := styledTextInput()
	pi.Placeholder = "Use current dir or provide a path"

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = normalStyle
	delegate.Styles.SelectedDesc = helpStyle
	delegate.Styles.NormalTitle = normalStyle
	delegate.Styles.NormalDesc = helpStyle
	delegate.Styles.DimmedTitle = helpStyle
	delegate.Styles.DimmedDesc = helpStyle
	delegate.ShowDescription = false

	li := list.New(makeProjectComponentItems(), delegate, 42, 12)
	li.Title = "Select Components"
	li.SetShowFilter(false)
	li.SetShowHelp(false)
	li.Styles.Title = lipgloss.NewStyle().Foreground(primaryText).Bold(true)
	li.Styles.PaginationStyle = helpStyle
	li.Styles.HelpStyle = helpStyle
	li.Styles.StatusBar = helpStyle
	li.SetShowStatusBar(false)
	li.SetShowPagination(false)

	starterKitList := list.New(makeStarterKitItems(), delegate, 42, 4)
	starterKitList.Title = "Starter Kit"
	starterKitList.SetShowFilter(false)
	starterKitList.SetShowHelp(false)
	starterKitList.Styles.Title = lipgloss.NewStyle().Foreground(primaryText).Bold(true)
	starterKitList.Styles.PaginationStyle = helpStyle
	starterKitList.Styles.HelpStyle = helpStyle
	starterKitList.Styles.StatusBar = helpStyle
	starterKitList.SetShowStatusBar(false)
	starterKitList.SetShowPagination(false)

	helpFormatList := list.New(makeHelpFormatItems(), delegate, 42, 2)
	helpFormatList.Title = "Help Format"
	helpFormatList.SetShowFilter(false)
	helpFormatList.SetShowHelp(false)
	helpFormatList.Styles.Title = lipgloss.NewStyle().Foreground(primaryText).Bold(true)
	helpFormatList.Styles.PaginationStyle = helpStyle
	helpFormatList.Styles.HelpStyle = helpStyle
	helpFormatList.Styles.StatusBar = helpStyle
	helpFormatList.SetShowStatusBar(false)
	helpFormatList.SetShowPagination(false)

	atlasModeList := list.New(makeAtlasModeItems(), delegate, 42, 4)
	atlasModeList.Title = "Atlas - Agent Support"
	atlasModeList.SetShowFilter(false)
	atlasModeList.SetShowHelp(false)
	atlasModeList.Styles.Title = lipgloss.NewStyle().Foreground(primaryText).Bold(true)
	atlasModeList.Styles.PaginationStyle = helpStyle
	atlasModeList.Styles.HelpStyle = helpStyle
	atlasModeList.Styles.StatusBar = helpStyle
	atlasModeList.SetShowStatusBar(false)
	atlasModeList.SetShowPagination(false)

	atlasAgents := makeAtlasAgentChoices()
	atlasAgentList := list.New(atlasAgents.items, delegate, 42, 4)
	atlasAgentList.Title = "Atlas Agents"
	atlasAgentList.SetShowFilter(false)
	atlasAgentList.SetShowHelp(false)
	atlasAgentList.Styles.Title = lipgloss.NewStyle().Foreground(primaryText).Bold(true)
	atlasAgentList.Styles.PaginationStyle = helpStyle
	atlasAgentList.Styles.HelpStyle = helpStyle
	atlasAgentList.Styles.StatusBar = helpStyle
	atlasAgentList.SetShowStatusBar(false)
	atlasAgentList.SetShowPagination(false)

	atlasSurfaceList := list.New(makeAtlasSurfaceItems(), delegate, 42, 3)
	atlasSurfaceList.Title = "Atlas Install"
	atlasSurfaceList.SetShowFilter(false)
	atlasSurfaceList.SetShowHelp(false)
	atlasSurfaceList.Styles.Title = lipgloss.NewStyle().Foreground(primaryText).Bold(true)
	atlasSurfaceList.Styles.PaginationStyle = helpStyle
	atlasSurfaceList.Styles.HelpStyle = helpStyle
	atlasSurfaceList.Styles.StatusBar = helpStyle
	atlasSurfaceList.SetShowStatusBar(false)
	atlasSurfaceList.SetShowPagination(false)

	components := project.DefaultSelectedComponents()
	return model{
		stage:                StageProjectName,
		projectInput:         ti,
		moduleInput:          styledTextInput(),
		pathInput:            pi,
		componentList:        li,
		helpFormatList:       helpFormatList,
		starterKitList:       starterKitList,
		starterKitApplicable: components.WebUI,
		componentLibrary:     true,
		atlasModeList:        atlasModeList,
		atlasAgentList:       atlasAgentList,
		atlasSurfaceList:     atlasSurfaceList,
		atlasMode:            atlasModeRecommended,
		atlasRecommendations: append([]string(nil), atlasAgents.recommended...),
		allowNonEmpty:        options.allowNonEmpty,
		config: project.Config{
			Render: project.RenderConfig{
				GoForjVersion: version.Semver(),
				Components:    components,
				StarterKit:    project.DefaultStarterKit(),
				HelpFormat:    project.DefaultHelpFormat(),
			},
		},
	}
}

// Init satisfies tea.Model and starts cursor blinking for text inputs.
func (m model) Init() tea.Cmd {
	return textinput.Blink
}

// applyComponentSelection commits wizard capabilities to their render-compatible component flags.
func (m *model) applyComponentSelection() {
	*m.components() = m.selectedComponentConfig().WithResolvedDependencies()
	m.starterKitApplicable = m.components().WebUI
	if !m.components().WebUI {
		m.config.Render.StarterKit = project.StarterKitNone
		m.config.Render.StarterKitOptions = nil
	}
}

// applyHelpFormatSelection commits the highlighted formatter before the wizard advances.
func (m *model) applyHelpFormatSelection() {
	index := m.helpFormatList.Index()
	if index < 0 || index >= len(m.helpFormatList.Items()) {
		m.config.Render.HelpFormat = project.DefaultHelpFormat()
		return
	}
	item, ok := m.helpFormatList.Items()[index].(HelpFormatItem)
	if !ok {
		m.config.Render.HelpFormat = project.DefaultHelpFormat()
		return
	}
	m.config.Render.HelpFormat = project.NormalizeHelpFormat(item.Key)
}

// applyStarterKitSelection commits the highlighted kit while honoring the Web UI capability boundary.
func (m *model) applyStarterKitSelection() {
	if !m.config.Render.Components.WebUI {
		m.config.Render.StarterKit = project.StarterKitNone
		m.config.Render.StarterKitOptions = nil
		return
	}
	index := m.starterKitList.Index()
	if index < 0 || index >= len(m.starterKitList.Items()) {
		m.config.Render.StarterKit = project.StarterKitNone
		m.config.Render.StarterKitOptions = nil
		return
	}
	item, ok := m.starterKitList.Items()[index].(StarterKitItem)
	if !ok {
		m.config.Render.StarterKit = project.StarterKitNone
		m.config.Render.StarterKitOptions = nil
		return
	}
	m.config.Render.StarterKit = project.NormalizeStarterKit(item.Key)
	if m.config.Render.StarterKit == project.StarterKitNone || m.componentLibrary {
		m.config.Render.StarterKitOptions = nil
		return
	}
	m.config.Render.StarterKitOptions = project.NewStarterKitOptions(false)
}

// applyExtrasSelection applies Demo's temporary constraints without erasing the unlocked component choices.
func (m *model) applyExtrasSelection() {
	m.demoAppEnabled = m.extrasIndex == 1
	selected := m.selectedComponentConfig()
	if !m.demoAppEnabled {
		selected.DemoApp = false
		selected.ResolveDependencies()
		*m.components() = selected
		m.resetResourcePreview()
		return
	}
	selected.DemoApp = true
	// Demo owns a MySQL-only compatibility contract until its generated SQL supports every database driver.
	selected.CLI = true
	selected.Auth = true
	selected.WebAPI = true
	selected.WebUI = true
	selected.Scheduler = true
	selected.Jobs = true
	selected.DatabaseMySQL = true
	selected.DatabasePostgres = false
	selected.DatabaseSQLite = false
	m.config.Render.StarterKit = project.StarterKitNone
	m.config.Render.StarterKitOptions = nil
	selected.ResolveDependencies()
	*m.components() = selected
	m.resetResourcePreview()
}

// resetResourcePreview invalidates owner-derived state after an earlier wizard choice changes.
func (m *model) resetResourcePreview() {
	m.resourcePreparation = nil
}

// selectedResourcePreparation returns the Path-reconciled renderer handoff or validates the ordinary defaults.
func (m model) selectedResourcePreparation() (newProjectResourcePreparation, error) {
	if m.resourcePreparation != nil {
		return cloneNewProjectResourcePreparation(*m.resourcePreparation), nil
	}
	plan, err := project.DefaultResourcePlan(m.config.Render.Components)
	if err != nil {
		return newProjectResourcePreparation{}, err
	}
	return resolveNewProjectResourcePreparation(plan, m.config.Render.Components, project.LocalServiceIntent{}, nil)
}

// applyAtlasModeSelection centralizes apply atlas mode selection behavior so callers follow the same contract.
func (m *model) applyAtlasModeSelection() {
	index := m.atlasModeList.Index()
	if index < 0 || index >= len(m.atlasModeList.Items()) {
		m.atlasMode = atlasModeRecommended
		return
	}
	item, ok := m.atlasModeList.Items()[index].(AtlasModeItem)
	if !ok {
		m.atlasMode = atlasModeRecommended
		return
	}
	m.atlasMode = item.Mode
}

// atlasInstallOptions centralizes atlas install options behavior so callers follow the same contract.
func (m model) atlasInstallOptions(root string) atlas.InstallOptions {
	surfaces := m.selectedAtlasSurfaces()
	return atlas.InstallOptions{
		Root:          root,
		Agents:        m.selectedAtlasAgents(),
		Guidelines:    &surfaces.guidelines,
		Skills:        &surfaces.skills,
		MCP:           &surfaces.mcp,
		NoInteraction: true,
	}
}

// atlasInstallEnabled centralizes atlas install enabled behavior so callers follow the same contract.
func (m model) atlasInstallEnabled() bool {
	return m.atlasMode != atlasModeSkip
}

// selectedAtlasAgents centralizes selected atlas agents behavior so callers follow the same contract.
func (m model) selectedAtlasAgents() []string {
	if m.atlasMode == atlasModeSkip {
		return nil
	}
	if m.atlasMode != atlasModeCustom {
		return append([]string(nil), m.atlasRecommendations...)
	}
	return m.selectedCustomAtlasAgents()
}

// selectedCustomAtlasAgents centralizes selected custom atlas agents behavior so callers follow the same contract.
func (m model) selectedCustomAtlasAgents() []string {
	names := []string{}
	for _, listItem := range m.atlasAgentList.Items() {
		item, ok := listItem.(AtlasAgentItem)
		if ok && item.Selected {
			names = append(names, item.Name)
		}
	}
	return names
}

type atlasSurfaceSelection struct {
	guidelines bool
	skills     bool
	mcp        bool
}

// any centralizes any behavior so callers follow the same contract.
func (s atlasSurfaceSelection) any() bool {
	return s.guidelines || s.skills || s.mcp
}

// selectedAtlasSurfaces centralizes selected atlas surfaces behavior so callers follow the same contract.
func (m model) selectedAtlasSurfaces() atlasSurfaceSelection {
	switch m.atlasMode {
	case atlasModeSkip:
		return atlasSurfaceSelection{}
	case atlasModeMinimal:
		return atlasSurfaceSelection{guidelines: true}
	case atlasModeRecommended:
		return atlasSurfaceSelection{guidelines: true, skills: true, mcp: true}
	}
	selection := atlasSurfaceSelection{}
	for _, listItem := range m.atlasSurfaceList.Items() {
		item, ok := listItem.(AtlasSurfaceItem)
		if !ok || !item.Selected {
			continue
		}
		switch item.Surface {
		case atlasSurfaceGuidelines:
			selection.guidelines = true
		case atlasSurfaceSkills:
			selection.skills = true
		case atlasSurfaceMCP:
			selection.mcp = true
		}
	}
	return selection
}

// toggleAtlasAgentSelection centralizes toggle atlas agent selection behavior so callers follow the same contract.
func (m *model) toggleAtlasAgentSelection() {
	index := m.atlasAgentList.Index()
	if index < 0 || index >= len(m.atlasAgentList.Items()) {
		return
	}
	item, ok := m.atlasAgentList.Items()[index].(AtlasAgentItem)
	if !ok {
		return
	}
	item.Selected = !item.Selected
	m.atlasAgentList.SetItem(index, item)
}

// toggleAtlasSurfaceSelection centralizes toggle atlas surface selection behavior so callers follow the same contract.
func (m *model) toggleAtlasSurfaceSelection() {
	index := m.atlasSurfaceList.Index()
	if index < 0 || index >= len(m.atlasSurfaceList.Items()) {
		return
	}
	item, ok := m.atlasSurfaceList.Items()[index].(AtlasSurfaceItem)
	if !ok {
		return
	}
	item.Selected = !item.Selected
	m.atlasSurfaceList.SetItem(index, item)
}

// Update advances the project wizard state in response to terminal input.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 && msg.Width < wizardWidth {
			m.termWidth = msg.Width
		} else {
			m.termWidth = wizardWidth
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.cancelled = true
			return m, tea.Quit
		case tea.KeyEsc:
			m.cancelled = true
			return m, tea.Quit
		}

		switch m.stage {
		case StageProjectName:
			switch msg.String() {
			case "enter":
				if strings.TrimSpace(m.projectInput.Value()) == "" {
					m.errorMsg = "Project name is required."
					return m, nil
				}
				m.config.ProjectName = m.projectInput.Value()
				m.stage = StageModuleName
				m.projectInput.Blur()
				m.moduleInput.Placeholder = "github.com/yourname/yourapp"
				m.moduleInput.Focus()
				m.moduleInput.CharLimit = 128
				m.moduleInput.Width = 40
				m.errorMsg = ""
				return m, nil
			}
			var cmd tea.Cmd
			m.projectInput, cmd = m.projectInput.Update(msg)
			return m, cmd

		case StageModuleName:
			switch msg.Type {
			case tea.KeyShiftTab, tea.KeyCtrlB, tea.KeyLeft:
				m.stage = StageProjectName
				m.moduleInput.Blur()
				m.projectInput.Focus()
				return m, nil
			}

			switch msg.String() {
			case "enter":
				modulePath := strings.TrimSpace(m.moduleInput.Value())
				if err := validateNewProjectModulePath(modulePath); err != nil {
					m.errorMsg = err.Error()
					return m, nil
				}
				m.config.GoModuleName = modulePath
				m.stage = StageSelectComponents
				m.moduleInput.Blur()
				m.errorMsg = ""
				return m, nil
			}
			var cmd tea.Cmd
			m.moduleInput, cmd = m.moduleInput.Update(msg)
			return m, cmd

		case StageSelectComponents:
			switch msg.Type {
			case tea.KeyShiftTab, tea.KeyCtrlB, tea.KeyLeft:
				m.stage = StageModuleName
				m.moduleInput.Focus()
				return m, nil
			}

			switch msg.String() {
			case "enter":
				m.applyComponentSelection()
				m.stage = StageHelpFormat
				return m, nil
			case "a":
				m.setAllComponents(true)
				return m, nil
			case "c":
				m.setAllComponents(false)
				return m, nil
			case " ":
				index := m.componentList.Index()
				item := m.componentList.Items()[index].(ListItem)
				if item.Key == project.ComponentCLI {
					return m, nil // prevent toggling CLI
				}
				if item.Selected {
					if blockedMessage, blocked := m.blockedDatabaseDeselectionMessage(item.Key); blocked {
						m.errorMsg = blockedMessage
						return m, nil
					}
				}
				definition, _ := project.ComponentDefinitionByKey(item.Key)
				if !item.Selected && definition.ExclusiveGroup != "" {
					m.deselectExclusiveComponents(item.Key, definition.ExclusiveGroup)
				}
				if item.Selected {
					m.deselectDependentComponents(item.Key)
				}
				item.Selected = !item.Selected
				m.componentList.SetItem(index, item)
				blockedMessage, blocked := m.blockedDeselectionMessage(item.Key, item.Selected)
				m.normalizeComponentSelections()
				if blocked {
					m.errorMsg = blockedMessage
				} else {
					m.errorMsg = ""
				}
				return m, nil
			}
			var cmd tea.Cmd
			m.componentList, cmd = m.componentList.Update(msg)
			return m, cmd

		case StageHelpFormat:
			switch msg.Type {
			case tea.KeyShiftTab, tea.KeyCtrlB, tea.KeyLeft:
				m.stage = StageSelectComponents
				return m, nil
			}
			switch msg.String() {
			case "enter":
				m.applyHelpFormatSelection()
				if m.starterKitApplicable {
					m.stage = StageStarterKit
				} else {
					m.config.Render.StarterKit = project.StarterKitNone
					m.config.Render.StarterKitOptions = nil
					m.stage = StageExtras
				}
				return m, nil
			}
			var cmd tea.Cmd
			m.helpFormatList, cmd = m.helpFormatList.Update(msg)
			m.applyHelpFormatSelection()
			return m, cmd

		case StageStarterKit:
			switch msg.Type {
			case tea.KeyShiftTab, tea.KeyCtrlB, tea.KeyLeft:
				m.stage = StageHelpFormat
				return m, nil
			}
			switch msg.String() {
			case " ":
				if m.highlightedStarterKit() != project.StarterKitNone {
					m.componentLibrary = !m.componentLibrary
				}
				return m, nil
			case "enter":
				m.applyStarterKitSelection()
				m.stage = StageExtras
				return m, nil
			}
			var cmd tea.Cmd
			m.starterKitList, cmd = m.starterKitList.Update(msg)
			return m, cmd

		case StageExtras:
			switch msg.Type {
			case tea.KeyShiftTab, tea.KeyCtrlB, tea.KeyLeft:
				if m.starterKitApplicable {
					m.stage = StageStarterKit
				} else {
					m.stage = StageHelpFormat
				}
				return m, nil
			}
			switch msg.String() {
			case "up", "k":
				m.extrasIndex = 0
				return m, nil
			case "down", "j":
				m.extrasIndex = 1
				return m, nil
			case " ":
				if m.extrasIndex == 1 {
					m.extrasIndex = 0
				} else {
					m.extrasIndex = 1
				}
				return m, nil
			case "enter":
				m.applyExtrasSelection()
				m.errorMsg = ""
				m.stage = StageAtlasSupport
				return m, nil
			}

		case StageAtlasSupport:
			switch msg.Type {
			case tea.KeyShiftTab, tea.KeyCtrlB, tea.KeyLeft:
				m.stage = StageExtras
				return m, nil
			}
			switch msg.String() {
			case "enter":
				m.applyAtlasModeSelection()
				if m.atlasMode == atlasModeCustom {
					m.stage = StageAtlasAgents
				} else {
					m.stage = StageProjectPath
					if m.pathInput.Value() == "" {
						m.pathInput.SetValue(m.defaultTargetPath())
					}
					m.pathInput.Focus()
				}
				return m, nil
			}
			var cmd tea.Cmd
			m.atlasModeList, cmd = m.atlasModeList.Update(msg)
			return m, cmd

		case StageAtlasAgents:
			switch msg.Type {
			case tea.KeyShiftTab, tea.KeyCtrlB, tea.KeyLeft:
				m.stage = StageAtlasSupport
				return m, nil
			}
			switch msg.String() {
			case "enter":
				if len(m.selectedAtlasAgents()) == 0 {
					m.errorMsg = "Select at least one agent or go back and choose Skip."
					return m, nil
				}
				m.errorMsg = ""
				m.stage = StageAtlasSurfaces
				return m, nil
			case " ":
				m.toggleAtlasAgentSelection()
				return m, nil
			}
			var cmd tea.Cmd
			m.atlasAgentList, cmd = m.atlasAgentList.Update(msg)
			return m, cmd

		case StageAtlasSurfaces:
			switch msg.Type {
			case tea.KeyShiftTab, tea.KeyCtrlB, tea.KeyLeft:
				m.stage = StageAtlasAgents
				return m, nil
			}
			switch msg.String() {
			case "enter":
				if !m.selectedAtlasSurfaces().any() {
					m.errorMsg = "Select at least one install option or go back and choose Skip."
					return m, nil
				}
				m.errorMsg = ""
				m.stage = StageProjectPath
				if m.pathInput.Value() == "" {
					m.pathInput.SetValue(m.defaultTargetPath())
				}
				m.pathInput.Focus()
				return m, nil
			case " ":
				m.toggleAtlasSurfaceSelection()
				return m, nil
			}
			var cmd tea.Cmd
			m.atlasSurfaceList, cmd = m.atlasSurfaceList.Update(msg)
			return m, cmd

		case StageProjectPath:
			switch msg.Type {
			case tea.KeyShiftTab, tea.KeyCtrlB, tea.KeyLeft:
				if m.atlasMode == atlasModeCustom {
					m.stage = StageAtlasSurfaces
				} else {
					m.stage = StageAtlasSupport
				}
				return m, nil
			}

			switch msg.String() {
			case "enter":
				if err := m.validatePathInput(); err != nil {
					m.errorMsg = err.Error()
					return m, nil
				}
				m.targetPath = m.projectPath()
				resourcePlan, err := project.DefaultResourcePlan(m.config.Render.Components)
				if err != nil {
					m.errorMsg = fmt.Sprintf("resolve App resources: %v", err)
					return m, nil
				}
				preparation, err := prepareNewProjectTargetResources(
					m.targetPath,
					resourcePlan,
					m.config.Render.Components,
					project.LocalServiceIntent{},
				)
				if err != nil {
					m.errorMsg = err.Error()
					return m, nil
				}
				m.resourcePreparation = &preparation
				m.errorMsg = ""
				m.stage = StageConfirm
				return m, nil
			}
			var cmd tea.Cmd
			m.pathInput, cmd = m.pathInput.Update(msg)
			if err := m.validatePathInput(); err != nil {
				m.errorMsg = err.Error()
			} else {
				m.errorMsg = ""
			}
			return m, cmd

		case StageConfirm:
			switch msg.Type {
			case tea.KeyShiftTab, tea.KeyCtrlB, tea.KeyLeft:
				m.stage = StageProjectPath
				m.pathInput.Focus()
				return m, nil
			}
			switch msg.String() {
			case "enter":
				if err := m.validateBeforeConfirm(); err != nil {
					m.errorMsg = err.Error()
					return m, nil
				}
				m.targetPath = m.projectPath()
				if err := m.finalizeConfig(); err != nil {
					m.errorMsg = err.Error()
					return m, nil
				}
				m.errorMsg = ""
				m.stage = StageDone
				return m, tea.Quit
			}
		}
	}

	return m, nil
}

// View renders the project wizard with stable panel widths for terminal redraws.
func (m model) View() string {
	var panels []string
	var actions []string

	// Project name panel.
	if m.stage == StageProjectName {
		panels = append(panels, m.panelWithTitle("Project Name", lipgloss.JoinVertical(
			lipgloss.Left,
			renderInputLine(m.projectInput),
		), m.termWidth, true))
		actions = []string{"Enter to continue", "Esc to cancel"}
	} else {
		panels = append(panels, m.panelWithTitle("Project Name", normalStyle.Render(m.projectInput.Value()), m.termWidth, false))
	}

	// Module panel.
	if m.stage >= StageModuleName {
		if m.stage == StageModuleName {
			panels = append(panels, m.panelWithTitle("Go Module Path", lipgloss.JoinVertical(
				lipgloss.Left,
				renderInputLine(m.moduleInput),
			), m.termWidth, true))
			actions = []string{"Enter to continue", "Shift+Tab to go back", "Esc to cancel"}
		} else {
			panels = append(panels, m.panelWithTitle("Go Module Path", normalStyle.Render(m.modulePreview()), m.termWidth, false))
		}
	}

	// Components panel.
	if m.stage >= StageSelectComponents {
		componentNames := strings.Join(m.selectedComponentNames(), ", ")
		if strings.TrimSpace(componentNames) == "" {
			componentNames = "CLI"
		}
		if m.stage == StageSelectComponents {
			componentBody := m.renderComponentList(m.termWidth)
			if strings.TrimSpace(m.errorMsg) != "" {
				componentBody = lipgloss.JoinVertical(
					lipgloss.Left,
					componentBody,
					"",
					errorStyle.Render("x "+m.errorMsg),
				)
			}
			panels = append(panels, m.panelWithTitleWithPadding("Components", lipgloss.JoinVertical(
				lipgloss.Left,
				componentBody,
			), m.termWidth, true, 0, 1))
			actions = []string{"Enter to continue", "Shift+Tab to go back", "Esc to cancel", "a: all", "c: clear"}
		} else {
			panels = append(panels, m.panelWithTitle("Components", normalStyle.Render(componentNames), m.termWidth, false))
		}
	}

	// CLI help format panel.
	if m.stage >= StageHelpFormat {
		helpFormatSummary := m.selectedHelpFormatSummary()
		if m.stage == StageHelpFormat {
			panels = append(panels, m.renderHelpFormatPanel())
			actions = []string{"Enter to continue", "Shift+Tab to go back", "Esc to cancel"}
		} else {
			panels = append(panels, m.panelWithTitle("Help Format", normalStyle.Render(helpFormatSummary), m.termWidth, false))
		}
	}

	// Starter kit panel.
	if m.stage >= StageStarterKit && m.starterKitApplicable {
		starterKitSummary := m.selectedStarterKitSummary()
		if m.stage == StageStarterKit {
			panels = append(panels, m.panelWithTitle("Starter Kit", lipgloss.JoinVertical(
				lipgloss.Left,
				m.renderStarterKitList(m.termWidth),
				"",
				m.renderStarterKitOptions(),
			), m.termWidth, true))
			actions = []string{"Space to toggle option", "Enter to continue", "Shift+Tab to go back", "Esc to cancel"}
		} else {
			panels = append(panels, m.panelWithTitle("Starter Kit", normalStyle.Render(starterKitSummary), m.termWidth, false))
		}
	}

	// Extras panel.
	if m.stage >= StageExtras {
		extrasSummary := "Off"
		if m.config.Render.Components.DemoApp {
			extrasSummary = "On (Generate monitoring reference app)"
		}
		if m.stage == StageExtras {
			onSelected := m.extrasIndex == 1
			offSelected := !onSelected
			offMarker := normalStyle.Render("●")
			onMarker := normalStyle.Render("○")
			if onSelected {
				offMarker = normalStyle.Render("○")
				onMarker = normalStyle.Render("●")
			}
			extrasBody := lipgloss.JoinVertical(
				lipgloss.Left,
				func() string {
					label := listNameStyle.Render("Off")
					if offSelected {
						label = listFocusedNameStyle.Render("Off")
					}
					return offMarker + " " + label
				}(),
				func() string {
					label := listNameStyle.Render("On (Generate monitoring reference app)")
					if onSelected {
						label = listFocusedNameStyle.Render("On (Generate monitoring reference app)")
					}
					return onMarker + " " + label
				}(),
			)
			if onSelected {
				extrasBody = lipgloss.JoinVertical(
					lipgloss.Left,
					extrasBody,
					"",
					listDescStyle.Render("Demo currently requires MySQL. Your database choice returns when Demo is turned off."),
				)
			}
			panels = append(panels, m.panelWithTitle("Extras · Demo App", extrasBody, m.termWidth, true))
			actions = []string{"Enter to continue", "Shift+Tab to go back", "Esc to cancel"}
		} else {
			panels = append(panels, m.panelWithTitle("Extras · Demo App", normalStyle.Render(extrasSummary), m.termWidth, false))
		}
	}

	// Atlas panel.
	if m.stage >= StageAtlasSupport {
		switch m.stage {
		case StageAtlasSupport:
			panels = append(panels, m.panelWithTitle("Atlas - Agent Support", lipgloss.JoinVertical(
				lipgloss.Left,
				renderAtlasPanelBanner(),
				"",
				m.renderAtlasModeList(m.termWidth),
				"",
				m.renderAtlasDetectedSummary(),
				"",
				m.renderAtlasInstallSummary(),
			), m.termWidth, true))
			actions = []string{"Enter to continue", "Shift+Tab to go back", "Esc to cancel"}
		case StageAtlasAgents:
			panels = append(panels, m.panelWithTitle("Atlas - Agent Support · Agents", lipgloss.JoinVertical(
				lipgloss.Left,
				renderAtlasPanelBanner(),
				"",
				m.renderAtlasAgentList(m.termWidth),
			), m.termWidth, true))
			actions = []string{"Enter to continue", "Shift+Tab to go back", "Esc to cancel", "space: toggle"}
		case StageAtlasSurfaces:
			panels = append(panels, m.panelWithTitle("Atlas - Agent Support · Install", lipgloss.JoinVertical(
				lipgloss.Left,
				renderAtlasPanelBanner(),
				"",
				m.renderAtlasSurfaceList(m.termWidth),
			), m.termWidth, true))
			actions = []string{"Enter to continue", "Shift+Tab to go back", "Esc to cancel", "space: toggle"}
		default:
			panels = append(panels, m.panelWithTitle("Atlas - Agent Support", normalStyle.Render(m.atlasSummary()), m.termWidth, false))
		}
	}

	// Path panel.
	if m.stage >= StageProjectPath {
		if m.stage == StageProjectPath {
			statusText, statusOK := m.pathStatus()
			if strings.TrimSpace(m.errorMsg) != "" {
				statusText = m.errorMsg
				statusOK = false
			}
			statusLine := statusErrorStyle.Render("x " + statusText)
			if statusOK {
				statusLine = statusOKStyle.Render("✓ " + statusText)
			}
			panels = append(panels, m.panelWithTitle("Project Path", lipgloss.JoinVertical(
				lipgloss.Left,
				renderInputLine(m.pathInput),
				statusLine,
			), m.termWidth, true))
			actions = []string{"Enter to continue", "Shift+Tab to go back", "Esc to cancel"}
		} else {
			panels = append(panels, m.panelWithTitle("Project Path", normalStyle.Render(m.projectPath()), m.termWidth, false))
		}
	}

	// Confirmation panel.
	if m.stage >= StageConfirm {
		componentNames := strings.Join(m.selectedComponentNames(), ", ")
		if strings.TrimSpace(componentNames) == "" {
			componentNames = "CLI"
		}
		rows := []keyValue{
			{"Project", m.projectInput.Value()},
			{"Directory", filepath.Base(m.projectPath())},
			{"Go module", m.modulePreview()},
			{"Path", m.projectPath()},
			{"Demo App", map[bool]string{true: "On", false: "Off"}[m.config.Render.Components.DemoApp]},
			{"Starter kit", m.selectedStarterKitSummary()},
			{"Component library", m.selectedComponentLibrarySummary()},
			{"Agent support", m.atlasSummary()},
			{"Components", componentNames},
		}
		if tools := newProjectDevelopmentToolsSummary(m.config.Render.Components); tools != "" {
			rows = append(rows, keyValue{"Development tools", tools})
		}
		rows = append(rows, keyValue{"Help format", m.selectedHelpFormatSummary()})
		confirmBody := lipgloss.JoinVertical(lipgloss.Left, renderKeyValueTable(rows))
		panels = append(panels, m.panelWithTitle("Confirm your project", confirmBody, m.termWidth, m.stage == StageConfirm))
		if m.stage == StageConfirm {
			actions = []string{"Enter to create", "Shift+Tab to go back", "Esc to cancel"}
		}
	}

	if m.stage == StageDone {
		panels = append(panels, m.panelWithTitle("Configuration complete", successStyle.Render("Project configuration complete. Creating project files..."), m.termWidth, false))
	}

	view := ""
	if len(panels) > 0 {
		view = lipgloss.JoinVertical(lipgloss.Left, panels...)
	}
	if len(actions) > 0 {
		view = lipgloss.JoinVertical(lipgloss.Left, view, renderFooter(actions, m.termWidth))
	}
	if m.errorMsg != "" && m.stage != StageProjectPath && m.stage != StageSelectComponents {
		view = lipgloss.JoinVertical(lipgloss.Left, view, errorStyle.Render(m.errorMsg))
	}
	return view + "\n"
}

// panelWithTitle centralizes panel with title behavior so callers follow the same contract.
func (m model) panelWithTitle(title, content string, termWidth int, active bool) string {
	return m.panelWithTitleWithPadding(title, content, termWidth, active, 1, 1)
}

// panelWithTitleWidth centralizes panel with title width behavior so callers follow the same contract.
func (m model) panelWithTitleWidth(title, content string, width int, active bool) string {
	return m.panelWithTitleWithPadding(title, content, width, active, 1, 1)
}

// panelWithTitleWithPadding centralizes panel with title with padding behavior so callers follow the same contract.
func (m model) panelWithTitleWithPadding(title, content string, termWidth int, active bool, leftPad, rightPad int) string {
	if content == "" {
		content = " "
	}
	if termWidth <= 0 {
		termWidth = wizardWidth
	}
	targetWidth := wizardWidth
	if termWidth < targetWidth {
		targetWidth = termWidth
	}
	if targetWidth < 16 {
		targetWidth = 16
	}

	contentWidth := targetWidth - 2 - leftPad - rightPad
	if contentWidth < 1 {
		contentWidth = 1
	}
	topInnerWidth := targetWidth - 2

	titleLabel := " " + title + " "
	titleFill := topInnerWidth - lipgloss.Width(titleLabel)
	if titleFill < 0 {
		titleFill = 0
	}
	titleStyle := panelTitleDoneStyle
	if active {
		titleStyle = panelTitleActiveStyle
	}
	titleRendered := titleStyle.Render(titleLabel)
	if active && bannerColorsEnabled() {
		titleRendered = colorizeGradientLine(titleLabel, false)
	}
	top := lipgloss.JoinHorizontal(
		lipgloss.Left,
		panelBorderStyle.Render("┌"),
		titleRendered,
		panelBorderStyle.Render(strings.Repeat("─", titleFill)),
		panelBorderStyle.Render("┐"),
	)

	contentBlock := lipgloss.NewStyle().Width(contentWidth).Render(content)
	lines := strings.Split(contentBlock, "\n")
	padded := make([]string, 0, len(lines)+2)
	leftPadding := strings.Repeat(" ", maxInt(0, leftPad))
	rightPadding := strings.Repeat(" ", maxInt(0, rightPad))
	for _, line := range lines {
		lineWidth := lipgloss.Width(line)
		if lineWidth < contentWidth {
			line += strings.Repeat(" ", contentWidth-lineWidth)
		}
		padded = append(padded, panelBorderStyle.Render("│")+leftPadding+line+rightPadding+panelBorderStyle.Render("│"))
	}
	bottom := panelBorderStyle.Render("└" + strings.Repeat("─", topInnerWidth) + "┘")

	box := lipgloss.JoinVertical(
		lipgloss.Left,
		top,
		lipgloss.JoinVertical(lipgloss.Left, padded...),
		bottom,
		"",
	)
	return lipgloss.NewStyle().MarginLeft(1).Render(box)
}

// renderComponentList keeps the render component list representation consistent.
func (m model) renderComponentList(termWidth int) string {
	items := m.componentList.Items()
	if len(items) == 0 {
		return ""
	}

	var rows []string
	for i, listItem := range m.componentList.Items() {
		item := listItem.(ListItem)
		isFocused := m.componentList.Index() == i
		caret := "  "
		if isFocused {
			caret = titleIndicatorStyle.Render("› ")
		}

		marker := normalStyle.Render("○")
		if item.Selected {
			marker = successStyle.Render("●")
		}
		if isFocused {
			glyph := "○"
			if item.Selected {
				glyph = "●"
			}
			marker = lipgloss.NewStyle().Foreground(accentColor).Render(glyph)
		}

		labelStyle := listOptionMutedStyle
		if item.Selected {
			labelStyle = listNameStyle
		}
		if isFocused {
			labelStyle = listFocusedNameStyle
		}
		descStyle := listDescStyle
		if isFocused {
			descStyle = listFocusedDescStyle
		}
		indent := ""
		if definition, ok := project.ComponentDefinitionByKey(item.Key); ok && definition.Parent != "" {
			indent = " "
		}
		label := labelStyle.Render(item.Name)
		line := indent + caret + marker + " " + label
		if strings.TrimSpace(item.Desc) != "" {
			line += " " + descStyle.Render("· "+item.Desc)
		}
		rows = append(rows, line)
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// renderStarterKitList keeps the render starter kit list representation consistent.
func (m model) renderStarterKitList(termWidth int) string {
	items := m.starterKitList.Items()
	if len(items) == 0 {
		return ""
	}

	var rows []string
	for i, listItem := range items {
		item, ok := listItem.(StarterKitItem)
		if !ok {
			continue
		}
		isFocused := m.starterKitList.Index() == i
		caret := "  "
		if isFocused {
			caret = titleIndicatorStyle.Render("› ")
		}

		marker := normalStyle.Render("○")
		if isFocused {
			marker = lipgloss.NewStyle().Foreground(accentColor).Render("●")
		}

		labelStyle := listOptionMutedStyle
		if isFocused {
			labelStyle = listFocusedNameStyle
		}
		descStyle := listDescStyle
		if isFocused {
			descStyle = listFocusedDescStyle
		}
		line := caret + marker + " " + labelStyle.Render(item.Label)
		if strings.TrimSpace(item.Desc) != "" {
			line += " " + descStyle.Render("· "+item.Desc)
		}
		rows = append(rows, line)
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// renderStarterKitOptions renders kit-specific choices as radio inputs that can grow into additional option rows.
func (m model) renderStarterKitOptions() string {
	if m.highlightedStarterKit() == project.StarterKitNone {
		return helpStyle.Render("Options · Not applicable")
	}

	lines := []string{normalStyle.Render("Options")}
	options := []starterKitOptionRow{
		{Label: "Component library", Enabled: m.componentLibrary},
	}
	for _, option := range options {
		onMarker := normalStyle.Render("○")
		offMarker := listFocusedNameStyle.Render("●")
		if option.Enabled {
			onMarker = listFocusedNameStyle.Render("●")
			offMarker = normalStyle.Render("○")
		}
		lines = append(lines,
			"  "+normalStyle.Render(option.Label),
			"    "+onMarker+" On",
			"    "+offMarker+" Off",
		)
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// renderAtlasModeList keeps the render atlas mode list representation consistent.
func (m model) renderAtlasModeList(termWidth int) string {
	items := m.atlasModeList.Items()
	if len(items) == 0 {
		return ""
	}
	rows := []string{}
	for i, listItem := range items {
		item, ok := listItem.(AtlasModeItem)
		if !ok {
			continue
		}
		isFocused := m.atlasModeList.Index() == i
		caret := "  "
		if isFocused {
			caret = titleIndicatorStyle.Render("› ")
		}
		marker := normalStyle.Render("○")
		if isFocused {
			marker = lipgloss.NewStyle().Foreground(accentColor).Render("●")
		}
		labelStyle := listOptionMutedStyle
		descStyle := listDescStyle
		if isFocused {
			labelStyle = listFocusedNameStyle
			descStyle = listFocusedDescStyle
		}
		line := caret + marker + " " + labelStyle.Render(item.Label)
		if strings.TrimSpace(item.Desc) != "" {
			line += " " + descStyle.Render("· "+item.Desc)
		}
		rows = append(rows, line)
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// renderAtlasAgentList keeps the render atlas agent list representation consistent.
func (m model) renderAtlasAgentList(termWidth int) string {
	items := m.atlasAgentList.Items()
	if len(items) == 0 {
		return ""
	}
	rows := []string{}
	for i, listItem := range items {
		item, ok := listItem.(AtlasAgentItem)
		if !ok {
			continue
		}
		isFocused := m.atlasAgentList.Index() == i
		caret := "  "
		if isFocused {
			caret = titleIndicatorStyle.Render("› ")
		}
		marker := normalStyle.Render("○")
		if item.Selected {
			marker = successStyle.Render("●")
		}
		if isFocused {
			glyph := "○"
			if item.Selected {
				glyph = "●"
			}
			marker = lipgloss.NewStyle().Foreground(accentColor).Render(glyph)
		}
		labelStyle := listOptionMutedStyle
		if item.Selected {
			labelStyle = listNameStyle
		}
		if isFocused {
			labelStyle = listFocusedNameStyle
		}
		desc := "available"
		if item.Detected {
			desc = "detected"
		}
		line := caret + marker + " " + labelStyle.Render(item.DisplayName) + " " + listDescStyle.Render("· "+desc)
		rows = append(rows, line)
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// renderAtlasSurfaceList keeps the render atlas surface list representation consistent.
func (m model) renderAtlasSurfaceList(termWidth int) string {
	items := m.atlasSurfaceList.Items()
	if len(items) == 0 {
		return ""
	}
	rows := []string{}
	for i, listItem := range items {
		item, ok := listItem.(AtlasSurfaceItem)
		if !ok {
			continue
		}
		isFocused := m.atlasSurfaceList.Index() == i
		caret := "  "
		if isFocused {
			caret = titleIndicatorStyle.Render("› ")
		}
		marker := normalStyle.Render("○")
		if item.Selected {
			marker = successStyle.Render("●")
		}
		if isFocused {
			glyph := "○"
			if item.Selected {
				glyph = "●"
			}
			marker = lipgloss.NewStyle().Foreground(accentColor).Render(glyph)
		}
		labelStyle := listOptionMutedStyle
		if item.Selected {
			labelStyle = listNameStyle
		}
		if isFocused {
			labelStyle = listFocusedNameStyle
		}
		line := caret + marker + " " + labelStyle.Render(item.Label)
		if strings.TrimSpace(item.Desc) != "" {
			line += " " + listDescStyle.Render("· "+item.Desc)
		}
		rows = append(rows, line)
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// renderHelpFormatPanel keeps formatter previews adjacent to the selected option.
func (m model) renderHelpFormatPanel() string {
	selection := m.panelWithTitleWidth("Help Format", m.renderHelpFormatList(), m.termWidth, true)
	highlighted := m.highlightedHelpFormat()
	if m.termWidth >= 118 {
		gap := 2
		available := m.termWidth - gap*2
		leftWidth := available / 3
		middleWidth := available / 3
		rightWidth := available - leftWidth - middleWidth
		previews := lipgloss.JoinHorizontal(
			lipgloss.Top,
			m.panelWithTitleWidth("Framework Preview", renderHelpPreview(project.HelpFormatFramework, leftWidth-6), leftWidth, highlighted == project.HelpFormatFramework),
			strings.Repeat(" ", gap),
			m.panelWithTitleWidth("Guided Preview", renderHelpPreview(project.HelpFormatGuided, middleWidth-6), middleWidth, highlighted == project.HelpFormatGuided),
			strings.Repeat(" ", gap),
			m.panelWithTitleWidth("External CLI Preview", renderHelpPreview(project.HelpFormatExternalCLI, rightWidth-6), rightWidth, highlighted == project.HelpFormatExternalCLI),
		)
		return lipgloss.JoinVertical(lipgloss.Left, selection, previews)
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		selection,
		m.panelWithTitleWidth("Preview", renderHelpPreview(highlighted, m.termWidth-6), m.termWidth, true),
	)
}

// renderHelpFormatList mirrors radio behavior because the Bubble list chrome is hidden.
func (m model) renderHelpFormatList() string {
	var rows []string
	for i, listItem := range m.helpFormatList.Items() {
		item := listItem.(HelpFormatItem)
		isFocused := m.helpFormatList.Index() == i
		caret := "  "
		if isFocused {
			caret = progressCurrentStyle.Render("› ")
		}
		labelStyle := listNameStyle
		if isFocused {
			labelStyle = listFocusedNameStyle
		}
		marker := normalStyle.Render("○")
		if project.NormalizeHelpFormat(item.Key) == project.NormalizeHelpFormat(m.config.Render.HelpFormat) {
			marker = normalStyle.Render("●")
		}
		line := caret + marker + " " + labelStyle.Render(item.Label)
		if strings.TrimSpace(item.Desc) != "" {
			line += " " + listDescStyle.Render("· "+item.Desc)
		}
		rows = append(rows, line)
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// highlightedHelpFormat reports the formatter currently under the cursor for live previews.
func (m model) highlightedHelpFormat() project.HelpFormat {
	index := m.helpFormatList.Index()
	if index < 0 || index >= len(m.helpFormatList.Items()) {
		return project.DefaultHelpFormat()
	}
	item, ok := m.helpFormatList.Items()[index].(HelpFormatItem)
	if !ok {
		return project.DefaultHelpFormat()
	}
	return project.NormalizeHelpFormat(item.Key)
}

// selectedHelpFormatSummary renders the committed or previewed formatter label for summary panels.
func (m model) selectedHelpFormatSummary() string {
	helpFormat := project.NormalizeHelpFormat(m.config.Render.HelpFormat)
	if m.stage == StageHelpFormat {
		helpFormat = m.highlightedHelpFormat()
	}
	if definition, ok := project.HelpFormatDefinitionByKey(helpFormat); ok {
		return definition.Label
	}
	return "Framework"
}

// renderHelpPreview clips real formatter output so wide examples fit inside wizard panels.
func renderHelpPreview(format project.HelpFormat, width int) string {
	preview := konghelp.Preview(string(project.NormalizeHelpFormat(format)))
	lines := strings.Split(preview, "\n")
	for i, line := range lines {
		lines[i] = lipgloss.NewStyle().MaxWidth(maxInt(20, width)).Render(line)
	}
	return strings.Join(lines, "\n")
}

// highlightedStarterKit returns the committed or previewed starter-kit selection.
func (m model) highlightedStarterKit() project.StarterKit {
	if !m.config.Render.Components.WebUI || m.config.Render.Components.DemoApp {
		return project.StarterKitNone
	}
	starterKit := project.NormalizeStarterKit(m.config.Render.StarterKit)
	if m.stage == StageStarterKit {
		index := m.starterKitList.Index()
		if index >= 0 && index < len(m.starterKitList.Items()) {
			if item, ok := m.starterKitList.Items()[index].(StarterKitItem); ok {
				starterKit = project.NormalizeStarterKit(item.Key)
			}
		}
	}
	return starterKit
}

// selectedStarterKitSummary centralizes selected starter kit summary behavior so callers follow the same contract.
func (m model) selectedStarterKitSummary() string {
	starterKit := m.highlightedStarterKit()
	if definition, ok := project.StarterKitDefinitionByKey(starterKit); ok {
		return definition.Label
	}
	return "None"
}

// selectedComponentLibrarySummary reports whether the selected starter kit includes its showcase.
func (m model) selectedComponentLibrarySummary() string {
	if m.highlightedStarterKit() == project.StarterKitNone {
		return "Not applicable"
	}
	if m.componentLibrary {
		return "On"
	}
	return "Off"
}

// renderAtlasDetectedSummary keeps the render atlas detected summary representation consistent.
func (m model) renderAtlasDetectedSummary() string {
	detected := []string{}
	for _, listItem := range m.atlasAgentList.Items() {
		item, ok := listItem.(AtlasAgentItem)
		if ok && item.Detected {
			detected = append(detected, item.DisplayName)
		}
	}
	if len(detected) == 0 {
		return labelKeyStyle.Render("Detected") + " " + labelSepStyle.Render("»") + " " + normalStyle.Render("None")
	}
	return labelKeyStyle.Render("Detected") + " " + labelSepStyle.Render("»") + " " + normalStyle.Render(strings.Join(detected, ", "))
}

// renderAtlasInstallSummary keeps the render atlas install summary representation consistent.
func (m model) renderAtlasInstallSummary() string {
	if m.previewAtlasMode() == atlasModeSkip {
		return labelKeyStyle.Render("Will install") + " " + labelSepStyle.Render("»") + " " + normalStyle.Render("Nothing")
	}
	agents := m.previewAtlasAgents()
	surfaces := m.previewAtlasSurfaceNames()
	if len(agents) == 0 || len(surfaces) == 0 {
		return labelKeyStyle.Render("Will install") + " " + labelSepStyle.Render("»") + " " + normalStyle.Render("Nothing")
	}
	rows := []string{labelKeyStyle.Render("Will install") + " " + labelSepStyle.Render("»")}
	for _, agent := range agents {
		rows = append(rows, "  "+normalStyle.Render(atlas.DisplayName(agent)+": "+strings.Join(surfaces, ", ")))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// atlasSummary centralizes atlas summary behavior so callers follow the same contract.
func (m model) atlasSummary() string {
	if m.atlasMode == atlasModeSkip {
		return "Skip"
	}
	agents := m.selectedAtlasAgents()
	surfaces := m.selectedAtlasSurfaceNames()
	if len(agents) == 0 || len(surfaces) == 0 {
		return "Skip"
	}
	display := make([]string, 0, len(agents))
	for _, agent := range agents {
		display = append(display, atlas.DisplayName(agent))
	}
	return strings.Join(display, ", ") + " · " + strings.Join(surfaces, ", ")
}

// previewAtlasMode centralizes preview atlas mode behavior so callers follow the same contract.
func (m model) previewAtlasMode() atlasMode {
	index := m.atlasModeList.Index()
	if index < 0 || index >= len(m.atlasModeList.Items()) {
		return atlasModeRecommended
	}
	item, ok := m.atlasModeList.Items()[index].(AtlasModeItem)
	if !ok {
		return atlasModeRecommended
	}
	return item.Mode
}

// previewAtlasAgents centralizes preview atlas agents behavior so callers follow the same contract.
func (m model) previewAtlasAgents() []string {
	mode := m.previewAtlasMode()
	if mode == atlasModeSkip {
		return nil
	}
	if mode != atlasModeCustom {
		return append([]string(nil), m.atlasRecommendations...)
	}
	return m.selectedCustomAtlasAgents()
}

// previewAtlasSurfaceNames centralizes preview atlas surface names behavior so callers follow the same contract.
func (m model) previewAtlasSurfaceNames() []string {
	switch m.previewAtlasMode() {
	case atlasModeSkip:
		return nil
	case atlasModeMinimal:
		return []string{"guidelines"}
	case atlasModeRecommended:
		return []string{"guidelines", "skills", "MCP"}
	default:
		return m.selectedAtlasSurfaceNames()
	}
}

// selectedAtlasSurfaceNames centralizes selected atlas surface names behavior so callers follow the same contract.
func (m model) selectedAtlasSurfaceNames() []string {
	selection := m.selectedAtlasSurfaces()
	names := []string{}
	if selection.guidelines {
		names = append(names, "guidelines")
	}
	if selection.skills {
		names = append(names, "skills")
	}
	if selection.mcp {
		names = append(names, "MCP")
	}
	return names
}

// selectedComponentNames reports effective render choices so temporary Demo constraints are described truthfully.
func (m model) selectedComponentNames() []string {
	components := m.config.Render.Components
	definitions := project.ProjectWizardComponentDefinitions()
	comps := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		if components.Enabled(definition.Key) {
			comps = append(comps, definition.Label)
		}
	}
	return comps
}

// renderInputLine keeps the render input line representation consistent.
func renderInputLine(input textinput.Model) string {
	view := input.View()
	width := input.Width
	if width < lipgloss.Width(view) {
		width = lipgloss.Width(view)
	}
	if width < 24 {
		width = 24
	}
	padding := strings.Repeat(" ", width-lipgloss.Width(view))
	return view + padding
}

// indentBlock centralizes indent block behavior so callers follow the same contract.
func indentBlock(content string, padLeft int) string {
	if padLeft <= 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	pad := strings.Repeat(" ", padLeft)
	for i, line := range lines {
		lines[i] = pad + line
	}
	return strings.Join(lines, "\n")
}

type keyValue struct {
	key   string
	value string
}

// renderKeyValueTable keeps the render key value table representation consistent.
func renderKeyValueTable(rows []keyValue) string {
	if len(rows) == 0 {
		return ""
	}
	longestKey := 0
	normalized := make([]keyValue, 0, len(rows))
	for _, row := range rows {
		key := strings.TrimSpace(row.key)
		value := strings.TrimSpace(row.value)
		if key == "" {
			continue
		}
		if value == "" {
			value = "<pending>"
		}
		if w := lipgloss.Width(key); w > longestKey {
			longestKey = w
		}
		normalized = append(normalized, keyValue{key: key, value: value})
	}
	if len(normalized) == 0 {
		return ""
	}

	var lines []string
	for _, row := range normalized {
		key := labelKeyStyle.Render(fmt.Sprintf("%*s", longestKey, row.key))
		value := normalStyle.Render(row.value)
		separator := labelSepStyle.Render("»")
		lines = append(lines, key+" "+separator+" "+value)
	}
	table := indentBlock(lipgloss.JoinVertical(lipgloss.Left, lines...), 2)
	return table
}

// styledTextInput centralizes styled text input behavior so callers follow the same contract.
func styledTextInput() textinput.Model {
	base := lipgloss.NewStyle().Foreground(primaryText)
	ti := textinput.New()
	ti.Prompt = ""
	ti.PromptStyle = base
	ti.TextStyle = base
	ti.PlaceholderStyle = helpStyle
	ti.CursorStyle = base
	ti.Width = 34
	return ti
}

// renderProgress reflects the conditional route actually traversed so Back and progress never disagree.
func (m model) renderProgress() string {
	steps := []struct {
		label string
		stage WizardStage
	}{
		{"Project", StageProjectName},
		{"Module", StageModuleName},
		{"Components", StageSelectComponents},
	}
	steps = append(steps, struct {
		label string
		stage WizardStage
	}{"Help", StageHelpFormat})
	if m.starterKitApplicable {
		steps = append(steps, struct {
			label string
			stage WizardStage
		}{"Starter", StageStarterKit})
	}
	steps = append(steps,
		struct {
			label string
			stage WizardStage
		}{"Extras", StageExtras},
		struct {
			label string
			stage WizardStage
		}{"Atlas", StageAtlasSupport},
		struct {
			label string
			stage WizardStage
		}{"Path", StageProjectPath},
		struct {
			label string
			stage WizardStage
		}{"Confirm", StageConfirm},
	)

	var parts []string
	for _, step := range steps {
		label := step.label
		prefix := "•"
		prefixStyle := progressPendingStyle
		labelStyle := progressPendingStyle

		switch {
		case m.stage > step.stage:
			prefix = "✔"
			prefixStyle = progressDoneMark
			labelStyle = progressDoneLabel
		case m.stage == step.stage:
			prefix = "▸"
			prefixStyle = progressCurrentStyle
			labelStyle = progressCurrentStyle
		}

		parts = append(parts, prefixStyle.Render(prefix)+" "+labelStyle.Render(label))
	}

	return strings.Join(parts, " ")
}

// setAllComponents changes projected capabilities while retaining the current database implementation.
func (m *model) setAllComponents(selected bool) {
	databaseKey := m.selectedDatabaseComponentKey()
	for idx, listItem := range m.componentList.Items() {
		item := listItem.(ListItem)
		if item.Name == "CLI" {
			item.Selected = true
			m.componentList.SetItem(idx, item)
			continue
		}
		if project.IsAppDatabaseComponent(item.Key) {
			item.Selected = selected && item.Key == databaseKey
			m.componentList.SetItem(idx, item)
			continue
		}
		item.Selected = selected
		m.componentList.SetItem(idx, item)
	}
	m.normalizeComponentSelections()
}

// selectedDatabaseComponentKey keeps an explicit engine selected and otherwise returns the catalog default.
func (m model) selectedDatabaseComponentKey() project.ComponentKey {
	for _, listItem := range m.componentList.Items() {
		item := listItem.(ListItem)
		if item.Selected && project.IsAppDatabaseComponent(item.Key) {
			return item.Key
		}
	}
	for _, definition := range project.ComponentCatalog() {
		if definition.DefaultSelected && project.IsAppDatabaseComponent(definition.Key) {
			return definition.Key
		}
	}
	return project.ComponentDatabaseMySQL
}

// setComponentSelected updates one concrete wizard component row.
func (m *model) setComponentSelected(key project.ComponentKey, selected bool) {
	for idx, listItem := range m.componentList.Items() {
		item := listItem.(ListItem)
		if item.Key != key {
			continue
		}
		item.Selected = selected
		m.componentList.SetItem(idx, item)
		return
	}
}

// deselectExclusiveComponents centralizes deselect exclusive components behavior so callers follow the same contract.
func (m *model) deselectExclusiveComponents(selectedKey project.ComponentKey, group string) {
	for _, definition := range project.ComponentCatalog() {
		if definition.Key == selectedKey || definition.ExclusiveGroup != group {
			continue
		}
		m.setComponentSelected(definition.Key, false)
	}
}

// deselectDependentComponents removes transitive children because leaving them selected would create invalid late-stage state.
func (m *model) deselectDependentComponents(key project.ComponentKey) {
	changed := true
	disabled := map[project.ComponentKey]bool{key: true}
	for changed {
		changed = false
		for _, definition := range project.ComponentCatalog() {
			if disabled[definition.Key] {
				continue
			}
			if definition.Parent == "" {
				continue
			}
			if !disabled[definition.Parent] {
				continue
			}
			disabled[definition.Key] = true
			changed = true
		}
	}
	for dependent := range disabled {
		if dependent == key {
			continue
		}
		m.setComponentSelected(dependent, false)
	}
}

// blockedDeselectionMessage explains when normalization restores a required capability.
func (m *model) blockedDeselectionMessage(key project.ComponentKey, nowSelected bool) (string, bool) {
	if nowSelected {
		return "", false
	}
	current := m.selectedComponentConfig()
	resolved := current.WithResolvedDependencies()
	if !resolved.Enabled(key) {
		return "", false
	}

	definition, ok := project.ComponentDefinitionByKey(key)
	if !ok {
		return "That component is still required by another selected component.", true
	}

	var blockers []string
	for _, candidate := range project.ComponentCatalog() {
		if !current.Enabled(candidate.Key) {
			continue
		}
		for _, required := range candidate.Requires {
			if required == key {
				blockers = append(blockers, candidate.Label)
				break
			}
		}
	}

	if len(blockers) == 0 {
		return fmt.Sprintf("%s remains enabled because another selected component requires it.", definition.Label), true
	}
	return fmt.Sprintf("%s remains enabled because %s requires it.", definition.Label, strings.Join(blockers, ", ")), true
}

// blockedDatabaseDeselectionMessage prevents required capabilities from losing their last concrete database.
func (m model) blockedDatabaseDeselectionMessage(key project.ComponentKey) (string, bool) {
	if !project.IsAppDatabaseComponent(key) {
		return "", false
	}
	for _, listItem := range m.componentList.Items() {
		item := listItem.(ListItem)
		if item.Key != key && item.Selected && project.IsAppDatabaseComponent(item.Key) {
			return "", false
		}
	}
	blockers := m.databaseCapabilityBlockers()
	if len(blockers) == 0 {
		return "", false
	}
	return fmt.Sprintf("Database remains enabled because %s requires it.", strings.Join(blockers, " or ")), true
}

// databaseCapabilityBlockers returns wizard selections that require a database without prescribing its driver.
func (m model) databaseCapabilityBlockers() []string {
	blockers := make([]string, 0, 3)
	for _, listItem := range m.componentList.Items() {
		item := listItem.(ListItem)
		if !item.Selected {
			continue
		}
		switch item.Key {
		case project.ComponentAuth:
			blockers = append(blockers, "Auth")
		case project.ComponentOAuth:
			blockers = append(blockers, "OAuth")
		}
	}
	if m.demoAppEnabled {
		blockers = append(blockers, "Demo App")
	}
	return blockers
}

// selectedComponentConfig expands concrete wizard rows into render flags and supplies a database required by auth.
func (m *model) selectedComponentConfig() project.Components {
	components := m.config.Render.Components
	for _, definition := range project.ProjectWizardComponentDefinitions() {
		components.SetEnabled(definition.Key, false)
	}
	for _, item := range m.componentList.Items() {
		it := item.(ListItem)
		if !it.Selected {
			continue
		}
		components.SetEnabled(it.Key, true)
	}
	if (components.Auth || components.OAuth || m.demoAppEnabled) && !components.HasDatabase() {
		components.SetEnabled(m.selectedDatabaseComponentKey(), true)
	}
	return components
}

// normalizeComponentSelections reflects resolved dependencies through concrete wizard rows.
func (m *model) normalizeComponentSelections() {
	components := m.selectedComponentConfig().WithResolvedDependencies()
	for idx, listItem := range m.componentList.Items() {
		item := listItem.(ListItem)
		item.Selected = components.Enabled(item.Key)
		m.componentList.SetItem(idx, item)
	}
}

// projectSlug centralizes project slug behavior so callers follow the same contract.
func (m model) projectSlug() string {
	name := strings.TrimSpace(m.projectInput.Value())
	if name == "" {
		return "<pending>"
	}
	name = strings.ToLower(name)
	parts := strings.Fields(name)
	slug := strings.Join(parts, "-")
	return slug
}

// modulePreview centralizes module preview behavior so callers follow the same contract.
func (m model) modulePreview() string {
	if val := strings.TrimSpace(m.moduleInput.Value()); val != "" {
		return val
	}
	slug := m.projectSlug()
	if slug == "<pending>" {
		return "github.com/you/<module>"
	}
	return "github.com/you/" + slug
}

// defaultTargetPath centralizes default target path behavior so callers follow the same contract.
func (m model) defaultTargetPath() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	entries, err := os.ReadDir(wd)
	if err == nil && len(entries) == 0 {
		return wd
	}
	slug := m.projectSlug()
	if slug == "<pending>" || slug == "" {
		return wd
	}
	return filepath.Join(wd, slug)
}

// projectPath centralizes project path behavior so callers follow the same contract.
func (m model) projectPath() string {
	input := strings.TrimSpace(m.pathInput.Value())
	if input == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "<unknown>"
		}
		// suggest cwd if empty, otherwise cwd/slug
		entries, err := os.ReadDir(wd)
		if err == nil && len(entries) == 0 {
			return wd
		}
		slug := m.projectSlug()
		if slug == "<pending>" || slug == "" {
			return wd
		}
		return filepath.Join(wd, slug)
	}

	if filepath.IsAbs(input) {
		return filepath.Clean(input)
	}

	wd, err := os.Getwd()
	if err != nil {
		return filepath.Clean(input)
	}
	return filepath.Clean(filepath.Join(wd, input))
}

// validateBeforeConfirm blocks confirmation until path, resource, and service contracts describe one renderable project.
func (m model) validateBeforeConfirm() error {
	if strings.TrimSpace(m.projectInput.Value()) == "" {
		return fmt.Errorf("Project name is required.")
	}
	if err := validateNewProjectModulePath(m.moduleInput.Value()); err != nil {
		return err
	}

	if err := m.validatePathInput(); err != nil {
		return err
	}
	_, err := m.selectedResourcePreparation()
	if err != nil {
		return fmt.Errorf("App resources are invalid: %w", err)
	}

	return nil
}

// validatePathInput centralizes validate path input behavior so callers follow the same contract.
func (m model) validatePathInput() error {
	target := m.projectPath()
	info, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // will create
		}
		return fmt.Errorf("Cannot stat target path: %v", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("Target path is not a directory: %s", target)
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return fmt.Errorf("Cannot read target path: %v", err)
	}
	if len(entries) > 0 {
		if m.allowNonEmpty {
			return nil
		}
		return fmt.Errorf("Target path is not empty: %s", target)
	}
	return nil
}

// pathStatus centralizes path status behavior so callers follow the same contract.
func (m model) pathStatus() (string, bool) {
	target := m.projectPath()
	info, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return "Path does not exist. It will be created.", true
		}
		return "Cannot stat target path.", false
	}
	if !info.IsDir() {
		return "Path is not a directory.", false
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return "Cannot read target path.", false
	}
	if len(entries) > 0 {
		if m.allowNonEmpty {
			return "Path exists and is not empty. Existing files will be preserved.", true
		}
		return "Path is not empty.", false
	}
	return "Path exists and is empty.", true
}

// ensureNewProjectConfigCanBeWritten prevents `forj new` from reinitializing an existing GoForj project.
func ensureNewProjectConfigCanBeWritten(configPath string) error {
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("target already contains .goforj.yml: %s", configPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("cannot stat .goforj.yml: %w", err)
	}
	return nil
}

// renderFooter keeps the action bar within the current terminal without shrinking the wizard's normal visual rhythm.
func renderFooter(actions []string, termWidth int) string {
	line := strings.Join(actions, " · ")
	width := lipgloss.Width(line)
	if width < wizardWidth {
		width = wizardWidth
	}
	if termWidth <= 0 {
		termWidth = wizardWidth
	}
	if termWidth < width {
		width = termWidth
	}
	bar := panelBorderStyle.Render(strings.Repeat("─", width))
	return lipgloss.JoinVertical(lipgloss.Left, bar, panelBorderStyle.Render(line))
}

// newProjectDevelopmentToolsSummary lists generated Compose tooling separately from App services.
func newProjectDevelopmentToolsSummary(components project.Components) string {
	tools := []string{}
	if components.Mail && components.Docker {
		tools = append(tools, "Mailpit")
	}
	if components.Observability && components.Docker {
		tools = append(tools, "VictoriaMetrics")
	}
	if components.Grafana && components.Docker {
		tools = append(tools, "Grafana")
	}
	return strings.Join(tools, " · ")
}

// renderAtlasPanelBanner keeps the render atlas panel banner representation consistent.
func renderAtlasPanelBanner() string {
	lines := []string{
		"   _  _____ _      _   ___ ",
		"  /_\\|_   _| |    /_\\ / __|",
		" / _ \\ | | | |__ / _ \\\\__ \\",
		"/_/ \\_\\|_| |____/_/ \\_\\___/",
	}
	for i, line := range lines {
		lines[i] = colorizeGradientLine(line, true)
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// packageJSONHasNpmDev checks whether the target-local frontend defines an npm dev script.
func packageJSONHasNpmDev(root string) bool {
	if strings.TrimSpace(root) == "" {
		return false
	}
	path := filepath.Join(root, projectlayout.FrontendDir(".", project.DefaultApp()), "package.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	var pkg map[string]interface{}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false
	}

	scripts, ok := pkg["scripts"].(map[string]interface{})
	if !ok {
		return false
	}

	_, exists := scripts["dev"]
	return exists
}

// NewProjectCmd owns the interactive project creation flow.
type NewProjectCmd struct {
	// AllowNonEmpty lets advanced users intentionally initialize inside a directory that already has files.
	AllowNonEmpty bool `name:"allow-non-empty" help:"Allow creating a project in a non-empty directory"`

	logger   *logger.AppLogger
	renderer *ProjectRenderer
}

// NewNewProjectCmd creates a project creation command.
func NewNewProjectCmd(logger *logger.AppLogger, renderer *ProjectRenderer) *NewProjectCmd {
	return &NewProjectCmd{
		logger:   logger,
		renderer: renderer,
	}
}

// Signature exposes the project wizard as the `forj new` command.
func (*NewProjectCmd) Signature() string {
	return `name:"new" help:"New project command"`
}

// Run starts the project wizard and renders the selected project into the target directory.
func (c *NewProjectCmd) Run() error {
	printNewProjectBanner()

	resultModel, err := tea.NewProgram(initialModelWithOptions(newProjectModelOptions{
		allowNonEmpty: c.AllowNonEmpty,
	})).Run()
	if err != nil {
		return fmt.Errorf("run project wizard: %w", err)
	}

	m, ok := resultModel.(model)
	if !ok {
		return fmt.Errorf("failed to capture wizard model")
	}

	if m.cancelled {
		return nil
	}
	return c.createProject(m)
}

// createProject publishes one completed wizard model without changing process-wide working-directory state.
func (c *NewProjectCmd) createProject(m model) error {
	if m.stage != StageDone {
		return fmt.Errorf("project wizard must be completed before creation")
	}
	if err := validateNewProjectModulePath(m.config.GoModuleName); err != nil {
		return err
	}
	var targetPath string
	targetPath = m.targetPath
	if targetPath == "" {
		targetPath = m.projectPath()
	}
	if targetPath == "" {
		return fmt.Errorf("target path could not be determined")
	}
	targetPath, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("resolve target path: %w", err)
	}

	if err := os.MkdirAll(targetPath, 0755); err != nil {
		return fmt.Errorf("failed to create target path: %w", err)
	}

	// StageConfirm finalized the model before the wizard returned it for publication.
	m.targetPath = targetPath

	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(m.config); err != nil {
		return fmt.Errorf("failed to encode .goforj.yml: %w", err)
	}
	configPath := filepath.Join(targetPath, ".goforj.yml")
	if err := ensureNewProjectConfigCanBeWritten(configPath); err != nil {
		return err
	}
	if writeErr := os.WriteFile(configPath, buf.Bytes(), 0644); writeErr != nil {
		return fmt.Errorf("failed to write .goforj.yml: %w", writeErr)
	}

	// project renderer
	preparation, err := m.selectedResourcePreparation()
	if err != nil {
		return fmt.Errorf("resolve App resources: %w", err)
	}
	i := ComponentRenderInput{
		renderAll:          true,
		initializeProject:  true,
		root:               targetPath,
		resourcePlan:       preparation.plan,
		localServiceIntent: preparation.serviceIntent,
		serviceConsumers:   preparation.serviceConsumers,
	}
	err = runWithLoader("Rendering project files", func() error {
		return c.renderer.Render(i)
	})
	if err != nil {
		return err
	}

	if m.atlasInstallEnabled() {
		err = runWithLoader("Installing Atlas agent support", func() error {
			_, installErr := atlas.RunInstall(context.Background(), m.atlasInstallOptions(targetPath))
			return installErr
		})
		if err != nil {
			return err
		}
		fmt.Printf("%s Installed Atlas agent support\n", console.SuccessMark())
	}
	fmt.Printf("%s Project initialized at %s\n", console.SuccessMark(), targetPath)

	return nil
}

// runWithLoader keeps project work synchronous while the shared console loader owns animation cleanup.
func runWithLoader(message string, fn func() error) error {
	loader := console.NewLoader(message)
	if err := loader.Start(); err != nil {
		return err
	}
	defer loader.Stop()
	return fn()
}
