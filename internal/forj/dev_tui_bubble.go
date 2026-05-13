package forj

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	charmansi "github.com/charmbracelet/x/ansi"
	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/project"
)

type devBubbleWriter struct {
	mu          sync.Mutex
	program     *tea.Program
	done        chan struct{}
	partial     string
	ansiTail    string
	disabled    bool
	footerLine  string
	defaultLine string
	statusLine  string
}

type devBubbleModel struct {
	width          int
	height         int
	lines          []string
	tools          []devToolLink
	footerLine     string
	statusLine     string
	footerEnabled  bool
	helpVisible    bool
	apiURL         string
	lighthouseURL  string
	dbQuery        bool
	appDebug       string
	followMode     bool
	viewportTop    int
	unreadCount    int
	filterVisible  bool
	componentShown map[string]bool
	searchMode     bool
	searchQuery    string
	searchMatches  []int
	searchIndex    int
	requestRestart func()
	requestRender  func()
}

type devAppendLinesMsg struct{ lines []string }
type devSetFooterMsg struct{ line string }
type devResetFooterMsg struct{}
type devSetStatusMsg struct{ line string }
type devClearStatusMsg struct{}
type devMarkStatusDoneMsg struct{}
type devSetFooterEnabledMsg struct{ enabled bool }
type devClearTranscriptMsg struct{}
type devRefreshEnvMsg struct {
	apiURL        string
	lighthouseURL string
	dbQuery       bool
	appDebug      string
	tools         []devToolLink
}
type devQuitMsg struct{}

var devTranscriptComponentPattern = regexp.MustCompile(`^\d{2}:\d{2}:\d{2}\.\d{3}\s+([A-Za-z][A-Za-z0-9_-]*)\s+`)

var devComponentFilterOrder = []string{
	"HTTP",
	"Jobs",
	"Scheduler",
	"System",
	"Error",
	"Database",
	"Cache",
}

func newDevBubbleWriter(config *project.Config, requestRestart func(), requestRender func()) *devBubbleWriter {
	apiURL := resolveAPIURL(nil)
	lighthouseURL := resolveLighthouseUIURL(nil)
	dbQuery, appDebug := loadDevRuntimeSettings()
	tools := collectDevToolLinks(config, nil)
	footer := buildDevFooterLineWithState(apiURL, lighthouseURL, dbQuery, appDebug)
	model := devBubbleModel{
		footerLine:     footer,
		footerEnabled:  true,
		tools:          tools,
		apiURL:         apiURL,
		lighthouseURL:  lighthouseURL,
		dbQuery:        dbQuery,
		appDebug:       appDebug,
		followMode:     true,
		componentShown: defaultDevComponentShown(),
		searchIndex:    -1,
		requestRestart: requestRestart,
		requestRender:  requestRender,
	}
	done := make(chan struct{})
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithoutSignalHandler())
	go func() {
		defer close(done)
		_, _ = program.Run()
	}()
	return &devBubbleWriter{
		program:     program,
		done:        done,
		footerLine:  footer,
		defaultLine: footer,
	}
}

func (w *devBubbleWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	raw := w.ansiTail + string(p)
	cleanRaw, tail := splitANSITail(raw)
	w.ansiTail = tail
	cleanRaw = sanitizeCSI(cleanRaw)
	cleanRaw = strings.ReplaceAll(cleanRaw, "\r", "")
	if w.disabled {
		return len(p), nil
	}
	input := w.partial + cleanRaw
	lines := strings.Split(input, "\n")
	w.partial = lines[len(lines)-1]
	if len(lines) > 1 {
		payload := append([]string(nil), lines[:len(lines)-1]...)
		w.program.Send(devAppendLinesMsg{lines: payload})
	}
	return len(p), nil
}

