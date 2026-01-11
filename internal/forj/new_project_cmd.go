package forj

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/goforj/goforj/internal/logger"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type WizardStage int

const (
	StageProjectName WizardStage = iota
	StageModuleName
	StageSelectComponents
	StageProjectPath
	StageConfirm
	StageDone
)

var (
	brandPrimary         = lipgloss.Color("#9fb7ed") // muted blue
	brandSecondary       = lipgloss.Color("#8dd3c7") // soft teal
	progressAccent       = lipgloss.Color("#a3e0cf") // subtle aqua
	surfaceBorder        = lipgloss.Color("#1f2937") // slate 800
	mutedColor           = lipgloss.Color("#e5e7eb") // gray 200
	helpColor            = lipgloss.Color("#94a3b8") // slate 400
	normalStyle          = lipgloss.NewStyle().Foreground(mutedColor)
	selectedStyle        = lipgloss.NewStyle().Foreground(brandPrimary).Bold(true)
	cursorStyle          = lipgloss.NewStyle().Foreground(brandSecondary).Bold(true)
	titleStyle           = lipgloss.NewStyle().Foreground(brandPrimary).Bold(true)
	subtitleStyle        = lipgloss.NewStyle().Foreground(helpColor).Italic(true)
	helpStyle            = lipgloss.NewStyle().Foreground(helpColor)
	ruleStyle            = lipgloss.NewStyle().Foreground(surfaceBorder)
	sectionLabelStyle    = lipgloss.NewStyle().Foreground(brandPrimary).Bold(true)
	headerLabelStyle     = lipgloss.NewStyle().Foreground(mutedColor)
	progressDoneStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("113")).Bold(true) // lime green
	progressCurrentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
	progressPendingStyle = lipgloss.NewStyle().Foreground(surfaceBorder) // match rule lines
	titleIndicatorStyle  = lipgloss.NewStyle().Foreground(helpColor)
	subLabelStyle        = helpStyle.Italic(true)
	errorStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("#f87171"))
	inputRuleStyle       = lipgloss.NewStyle().Foreground(brandPrimary)
	labelKeyStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
	labelSepStyle        = lipgloss.NewStyle().Foreground(brandSecondary)
)

type ListItem struct {
	Name     string
	Desc     string
	Selected bool
}

func (i ListItem) Title() string       { return i.Name }
func (i ListItem) Description() string { return i.Desc }
func (i ListItem) FilterValue() string { return i.Name }

type model struct {
	stage              WizardStage
	projectInput       textinput.Model
	moduleInput        textinput.Model
	pathInput          textinput.Model
	componentList      list.Model
	selectedComponents []string
	config             ProjectConfig
	cancelled          bool
	errorMsg           string
	targetPath         string
}

