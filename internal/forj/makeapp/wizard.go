package makeapp

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/goforj/goforj/internal/konghelp"
	"github.com/goforj/goforj/project"
)

var (
	wizardPrimaryStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#f5f6f7"))
	wizardMutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#8b93a1"))
	wizardAccentStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#8C97E6")).Bold(true)
	wizardBorderStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#f5f6f7"))
)

type componentItem struct {
	Key      project.ComponentKey
	Name     string
	Desc     string
	Selected bool
}

// Title satisfies the Bubbles list item contract for component rows.
func (i componentItem) Title() string { return i.Name }

// Description satisfies the Bubbles list item contract for component rows.
func (i componentItem) Description() string { return i.Desc }

// FilterValue satisfies the Bubbles list item contract even though filtering is disabled.
func (i componentItem) FilterValue() string { return i.Name }

type starterKitItem struct {
	Key      project.StarterKit
	Label    string
	Desc     string
	Selected bool
}

// Title satisfies the Bubbles list item contract for starter-kit rows.
func (i starterKitItem) Title() string { return i.Label }

// Description satisfies the Bubbles list item contract for starter-kit rows.
func (i starterKitItem) Description() string { return i.Desc }

// FilterValue satisfies the Bubbles list item contract even though filtering is disabled.
func (i starterKitItem) FilterValue() string { return i.Label }

type helpFormatItem struct {
	Key      project.HelpFormat
	Label    string
	Desc     string
	Selected bool
}

// Title satisfies the Bubbles list item contract for help-format rows.
func (i helpFormatItem) Title() string { return i.Label }

// Description satisfies the Bubbles list item contract for help-format rows.
func (i helpFormatItem) Description() string { return i.Desc }

// FilterValue satisfies the Bubbles list item contract even though filtering is disabled.
func (i helpFormatItem) FilterValue() string { return i.Label }

type appWizardStage int

const (
	appWizardComponents appWizardStage = iota
	appWizardHelpFormat
	appWizardStarterKit
	appWizardDevRun
	appWizardConfirm
	appWizardDone
)

type appWizardModel struct {
	appName        string
	stage          appWizardStage
	componentList  list.Model
	helpFormatList list.Model
	starterKitList list.Model
	devRunInput    textinput.Model
	available      project.Components
	components     project.Components
	helpFormat     project.HelpFormat
	starterKit     project.StarterKit
	devRunEnabled  bool
	cancelled      bool
	termWidth      int
}

// runAppWizard selects components, starter kit, and dev run behavior for a new app.
func runAppWizard(appName string, config *project.Config) (project.Components, project.StarterKit, project.HelpFormat, string, bool, error) {
	initial := initialAppWizardModel(appName, config)
	result, err := tea.NewProgram(initial).Run()
	if err != nil {
		return project.Components{}, project.StarterKitNone, project.DefaultHelpFormat(), "", false, err
	}
	model, ok := result.(appWizardModel)
	if !ok || model.cancelled {
		return project.Components{}, project.StarterKitNone, project.DefaultHelpFormat(), "", true, nil
	}
	return model.components, model.starterKit, model.helpFormat, model.devRunCommand(), false, nil
}