func (w *devBubbleWriter) Close() error {
	w.mu.Lock()
	if w.partial != "" {
		w.program.Send(devAppendLinesMsg{lines: []string{w.partial}})
		w.partial = ""
	}
	w.mu.Unlock()
	w.program.Send(devQuitMsg{})
	<-w.done
	return nil
}

func (w *devBubbleWriter) DisableFooter() {
	w.mu.Lock()
	w.disabled = true
	w.mu.Unlock()
	w.program.Send(devSetFooterEnabledMsg{enabled: false})
}

func (w *devBubbleWriter) EnableFooter() {
	w.mu.Lock()
	w.disabled = false
	w.mu.Unlock()
	w.program.Send(devSetFooterEnabledMsg{enabled: true})
}

func (w *devBubbleWriter) SetFooterLine(line string) {
	w.mu.Lock()
	w.footerLine = line
	w.mu.Unlock()
	w.program.Send(devSetFooterMsg{line: line})
}

func (w *devBubbleWriter) ResetFooterLine() {
	w.mu.Lock()
	line := w.defaultLine
	w.footerLine = line
	w.mu.Unlock()
	w.program.Send(devResetFooterMsg{})
}

func (w *devBubbleWriter) SetStatusLine(line string) {
	w.mu.Lock()
	w.statusLine = line
	w.mu.Unlock()
	w.program.Send(devSetStatusMsg{line: line})
}

func (w *devBubbleWriter) MarkStatusDone() {
	w.mu.Lock()
	line := strings.TrimSpace(w.statusLine)
	w.statusLine = ""
	w.mu.Unlock()
	if line != "" {
		w.program.Send(devAppendLinesMsg{lines: []string{console.SuccessMark() + " " + line}})
	}
	w.program.Send(devMarkStatusDoneMsg{})
}

func (w *devBubbleWriter) ClearStatusLine() {
	w.mu.Lock()
	w.statusLine = ""
	w.mu.Unlock()
	w.program.Send(devClearStatusMsg{})
}

func (w *devBubbleWriter) HasStatusLine() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.TrimSpace(w.statusLine) != ""
}

func (w *devBubbleWriter) ClearBuffer() {
	w.mu.Lock()
	w.partial = ""
	w.ansiTail = ""
	w.mu.Unlock()
	w.program.Send(devClearTranscriptMsg{})
}

func (w *devBubbleWriter) RefreshEnv(config *project.Config) {
	apiURL := resolveAPIURL(nil)
	lighthouseURL := resolveLighthouseUIURL(nil)
	dbQuery, appDebug := loadDevRuntimeSettings()
	tools := collectDevToolLinks(config, nil)
	footer := buildDevFooterLineWithState(apiURL, lighthouseURL, dbQuery, appDebug)
	w.mu.Lock()
	w.footerLine = footer
	w.defaultLine = footer
	w.mu.Unlock()
	w.program.Send(devRefreshEnvMsg{
		apiURL:        apiURL,
		lighthouseURL: lighthouseURL,
		dbQuery:       dbQuery,
		appDebug:      appDebug,
		tools:         tools,
	})
}

func (m devBubbleModel) Init() tea.Cmd { return nil }

