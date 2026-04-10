package forj

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
	"github.com/goforj/goforj/version"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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
	StageExtras
	StageRuntime
	StageProjectPath
	StageConfirm
	StageDone
)

var (
	primaryText           = lipgloss.Color("#f5f6f7") // off-white
	mutedText             = lipgloss.Color("#8b93a1") // gray
	accentColor           = lipgloss.Color("#8C97E6") // soft blue-violet
	successColor          = lipgloss.Color("#7fcb96") // soft green
	errorColor            = lipgloss.Color("#c97b7b") // muted red
	ruleColor             = lipgloss.Color("#1f2937")
	normalStyle           = lipgloss.NewStyle().Foreground(primaryText)
	successStyle          = lipgloss.NewStyle().Foreground(successColor)
	titleStyle            = lipgloss.NewStyle().Foreground(primaryText).Bold(true)
	subtitleStyle         = lipgloss.NewStyle().Foreground(mutedText).Italic(true)
	helpStyle             = lipgloss.NewStyle().Foreground(mutedText)
	ruleStyle             = lipgloss.NewStyle().Foreground(ruleColor)
	sectionLabelStyle     = lipgloss.NewStyle().Foreground(primaryText).Bold(true)
	headerLabelStyle      = lipgloss.NewStyle().Foreground(primaryText)
	progressDoneMark      = lipgloss.NewStyle().Foreground(successColor)
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
	listCursorStyle       = lipgloss.NewStyle().Foreground(primaryText)
	listCheckStyle        = lipgloss.NewStyle().Foreground(successColor)
	panelTitleDoneStyle   = lipgloss.NewStyle().Foreground(mutedText)
	panelTitleActiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#c6e5ff"))
	listOptionMutedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#636b78"))
	panelBorderStyle      = lipgloss.NewStyle().Foreground(primaryText)
	statusOKStyle         = lipgloss.NewStyle().Foreground(successColor)
	statusErrorStyle      = lipgloss.NewStyle().Foreground(errorColor)
)

type ListItem struct {
	Name     string
	Desc     string
	Selected bool
}

func (i ListItem) Title() string       { return i.Name }
func (i ListItem) Description() string { return i.Desc }
func (i ListItem) FilterValue() string { return i.Name }

type QueueDriverItem struct {
	Driver string
	Label  string
	Desc   string
}

func (i QueueDriverItem) Title() string       { return i.Label }
func (i QueueDriverItem) Description() string { return i.Desc }
func (i QueueDriverItem) FilterValue() string { return i.Label }

type queueDriverOption struct {
	Name  string
	Title string
	Desc  string
}

func queueDriverOptions() []queueDriverOption {
	return []queueDriverOption{
		{Name: "null", Title: "Null", Desc: "accept jobs and drop them immediately"},
		{Name: "redis", Title: "Redis", Desc: "distributed async queue via Redis"},
		{Name: "nats", Title: "NATS", Desc: "distributed async queue via NATS"},
		{Name: "sqs", Title: "SQS", Desc: "distributed async queue via AWS SQS"},
		{Name: "rabbitmq", Title: "RabbitMQ", Desc: "distributed async queue via RabbitMQ"},
		{Name: "sqlite", Title: "SQLite", Desc: "SQL-backed queue via SQLite"},
		{Name: "postgres", Title: "Postgres", Desc: "SQL-backed queue via Postgres"},
		{Name: "mysql", Title: "MySQL", Desc: "SQL-backed queue via MySQL"},
		{Name: "workerpool", Title: "Workerpool", Desc: "in-process async worker pool"},
		{Name: "sync", Title: "Sync", Desc: "inline, in-process execution"},
	}
}

func normalizeQueueDriver(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "null", "redis", "nats", "sqs", "rabbitmq", "sqlite", "postgres", "mysql", "sync", "workerpool":
		return normalized
	default:
		return ""
	}
}

type model struct {
	stage              WizardStage
	projectInput       textinput.Model
	moduleInput        textinput.Model
	pathInput          textinput.Model
	componentList      list.Model
	queueDriverList    list.Model
	selectedComponents []string
	config             project.Config
	cancelled          bool
	errorMsg           string
	targetPath         string
	termWidth          int
	extrasIndex        int
	demoAppEnabled     bool
}

