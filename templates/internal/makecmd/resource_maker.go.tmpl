package makecmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

const resourceWizardWidth = 58

var (
	resourcePrimaryStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#f5f6f7"))
	resourceMutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#8b93a1"))
	resourceAccentStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#8C97E6")).Bold(true)
	resourceErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#c97b7b"))
	resourceBorderStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#f5f6f7"))
)

// ResourceInput carries raw command input keyed by resource field name.
type ResourceInput map[string]string

// ResourceValues carries resolved command input keyed by resource field name.
type ResourceValues map[string]string

// FieldDefault computes a default value from already resolved values.
type FieldDefault func(ResourceValues) string

// FieldValidator validates a resolved field value.
type FieldValidator func(string) error

// EnvWriteBuilder builds env writes from resolved values.
type EnvWriteBuilder func(ResourceValues) []EnvWrite

// ResourceMaker defines a reusable make command contract.
type ResourceMaker struct {
	CommandName string
	Kind        string
	Fields      []ResourceField
	Env         ResourceEnv
	Examples    []string
	BuildWrites EnvWriteBuilder
}

// ResourceField defines one field shared by direct CLI input and wizard input.
type ResourceField struct {
	Name                  string
	Label                 string
	Prompt                string
	Description           string
	Required              bool
	PromptWhenInteractive bool
	Default               FieldDefault
	Validate              FieldValidator
}

// ResourceEnv defines where env writes should be applied.
type ResourceEnv struct {
	File        string
	Section     string
	InsertAfter string
}

// ResourceMakerResult describes the completed resource maker run.
type ResourceMakerResult struct {
	Values  ResourceValues
	Changes []EnvChange
}

// Run resolves resource input, writes env values, and returns the resulting changes.
func (m ResourceMaker) Run(input ResourceInput) (ResourceMakerResult, error) {
	values, err := m.resolveValues(input)
	if err != nil {
		return ResourceMakerResult{}, err
	}
	writes := m.BuildWrites(values)
	changes, err := ApplyEnvSectionWrites(m.Env.File, m.Env.Section, m.Env.InsertAfter, writes)
	if err != nil {
		return ResourceMakerResult{}, err
	}
	return ResourceMakerResult{Values: values, Changes: changes}, nil
}

// resolveValues resolves command values from direct input and optional prompts.
func (m ResourceMaker) resolveValues(input ResourceInput) (ResourceValues, error) {
	if m.shouldPrompt(input) {
		return promptResourceFields(m, input)
	}
	return m.resolveValuesWithoutPrompt(input)
}

// shouldPrompt reports whether missing required input should start the wizard.
func (m ResourceMaker) shouldPrompt(input ResourceInput) bool {
	if !resourceMakerInteractive() {
		return false
	}
	for _, field := range m.Fields {
		if field.Required && strings.TrimSpace(input[field.Name]) == "" {
			return true
		}
	}
	return false
}

// resolveValuesWithoutPrompt resolves command values without opening the wizard.
func (m ResourceMaker) resolveValuesWithoutPrompt(input ResourceInput) (ResourceValues, error) {
	values := make(ResourceValues, len(m.Fields))
	for _, field := range m.Fields {
		value := strings.TrimSpace(input[field.Name])
		if value == "" && field.Default != nil {
			value = strings.TrimSpace(field.Default(values))
		}
		if value == "" && field.Required {
			return nil, m.missingFieldError(field)
		}
		if value != "" && field.Validate != nil {
			if err := field.Validate(value); err != nil {
				return nil, err
			}
		}
		values[field.Name] = value
	}
	return values, nil
}

// missingFieldError builds the clean non-interactive missing field error.
func (m ResourceMaker) missingFieldError(field ResourceField) error {
	label := strings.TrimSpace(field.Label)
	if label == "" {
		label = field.Name
	}
	message := fmt.Sprintf("missing %s", strings.ToLower(label))
	if example := m.firstExample(); example != "" {
		message += "\nexample: " + example
	}
	return fmt.Errorf("%s", message)
}

// firstExample returns the first configured command example.
func (m ResourceMaker) firstExample() string {
	for _, example := range m.Examples {
		if clean := strings.TrimSpace(example); clean != "" {
			return clean
		}
	}
	return ""
}