func (m devBubbleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case devAppendLinesMsg:
		if !m.followMode {
			m.unreadCount += countDevVisibleLines(msg.lines, m.componentShown)
		}
		m.lines = append(m.lines, msg.lines...)
		m.updateSearchMatches()
	case devSetFooterMsg:
		m.footerLine = msg.line
	case devResetFooterMsg:
	case devSetStatusMsg:
		m.statusLine = msg.line
	case devClearStatusMsg, devMarkStatusDoneMsg:
		m.statusLine = ""
	case devSetFooterEnabledMsg:
		m.footerEnabled = msg.enabled
	case devClearTranscriptMsg:
		m.lines = nil
		m.viewportTop = 0
		m.unreadCount = 0
		m.searchMatches = nil
		m.searchIndex = -1
	case devRefreshEnvMsg:
		m.apiURL = msg.apiURL
		m.lighthouseURL = msg.lighthouseURL
		m.dbQuery = msg.dbQuery
		m.appDebug = msg.appDebug
		m.tools = msg.tools
		m.footerLine = buildDevFooterLineWithState(msg.apiURL, msg.lighthouseURL, msg.dbQuery, msg.appDebug)
	case devQuitMsg:
		return m, tea.Quit
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, devForwardInterruptCmd()
		}
		if m.searchMode {
			switch msg.String() {
			case "esc":
				m.searchMode = false
				m.searchQuery = ""
				m.updateSearchMatches()
			case "enter":
				m.searchMode = false
				m.updateSearchMatches()
				m.jumpToCurrentSearchMatch()
			case "backspace":
				if len(m.searchQuery) > 0 {
					runes := []rune(m.searchQuery)
					m.searchQuery = string(runes[:len(runes)-1])
					m.updateSearchMatches()
				}
			default:
				if len(msg.Runes) > 0 && !msg.Alt {
					m.searchQuery += string(msg.Runes)
					m.updateSearchMatches()
				}
			}
			return m, nil
		}
		if m.filterVisible {
			switch msg.String() {
			case "esc", "f":
				m.filterVisible = false
			case "a":
				m.componentShown = defaultDevComponentShown()
				m.unreadCount = 0
				m.viewportTop = 0
				m.followMode = true
				m.updateSearchMatches()
			case "1", "2", "3", "4", "5", "6", "7":
				index := int(msg.Runes[0] - '1')
				if index >= 0 && index < len(devComponentFilterOrder) {
					component := devComponentFilterOrder[index]
					m.componentShown[component] = !m.componentShown[component]
					m.unreadCount = 0
					m.viewportTop = 0
					m.followMode = true
					m.updateSearchMatches()
				}
			}
			return m, nil
		}
		switch msg.String() {
		case "?":
			m.helpVisible = !m.helpVisible
		case "esc":
			if m.helpVisible {
				m.helpVisible = false
			} else if strings.TrimSpace(m.searchQuery) != "" || len(m.searchMatches) > 0 {
				m.searchMode = false
				m.searchQuery = ""
				m.searchMatches = nil
				m.searchIndex = -1
			}
		case "/":
			m.searchMode = true
			m.searchQuery = ""
			m.searchMatches = nil
			m.searchIndex = -1
		case "tab":
			m.jumpSearch(1)
		case "shift+tab":
			m.jumpSearch(-1)
		case "f":
			m.filterVisible = !m.filterVisible
		case "up", "k":
			m.scrollUp(1)
		case "down", "j":
			m.scrollDown(1)
		case "pgup", "b":
			m.scrollUp(m.bodyHeight())
		case "pgdown", " ":
			m.scrollDown(m.bodyHeight())
		case "home", "g":
			m.scrollToTop()
		case "end", "G":
			m.scrollToBottom()
		case "r":
			if !m.helpVisible && m.requestRestart != nil {
				m.requestRestart()
				m.lines = append(m.lines, console.ActionMark()+" Restart requested")
			}
		case "ctrl+r":
			if !m.helpVisible && m.requestRender != nil {
				m.requestRender()
				m.lines = append(m.lines, console.ActionMark()+" Render requested")
			}
		case "c":
			if !m.helpVisible {
				m.lines = nil
			}
		case "q":
			if !m.helpVisible {
				if err := toggleDevQueryLogging(); err == nil {
					m.dbQuery, m.appDebug = loadDevRuntimeSettings()
					m.footerLine = buildDevFooterLineWithState(m.apiURL, m.lighthouseURL, m.dbQuery, m.appDebug)
					if m.requestRestart != nil {
						m.requestRestart()
					}
					m.lines = append(m.lines, console.SuccessMark()+" DB_QUERY_LOGGING="+map[bool]string{true: "true", false: "false"}[m.dbQuery])
				}
			}
		case "o":
			if !m.helpVisible && m.lighthouseURL != "" {
				_ = openURL(m.lighthouseURL)
			}
		case "a":
			if !m.helpVisible && m.apiURL != "" {
				_ = openURL(m.apiURL)
			}
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			if !m.helpVisible {
				index := int(msg.Runes[0] - '1')
				if index >= 0 && index < len(m.tools) && strings.TrimSpace(m.tools[index].URL) != "" {
					_ = openURL(m.tools[index].URL)
				}
			}
		case ")", "!", "@", "#":
			if !m.helpVisible {
				level := map[string]string{")": "0", "!": "1", "@": "2", "#": "3"}[msg.String()]
				if err := setDevAppDebugLevel(level); err == nil {
					m.dbQuery, m.appDebug = loadDevRuntimeSettings()
					m.footerLine = buildDevFooterLineWithState(m.apiURL, m.lighthouseURL, m.dbQuery, m.appDebug)
					if m.requestRestart != nil {
						m.requestRestart()
					}
					m.lines = append(m.lines, console.SuccessMark()+" APP_DEBUG="+level)
				}
			}
		}
	}
	return m, nil
}