func (m *model) finalizeConfig() {
	m.config.UpdatedAt = time.Now().Format("2006-01-02 15:04:05 MST")

	// Reset slices before populating.
	m.config.Dev = DevConfig{
		Pre: []DevTask{
			{
				Name: "Run Wire generate",
				Cmd:  "cd wire && wire",
			},
			{
				Name: "Initial build",
				Cmd:  "go build -o ./bin/app",
			},
		},
		SoundOnWatchError: true,
		DownOnExit:        true,
	}

	if m.config.Components.Docker {
		m.config.Dev.Pre = append(m.config.Dev.Pre, DevTask{
			Name: "Run Docker Compose",
			Cmd:  "docker-compose up -d",
		})
		m.config.Dev.Down = append(m.config.Dev.Down, DevTask{
			Name: "Docker Compose Down",
			Cmd:  "docker-compose down",
		})

		if m.config.Components.Database {
			m.config.Dev.Pre = append(m.config.Dev.Pre, DevTask{
				Name: "Waiting for Database to be ready",
				Cmd:  "docker-compose exec -T mysql sh -c 'while ! mysqladmin ping -h \"mysql\" --silent; do sleep .5; done'",
			})
		}
	}

	needsApp := m.config.Components.WebAPI || m.config.Components.WebUI || m.config.Components.Scheduler || m.config.Components.Jobs
	if needsApp {
		m.config.Dev.Watches = append(m.config.Dev.Watches, DevWatch{
			Name:  "Build App",
			Watch: "-file .go -file .env -xdir forj -xdir _data -xfile '.*inject.*\\.go$' -postpone",
			Exec:  "go build -o ./bin/app",
		})
	}

	if m.config.Components.WebAPI || m.config.Components.WebUI {
		m.config.Dev.Watches = append(m.config.Dev.Watches, DevWatch{
			Name:  "API",
			Watch: "-file ./bin/app",
			Exec:  "./bin/app http:serve",
		})
	}

	if m.config.Components.Scheduler {
		m.config.Dev.Watches = append(m.config.Dev.Watches, DevWatch{
			Name:  "Scheduler",
			Watch: "-file ./bin/app",
			Exec:  "./bin/app schedule:run",
		})
	}

	if m.config.Components.Jobs {
		m.config.Dev.Watches = append(m.config.Dev.Watches, DevWatch{
			Name:  "Jobs",
			Watch: "-file ./bin/app",
			Exec:  "./bin/app queue:work",
		})
	}

	m.config.Dev.Watches = append(m.config.Dev.Watches, DevWatch{
		Name:  "Wire",
		Watch: "-file .go -cd ./wire -xfile ./wire/wire_gen.go -xdir forj -postpone",
		Exec:  "wire",
	})

	if m.config.Components.WebUI && packageJSONHasNpmDev() {
		m.config.Dev.Watches = append(m.config.Dev.Watches, DevWatch{
			Name:  "NPM",
			Watch: "-cd ./frontend -xdir _data -xdir .",
			Exec:  "npm run dev",
		})
	}
}

func initialModel() model {
	ti := styledTextInput()
	ti.Placeholder = "My Awesome App"
	ti.Focus()
	ti.CharLimit = 64

	pi := styledTextInput()
	pi.Placeholder = "Use current dir or provide a path"

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = selectedStyle
	delegate.Styles.SelectedDesc = helpStyle
	delegate.Styles.NormalTitle = normalStyle
	delegate.Styles.NormalDesc = helpStyle
	delegate.Styles.DimmedTitle = helpStyle
	delegate.Styles.DimmedDesc = helpStyle
	delegate.ShowDescription = false

	li := list.New(makeComponentItems(), delegate, 42, 12)
	li.Title = "Select Components"
	li.SetShowFilter(false)
	li.SetShowHelp(false)
	li.Styles.Title = lipgloss.NewStyle().Foreground(brandSecondary).Bold(true)
	li.Styles.PaginationStyle = helpStyle
	li.Styles.HelpStyle = helpStyle
	li.Styles.StatusBar = helpStyle
	li.SetShowStatusBar(false)
	li.SetShowPagination(false)

	return model{
		stage:         StageProjectName,
		projectInput:  ti,
		moduleInput:   styledTextInput(),
		pathInput:     pi,
		componentList: li,
	}
}