// promptResourceFields prompts for missing resource fields using the reusable wizard renderer.
func promptResourceFields(maker ResourceMaker, input ResourceInput) (ResourceValues, error) {
	model := newResourceWizardModel(maker, input)
	final, err := tea.NewProgram(model).Run()
	if err != nil {
		return nil, err
	}
	wizard, ok := final.(resourceWizardModel)
	if !ok {
		return nil, fmt.Errorf("unexpected prompt result")
	}
	if wizard.cancelled {
		return nil, fmt.Errorf("cancelled")
	}
	return wizard.values, nil
}

// resourceWizardModel renders a compact resource creation wizard.
type resourceWizardModel struct {
	maker         ResourceMaker
	input         textinput.Model
	values        ResourceValues
	promptIndexes []int
	promptPos     int
	defaultValue  string
	errorMessage  string
	done          bool
	cancelled     bool
	termWidth     int
}

// newResourceWizardModel builds a resource wizard from the maker field specs.
func newResourceWizardModel(maker ResourceMaker, input ResourceInput) resourceWizardModel {
	values := make(ResourceValues, len(maker.Fields))
	promptIndexes := make([]int, 0, len(maker.Fields))
	for index, field := range maker.Fields {
		value := strings.TrimSpace(input[field.Name])
		if value != "" {
			values[field.Name] = value
			continue
		}
		if field.Required || field.PromptWhenInteractive {
			promptIndexes = append(promptIndexes, index)
			continue
		}
		if field.Default != nil {
			values[field.Name] = strings.TrimSpace(field.Default(values))
		}
	}
	model := resourceWizardModel{
		maker:         maker,
		values:        values,
		promptIndexes: promptIndexes,
		termWidth:     resourceWizardWidth,
	}
	model.configureInput()
	return model
}