func devForwardInterruptCmd() tea.Cmd {
	return func() tea.Msg {
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
		return nil
	}
}

func (m devBubbleModel) View() string {
	if m.helpVisible {
		return renderDevHotkeyModal(m.tools, m.dbQuery, m.appDebug)
	}
	if m.filterVisible {
		return renderDevFilterModal(m.componentShown)
	}
	width := m.width
	if width <= 0 {
		width = 120
	}
	height := m.height
	if height <= 0 {
		height = 30
	}
	footerLines := 0
	headerLines := 0
	footer := ""
	header := ""
	if len(m.tools) > 0 {
		header = buildDevResourceHeaderLine(m.tools) + "\n" + buildDevFooterSeparatorLine()
		headerLines = 2
	}
	if m.footerEnabled {
		footer = buildDevFooterSeparatorLine() + "\n" + m.footerLine
		footerLines = 2
	}
	status := m.contextStatusLine()
	statusDecorated := ""
	statusLines := 0
	if strings.TrimSpace(status) != "" {
		if strings.TrimSpace(m.statusLine) != "" {
			statusDecorated = console.Colorize(console.ColorGreen, "•") + " " + status
		} else {
			statusDecorated = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#A1A1AA"}).
				Render(status)
		}
		statusLines = 1
	}
	bodyHeight := height - footerLines - statusLines - headerLines
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	lines := wrapDevTranscriptLines(filterDevTranscriptLines(m.lines, m.componentShown), width)
	lines = normalizeDevTranscriptLines(lines, header != "")
	var viewportStart int
	lines, viewportStart = m.viewportLines(lines, bodyHeight)
	lines = decorateDevSearchMatches(lines, viewportStart, m.searchMatches, m.searchIndex)
	body := strings.Join(lines, "\n")
	if pad := bodyHeight - len(lines); pad > 0 {
		if body != "" {
			body += "\n"
		}
		body += strings.Repeat("\n", pad-1)
	}
	parts := make([]string, 0, 4)
	if header != "" {
		parts = append(parts, header)
	}
	parts = append(parts, body)
	if statusDecorated != "" {
		parts = append(parts, statusDecorated)
	}
	if footer != "" {
		parts = append(parts, footer)
	}
	return strings.Join(parts, "\n")
}

func (m *devBubbleModel) bodyHeight() int {
	width := m.width
	if width <= 0 {
		width = 120
	}
	height := m.height
	if height <= 0 {
		height = 30
	}
	headerLines := 0
	if len(m.tools) > 0 {
		headerLines = 2
	}
	footerLines := 0
	if m.footerEnabled {
		footerLines = 2
	}
	statusLines := 0
	if strings.TrimSpace(m.contextStatusLine()) != "" {
		statusLines = 1
	}
	bodyHeight := height - footerLines - headerLines - statusLines
	if bodyHeight < 1 {
		return 1
	}
	return bodyHeight
}