func makeComponentItems() []list.Item {
	return []list.Item{
		ListItem{Name: "CLI", Selected: true},
		ListItem{Name: "Docker", Desc: "Builds docker-compose.yml dependencies for your app"},
		ListItem{Name: "Web API"},
		ListItem{Name: "Web UI"},
		ListItem{Name: "Database"},
		ListItem{Name: "Scheduler", Desc: "Cron jobs and scheduled tasks. go-cron with fluent support"},
		ListItem{Name: "Jobs", Desc: "Asynq"},
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m *model) applyComponentSelection() {
	// Reset before applying current selections.
	m.config.Components = Components{}

	for _, item := range m.componentList.Items() {
		it := item.(ListItem)
		if !it.Selected {
			continue
		}
		switch it.Name {
		case "CLI":
			m.config.Components.CLI = true
		case "Docker":
			m.config.Components.Docker = true
		case "Web API":
			m.config.Components.WebAPI = true
		case "Web UI":
			m.config.Components.WebUI = true
		case "Database":
			m.config.Components.Database = true
		case "Scheduler":
			m.config.Components.Scheduler = true
		case "Jobs":
			m.config.Components.Jobs = true
		}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
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
				if strings.TrimSpace(m.moduleInput.Value()) == "" {
					m.errorMsg = "Go module path is required."
					return m, nil
				}
				m.config.GoModuleName = m.moduleInput.Value()
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
				m.stage = StageProjectPath
				if m.pathInput.Value() == "" {
					m.pathInput.SetValue(m.defaultTargetPath())
				}
				m.pathInput.Focus()
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
				if item.Name == "CLI" {
					return m, nil // prevent toggling CLI
				}
				item.Selected = !item.Selected
				m.componentList.SetItem(index, item)
				return m, nil
			}
			var cmd tea.Cmd
			m.componentList, cmd = m.componentList.Update(msg)
			return m, cmd

		case StageProjectPath:
			switch msg.Type {
			case tea.KeyShiftTab, tea.KeyCtrlB, tea.KeyLeft:
				m.stage = StageSelectComponents
				return m, nil
			}

			switch msg.String() {
			case "enter":
				if err := m.validatePathInput(); err != nil {
					m.errorMsg = err.Error()
					return m, nil
				}
				m.errorMsg = ""
				m.targetPath = m.projectPath()
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
				m.finalizeConfig()
				m.errorMsg = ""
				m.stage = StageDone
				return m, tea.Quit
			}
		}
	}

	return m, nil
}