const wizardWidth = 90

func (m *model) components() *project.Components {
	return &m.config.Render.Components
}

func (m *model) finalizeConfig() {
	m.config.UpdatedAt = time.Now().Format("2006-01-02 15:04:05 MST")
	m.config.Render.GoForjVersion = version.Semver()
	components := m.components()
	if components.Jobs {
		m.config.Render.QueueDriver = normalizeQueueDriver(m.config.Render.QueueDriver)
		if m.config.Render.QueueDriver == "" {
			m.config.Render.QueueDriver = "redis"
		}
	}

	// Reset slices before populating.
	m.config.Dev = project.DevConfig{
		Pre: []project.DevTask{
			{
				Name: "Initial build",
				Cmd:  "forj build -o ./bin/app",
			},
		},
		SoundOnWatchError: true,
		AutoMigrate:       components.HasDatabase(),
		DownOnExit:        true,
		WirePaths:         []string{"wire"},
	}

	if components.Docker {
		m.config.Dev.Pre = append(m.config.Dev.Pre, project.DevTask{
			Name: "Run Docker Compose",
			Cmd:  "docker-compose up -d",
		})
		m.config.Dev.Down = append(m.config.Dev.Down, project.DevTask{
			Name: "Docker Compose Down",
			Cmd:  "docker-compose down",
		})

		if components.HasDatabase() && !components.DatabaseSQLite {
			waitCmd := "docker-compose exec -T mysql sh -c 'while ! mysqladmin ping -h \"mysql\" --silent; do sleep .5; done; mysql -h \"mysql\" -uroot -p\"$MARIADB_ROOT_PASSWORD\" -e \"CREATE DATABASE IF NOT EXISTS \\`$MARIADB_DATABASE\\`;\"'"
			if components.DatabasePostgres {
				waitCmd = "docker-compose exec -T postgres sh -c 'until pg_isready -h \"postgres\" -p 5432; do sleep .5; done; psql -U \"$POSTGRES_USER\" -h \"postgres\" -d postgres -v ON_ERROR_STOP=1 -tc \"SELECT 1 FROM pg_database WHERE datname = '\\''$POSTGRES_DB'\\''\" | grep -q 1 || psql -U \"$POSTGRES_USER\" -h \"postgres\" -d postgres -v ON_ERROR_STOP=1 -c \"CREATE DATABASE \\\"$POSTGRES_DB\\\";\"'"
			}
			m.config.Dev.Pre = append(m.config.Dev.Pre, project.DevTask{
				Name: "Waiting for Database to be ready",
				Cmd:  waitCmd,
			})
		}
	}

	needsApp := components.WebAPI || components.WebUI || components.Scheduler || components.Jobs
	if needsApp {
		m.config.Dev.Watches = append(m.config.Dev.Watches, project.DevWatch{
			Name:  "Build App",
			Watch: "-file .go -file .env -file .env.* -xdir forj -xdir _data -xfile wire/wire_gen\\.go$ -postpone",
			Exec:  "forj build -o ./bin/app",
		})
	}

	if components.WebAPI || components.WebUI || components.Scheduler || components.Jobs {
		m.config.Dev.Watches = append(m.config.Dev.Watches, project.DevWatch{
			Name:  "Run App",
			Watch: "-file ./bin/app -file .env -file .env.*",
			Exec:  "./bin/app run",
		})
	}

	if components.WebUI && packageJSONHasNpmDev() {
		m.config.Dev.Watches = append(m.config.Dev.Watches, project.DevWatch{
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
	delegate.Styles.SelectedTitle = normalStyle
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
	li.Styles.Title = lipgloss.NewStyle().Foreground(primaryText).Bold(true)
	li.Styles.PaginationStyle = helpStyle
	li.Styles.HelpStyle = helpStyle
	li.Styles.StatusBar = helpStyle
	li.SetShowStatusBar(false)
	li.SetShowPagination(false)

	runtimeList := list.New(makeQueueDriverItems(), delegate, 42, 6)
	runtimeList.Title = "Queue Driver"
	runtimeList.SetShowFilter(false)
	runtimeList.SetShowHelp(false)
	runtimeList.Styles.Title = lipgloss.NewStyle().Foreground(primaryText).Bold(true)
	runtimeList.Styles.PaginationStyle = helpStyle
	runtimeList.Styles.HelpStyle = helpStyle
	runtimeList.Styles.StatusBar = helpStyle
	runtimeList.SetShowStatusBar(false)
	runtimeList.SetShowPagination(false)

	return model{
		stage:           StageProjectName,
		projectInput:    ti,
		moduleInput:     styledTextInput(),
		pathInput:       pi,
		componentList:   li,
		queueDriverList: runtimeList,
		config: project.Config{
			Render: project.RenderConfig{
				QueueDriver:   "redis",
				GoForjVersion: version.Semver(),
				Components: project.Components{
					CLI: true,
				},
			},
		},
	}
}

func makeComponentItems() []list.Item {
	return []list.Item{
		ListItem{Name: "CLI", Selected: true},
		ListItem{Name: "Docker", Desc: "Builds docker-compose.yml dependencies for your app"},
		ListItem{Name: "Auth", Desc: "Users, sessions, and generated authentication scaffolding"},
		ListItem{Name: "Web API"},
		ListItem{Name: "Web UI"},
		ListItem{Name: "Database (MySQL)"},
		ListItem{Name: "Database (Postgres)"},
		ListItem{Name: "Database (SQLite)"},
		ListItem{Name: "Scheduler", Desc: "Cron jobs and scheduled tasks. go-cron with fluent support"},
		ListItem{Name: "Jobs", Desc: "Asynq"},
		ListItem{Name: "Stress Test", Desc: "Synthetic queue stress jobs and scheduler tick command"},
	}
}

func makeQueueDriverItems() []list.Item {
	options := queueDriverOptions()
	items := make([]list.Item, 0, len(options))
	for _, option := range options {
		items = append(items, QueueDriverItem{
			Driver: option.Name,
			Label:  option.Title,
			Desc:   option.Desc,
		})
	}
	return items
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m *model) applyComponentSelection() {
	// Reset before applying current selections.
	*m.components() = project.Components{}
	components := m.components()

	for _, item := range m.componentList.Items() {
		it := item.(ListItem)
		if !it.Selected {
			continue
		}
		switch it.Name {
		case "CLI":
			components.CLI = true
		case "Docker":
			components.Docker = true
		case "Auth":
			components.Auth = true
		case "Web API":
			components.WebAPI = true
		case "Web UI":
			components.WebUI = true
		case "Database (MySQL)":
			components.DatabaseMySQL = true
		case "Database (Postgres)":
			components.DatabasePostgres = true
		case "Database (SQLite)":
			components.DatabaseSQLite = true
		case "Scheduler":
			components.Scheduler = true
		case "Jobs":
			components.Jobs = true
		case "Stress Test":
			components.StressTest = true
		}
	}
	if components.StressTest {
		components.Jobs = true
	}
}

func (m *model) applyExtrasSelection() {
	m.demoAppEnabled = m.extrasIndex == 1
	components := m.components()
	components.DemoApp = m.demoAppEnabled
	if !m.demoAppEnabled {
		return
	}
	// Demo App profile requires core runtime surfaces.
	components.CLI = true
	components.Auth = true
	components.WebAPI = true
	components.WebUI = true
	components.Scheduler = true
	components.Jobs = true
	components.DatabaseMySQL = true
	components.DatabasePostgres = false
	components.DatabaseSQLite = false
}

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
				m.stage = StageExtras
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
				if item.Name == "Database (MySQL)" && !item.Selected {
					m.deselectComponent("Database (Postgres)")
					m.deselectComponent("Database (SQLite)")
				}
				if item.Name == "Database (Postgres)" && !item.Selected {
					m.deselectComponent("Database (MySQL)")
					m.deselectComponent("Database (SQLite)")
				}
				if item.Name == "Database (SQLite)" && !item.Selected {
					m.deselectComponent("Database (MySQL)")
					m.deselectComponent("Database (Postgres)")
				}
				if item.Name == "Stress Test" && !item.Selected {
					m.selectComponent("Jobs")
				}
				if item.Name == "Jobs" && item.Selected {
					m.deselectComponent("Stress Test")
				}
				item.Selected = !item.Selected
				m.componentList.SetItem(index, item)
				return m, nil
			}
			var cmd tea.Cmd
			m.componentList, cmd = m.componentList.Update(msg)
			return m, cmd

		case StageExtras:
			switch msg.Type {
			case tea.KeyShiftTab, tea.KeyCtrlB, tea.KeyLeft:
				m.stage = StageSelectComponents
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
				if m.config.Render.Components.Jobs {
					target := normalizeQueueDriver(m.config.Render.QueueDriver)
					if target == "" {
						target = "redis"
					}
					for idx, item := range m.queueDriverList.Items() {
						driverItem, ok := item.(QueueDriverItem)
						if ok && driverItem.Driver == target {
							m.queueDriverList.Select(idx)
							break
						}
					}
					m.stage = StageRuntime
				} else {
					m.stage = StageProjectPath
				}
				if m.pathInput.Value() == "" {
					m.pathInput.SetValue(m.defaultTargetPath())
				}
				if m.stage == StageProjectPath {
					m.pathInput.Focus()
				}
				return m, nil
			}

		case StageRuntime:
			switch msg.Type {
			case tea.KeyShiftTab, tea.KeyCtrlB, tea.KeyLeft:
				m.stage = StageExtras
				return m, nil
			}
			switch msg.String() {
			case "enter":
				index := m.queueDriverList.Index()
				if index < 0 || index >= len(m.queueDriverList.Items()) {
					m.config.Render.QueueDriver = "redis"
				} else {
					if item, ok := m.queueDriverList.Items()[index].(QueueDriverItem); ok {
						m.config.Render.QueueDriver = item.Driver
					}
				}
				m.stage = StageProjectPath
				if m.pathInput.Value() == "" {
					m.pathInput.SetValue(m.defaultTargetPath())
				}
				m.pathInput.Focus()
				return m, nil
			}
			var cmd tea.Cmd
			m.queueDriverList, cmd = m.queueDriverList.Update(msg)
			return m, cmd

		case StageProjectPath:
			switch msg.Type {
			case tea.KeyShiftTab, tea.KeyCtrlB, tea.KeyLeft:
				if m.config.Render.Components.Jobs {
					m.stage = StageRuntime
				} else {
					m.stage = StageExtras
				}
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
			panels = append(panels, m.panelWithTitle("Components", lipgloss.JoinVertical(
				lipgloss.Left,
				m.renderComponentList(m.termWidth),
			), m.termWidth, true))
			actions = []string{"Enter to continue", "Shift+Tab to go back", "Esc to cancel", "a: all", "c: clear"}
		} else {
			panels = append(panels, m.panelWithTitle("Components", normalStyle.Render(componentNames), m.termWidth, false))
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
			offPointer := "  "
			onPointer := "  "
			if offSelected {
				offPointer = listCursorStyle.Render("› ")
			}
			if onSelected {
				onPointer = listCursorStyle.Render("› ")
			}
			offMarker := normalStyle.Render("●")
			onMarker := normalStyle.Render("○")
			if onSelected {
				offMarker = normalStyle.Render("○")
				onMarker = normalStyle.Render("●")
			}
			extrasBody := lipgloss.JoinVertical(
				lipgloss.Left,
				offPointer+offMarker+" "+listNameStyle.Render("Off"),
				onPointer+onMarker+" "+listNameStyle.Render("On (Generate monitoring reference app)"),
			)
			panels = append(panels, m.panelWithTitle("Extras · Demo App", extrasBody, m.termWidth, true))
			actions = []string{"Enter to continue", "Shift+Tab to go back", "Esc to cancel"}
		} else {
			panels = append(panels, m.panelWithTitle("Extras · Demo App", normalStyle.Render(extrasSummary), m.termWidth, false))
		}
	}

	// Runtime panel.
	if m.stage >= StageRuntime && m.config.Render.Components.Jobs {
		driver := selectedQueueDriverSummary(m)

		if m.stage == StageRuntime {
			panels = append(panels, m.panelWithTitle("Runtime · Queue Driver", lipgloss.JoinVertical(
				lipgloss.Left,
				m.renderQueueDriverList(m.termWidth),
			), m.termWidth, true))
			actions = []string{"Enter to continue", "Shift+Tab to go back", "Esc to cancel"}
		} else {
			panels = append(panels, m.panelWithTitle("Runtime · Queue Driver", normalStyle.Render(driver), m.termWidth, false))
		}
	}

	// Path panel.
	if m.stage >= StageProjectPath {
		if m.stage == StageProjectPath {
			statusText, statusOK := m.pathStatus()
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
		confirmBody := lipgloss.JoinVertical(
			lipgloss.Left,
			renderKeyValueTable([]keyValue{
				{"Project", m.projectInput.Value()},
				{"Directory", m.projectSlug()},
				{"Go module", m.modulePreview()},
				{"Path", m.projectPath()},
				{"Demo App", map[bool]string{true: "On", false: "Off"}[m.config.Render.Components.DemoApp]},
				{"Queue driver", selectedQueueDriverSummary(m)},
				{"Components", componentNames},
			}),
		)
		panels = append(panels, m.panelWithTitle("Confirm your project", confirmBody, m.termWidth, m.stage == StageConfirm))
		if m.stage == StageConfirm {
			actions = []string{"Enter to create", "Shift+Tab to go back", "Esc to cancel"}
		}
	}

	if m.stage == StageDone {
		panels = append(panels, m.panelWithTitle("Project initialized", successStyle.Render("Project initialized and .goforj.yml created!"), m.termWidth, false))
	}

	view := ""
	if len(panels) > 0 {
		view = lipgloss.JoinVertical(lipgloss.Left, panels...)
	}
	if len(actions) > 0 {
		view = lipgloss.JoinVertical(lipgloss.Left, view, renderFooter(actions, m.termWidth))
	}
	if m.errorMsg != "" && m.stage != StageProjectPath {
		view = lipgloss.JoinVertical(lipgloss.Left, view, errorStyle.Render(m.errorMsg))
	}
	return view + "\n"
}

func (m model) panelWithTitle(title, content string, termWidth int, active bool) string {
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

	contentWidth := targetWidth - 4
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
	for _, line := range lines {
		lineWidth := lipgloss.Width(line)
		if lineWidth < contentWidth {
			line += strings.Repeat(" ", contentWidth-lineWidth)
		}
		padded = append(padded, panelBorderStyle.Render("│")+" "+line+" "+panelBorderStyle.Render("│"))
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

func (m model) renderComponentList(termWidth int) string {
	items := m.componentList.Items()
	if len(items) == 0 {
		return ""
	}

	var rows []string
	for i, listItem := range m.componentList.Items() {
		item := listItem.(ListItem)
		isFocused := m.componentList.Index() == i
		pointer := "  "
		if isFocused {
			pointer = listCursorStyle.Render("› ")
		}

		marker := normalStyle.Render("○")
		if item.Selected {
			marker = normalStyle.Render("●")
		}

		label := item.Name
		if strings.TrimSpace(item.Desc) != "" {
			label += " (" + item.Desc + ")"
		}
		labelStyle := listOptionMutedStyle
		if item.Selected {
			labelStyle = listNameStyle
		} else if isFocused {
			labelStyle = listDescStyle
		}
		rows = append(rows, pointer+marker+" "+labelStyle.Render(label))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (m model) renderQueueDriverList(termWidth int) string {
	items := m.queueDriverList.Items()
	if len(items) == 0 {
		return ""
	}

	var rows []string
	for i, listItem := range items {
		item, ok := listItem.(QueueDriverItem)
		if !ok {
			continue
		}
		isFocused := m.queueDriverList.Index() == i
		pointer := "  "
		if isFocused {
			pointer = listCursorStyle.Render("› ")
		}

		marker := normalStyle.Render("○")
		if isFocused {
			marker = normalStyle.Render("●")
		}

		label := item.Label
		if strings.TrimSpace(item.Desc) != "" {
			label += " (" + item.Desc + ")"
		}
		labelStyle := listOptionMutedStyle
		if isFocused {
			labelStyle = listNameStyle
		}
		rows = append(rows, pointer+marker+" "+labelStyle.Render(label))
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

func selectedQueueDriverSummary(m model) string {
	if !m.config.Render.Components.Jobs {
		return "n/a"
	}

	driver := normalizeQueueDriver(m.config.Render.QueueDriver)
	if driver == "" {
		index := m.queueDriverList.Index()
		if index >= 0 && index < len(m.queueDriverList.Items()) {
			if item, ok := m.queueDriverList.Items()[index].(QueueDriverItem); ok {
				driver = item.Driver
			}
		}
		if driver == "" {
			driver = "redis"
		}
	}
	for _, option := range queueDriverOptions() {
		if option.Name == driver {
			return option.Title
		}
	}
	return driver
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
	return view + padding
}

func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	var lines []string
	var current string
	for _, word := range words {
		if current == "" {
			current = word
			continue
		}
		if lipgloss.Width(current)+1+lipgloss.Width(word) <= width {
			current += " " + word
			continue
		}
		lines = append(lines, current)
		current = word
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
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

func renderSectionHeader(title string, termWidth int) string {
	label := headerLabelStyle.Render(title)
	prefix := ruleStyle.Render("─") + " " + label
	width := lipgloss.Width(prefix)
	if width < wizardWidth {
		width = wizardWidth
	}
	if termWidth <= 0 {
		termWidth = wizardWidth
	}
	if termWidth < width {
		width = termWidth
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
	return table
}

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

func (m model) renderProgress() string {
	steps := []struct {
		label string
		stage WizardStage
	}{
		{"Project", StageProjectName},
		{"Module", StageModuleName},
		{"Components", StageSelectComponents},
		{"Extras", StageExtras},
		{"Runtime", StageRuntime},
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

func (m *model) setAllComponents(selected bool) {
	for idx, listItem := range m.componentList.Items() {
		item := listItem.(ListItem)
		if item.Name == "CLI" {
			item.Selected = true
			m.componentList.SetItem(idx, item)
			continue
		}
		if item.Name == "Database (Postgres)" || item.Name == "Database (SQLite)" {
			item.Selected = false
			m.componentList.SetItem(idx, item)
			continue
		}
		item.Selected = selected
		m.componentList.SetItem(idx, item)
	}
}

// deselectComponent clears a component selection by name.
func (m *model) deselectComponent(name string) {
	for idx, listItem := range m.componentList.Items() {
		item := listItem.(ListItem)
		if item.Name != name {
			continue
		}
		item.Selected = false
		m.componentList.SetItem(idx, item)
		return
	}
}

func (m *model) selectComponent(name string) {
	for idx, listItem := range m.componentList.Items() {
		item := listItem.(ListItem)
		if item.Name != name {
			continue
		}
		item.Selected = true
		m.componentList.SetItem(idx, item)
		return
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
		return "Path is not empty.", false
	}
	return "Path exists and is empty.", true
}

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

func (*NewProjectCmd) Signature() string {
	return `name:"new" help:"New project command"`
}

func NewNewProjectCmd(logger *logger.AppLogger, renderer *ProjectRenderer) *NewProjectCmd {
	return &NewProjectCmd{
		logger:   logger,
		renderer: renderer,
	}
}

func (c *NewProjectCmd) Run() error {
	printNewProjectBanner()

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
	err = runWithLoader("Rendering project files", func() error {
		return c.renderer.Render(i)
	})
	if err != nil {
		return err
	}

	return nil
}

func runWithLoader(message string, fn func() error) error {
	done := make(chan struct{})
	var fnErr atomic.Value

	go func() {
		defer close(done)
		if err := fn(); err != nil {
			fnErr.Store(err)
		}
	}()

	frames := []string{"|", "/", "-", "\\"}
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()

	index := 0
	fmt.Printf("%s %s %s\r", console.ActionMark(), message, frames[index])

	for {
		select {
		case <-done:
			fmt.Print("\r")
			if err, ok := fnErr.Load().(error); ok && err != nil {
				return err
			}
			return nil
		case <-ticker.C:
			index = (index + 1) % len(frames)
			fmt.Printf("%s %s %s\r", console.ActionMark(), message, frames[index])
		}
	}
}