func (m *devBubbleModel) visibleTranscriptLines() []string {
	width := m.width
	if width <= 0 {
		width = 120
	}
	lines := wrapDevTranscriptLines(filterDevTranscriptLines(m.lines, m.componentShown), width)
	return normalizeDevTranscriptLines(lines, len(m.tools) > 0)
}

func (m *devBubbleModel) viewportLines(lines []string, bodyHeight int) ([]string, int) {
	if len(lines) <= bodyHeight {
		m.viewportTop = 0
		m.followMode = true
		m.unreadCount = 0
		return lines, 0
	}
	maxTop := len(lines) - bodyHeight
	if m.followMode {
		m.viewportTop = maxTop
		m.unreadCount = 0
		return lines[maxTop:], maxTop
	}
	if m.viewportTop < 0 {
		m.viewportTop = 0
	}
	if m.viewportTop > maxTop {
		m.viewportTop = maxTop
	}
	return lines[m.viewportTop : m.viewportTop+bodyHeight], m.viewportTop
}

func (m *devBubbleModel) scrollUp(n int) {
	lines := m.visibleTranscriptLines()
	bodyHeight := m.bodyHeight()
	if len(lines) <= bodyHeight {
		m.followMode = true
		m.viewportTop = 0
		m.unreadCount = 0
		return
	}
	maxTop := len(lines) - bodyHeight
	if m.followMode {
		m.followMode = false
		m.viewportTop = maxTop
	}
	m.viewportTop -= n
	if m.viewportTop < 0 {
		m.viewportTop = 0
	}
}

func (m *devBubbleModel) scrollDown(n int) {
	lines := m.visibleTranscriptLines()
	bodyHeight := m.bodyHeight()
	if len(lines) <= bodyHeight {
		m.followMode = true
		m.viewportTop = 0
		m.unreadCount = 0
		return
	}
	maxTop := len(lines) - bodyHeight
	if m.followMode {
		return
	}
	m.viewportTop += n
	if m.viewportTop >= maxTop {
		m.scrollToBottom()
		return
	}
}

func (m *devBubbleModel) scrollToTop() {
	m.followMode = false
	m.viewportTop = 0
}

func (m *devBubbleModel) scrollToBottom() {
	m.followMode = true
	m.viewportTop = 0
	m.unreadCount = 0
}

func (m *devBubbleModel) updateSearchMatches() {
	if strings.TrimSpace(m.searchQuery) == "" {
		m.searchMatches = nil
		m.searchIndex = -1
		return
	}
	query := strings.ToLower(m.searchQuery)
	lines := m.visibleTranscriptLines()
	matches := make([]int, 0, len(lines))
	for i, line := range lines {
		if strings.Contains(strings.ToLower(stripANSIForSearch(line)), query) {
			matches = append(matches, i)
		}
	}
	m.searchMatches = matches
	if len(matches) == 0 {
		m.searchIndex = -1
		return
	}
	if m.searchIndex < 0 || m.searchIndex >= len(matches) {
		m.searchIndex = 0
	}
}

func (m *devBubbleModel) jumpSearch(delta int) {
	if len(m.searchMatches) == 0 {
		return
	}
	if m.searchIndex < 0 {
		m.searchIndex = 0
	} else {
		m.searchIndex = (m.searchIndex + delta + len(m.searchMatches)) % len(m.searchMatches)
	}
	m.jumpToCurrentSearchMatch()
}

func (m *devBubbleModel) jumpToCurrentSearchMatch() {
	if len(m.searchMatches) == 0 || m.searchIndex < 0 || m.searchIndex >= len(m.searchMatches) {
		return
	}
	bodyHeight := m.bodyHeight()
	target := m.searchMatches[m.searchIndex]
	m.followMode = false
	m.viewportTop = target - bodyHeight/2
	if m.viewportTop < 0 {
		m.viewportTop = 0
	}
	lines := m.visibleTranscriptLines()
	maxTop := len(lines) - bodyHeight
	if maxTop < 0 {
		maxTop = 0
	}
	if m.viewportTop > maxTop {
		m.viewportTop = maxTop
	}
}