func (m model) View() string {
	titleLine := lipgloss.JoinHorizontal(lipgloss.Left, helpStyle.Render("❯"), " ", titleStyle.Render("GoForj Project Wizard"))
	header := lipgloss.JoinVertical(
		lipgloss.Left,
		"",
		titleLine,
		subtitleStyle.Render("Designed defaults. Built to extend."),
		"",
		m.renderProgress(),
	)

	var body string
	var actions []string

	switch m.stage {
	case StageProjectName:
		inputBlock := lipgloss.JoinVertical(
			lipgloss.Left,
			subLabelStyle.Render("Give your application a human-friendly name."),
			"",
			renderInputLine(m.projectInput),
		)
		body = m.panelWithTitle("Project Name", lipgloss.JoinVertical(
			lipgloss.Left,
			indentBlock(inputBlock, 2),
			"",
			renderSectionHeader("Project Settings"),
			renderKeyValueTable([]keyValue{
				{"Project", m.projectInput.Value()},
				{"Directory", m.projectSlug()},
				{"Go module", m.modulePreview()},
				{"Path", m.projectPath()},
			}),
		))
		actions = []string{"Enter to continue", "Esc to cancel"}
	case StageModuleName:
		inputBlock := lipgloss.JoinVertical(
			lipgloss.Left,
			subLabelStyle.Render("Use your desired module import path."),
			"",
			renderInputLine(m.moduleInput),
		)
		body = m.panelWithTitle("Go Module Path", lipgloss.JoinVertical(
			lipgloss.Left,
			indentBlock(inputBlock, 2),
			"",
			renderSectionHeader("Project Settings"),
			renderKeyValueTable([]keyValue{
				{"Project", m.projectInput.Value()},
				{"Directory", m.projectSlug()},
				{"Go module", m.modulePreview()},
				{"Path", m.projectPath()},
			}),
		))
		actions = []string{"Enter to continue", "Shift+Tab to go back", "Esc to cancel"}
	case StageProjectPath:
		inputBlock := lipgloss.JoinVertical(
			lipgloss.Left,
			subLabelStyle.Render("Choose where to create the project. Empty dir required."),
			"",
			renderInputLine(m.pathInput),
		)
		body = m.panelWithTitle("Project Path", lipgloss.JoinVertical(
			lipgloss.Left,
			indentBlock(inputBlock, 2),
			"",
			renderSectionHeader("Project Settings"),
			renderKeyValueTable([]keyValue{
				{"Project", m.projectInput.Value()},
				{"Directory", m.projectSlug()},
				{"Go module", m.modulePreview()},
				{"Path", m.projectPath()},
			}),
			helpStyle.Render(m.pathStatus()),
		))
		actions = []string{"Enter to continue", "Shift+Tab to go back", "Esc to cancel"}
	case StageSelectComponents:
		componentNames := strings.Join(m.selectedComponentNames(), ", ")
		if componentNames == "" {
			componentNames = "CLI"
		}
		inputBlock := lipgloss.JoinVertical(
			lipgloss.Left,
			subLabelStyle.Render("Use arrows to move, space to toggle, enter to review."),
			"",
			m.renderComponentList(),
		)
		body = m.panelWithTitle("Components", lipgloss.JoinVertical(
			lipgloss.Left,
			indentBlock(inputBlock, 2),
			"",
			renderSectionHeader("Project Settings"),
			renderKeyValueTable([]keyValue{
				{"Project", m.projectInput.Value()},
				{"Directory", m.projectSlug()},
				{"Go module", m.modulePreview()},
				{"Path", m.projectPath()},
				{"Components", componentNames},
			}),
		))
		actions = []string{"Enter to review", "Shift+Tab to go back", "Esc to cancel", "a: all", "c: clear"}
	case StageConfirm:
		inputBlock := lipgloss.JoinVertical(
			lipgloss.Left,
			subLabelStyle.Render("Review project settings and press enter to continue."),
			"",
		)
		body = m.panelWithTitle("Confirm your project", lipgloss.JoinVertical(
			lipgloss.Left,
			indentBlock(inputBlock, 2),
			renderSectionHeader("Project Settings"),
			renderKeyValueTable([]keyValue{
				{"Project", m.projectInput.Value()},
				{"Directory", m.projectSlug()},
				{"Go module", m.modulePreview()},
				{"Path", m.projectPath()},
				{"Components", strings.Join(m.selectedComponentNames(), ", ")},
			}),
		))
		actions = []string{"Enter to create", "Shift+Tab to go back", "Esc to cancel"}
	case StageDone:
		body = m.panelWithTitle("Project initialized", lipgloss.JoinVertical(
			lipgloss.Left,
			selectedStyle.Render("Project initialized and .goforj.yml created!"),
			helpStyle.Render("Next: run `forj render` or explore your scaffold."),
		))
	}

	view := lipgloss.JoinVertical(lipgloss.Left, header, "")
	if body != "" {
		view = lipgloss.JoinVertical(lipgloss.Left, view, body)
	}
	if len(actions) > 0 {
		view = lipgloss.JoinVertical(lipgloss.Left, view, renderFooter(actions))
	}
	if m.errorMsg != "" {
		view = lipgloss.JoinVertical(lipgloss.Left, view, errorStyle.Render(m.errorMsg))
	}
	return view + "\n"
}