// initialAppWizardModel builds the initial wizard state from project defaults.
func initialAppWizardModel(appName string, config *project.Config) appWizardModel {
	available := config.Render.Components
	components := project.AppDefaultComponents(available)
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	componentList := list.New(makeAppComponentItems(available, components), delegate, 42, 10)
	componentList.Title = "App Components"
	componentList.SetShowFilter(false)
	componentList.SetShowHelp(false)
	componentList.SetShowStatusBar(false)
	componentList.SetShowPagination(false)
	starterKit := project.NormalizeStarterKit(config.Render.StarterKit)
	if !components.WebUI {
		starterKit = project.StarterKitNone
	}
	starterKitList := list.New(makeStarterKitItems(starterKit), delegate, 42, 4)
	starterKitList.Title = "Starter Kit"
	starterKitList.SetShowFilter(false)
	starterKitList.SetShowHelp(false)
	starterKitList.SetShowStatusBar(false)
	starterKitList.SetShowPagination(false)
	helpFormat := project.NormalizeHelpFormat(config.Render.HelpFormat)
	helpFormatList := list.New(makeHelpFormatItems(helpFormat), delegate, 34, 2)
	helpFormatList.Title = "Help Format"
	helpFormatList.SetShowFilter(false)
	helpFormatList.SetShowHelp(false)
	helpFormatList.SetShowStatusBar(false)
	helpFormatList.SetShowPagination(false)
	devRunInput := textinput.New()
	devRunInput.Placeholder = "run"
	devRunInput.SetValue("run")
	devRunInput.Prompt = ""
	devRunInput.CharLimit = 160
	devRunInput.Width = 64
	devRunInput.Focus()
	return appWizardModel{
		appName:        appName,
		stage:          appWizardComponents,
		componentList:  componentList,
		helpFormatList: helpFormatList,
		starterKitList: starterKitList,
		devRunInput:    devRunInput,
		available:      available,
		components:     components,
		helpFormat:     helpFormat,
		starterKit:     starterKit,
		devRunEnabled:  components.HasRuntime(),
		termWidth:      132,
	}
}

// Init satisfies tea.Model; make:app does not need an initial asynchronous command.
func (m appWizardModel) Init() tea.Cmd { return nil }