func (m devBubbleModel) contextStatusLine() string {
	if strings.TrimSpace(m.statusLine) != "" {
		return m.statusLine
	}
	if m.searchMode {
		return "Find /" + m.searchQuery + "  [Enter apply] [Esc clear]"
	}
	parts := make([]string, 0, 4)
	if strings.TrimSpace(m.searchQuery) != "" {
		matchState := "0/0"
		if len(m.searchMatches) > 0 && m.searchIndex >= 0 {
			matchState = fmt.Sprintf("%d/%d", m.searchIndex+1, len(m.searchMatches))
		}
		parts = append(parts, fmt.Sprintf("Find %s (%s)  [Tab next] [Shift+Tab prev] [Esc clear]", m.searchQuery, matchState))
	}
	if !m.followMode {
		follow := "Follow OFF"
		if m.unreadCount > 0 {
			follow = fmt.Sprintf("Follow OFF · %d new", m.unreadCount)
		}
		parts = append(parts, follow)
	}
	if active := activeDevComponentFilters(m.componentShown); len(active) > 0 {
		parts = append(parts, "Filters "+strings.Join(active, ","))
	}
	return strings.Join(parts, "  |  ")
}

func defaultDevComponentShown() map[string]bool {
	shown := make(map[string]bool, len(devComponentFilterOrder))
	for _, component := range devComponentFilterOrder {
		shown[component] = true
	}
	return shown
}

func countDevVisibleLines(lines []string, shown map[string]bool) int {
	count := 0
	for _, line := range lines {
		if devTranscriptLineVisible(line, shown) {
			count++
		}
	}
	return count
}

func filterDevTranscriptLines(lines []string, shown map[string]bool) []string {
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if devTranscriptLineVisible(line, shown) {
			filtered = append(filtered, line)
		}
	}
	return filtered
}

func devTranscriptLineVisible(line string, shown map[string]bool) bool {
	component := devTranscriptComponent(line)
	if component == "" {
		return true
	}
	visible, ok := shown[component]
	if !ok {
		return true
	}
	return visible
}

func devTranscriptComponent(line string) string {
	plain := stripANSIForSearch(line)
	matches := devTranscriptComponentPattern.FindStringSubmatch(plain)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

func activeDevComponentFilters(shown map[string]bool) []string {
	active := make([]string, 0, len(shown))
	for _, component := range devComponentFilterOrder {
		if shown[component] {
			active = append(active, component)
		}
	}
	if len(active) == len(devComponentFilterOrder) {
		return nil
	}
	return active
}

func stripANSIForSearch(s string) string {
	return ansiCSI.ReplaceAllString(s, "")
}

func decorateDevSearchMatches(lines []string, viewportStart int, matches []int, currentMatch int) []string {
	if len(lines) == 0 || len(matches) == 0 {
		return lines
	}
	matchSet := make(map[int]bool, len(matches))
	currentLine := -1
	if currentMatch >= 0 && currentMatch < len(matches) {
		currentLine = matches[currentMatch]
	}
	for _, match := range matches {
		matchSet[match] = true
	}
	highlighted := append([]string(nil), lines...)
	for i, line := range highlighted {
		global := viewportStart + i
		if !matchSet[global] {
			continue
		}
		if global == currentLine {
			highlighted[i] = renderDevCurrentSearchMatch(line)
			continue
		}
		highlighted[i] = renderDevSearchMatch(line)
	}
	return highlighted
}

func renderDevSearchMatch(line string) string {
	return applyDevLineHighlight(line, "\x1b[48;5;236m")
}

func renderDevCurrentSearchMatch(line string) string {
	return applyDevLineHighlight(line, "\x1b[48;5;24m\x1b[1m")
}

func applyDevLineHighlight(line, prefix string) string {
	if line == "" {
		return prefix + "\x1b[0m"
	}
	reapplied := strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+prefix)
	return prefix + reapplied + "\x1b[0m"
}

