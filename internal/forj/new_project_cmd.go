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
	panelStyle           = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(surfaceBorder).Padding(1, 2)
	helpStyle            = lipgloss.NewStyle().Foreground(helpColor)
	sectionLabelStyle    = lipgloss.NewStyle().Foreground(brandPrimary).Bold(true)
	progressDoneStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e")) // muted green
	progressCurrentStyle = lipgloss.NewStyle().Foreground(brandPrimary).Bold(true)   // bold current
	progressPendingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))     // faded gray
	titleIndicatorStyle  = lipgloss.NewStyle().Foreground(helpColor)
	subLabelStyle        = helpStyle.Italic(true)
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
	componentList      list.Model
	selectedComponents []string
	config             ProjectConfig
	cancelled          bool
}

func initialModel() model {
	ti := styledTextInput()
	ti.Placeholder = "My Awesome App"
	ti.Focus()
	ti.CharLimit = 64

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
				m.config.ProjectName = m.projectInput.Value()
				m.stage = StageModuleName
				m.projectInput.Blur()
				m.moduleInput.Placeholder = "github.com/yourname/yourapp"
				m.moduleInput.Focus()
				m.moduleInput.CharLimit = 128
				m.moduleInput.Width = 40
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
				m.config.GoModuleName = m.moduleInput.Value()
				m.stage = StageSelectComponents
				m.moduleInput.Blur()
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
				m.stage = StageConfirm
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

		case StageConfirm:
			switch msg.Type {
			case tea.KeyShiftTab, tea.KeyCtrlB, tea.KeyLeft:
				m.stage = StageSelectComponents
				return m, nil
			}
			switch msg.String() {
			case "enter":
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
		subtitleStyle.Render("Opinionated defaults with room to grow."),
		"",
		m.renderProgress(),
	)

	var body string
	var actions []string

	switch m.stage {
	case StageProjectName:
		body = m.panelWithTitle("Project Name", lipgloss.JoinVertical(
			lipgloss.Left,
			subLabelStyle.Render("Give your application a human-friendly name."),
			"",
			m.projectInput.View(),
			"",
			helpStyle.Render("Directory: "+m.projectSlug()),
			helpStyle.Render("Go module: "+m.modulePreview()),
		))
		actions = []string{"⏎ Continue", "⎋ Cancel"}
	case StageModuleName:
		body = m.panelWithTitle("Go Module Path", lipgloss.JoinVertical(
			lipgloss.Left,
			subLabelStyle.Render("Use your desired module import path."),
			"",
			m.moduleInput.View(),
			"",
			helpStyle.Render("Preview: "+m.modulePreview()),
		))
		actions = []string{"⏎ Continue", "⇧⇥ Back", "⎋ Cancel"}
	case StageSelectComponents:
		body = m.panelWithTitle("Components", lipgloss.JoinVertical(
			lipgloss.Left,
			subLabelStyle.Render("Use arrows to move, space to toggle, enter to review."),
			"",
			m.renderComponentList(),
		))
		actions = []string{"⏎ Review", "⇧⇥ Back", "⎋ Cancel"}
	case StageConfirm:
		body = m.panelWithTitle("Confirm your project", lipgloss.JoinVertical(
			lipgloss.Left,
			subLabelStyle.Render("Review before creating files."),
			m.renderSummary(),
		))
		actions = []string{"⏎ Create", "⇧⇥ Back", "⎋ Cancel"}
	case StageDone:
		m.config.UpdatedAt = time.Now().Format("2006-01-02 15:04:05 MST")

		// Inject standard watch commands
		m.config.PreDev = []DevTask{
			{
				Name: "Run Wire generate",
				Cmd:  "go install github.com/google/wire/cmd/wire@latest && cd wire && wire",
			},
			{
				Name: "Watcher Go Install",
				Cmd:  "go install github.com/bokwoon95/wgo@latest",
			},
		}

		if m.config.Components.Docker {
			m.config.PreDev = append(m.config.PreDev, DevTask{
				Name: "Run Docker Compose",
				Cmd:  "docker-compose up -d",
			})

			// wait for mysql
			if m.config.Components.Database {
				m.config.PreDev = append(m.config.PreDev, DevTask{
					Name: "Waiting for Database to be ready",
					Cmd:  "docker-compose exec -T mysql sh -c 'while ! mysqladmin ping -h \"mysql\" --silent; do sleep .5; done'",
				})
			}
		}

		// might change this later
		if m.config.Components.WebAPI {
			m.config.DevWatches = []DevWatch{
				{
					Name:  "App",
					Watch: "-verbose -file .env -file .go -xdir forj -xdir _data -xdir ./frontend/node_modules -file .html",
					Exec:  "go run main.go http:serve",
				},
			}
		}

		// add scheduler watcher if enabled
		if m.config.Components.Scheduler {
			m.config.DevWatches = append(m.config.DevWatches, DevWatch{
				Name:  "Scheduler",
				Watch: "-file .env -file .go -xdir forj -xdir _data -xdir ./frontend/node_modules -file .html",
				Exec:  "go run main.go schedule:run",
			})
		}

		// add jobs watcher if enabled
		if m.config.Components.Jobs {
			m.config.DevWatches = append(m.config.DevWatches, DevWatch{
				Name:  "Jobs",
				Watch: "-file .env -file .go -xdir forj -xdir _data -xdir ./frontend/node_modules -file .html",
				Exec:  "go run main.go queue:work",
			})
		}

		m.config.DevWatches = append(m.config.DevWatches, DevWatch{
			Name:  "Wire",
			Watch: "-file .go -cd ./wire -xfile ./wire/wire_gen.go -xdir forj -postpone",
			Exec:  "wire",
		})

		if m.config.Components.WebUI && packageJSONHasNpmDev() {
			m.config.DevWatches = append(m.config.DevWatches, DevWatch{
				Name:  "NPM",
				Watch: "-cd ./frontend -xdir _data -xdir .",
				Exec:  "npm run dev",
			})
		}

		var buf bytes.Buffer
		encoder := yaml.NewEncoder(&buf)
		encoder.SetIndent(2)
		err := encoder.Encode(m.config)
		if err != nil {
			return "Error generating config!"
		}
		_ = os.WriteFile(".goforj.yml", buf.Bytes(), 0644)

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

		checkbox := normalStyle.Render("[ ]")
		if item.Selected {
			checkbox = selectedStyle.Render("[✓]")
		}

		name := normalStyle.Render(item.Name)
		if item.Selected {
			name = selectedStyle.Render(item.Name)
		}

		desc := ""
		if item.Desc != "" {
			desc = helpStyle.Render(" — " + item.Desc)
		}

		if item.Name == "CLI" {
			checkbox = selectedStyle.Render("[✓]")
			name = selectedStyle.Render(item.Name + " (required)")
		}

		rows = append(rows, fmt.Sprintf("%s %s %s%s", cursor, checkbox, name, desc))
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
	// Manual render to keep title inline with border width.
	contentLines := strings.Split(content, "\n")
	if len(contentLines) == 0 {
		contentLines = []string{""}
	}

	padLeft, padRight := 2, 2
	padTop, padBottom := 1, 1

	innerWidth := 0
	for _, line := range contentLines {
		if w := lipgloss.Width(line); w > innerWidth {
			innerWidth = w
		}
	}

	label := sectionLabelStyle.Render(" " + title + " ")
	totalInner := innerWidth + padLeft + padRight
	totalWidth := totalInner + 2 // borders

	dashes := totalWidth - lipgloss.Width(label) - 3
	if dashes < 0 {
		dashes = 0
	}

	border := lipgloss.NewStyle().Foreground(surfaceBorder)
	top := lipgloss.JoinHorizontal(lipgloss.Left,
		border.Render("╭─"),
		label,
		border.Render(strings.Repeat("─", dashes)+"╮"),
	)

	emptyLine := border.Render("│") + strings.Repeat(" ", totalInner) + border.Render("│")
	var middle []string
	for i := 0; i < padTop; i++ {
		middle = append(middle, emptyLine)
	}
	for _, line := range contentLines {
		padded := line + strings.Repeat(" ", innerWidth-lipgloss.Width(line))
		middle = append(middle, border.Render("│")+strings.Repeat(" ", padLeft)+padded+strings.Repeat(" ", padRight)+border.Render("│"))
	}
	for i := 0; i < padBottom; i++ {
		middle = append(middle, emptyLine)
	}

	bottom := border.Render("╰" + strings.Repeat("─", totalWidth-2) + "╯")

	all := []string{top}
	all = append(all, middle...)
	all = append(all, bottom)
	return strings.Join(all, "\n")
}

func styledTextInput() textinput.Model {
	base := lipgloss.NewStyle().Foreground(mutedColor)
	ti := textinput.New()
	ti.Prompt = "▸ "
	ti.PromptStyle = base.Foreground(brandSecondary)
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
		{"Confirm", StageConfirm},
	}

	var parts []string
	for _, step := range steps {
		icon := "›"
		style := progressPendingStyle

		switch {
		case m.stage > step.stage:
			icon = "✓"
			style = progressDoneStyle
		case m.stage == step.stage:
			icon = "▶"
			style = progressCurrentStyle
		}

		parts = append(parts, style.Render(fmt.Sprintf("%s %s", icon, step.label)))
	}

	return strings.Join(parts, " ")
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

func renderFooter(actions []string) string {
	line := strings.Join(actions, "     ")
	bar := helpStyle.Render(strings.Repeat("─", lipgloss.Width(line)))
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

	// check if .goforj.yml already exists
	if _, err := os.Stat(".goforj.yml"); err != nil {
		return err
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