func (m model) renderComponentList() string {
	var rows []string
	for i, listItem := range m.componentList.Items() {
		item := listItem.(ListItem)

		cursor := " "
		if m.componentList.Index() == i {
			cursor = cursorStyle.Render("›")
		}

		icon := normalStyle.Render("○")
		name := normalStyle.Render(item.Name)
		if item.Selected {
			icon = selectedStyle.Render("●")
			name = selectedStyle.Render(item.Name)
		}

		desc := ""
		if item.Desc != "" {
			desc = helpStyle.Render(" - " + item.Desc)
		}

		if item.Name == "CLI" {
			icon = selectedStyle.Render("●")
			name = selectedStyle.Render(item.Name + " (required)")
		}

		rows = append(rows, fmt.Sprintf("%s %s %s%s", cursor, icon, name, desc))
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (m model) renderSummary() string {
	project := m.config.ProjectName
	if project == "" {
		project = "<not set>"
	}

	module := m.config.GoModuleName
	if module == "" {
		module = "<not set>"
	}

	components := m.selectedComponentNames()
	if len(components) == 0 {
		components = []string{"CLI"}
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		fmt.Sprintf("%s %s", sectionLabelStyle.Render("Project"), normalStyle.Render(project)),
		fmt.Sprintf("%s %s", sectionLabelStyle.Render("Module"), normalStyle.Render(module)),
		fmt.Sprintf("%s %s", sectionLabelStyle.Render("Components"), normalStyle.Render(strings.Join(components, ", "))),
	)
}

func (m model) selectedComponentNames() []string {
	var comps []string
	for _, item := range m.componentList.Items() {
		it := item.(ListItem)
		if it.Selected {
			comps = append(comps, it.Name)
		}
	}
	return comps
}

func (m model) panelWithTitle(title, content string) string {
	contentLines := strings.Split(content, "\n")
	if len(contentLines) == 0 {
		contentLines = []string{""}
	}

	innerWidth := 0
	for _, line := range contentLines {
		if w := lipgloss.Width(line); w > innerWidth {
			innerWidth = w
		}
	}
	if innerWidth < 72 {
		innerWidth = 72
	}
	dash := ruleStyle.Render("─")
	label := headerLabelStyle.Render(" " + title)
	header := lipgloss.JoinHorizontal(
		lipgloss.Left,
		dash,
		label,
		" ",
		ruleStyle.Render(strings.Repeat("─", innerWidth-lipgloss.Width(dash+label)-1)),
	)
	return lipgloss.JoinVertical(lipgloss.Left, header, "", content)
}

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
	line := view + padding
	underline := ruleStyle.Render(strings.Repeat("─", width))
	return lipgloss.JoinVertical(lipgloss.Left, line, underline)
}

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

func renderSectionHeader(title string) string {
	label := headerLabelStyle.Render(title)
	prefix := ruleStyle.Render("─") + " " + label
	width := lipgloss.Width(prefix)
	if width < 72 {
		width = 72
	}
	header := lipgloss.JoinHorizontal(
		lipgloss.Left,
		ruleStyle.Render("─"),
		" ",
		label,
		" ",
		ruleStyle.Render(strings.Repeat("─", width-lipgloss.Width(prefix)-1)),
	)
	return header
}

type keyValue struct {
	key   string
	value string
}

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
	return "\n" + table + "\n"
}

func styledTextInput() textinput.Model {
	base := lipgloss.NewStyle().Foreground(mutedColor)
	ti := textinput.New()
	ti.Prompt = ""
	ti.PromptStyle = base
	ti.TextStyle = base
	ti.PlaceholderStyle = base.Foreground(helpColor)
	ti.CursorStyle = base.Foreground(brandPrimary)
	ti.Width = 34
	return ti
}

func (m model) renderProgress() string {
	steps := []struct {
		label string
		stage WizardStage
	}{
		{"Project", StageProjectName},
		{"Module", StageModuleName},
		{"Components", StageSelectComponents},
		{"Path", StageProjectPath},
		{"Confirm", StageConfirm},
	}

	var parts []string
	for _, step := range steps {
		label := step.label
		prefix := "•"
		prefixStyle := progressPendingStyle
		labelStyle := progressPendingStyle

		switch {
		case m.stage > step.stage:
			prefix = "✔"
			prefixStyle = progressDoneStyle
			labelStyle = progressDoneStyle
		case m.stage == step.stage:
			prefix = "▸"
			prefixStyle = progressCurrentStyle
			labelStyle = progressCurrentStyle
		}

		parts = append(parts, prefixStyle.Render(prefix)+" "+labelStyle.Render(label))
	}

	return strings.Join(parts, " ")
}