func wrapDevTranscriptLines(lines []string, width int) []string {
	if width <= 0 || len(lines) == 0 {
		return lines
	}
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			wrapped = append(wrapped, "")
			continue
		}
		wrapped = append(wrapped, wrapDevTranscriptLine(line, width)...)
	}
	return wrapped
}

func wrapDevTranscriptLine(line string, width int) []string {
	const continuationPrefix = "  · "

	if width <= 0 {
		return []string{line}
	}

	if prefix, metadata, ok := splitDevTranscriptMetadataBoundary(line); ok {
		prefixWidth := charmansi.StringWidth(prefix)
		indentWidth := charmansi.StringWidth(continuationPrefix)
		if prefixWidth > 0 && prefixWidth < width && indentWidth < width {
			firstWidth := width - prefixWidth
			metadataParts := strings.Split(charmansi.Wrap(metadata, firstWidth, ""), "\n")
			if len(metadataParts) > 0 {
				lines := make([]string, 0, len(metadataParts))
				lines = append(lines, prefix+metadataParts[0])
				continuationWidth := width - indentWidth
				if continuationWidth < 1 {
					continuationWidth = width
				}
				for _, part := range metadataParts[1:] {
					wrappedPart := strings.Split(charmansi.Wrap(part, continuationWidth, ""), "\n")
					for _, continuation := range wrappedPart {
						lines = append(lines, continuationPrefix+continuation)
					}
				}
				return lines
			}
		}
	}

	return strings.Split(charmansi.Wrap(line, width, ""), "\n")
}

func splitDevTranscriptMetadataBoundary(line string) (string, string, bool) {
	idx := strings.IndexRune(line, '→')
	if idx < 0 {
		return "", "", false
	}

	split := idx + len("→")
	for split < len(line) {
		switch {
		case strings.HasPrefix(line[split:], "\x1b["):
			end := strings.IndexByte(line[split:], 'm')
			if end < 0 {
				return "", "", false
			}
			split += end + 1
		case line[split] == ' ':
			split++
		default:
			return line[:split], line[split:], true
		}
	}

	return line, "", true
}

func normalizeDevTranscriptLines(lines []string, hasHeader bool) []string {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	lines = lines[start:]
	if !hasHeader || len(lines) == 0 {
		return lines
	}
	separator := strings.TrimSpace(buildDevFooterSeparatorLine())
	if strings.TrimSpace(lines[0]) == separator {
		return lines[1:]
	}
	return lines
}

func toggleDevQueryLogging() error {
	content, err := os.ReadFile(".env")
	if err != nil {
		return err
	}
	current := strings.TrimSpace(readEnvKey(string(content), "DB_QUERY_LOGGING"))
	next := "true"
	if current == "1" || strings.EqualFold(current, "true") {
		next = "false"
	}
	updated := updateEnvKey(string(content), "DB_QUERY_LOGGING", next)
	return os.WriteFile(".env", []byte(updated), 0o644)
}

func setDevAppDebugLevel(level string) error {
	content, err := os.ReadFile(".env")
	if err != nil {
		return err
	}
	updated := updateEnvKey(string(content), "APP_DEBUG", level)
	return os.WriteFile(".env", []byte(updated), 0o644)
}

func buildDevOutputWritersBubble(config *project.Config, requestRestart func(), requestRender func()) (io.Writer, io.Writer, func(), func()) {
	writer := newDevBubbleWriter(config, requestRestart, requestRender)
	return writer, writer, func() { _ = writer.Close() }, func() { writer.RefreshEnv(config) }
}