// Update advances wizard state while keeping selected components and starter kits synchronized.
func (m appWizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termWidth = 132
		if msg.Width > 0 && msg.Width < 132 {
			m.termWidth = msg.Width
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.cancelled = true
			return m, tea.Quit
		}
		switch m.stage {
		case appWizardComponents:
			switch msg.String() {
			case "enter":
				m.applyComponentSelection()
				m.stage = appWizardHelpFormat
				return m, nil
			case "a":
				m.setAllComponents(true)
				return m, nil
			case "c":
				m.setAllComponents(false)
				return m, nil
			case " ":
				index := m.componentList.Index()
				item := m.componentList.Items()[index].(componentItem)
				if item.Key == project.ComponentCLI {
					return m, nil
				}
				definition, _ := project.ComponentDefinitionByKey(item.Key)
				if !item.Selected && definition.ExclusiveGroup != "" {
					m.deselectExclusiveComponents(item.Key, definition.ExclusiveGroup)
				}
				item.Selected = !item.Selected
				m.componentList.SetItem(index, item)
				if !item.Selected {
					m.deselectDependentComponents(item.Key)
				}
				m.normalizeComponentSelections()
				return m, nil
			}
			var cmd tea.Cmd
			m.componentList, cmd = m.componentList.Update(msg)
			return m, cmd
		case appWizardHelpFormat:
			switch msg.Type {
			case tea.KeyShiftTab, tea.KeyCtrlB, tea.KeyLeft:
				m.stage = appWizardComponents
				return m, nil
			}
			if msg.String() == "enter" {
				m.applyHelpFormatSelection()
				if m.components.WebUI {
					m.stage = appWizardStarterKit
				} else {
					m.stage = appWizardDevRun
				}
				return m, nil
			}
			var cmd tea.Cmd
			m.helpFormatList, cmd = m.helpFormatList.Update(msg)
			m.syncHelpFormatSelectionFromCursor()
			return m, cmd
		case appWizardStarterKit:
			switch msg.Type {
			case tea.KeyShiftTab, tea.KeyCtrlB, tea.KeyLeft:
				m.stage = appWizardHelpFormat
				return m, nil
			}
			if msg.String() == "enter" {
				m.applyStarterKitSelection()
				m.stage = appWizardDevRun
				return m, nil
			}
			var cmd tea.Cmd
			m.starterKitList, cmd = m.starterKitList.Update(msg)
			m.syncStarterKitSelectionFromCursor()
			return m, cmd
		case appWizardDevRun:
			switch msg.Type {
			case tea.KeyShiftTab, tea.KeyCtrlB, tea.KeyLeft:
				if m.components.WebUI {
					m.stage = appWizardStarterKit
				} else {
					m.stage = appWizardHelpFormat
				}
				return m, nil
			}
			switch msg.String() {
			case "enter":
				m.stage = appWizardConfirm
				return m, nil
			case "y":
				m.devRunEnabled = true
				m.devRunInput.Focus()
				return m, nil
			case "n":
				m.devRunEnabled = false
				m.devRunInput.Blur()
				return m, nil
			}
			if !m.devRunEnabled {
				return m, nil
			}
			var cmd tea.Cmd
			m.devRunInput, cmd = m.devRunInput.Update(msg)
			return m, cmd
		case appWizardConfirm:
			switch msg.Type {
			case tea.KeyShiftTab, tea.KeyCtrlB, tea.KeyLeft:
				m.stage = appWizardDevRun
				return m, nil
			}
			if msg.String() == "enter" {
				m.stage = appWizardDone
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

// View renders the current wizard stage with enough padding to keep terminal redraws stable.
func (m appWizardModel) View() string {
	componentNames := strings.Join(selectedComponentNamesFromItems(m.componentList.Items()), ", ")
	if strings.TrimSpace(componentNames) == "" {
		componentNames = "None"
	}
	var panels []string
	var actions []string
	switch m.stage {
	case appWizardComponents:
		panels = append(panels, wizardPanel("App", wizardPrimaryStyle.Render(m.appName), m.termWidth, false))
		panels = append(panels, wizardPanel("Components", m.renderComponentList(), m.termWidth, true))
		actions = []string{"Space to toggle", "A select all", "C select none", "Enter to continue", "Esc to cancel"}
	case appWizardStarterKit:
		panels = append(panels, wizardPanel("Components", wizardPrimaryStyle.Render(componentNames), m.termWidth, false))
		panels = append(panels, wizardPanel("Starter Kit", m.renderStarterKitList(), m.termWidth, true))
		actions = []string{"Enter to continue", "Shift+Tab to go back", "Esc to cancel"}
	case appWizardHelpFormat:
		panels = append(panels, wizardPanel("Components", wizardPrimaryStyle.Render(componentNames), m.termWidth, false))
		panels = append(panels, m.renderHelpFormatStage())
		actions = []string{"Enter to continue", "Shift+Tab to go back", "Esc to cancel"}
	case appWizardDevRun:
		panels = append(panels, wizardPanel("Components", wizardPrimaryStyle.Render(componentNames), m.termWidth, false))
		panels = append(panels, wizardPanel("Dev Run", m.renderDevRunStage(), m.termWidth, true))
		actions = []string{"Y run app", "N skip app", "Enter to continue", "Shift+Tab to go back", "Esc to cancel"}
	case appWizardConfirm:
		panels = append(panels, wizardPanel("Confirm app", renderKeyValueTable([]keyValue{
			{"App", m.appName},
			{"Components", componentNames},
			{"Starter kit", m.selectedStarterKitSummary()},
			{"Help format", m.selectedHelpFormatSummary()},
			{"Dev run", m.selectedDevRunSummary()},
		}), m.termWidth, true))
		actions = []string{"Enter to create", "Shift+Tab to go back", "Esc to cancel"}
	}
	view := lipgloss.JoinVertical(lipgloss.Left, panels...)
	if len(actions) > 0 {
		view = lipgloss.JoinVertical(lipgloss.Left, view, wizardFooter(actions, m.termWidth))
	}
	return "\n" + view
}

// renderDevRunStage renders whether the app participates in the forj dev lifecycle.
func (m appWizardModel) renderDevRunStage() string {
	enabled := "No"
	if m.devRunEnabled {
		enabled = "Yes"
	}
	command := wizardMutedStyle.Render("Not managed by forj dev")
	if m.devRunEnabled {
		command = m.devRunInput.View()
		if strings.TrimSpace(command) == "" {
			command = wizardMutedStyle.Render("run")
		}
	}
	return renderKeyValueTable([]keyValue{
		{"Manage in dev", enabled},
		{"Command", command},
	})
}

// devRunCommand returns the runtime suffix stored in the app's native dev
// lifecycle config. Empty keeps the app out of the default dev loop.
func (m appWizardModel) devRunCommand() string {
	if !m.devRunEnabled {
		return ""
	}
	command := strings.TrimSpace(m.devRunInput.Value())
	if command == "" {
		return "run"
	}
	return command
}

// applyComponentSelection normalizes visible choices into the app component contract.
func (m *appWizardModel) applyComponentSelection() {
	var keys []project.ComponentKey
	for _, item := range m.componentList.Items() {
		component := item.(componentItem)
		if component.Selected {
			keys = append(keys, component.Key)
		}
	}
	components, err := project.AppComponentsFromKeys(m.available, keys)
	if err == nil {
		m.components = components
	}
	if command := strings.TrimSpace(m.devRunInput.Value()); command == "" || command == "run" {
		m.devRunEnabled = m.components.HasRuntime()
	}
	if !m.components.WebUI {
		m.starterKit = project.StarterKitNone
		m.setStarterKitSelected(project.StarterKitNone)
	}
}

// deselectExclusiveComponents enforces one visible choice per exclusive component group.
func (m *appWizardModel) deselectExclusiveComponents(selectedKey project.ComponentKey, group string) {
	for _, definition := range project.AppComponentDefinitions(m.available) {
		if definition.Key == selectedKey || definition.ExclusiveGroup != group {
			continue
		}
		m.setComponentSelected(definition.Key, false)
	}
}

// setAllComponents bulk-updates visible app components while keeping CLI and database exclusivity coherent.
func (m *appWizardModel) setAllComponents(selected bool) {
	for idx, raw := range m.componentList.Items() {
		item := raw.(componentItem)
		switch item.Key {
		case project.ComponentCLI:
			item.Selected = true
		case project.ComponentDatabasePostgres, project.ComponentDatabaseSQLite:
			item.Selected = false
		default:
			item.Selected = selected
		}
		m.componentList.SetItem(idx, item)
	}
	m.normalizeComponentSelections()
}

// deselectDependentComponents removes visible app surfaces that would otherwise force the component back on.
func (m *appWizardModel) deselectDependentComponents(key project.ComponentKey) {
	for _, definition := range project.AppComponentDefinitions(m.available) {
		if definition.Key == key || !project.AppComponentRequires(definition.Key, key) {
			continue
		}
		if !m.componentSelected(definition.Key) {
			continue
		}
		m.setComponentSelected(definition.Key, false)
		m.deselectDependentComponents(definition.Key)
	}
}

// componentSelected reports visible list state without normalizing dependency rules first.
func (m *appWizardModel) componentSelected(key project.ComponentKey) bool {
	for _, raw := range m.componentList.Items() {
		item := raw.(componentItem)
		if item.Key == key {
			return item.Selected
		}
	}
	return false
}

// setComponentSelected updates list state without rebuilding the cursor position.
func (m *appWizardModel) setComponentSelected(key project.ComponentKey, selected bool) {
	for idx, raw := range m.componentList.Items() {
		item := raw.(componentItem)
		if item.Key != key {
			continue
		}
		item.Selected = selected
		m.componentList.SetItem(idx, item)
		return
	}
}

// normalizeComponentSelections keeps dependent wizard rows selected after a toggle.
func (m *appWizardModel) normalizeComponentSelections() {
	var keys []project.ComponentKey
	for _, raw := range m.componentList.Items() {
		item := raw.(componentItem)
		if item.Selected {
			keys = append(keys, item.Key)
		}
	}
	components, err := project.AppComponentsFromKeys(m.available, keys)
	if err != nil {
		return
	}
	for idx, raw := range m.componentList.Items() {
		item := raw.(componentItem)
		item.Selected = components.Enabled(item.Key)
		m.componentList.SetItem(idx, item)
	}
}

// applyStarterKitSelection commits the highlighted starter kit before confirmation.
func (m *appWizardModel) applyStarterKitSelection() {
	index := m.starterKitList.Index()
	if index < 0 || index >= len(m.starterKitList.Items()) {
		m.starterKit = project.StarterKitNone
		return
	}
	item, ok := m.starterKitList.Items()[index].(starterKitItem)
	if !ok {
		m.starterKit = project.StarterKitNone
		return
	}
	m.starterKit = project.NormalizeStarterKit(item.Key)
	m.setStarterKitSelected(m.starterKit)
}

// applyHelpFormatSelection commits the highlighted formatter before confirmation.
func (m *appWizardModel) applyHelpFormatSelection() {
	index := m.helpFormatList.Index()
	if index < 0 || index >= len(m.helpFormatList.Items()) {
		m.helpFormat = project.DefaultHelpFormat()
		return
	}
	item, ok := m.helpFormatList.Items()[index].(helpFormatItem)
	if !ok {
		m.helpFormat = project.DefaultHelpFormat()
		return
	}
	m.helpFormat = project.NormalizeHelpFormat(item.Key)
	m.setHelpFormatSelected(m.helpFormat)
}

// renderComponentList renders component rows manually because the default list chrome is hidden.
func (m appWizardModel) renderComponentList() string {
	var rows []string
	for i, raw := range m.componentList.Items() {
		item := raw.(componentItem)
		caret := "  "
		if m.componentList.Index() == i {
			caret = wizardAccentStyle.Render("› ")
		}
		marker := "○"
		if item.Selected {
			marker = "●"
		}
		label := wizardPrimaryStyle.Render(item.Name)
		if m.componentList.Index() == i {
			label = wizardAccentStyle.Render(item.Name)
		}
		line := caret + marker + " " + label
		if strings.TrimSpace(item.Desc) != "" {
			line += " " + wizardMutedStyle.Render("· "+item.Desc)
		}
		rows = append(rows, line)
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// renderHelpFormatStage renders formatter choices with a live preview of real formatter output.
func (m appWizardModel) renderHelpFormatStage() string {
	selection := wizardPanelWithWidth("Help Format", m.renderHelpFormatList(), m.termWidth, true)
	highlighted := m.highlightedHelpFormat()
	if m.termWidth >= 118 {
		gap := 2
		available := m.termWidth - gap*2
		leftWidth := available / 3
		middleWidth := available / 3
		rightWidth := available - leftWidth - middleWidth
		previews := lipgloss.JoinHorizontal(
			lipgloss.Top,
			wizardPanelWithWidth("Framework Preview", previewText(project.HelpFormatFramework, leftWidth-6), leftWidth, highlighted == project.HelpFormatFramework),
			strings.Repeat(" ", gap),
			wizardPanelWithWidth("Guided Preview", previewText(project.HelpFormatGuided, middleWidth-6), middleWidth, highlighted == project.HelpFormatGuided),
			strings.Repeat(" ", gap),
			wizardPanelWithWidth("External CLI Preview", previewText(project.HelpFormatExternalCLI, rightWidth-6), rightWidth, highlighted == project.HelpFormatExternalCLI),
		)
		return lipgloss.JoinVertical(lipgloss.Left, selection, previews)
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		selection,
		wizardPanelWithWidth("Preview", previewText(highlighted, m.termWidth-6), m.termWidth, true),
	)
}

// renderHelpFormatList renders formatter rows as radio choices.
func (m appWizardModel) renderHelpFormatList() string {
	var rows []string
	for i, raw := range m.helpFormatList.Items() {
		item := raw.(helpFormatItem)
		caret := "  "
		if m.helpFormatList.Index() == i {
			caret = wizardAccentStyle.Render("› ")
		}
		label := wizardPrimaryStyle.Render(item.Label)
		if m.helpFormatList.Index() == i {
			label = wizardAccentStyle.Render(item.Label)
		}
		marker := "○"
		if item.Selected {
			marker = "●"
		}
		line := caret + marker + " " + label
		if strings.TrimSpace(item.Desc) != "" {
			line += " " + wizardMutedStyle.Render("· "+item.Desc)
		}
		rows = append(rows, line)
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// renderStarterKitList renders starter-kit rows manually so selected state is visible.
func (m appWizardModel) renderStarterKitList() string {
	var rows []string
	for i, raw := range m.starterKitList.Items() {
		item := raw.(starterKitItem)
		caret := "  "
		if m.starterKitList.Index() == i {
			caret = wizardAccentStyle.Render("› ")
		}
		label := wizardPrimaryStyle.Render(item.Label)
		if m.starterKitList.Index() == i {
			label = wizardAccentStyle.Render(item.Label)
		}
		marker := "○"
		if item.Selected {
			marker = "●"
		}
		line := caret + marker + " " + label
		if strings.TrimSpace(item.Desc) != "" {
			line += " " + wizardMutedStyle.Render("· "+item.Desc)
		}
		rows = append(rows, line)
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// syncHelpFormatSelectionFromCursor makes cursor movement behave like a radio selection.
func (m *appWizardModel) syncHelpFormatSelectionFromCursor() {
	m.helpFormat = m.highlightedHelpFormat()
	m.setHelpFormatSelected(m.helpFormat)
}

// highlightedHelpFormat returns the formatter under the cursor.
func (m appWizardModel) highlightedHelpFormat() project.HelpFormat {
	index := m.helpFormatList.Index()
	if index < 0 || index >= len(m.helpFormatList.Items()) {
		return project.DefaultHelpFormat()
	}
	item, ok := m.helpFormatList.Items()[index].(helpFormatItem)
	if !ok {
		return project.DefaultHelpFormat()
	}
	return project.NormalizeHelpFormat(item.Key)
}

// setHelpFormatSelected keeps only one help format marked as selected.
func (m *appWizardModel) setHelpFormatSelected(selected project.HelpFormat) {
	selected = project.NormalizeHelpFormat(selected)
	for idx, raw := range m.helpFormatList.Items() {
		item := raw.(helpFormatItem)
		item.Selected = project.NormalizeHelpFormat(item.Key) == selected
		m.helpFormatList.SetItem(idx, item)
	}
}

// syncStarterKitSelectionFromCursor makes cursor movement behave like a radio selection.
func (m *appWizardModel) syncStarterKitSelectionFromCursor() {
	index := m.starterKitList.Index()
	if index < 0 || index >= len(m.starterKitList.Items()) {
		return
	}
	item, ok := m.starterKitList.Items()[index].(starterKitItem)
	if !ok {
		return
	}
	m.starterKit = project.NormalizeStarterKit(item.Key)
	m.setStarterKitSelected(m.starterKit)
}

// setStarterKitSelected keeps only one starter kit marked as selected.
func (m *appWizardModel) setStarterKitSelected(selected project.StarterKit) {
	selected = project.NormalizeStarterKit(selected)
	for idx, raw := range m.starterKitList.Items() {
		item := raw.(starterKitItem)
		item.Selected = project.NormalizeStarterKit(item.Key) == selected
		m.starterKitList.SetItem(idx, item)
	}
}

// selectedHelpFormatSummary reports the selected CLI help layout.
func (m appWizardModel) selectedHelpFormatSummary() string {
	helpFormat := m.helpFormat
	if m.stage == appWizardHelpFormat {
		helpFormat = m.highlightedHelpFormat()
	}
	if definition, ok := project.HelpFormatDefinitionByKey(helpFormat); ok {
		return definition.Label
	}
	return "Framework"
}

// selectedStarterKitSummary reports the effective starter kit for the confirmation panel.
func (m appWizardModel) selectedStarterKitSummary() string {
	if !m.components.WebUI {
		return "None"
	}
	starterKit := m.starterKit
	if m.stage == appWizardStarterKit {
		index := m.starterKitList.Index()
		if index >= 0 && index < len(m.starterKitList.Items()) {
			if item, ok := m.starterKitList.Items()[index].(starterKitItem); ok {
				starterKit = project.NormalizeStarterKit(item.Key)
			}
		}
	}
	if definition, ok := project.StarterKitDefinitionByKey(starterKit); ok {
		return definition.Label
	}
	return "None"
}

// selectedDevRunSummary reports whether forj dev will manage the app and with which command.
func (m appWizardModel) selectedDevRunSummary() string {
	if !m.devRunEnabled {
		return "No"
	}
	command := strings.TrimSpace(m.devRunInput.Value())
	if command == "" {
		command = "run"
	}
	return "Yes, " + command
}

// makeHelpFormatItems converts formatter definitions into radio-style list rows.
func makeHelpFormatItems(selected project.HelpFormat) []list.Item {
	selected = project.NormalizeHelpFormat(selected)
	definitions := project.HelpFormatCatalog()
	items := make([]list.Item, 0, len(definitions))
	for _, definition := range definitions {
		items = append(items, helpFormatItem{
			Key:      definition.Key,
			Label:    definition.Label,
			Desc:     definition.Description,
			Selected: project.NormalizeHelpFormat(definition.Key) == selected,
		})
	}
	return items
}

// makeAppComponentItems converts component definitions into list rows with current selection state.
func makeAppComponentItems(available project.Components, selected project.Components) []list.Item {
	definitions := project.AppComponentDefinitions(available)
	items := make([]list.Item, 0, len(definitions))
	for _, definition := range definitions {
		items = append(items, componentItem{
			Key:      definition.Key,
			Name:     definition.Label,
			Desc:     definition.Description,
			Selected: selected.Enabled(definition.Key),
		})
	}
	return items
}

// makeStarterKitItems converts starter-kit definitions into radio-style list rows.
func makeStarterKitItems(selected project.StarterKit) []list.Item {
	selected = project.NormalizeStarterKit(selected)
	definitions := project.StarterKitCatalog()
	items := make([]list.Item, 0, len(definitions))
	for _, definition := range definitions {
		items = append(items, starterKitItem{
			Key:      definition.Key,
			Label:    definition.Label,
			Desc:     definition.Description,
			Selected: project.NormalizeStarterKit(definition.Key) == selected,
		})
	}
	return items
}

// previewText renders and clips real formatter output for the wizard preview panes.
func previewText(format project.HelpFormat, width int) string {
	preview := konghelp.Preview(string(project.NormalizeHelpFormat(format)))
	lines := strings.Split(preview, "\n")
	for i, line := range lines {
		lines[i] = lipgloss.NewStyle().MaxWidth(maxInt(20, width)).Render(line)
	}
	return strings.Join(lines, "\n")
}

// selectedComponentNamesFromItems preserves visible order for wizard summaries.
func selectedComponentNamesFromItems(items []list.Item) []string {
	names := make([]string, 0)
	for _, item := range items {
		component := item.(componentItem)
		if component.Selected {
			names = append(names, component.Name)
		}
	}
	return names
}

type keyValue struct {
	key   string
	value string
}

func renderKeyValueTable(rows []keyValue) string {
	longestKey := 0
	for _, row := range rows {
		if width := lipgloss.Width(row.key); width > longestKey {
			longestKey = width
		}
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, fmt.Sprintf("%*s » %s", longestKey, row.key, row.value))
	}
	return strings.Join(lines, "\n")
}

// wizardPanel uses explicit width accounting because styled title text has ANSI escape bytes.
func wizardPanel(title string, content string, termWidth int, active bool) string {
	if termWidth <= 0 || termWidth > 90 {
		termWidth = 90
	}
	return wizardPanelWithWidth(title, content, termWidth, active)
}

// wizardPanelWithWidth renders a panel at an explicit width for side-by-side panes.
func wizardPanelWithWidth(title string, content string, termWidth int, active bool) string {
	titleStyle := wizardMutedStyle
	if active {
		titleStyle = wizardAccentStyle
	}
	header := wizardBorderStyle.Render("┌ " + titleStyle.Render(title) + " " + strings.Repeat("─", maxInt(0, termWidth-lipgloss.Width(title)-4)) + "┐")
	footer := wizardBorderStyle.Render("└" + strings.Repeat("─", maxInt(0, termWidth-2)) + "┘")
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = wizardBorderStyle.Render("│") + " " + line + strings.Repeat(" ", maxInt(0, termWidth-lipgloss.Width(line)-3)) + wizardBorderStyle.Render("│")
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, lipgloss.JoinVertical(lipgloss.Left, lines...), footer, "")
}

// wizardFooter keeps action hints visually separate from boxed wizard content.
func wizardFooter(actions []string, termWidth int) string {
	return wizardMutedStyle.Render(strings.Join(actions, " · "))
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