func (m *model) setAllComponents(selected bool) {
	for idx, listItem := range m.componentList.Items() {
		item := listItem.(ListItem)
		if item.Name == "CLI" {
			item.Selected = true
			m.componentList.SetItem(idx, item)
			continue
		}
		item.Selected = selected
		m.componentList.SetItem(idx, item)
	}
}

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

func (m model) validateBeforeConfirm() error {
	if strings.TrimSpace(m.projectInput.Value()) == "" {
		return fmt.Errorf("Project name is required.")
	}
	if strings.TrimSpace(m.moduleInput.Value()) == "" {
		return fmt.Errorf("Go module path is required.")
	}

	if err := m.validatePathInput(); err != nil {
		return err
	}

	return nil
}

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
		return fmt.Errorf("Target path is not empty: %s", target)
	}
	return nil
}

func (m model) pathStatus() string {
	target := m.projectPath()
	info, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return "Path does not exist. It will be created."
		}
		return "Cannot stat target path."
	}
	if !info.IsDir() {
		return "Path is not a directory."
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return "Cannot read target path."
	}
	if len(entries) > 0 {
		return "Path is not empty."
	}
	return "Path exists and is empty."
}

func renderFooter(actions []string) string {
	line := strings.Join(actions, " · ")
	width := lipgloss.Width(line)
	if width < 72 {
		width = 72
	}
	bar := ruleStyle.Render(strings.Repeat("─", width))
	return lipgloss.JoinVertical(lipgloss.Left, bar, helpStyle.Render(line))
}

// packageJSONHasNpmDev checks if ./frontend/package.json defines an npm run dev script.
func packageJSONHasNpmDev() bool {
	path := filepath.Join("frontend", "package.json")
	data, err := ioutil.ReadFile(path)
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

type NewProjectCmd struct {
	logger   *logger.AppLogger
	renderer *ProjectRenderer
}

func NewNewProjectCmd(logger *logger.AppLogger, renderer *ProjectRenderer) *NewProjectCmd {
	return &NewProjectCmd{
		logger:   logger,
		renderer: renderer,
	}
}

func (c *NewProjectCmd) Run() error {
	// Run the wizard
	resultModel, err := tea.NewProgram(initialModel()).Run()
	if err != nil {
		fmt.Print("Error running GoForj wizard:", err)
		os.Exit(1)
	}

	if m, ok := resultModel.(model); ok && m.cancelled {
		c.logger.Info().Msg("Project creation cancelled")
		return nil
	}

	var targetPath string
	if m, ok := resultModel.(model); ok {
		targetPath = m.targetPath
		if targetPath == "" {
			targetPath = m.projectPath()
		}
	}
	if targetPath == "" {
		return fmt.Errorf("target path could not be determined")
	}

	if err := os.MkdirAll(targetPath, 0755); err != nil {
		return fmt.Errorf("failed to create target path: %w", err)
	}
	if err := os.Chdir(targetPath); err != nil {
		return fmt.Errorf("failed to change to target path: %w", err)
	}

	// write .goforj.yml in target path using the model config
	if m, ok := resultModel.(model); ok {
		m.finalizeConfig()
		m.targetPath = targetPath

		var buf bytes.Buffer
		encoder := yaml.NewEncoder(&buf)
		encoder.SetIndent(2)
		if err := encoder.Encode(m.config); err != nil {
			return fmt.Errorf("failed to encode .goforj.yml: %w", err)
		}
		configPath := filepath.Join(targetPath, ".goforj.yml")
		if writeErr := os.WriteFile(configPath, buf.Bytes(), 0644); writeErr != nil {
			return fmt.Errorf("failed to write .goforj.yml: %w", writeErr)
		}
	} else {
		return fmt.Errorf("failed to capture wizard model for config write")
	}

	// project renderer
	i := ComponentRenderInput{}
	i.renderAll = true
	err = c.renderer.Render(i)
	if err != nil {
		return err
	}

	return nil
}