// Init starts the text input cursor.
func (m resourceWizardModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles prompt key events and input changes.
func (m resourceWizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.termWidth = msg.Width
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "enter":
			value := strings.TrimSpace(m.input.Value())
			if value == "" {
				value = m.defaultValue
			}
			field := m.currentField()
			if value == "" && field.Required {
				m.errorMessage = "required"
				return m, nil
			}
			if value != "" && field.Validate != nil {
				if err := field.Validate(value); err != nil {
					m.errorMessage = err.Error()
					return m, nil
				}
			}
			m.values[field.Name] = value
			m.errorMessage = ""
			m.promptPos++
			if m.promptPos >= len(m.promptIndexes) {
				if err := m.finalizeValues(); err != nil {
					m.errorMessage = err.Error()
					m.promptPos--
					return m, nil
				}
				m.done = true
				return m, tea.Quit
			}
			m.configureInput()
			return m, textinput.Blink
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// View renders the wizard.
func (m resourceWizardModel) View() string {
	if m.done || m.cancelled {
		return ""
	}
	view := strings.Join(m.panels(), "\n")
	if strings.TrimSpace(m.errorMessage) != "" {
		view += "\n" + resourceErrorStyle.Render("x "+m.errorMessage)
	}
	view += "\n" + resourceMutedStyle.Render("Enter to continue · Esc to cancel")
	return view + "\n"
}

// configureInput prepares the text input for the current field.
func (m *resourceWizardModel) configureInput() {
	if m.promptPos >= len(m.promptIndexes) {
		return
	}
	field := m.currentField()
	defaultValue := ""
	if field.Default != nil {
		defaultValue = strings.TrimSpace(field.Default(m.values))
	}
	input := textinput.New()
	input.Prompt = resourceAccentStyle.Render("› ")
	input.SetValue(defaultValue)
	input.Focus()
	m.input = input
	m.defaultValue = defaultValue
}

// currentField returns the field currently being prompted.
func (m resourceWizardModel) currentField() ResourceField {
	if m.promptPos >= len(m.promptIndexes) {
		return ResourceField{}
	}
	return m.maker.Fields[m.promptIndexes[m.promptPos]]
}

// finalizeValues fills defaults and validates the completed wizard values.
func (m *resourceWizardModel) finalizeValues() error {
	for _, field := range m.maker.Fields {
		value := strings.TrimSpace(m.values[field.Name])
		if value == "" && field.Default != nil {
			value = strings.TrimSpace(field.Default(m.values))
			m.values[field.Name] = value
		}
		if value == "" && field.Required {
			return fmt.Errorf("%s is required", resourceFieldPromptLabel(field))
		}
		if value != "" && field.Validate != nil {
			if err := field.Validate(value); err != nil {
				return err
			}
		}
	}
	return nil
}

// title returns the wizard panel title.
func (m resourceWizardModel) title() string {
	if clean := strings.TrimSpace(m.maker.Kind); clean != "" {
		return strTitle(clean)
	}
	return "Resource"
}

// panels returns completed field panels plus the active input panel.
func (m resourceWizardModel) panels() []string {
	panels := make([]string, 0, len(m.maker.Fields))
	currentIndex := -1
	if m.promptPos < len(m.promptIndexes) {
		currentIndex = m.promptIndexes[m.promptPos]
	}
	for index, field := range m.maker.Fields {
		if index > currentIndex && currentIndex >= 0 {
			break
		}
		title := resourceFieldPromptLabel(field)
		if index == currentIndex {
			panels = append(panels, resourceWizardPanel(title, resourceWizardActiveContent(field, m.input, m.termWidth), m.termWidth, true))
			continue
		}
		value := strings.TrimSpace(m.values[field.Name])
		if value != "" {
			panels = append(panels, resourceWizardPanel(title, resourcePrimaryStyle.Render(value), m.termWidth, false))
		}
	}
	return panels
}

// resourceWizardActiveContent renders optional field description plus the prompt.
func resourceWizardActiveContent(field ResourceField, input textinput.Model, termWidth int) string {
	lines := make([]string, 0, 3)
	lines = append(lines, resourceActiveInputView(input))
	description := strings.TrimSpace(field.Description)
	if description != "" {
		lines = append(lines, "")
		for _, line := range wrapResourceWizardText(description, resourceWizardContentWidth(termWidth)) {
			lines = append(lines, resourceMutedStyle.Render(line))
		}
	}
	return strings.Join(lines, "\n")
}

// resourceActiveInputView renders the active input without widget-managed padding.
func resourceActiveInputView(input textinput.Model) string {
	value := strings.TrimSpace(input.Value())
	if value == "" {
		return resourceAccentStyle.Render("›")
	}
	return resourceAccentStyle.Render("›") + " " + resourcePrimaryStyle.Render(value)
}

// resourceWizardPanel draws the compact resource wizard box.
func resourceWizardPanel(title string, content string, termWidth int, active bool) string {
	contentWidth := resourceWizardContentWidth(termWidth)
	innerWidth := contentWidth + 2
	titleLabel := " " + strings.TrimSpace(title) + " "
	if active {
		titleLabel = " " + resourcePrimaryStyle.Render(strings.TrimSpace(title)) + " "
	} else {
		titleLabel = " " + resourceMutedStyle.Render(strings.TrimSpace(title)) + " "
	}
	topFill := innerWidth - lipgloss.Width(titleLabel)
	if topFill < 1 {
		topFill = 1
	}
	contentLines := strings.Split(content, "\n")
	lines := make([]string, 0, len(contentLines)+2)
	lines = append(lines, resourceBorderStyle.Render("┌"+titleLabel+strings.Repeat("─", topFill)+"┐"))
	for _, line := range contentLines {
		lines = append(lines, resourceBorderStyle.Render("│")+" "+padRight(line, contentWidth)+" "+resourceBorderStyle.Render("│"))
	}
	lines = append(lines, resourceBorderStyle.Render("└"+strings.Repeat("─", innerWidth)+"┘"))
	return strings.Join(lines, "\n")
}

// resourceWizardContentWidth returns the visible width inside the wizard border.
func resourceWizardContentWidth(termWidth int) int {
	width := resourceWizardWidth
	if termWidth > 0 && termWidth < width {
		width = termWidth
	}
	if width < 34 {
		width = 34
	}
	return width - 4
}

// wrapResourceWizardText wraps text to the visible wizard content width.
func wrapResourceWizardText(value string, width int) []string {
	words := strings.Fields(value)
	if len(words) == 0 {
		return nil
	}
	lines := make([]string, 0, 2)
	current := words[0]
	for _, word := range words[1:] {
		next := current + " " + word
		if lipgloss.Width(next) <= width {
			current = next
			continue
		}
		lines = append(lines, current)
		current = word
	}
	lines = append(lines, current)
	return lines
}

// padRight pads a string to a visible display width.
func padRight(value string, width int) string {
	padding := width - lipgloss.Width(value)
	if padding <= 0 {
		return value
	}
	return value + strings.Repeat(" ", padding)
}

// strTitle returns a simple title-cased display value.
func strTitle(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

// resourceFieldPromptLabel returns the display label for a field prompt.
func resourceFieldPromptLabel(field ResourceField) string {
	label := strings.TrimSpace(field.Prompt)
	if label == "" {
		label = strings.TrimSpace(field.Label)
	}
	if label == "" {
		label = field.Name
	}
	return label
}

// resourceMakerInteractive reports whether prompts can safely be shown.
func resourceMakerInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}
