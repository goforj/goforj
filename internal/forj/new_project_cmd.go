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
	StageDone
)

var (
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true) // Green
	cursorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true) // Cyan
	normalStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))             // Gray
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
	history            string
	projectInput       textinput.Model
	moduleInput        textinput.Model
	componentList      list.Model
	selectedComponents []string
	config             ProjectConfig
	cancelled          bool
}

func initialModel() model {
	ti := textinput.New()
	ti.Placeholder = "My Awesome App"
	ti.Focus()
	ti.CharLimit = 64
	ti.Width = 30

	li := list.New(makeComponentItems(), list.NewDefaultDelegate(), 30, 10)
	li.Title = "Select Components (space to toggle, enter to continue)"
	li.SetShowFilter(false)

	return model{
		stage:         StageProjectName,
		projectInput:  ti,
		moduleInput:   textinput.New(),
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
				m.history += fmt.Sprintf("🚀 Project Name: %s\n\n", m.projectInput.Value())
				m.config.ProjectName = m.projectInput.Value()
				m.stage = StageModuleName
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
			switch msg.String() {
			case "enter":
				m.history += fmt.Sprintf("📦 Module Path: %s\n\n", m.moduleInput.Value())
				m.config.GoModuleName = m.moduleInput.Value()
				m.stage = StageSelectComponents
				return m, nil
			}
			var cmd tea.Cmd
			m.moduleInput, cmd = m.moduleInput.Update(msg)
			return m, cmd

		case StageSelectComponents:
			switch msg.String() {
			case "enter":
				for _, item := range m.componentList.Items() {
					it := item.(ListItem)
					if it.Selected {
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

				m.stage = StageDone
				return m, tea.Quit
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
		}
	}

	return m, nil
}

func (m model) View() string {
	output := m.history

	switch m.stage {
	case StageProjectName:
		output += fmt.Sprintf("🚀 Enter your project name:\n\n%s\n\n(Press Enter to continue)", m.projectInput.View())
	case StageModuleName:
		output += fmt.Sprintf("📦 Enter your Go module path:\n\n%s\n\n(Press Enter to continue)", m.moduleInput.View())
	case StageSelectComponents:
		var s string
		s += "🛠  Choose your application components:\n\n"

		for i, listItem := range m.componentList.Items() {
			item := listItem.(ListItem)

			// Cursor
			cursor := " "
			if m.componentList.Index() == i {
				cursor = cursorStyle.Render(">")
			}

			// Special handling for CLI (fixed)
			if item.Name == "CLI" {
				checkbox := selectedStyle.Render("[✓]")
				name := selectedStyle.Render(item.Name + " (required)")
				s += fmt.Sprintf("%s %s %s\n", cursor, checkbox, name)
				continue
			}

			// Normal component rows
			checkbox := normalStyle.Render("[ ]")
			if item.Selected {
				checkbox = selectedStyle.Render("[✓]")
			}

			name := normalStyle.Render(item.Name)
			if item.Selected {
				name = selectedStyle.Render(item.Name)
			}

			s += fmt.Sprintf("%s %s %s\n", cursor, checkbox, name)
		}

		output += fmt.Sprintf("%s\n(Use arrows to move, space to toggle, enter to finish)", s)
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

		output += "🎉 Project initialized and .goforj.yml created!\n\n"
	}
	return output
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
