package forj

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	charmansi "github.com/charmbracelet/x/ansi"
	"github.com/goforj/console"
	"github.com/goforj/goforj/project"
	"golang.org/x/term"
)

const devBubbleFlushDelay = 20 * time.Millisecond

type devBubbleWriter struct {
	mu          sync.Mutex
	program     *tea.Program
	done        chan struct{}
	partial     string
	ansiTail    string
	pending     []string
	flushTimer  *time.Timer
	disabled    bool
	footerLine  string
	defaultLine string
	statusLine  string
	inputState  *term.State
	outputState *term.State
	closed      bool
}

type devBubbleModel struct {
	width            int
	height           int
	lines            []string
	tools            []devToolLink
	commands         []devAppCommandOption
	commandError     string
	footerLine       string
	statusLine       string
	footerEnabled    bool
	helpVisible      bool
	apiURL           string
	lighthouseURL    string
	dbQuery          bool
	appDebug         string
	followMode       bool
	viewportTop      int
	unreadCount      int
	filterVisible    bool
	commandVisible   bool
	commandIndex     int
	commandArgs      string
	commandArgsFocus bool
	commandJump      string
	commandJumpAt    time.Time
	componentShown   map[string]bool
	searchMode       bool
	searchQuery      string
	searchMatches    []int
	searchIndex      int
	cachedLines      []string
	cacheWidth       int
	cacheHasHeader   bool
	cacheValid       bool
	requestRestart   func()
	requestRender    func()
	requestCommand   func(devShellCommandRequest)
}

type devAppendLinesMsg struct{ lines []string }

// devResetFooterMsg carries the refreshed default into Bubble's independently owned model state.
type devResetFooterMsg struct{ line string }
type devSetStatusMsg struct{ line string }
type devClearStatusMsg struct{}
type devMarkStatusDoneMsg struct{}
type devSetFooterEnabledMsg struct{ enabled bool }
type devRefreshEnvMsg struct {
	apiURL        string
	lighthouseURL string
	dbQuery       bool
	appDebug      string
	tools         []devToolLink
	commands      []devAppCommandOption
	commandError  string
}
type devQuitMsg struct{}

var devTranscriptComponentPattern = regexp.MustCompile(`^\d{2}:\d{2}:\d{2}\.\d{3}\s+(?:[a-z][a-z0-9_-]*\s+)?([A-Za-z][A-Za-z0-9_-]*)\s+`)
var devAppCommandLinePattern = regexp.MustCompile(`^\s{2}(\S+)\s{2,}(.+)$`)
var devMetadataSeparatorPattern = regexp.MustCompile(`\s*(?:\x1b\[[0-9;]*m)*·(?:\x1b\[[0-9;]*m)*\s*`)
var devComponentFilterOrder = []string{
	"HTTP",
	"Jobs",
	"Scheduler",
	"System",
	"Error",
	"Database",
	"Cache",
}

type devAppCommandOption struct {
	Name        string
	Help        string
	AcceptsArgs bool
}

type devBubbleRuntimeState struct {
	apiURL        string
	lighthouseURL string
	dbQuery       bool
	appDebug      string
	tools         []devToolLink
	commands      []devAppCommandOption
	commandError  string
	footerLine    string
}

func newDevBubbleWriter(config *project.Config, requestRestart func(), requestRender func(), requestCommand func(devShellCommandRequest)) *devBubbleWriter {
	state := loadDevBubbleRuntimeState(config)
	inputState, outputState := captureDevTerminalState()
	model := devBubbleModel{
		footerLine:     state.footerLine,
		footerEnabled:  true,
		tools:          state.tools,
		commands:       state.commands,
		commandError:   state.commandError,
		apiURL:         state.apiURL,
		lighthouseURL:  state.lighthouseURL,
		dbQuery:        state.dbQuery,
		appDebug:       state.appDebug,
		followMode:     true,
		componentShown: defaultDevComponentShown(),
		searchIndex:    -1,
		requestRestart: requestRestart,
		requestRender:  requestRender,
		requestCommand: requestCommand,
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
		footerLine:  state.footerLine,
		defaultLine: state.footerLine,
		inputState:  inputState,
		outputState: outputState,
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
		w.pending = append(w.pending, payload...)
		w.scheduleFlushLocked()
	}
	return len(p), nil
}

func (w *devBubbleWriter) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	w.stopFlushTimerLocked()
	w.flushPendingLocked()
	if w.partial != "" {
		w.program.Send(devAppendLinesMsg{lines: []string{w.partial}})
		w.partial = ""
	}
	w.mu.Unlock()
	defer restoreDevTerminalState(w.inputState, w.outputState)
	w.program.Send(devQuitMsg{})
	select {
	case <-w.done:
	case <-time.After(500 * time.Millisecond):
		w.program.Kill()
		<-w.done
	}
	return nil
}

// captureDevTerminalState saves the caller's terminal modes before Bubble Tea mutates them.
func captureDevTerminalState() (*term.State, *term.State) {
	var inputState *term.State
	var outputState *term.State
	if term.IsTerminal(int(os.Stdin.Fd())) {
		inputState, _ = term.GetState(int(os.Stdin.Fd()))
	}
	if term.IsTerminal(int(os.Stdout.Fd())) {
		outputState, _ = term.GetState(int(os.Stdout.Fd()))
	}
	return inputState, outputState
}

// restoreDevTerminalState defensively resets terminal modes that can leak after fatal dev exits.
func restoreDevTerminalState(inputState *term.State, outputState *term.State) {
	if inputState != nil {
		_ = term.Restore(int(os.Stdin.Fd()), inputState)
	}
	if outputState != nil {
		_ = term.Restore(int(os.Stdout.Fd()), outputState)
	}
	forceDevTerminalModeReset()
}

// forceDevTerminalModeReset uses the controlling TTY because stdout may not be the terminal input device.
func forceDevTerminalModeReset() {
	if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		defer tty.Close()
		runDevTTYSttySane(tty)
		_, _ = io.WriteString(tty, devTerminalModeResetSequence)
		return
	}
	if term.IsTerminal(int(os.Stdout.Fd())) {
		runDevTTYSttySane(os.Stdout)
		_, _ = io.WriteString(os.Stdout, devTerminalModeResetSequence)
	}
}

// runDevTTYSttySane restores canonical output flags such as newline carriage return mapping.
func runDevTTYSttySane(tty *os.File) {
	cmd := exec.Command("stty", "sane")
	cmd.Stdin = tty
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	_ = cmd.Run()
}

const devTerminalModeResetSequence = "\x1b[>u\x1b[<u\x1b[?1004l\x1b[?2004l\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?1049l\x1b[?25h\r\n"

func (w *devBubbleWriter) scheduleFlushLocked() {
	if len(w.pending) == 0 {
		return
	}
	if w.flushTimer == nil {
		w.flushTimer = time.AfterFunc(devBubbleFlushDelay, w.flushPending)
		return
	}
	w.flushTimer.Reset(devBubbleFlushDelay)
}

func (w *devBubbleWriter) stopFlushTimerLocked() {
	if w.flushTimer == nil {
		return
	}
	w.flushTimer.Stop()
	w.flushTimer = nil
}

func (w *devBubbleWriter) flushPending() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.flushPendingLocked()
}

func (w *devBubbleWriter) flushPendingLocked() {
	if len(w.pending) == 0 {
		return
	}
	payload := append([]string(nil), w.pending...)
	w.pending = nil
	if w.flushTimer != nil {
		w.flushTimer.Stop()
		w.flushTimer = nil
	}
	w.program.Send(devAppendLinesMsg{lines: payload})
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

// ResetFooterLine restores the latest environment-derived footer after transient dev work completes.
func (w *devBubbleWriter) ResetFooterLine() {
	w.mu.Lock()
	line := w.defaultLine
	w.footerLine = line
	w.mu.Unlock()
	w.program.Send(devResetFooterMsg{line: line})
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

func (w *devBubbleWriter) RefreshEnv(config *project.Config) {
	state := loadDevBubbleRuntimeState(config)
	w.mu.Lock()
	w.footerLine = state.footerLine
	w.defaultLine = state.footerLine
	w.mu.Unlock()
	w.program.Send(devRefreshEnvMsg{
		apiURL:        state.apiURL,
		lighthouseURL: state.lighthouseURL,
		dbQuery:       state.dbQuery,
		appDebug:      state.appDebug,
		tools:         state.tools,
		commands:      state.commands,
		commandError:  state.commandError,
	})
}

func loadDevBubbleRuntimeState(config *project.Config) devBubbleRuntimeState {
	apiURL := resolveAPIURL(nil)
	lighthouseURL := resolveLighthouseUIURL(nil)
	dbQuery, appDebug := loadDevRuntimeSettings()
	tools := collectDevToolLinks(config, nil)
	commands, commandError := loadDevAppCommands()
	return devBubbleRuntimeState{
		apiURL:        apiURL,
		lighthouseURL: lighthouseURL,
		dbQuery:       dbQuery,
		appDebug:      appDebug,
		tools:         tools,
		commands:      commands,
		commandError:  commandError,
		footerLine:    buildDevFooterLineWithState(apiURL, lighthouseURL, dbQuery, appDebug),
	}
}

func (m devBubbleModel) Init() tea.Cmd { return nil }

// Update serializes terminal events through Bubble Tea so transcript and control state stay synchronized.
func (m devBubbleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.invalidateVisibleTranscriptCache()
	case devAppendLinesMsg:
		if !m.followMode {
			m.unreadCount += countDevVisibleLines(msg.lines, m.componentShown)
		}
		m.lines = append(m.lines, msg.lines...)
		m.invalidateVisibleTranscriptCache()
		m.updateSearchMatches()
	case devResetFooterMsg:
		m.footerLine = msg.line
	case devSetStatusMsg:
		m.statusLine = msg.line
	case devClearStatusMsg, devMarkStatusDoneMsg:
		m.statusLine = ""
	case devSetFooterEnabledMsg:
		m.footerEnabled = msg.enabled
	case devRefreshEnvMsg:
		m.apiURL = msg.apiURL
		m.lighthouseURL = msg.lighthouseURL
		m.dbQuery = msg.dbQuery
		m.appDebug = msg.appDebug
		if len(msg.tools) > 0 {
			m.tools = msg.tools
		}
		m.commands = msg.commands
		m.commandError = msg.commandError
		m.footerLine = buildDevFooterLineWithState(msg.apiURL, msg.lighthouseURL, msg.dbQuery, msg.appDebug)
		m.invalidateVisibleTranscriptCache()
	case devQuitMsg:
		return m, tea.Quit
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, devForwardInterruptCmd()
		}
		helpVisible := m.helpVisible
		if m.searchMode {
			switch msg.String() {
			case "esc":
				m.searchMode = false
				m.searchQuery = ""
				m.updateSearchMatches()
				m.scrollToBottom()
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
				m.invalidateVisibleTranscriptCache()
				m.updateSearchMatches()
			case "1", "2", "3", "4", "5", "6", "7":
				index := int(msg.Runes[0] - '1')
				if index >= 0 && index < len(devComponentFilterOrder) {
					component := devComponentFilterOrder[index]
					m.componentShown[component] = !m.componentShown[component]
					m.unreadCount = 0
					m.viewportTop = 0
					m.followMode = true
					m.invalidateVisibleTranscriptCache()
					m.updateSearchMatches()
				}
			}
			return m, nil
		}
		if m.commandVisible {
			switch msg.String() {
			case "esc", "x":
				m.commandVisible = false
				m.commandArgs = ""
				m.commandArgsFocus = false
				m.commandJump = ""
			case "up", "k":
				m.moveCommandSelection(-1)
			case "down", "j":
				m.moveCommandSelection(1)
			case "tab":
				if m.selectedCommandAcceptsArgs() {
					m.commandArgsFocus = true
				}
			case "shift+tab":
				m.commandArgsFocus = false
			case "enter":
				req := m.selectedCommandRequest()
				if req.ShellCommand != "" {
					m.commandVisible = false
					m.commandArgs = ""
					m.commandArgsFocus = false
					m.commandJump = ""
					m.requestCommand(req)
				}
			case "backspace":
				if m.commandArgsFocus && len(m.commandArgs) > 0 {
					runes := []rune(m.commandArgs)
					m.commandArgs = string(runes[:len(runes)-1])
				} else if len(m.commandJump) > 0 {
					runes := []rune(m.commandJump)
					m.commandJump = string(runes[:len(runes)-1])
					m.commandJumpAt = time.Now()
					m.jumpCommandSelection(m.commandJump)
				}
			default:
				if len(msg.Runes) > 0 && !msg.Alt {
					m.handleCommandTextInput(string(msg.Runes))
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
				m.scrollToBottom()
			}
		case "/":
			if m.helpVisible {
				m.helpVisible = false
			}
			m.searchMode = true
			m.searchQuery = ""
			m.searchMatches = nil
			m.searchIndex = -1
		case "tab":
			m.jumpSearch(1)
		case "shift+tab":
			m.jumpSearch(-1)
		case "f":
			if m.helpVisible {
				m.helpVisible = false
			}
			m.filterVisible = !m.filterVisible
		case "x":
			if m.helpVisible {
				m.helpVisible = false
			}
			m.commandVisible = !m.commandVisible
			m.commandArgsFocus = false
			if m.commandVisible && m.commandIndex >= len(m.commands) {
				m.commandIndex = 0
			}
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
		case "end", "G", "l":
			m.scrollToBottom()
		case "r":
			if helpVisible {
				m.helpVisible = false
			}
			m.requestRestart()
			m.lines = append(m.lines, console.ActionMark()+" Restart requested")
		case "ctrl+r":
			if helpVisible {
				m.helpVisible = false
			}
			m.requestRender()
			m.lines = append(m.lines, console.ActionMark()+" Render requested")
		case "c":
			if helpVisible {
				m.helpVisible = false
			}
			m.lines = nil
			m.viewportTop = 0
			m.unreadCount = 0
			m.searchQuery = ""
			m.searchMatches = nil
			m.searchIndex = -1
			m.invalidateVisibleTranscriptCache()
		case "q":
			if helpVisible {
				m.helpVisible = false
			}
			if err := toggleDevQueryLogging(); err == nil {
				queryOn, _ := loadDevRuntimeSettings()
				m.applyRuntimeSettingChange(console.SuccessMark() + " DB_QUERY_LOGGING=" + map[bool]string{true: "true", false: "false"}[queryOn])
			}
		case "o":
			if m.lighthouseURL != "" {
				if helpVisible {
					m.helpVisible = false
				}
				_ = openURL(m.lighthouseURL)
			}
		case "a":
			if m.apiURL != "" {
				if helpVisible {
					m.helpVisible = false
				}
				_ = openURL(m.apiURL)
			}
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			index := int(msg.Runes[0] - '1')
			if index >= 0 && index < len(m.tools) && strings.TrimSpace(m.tools[index].URL) != "" {
				if helpVisible {
					m.helpVisible = false
				}
				_ = openURL(m.tools[index].URL)
			}
		case ")", "!", "@", "#":
			if helpVisible {
				m.helpVisible = false
			}
			level := map[string]string{")": "0", "!": "1", "@": "2", "#": "3"}[msg.String()]
			if err := setDevAppDebugLevel(level); err == nil {
				m.applyRuntimeSettingChange(console.SuccessMark() + " APP_DEBUG=" + level)
			}
		}
	}
	return m, nil
}

// devForwardInterruptCmd forwards Ctrl+C through the platform signal path observed by DevCmd.
func devForwardInterruptCmd() tea.Cmd {
	return devForwardInterruptCmdWith(signalDevInterrupt)
}

// devForwardInterruptCmdWith converts the platform interrupt into the message shape Bubble Tea expects.
func devForwardInterruptCmdWith(signalInterrupt func() error) tea.Cmd {
	return func() tea.Msg {
		_ = signalInterrupt()
		return nil
	}
}

func (m devBubbleModel) View() string {
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
	header = buildDevResourceHeaderLine(m.tools) + "\n" + buildDevFooterSeparatorLine()
	headerLines = 2
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
			prefix := lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#0369A1", Dark: "#38BDF8"}).
				Bold(true).
				Render("◆")
			body := lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#4B5563", Dark: "#CBD5E1"}).
				Render(status)
			statusDecorated = prefix + " " + body
		}
		statusLines = 1
	}
	bodyHeight := height - footerLines - statusLines - headerLines
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	lines := m.visibleTranscriptLines()
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
	base := strings.Join(parts, "\n")
	overlay := m.currentOverlay()
	if overlay == "" {
		return base
	}
	return renderDevBubbleOverlay(base, overlay, width, height)
}

func (m devBubbleModel) currentOverlay() string {
	switch {
	case m.commandVisible:
		return buildDevCommandModalBox(m.commands, m.commandIndex, m.commandArgs, m.commandArgsFocus, m.commandError)
	case m.filterVisible:
		return buildDevFilterModalBox(m.componentShown)
	case m.helpVisible:
		return buildDevHotkeyModalBox(m.tools, m.dbQuery, m.appDebug)
	default:
		return ""
	}
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
	headerLines := 2
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

// applyRuntimeSettingChange restarts the App so the process and newly persisted footer state agree.
func (m *devBubbleModel) applyRuntimeSettingChange(successLine string) {
	m.dbQuery, m.appDebug = loadDevRuntimeSettings()
	m.footerLine = buildDevFooterLineWithState(m.apiURL, m.lighthouseURL, m.dbQuery, m.appDebug)
	m.requestRestart()
	m.lines = append(m.lines, successLine)
}

func (m *devBubbleModel) visibleTranscriptLines() []string {
	width := m.width
	if width <= 0 {
		width = 120
	}
	hasHeader := true
	if m.cacheValid && m.cacheWidth == width && m.cacheHasHeader == hasHeader {
		return m.cachedLines
	}
	lines := wrapDevTranscriptLines(filterDevTranscriptLines(m.lines, m.componentShown), width)
	lines = normalizeDevTranscriptLines(lines, hasHeader)
	m.cachedLines = lines
	m.cacheWidth = width
	m.cacheHasHeader = hasHeader
	m.cacheValid = true
	return lines
}

func (m *devBubbleModel) invalidateVisibleTranscriptCache() {
	m.cachedLines = nil
	m.cacheWidth = 0
	m.cacheHasHeader = false
	m.cacheValid = false
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
	if m.commandVisible {
		return ""
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
		parts = append(parts, fmt.Sprintf("Find %s (%s)  [Tab/Shift+Tab] [Esc]", m.searchQuery, matchState))
	}
	if !m.followMode {
		follow := "Paused  [PgUp(b)/PgDn(space)] [l live]"
		if m.unreadCount > 0 {
			follow = fmt.Sprintf("%s · %d new", follow, m.unreadCount)
		}
		parts = append(parts, follow)
	}
	if active := activeDevComponentFilters(m.componentShown); len(active) > 0 {
		parts = append(parts, "Filters "+strings.Join(active, ","))
	}
	return strings.Join(parts, "  |  ")
}

func (m *devBubbleModel) moveCommandSelection(delta int) {
	if len(m.commands) == 0 {
		m.commandIndex = 0
		return
	}
	m.commandIndex = (m.commandIndex + delta + len(m.commands)) % len(m.commands)
	m.commandArgs = ""
	m.commandArgsFocus = false
	m.commandJump = ""
}

func (m *devBubbleModel) handleCommandTextInput(text string) {
	if text == "" {
		return
	}
	if m.commandArgsFocus && m.selectedCommandAcceptsArgs() {
		m.commandArgs += text
		return
	}
	now := time.Now()
	if now.Sub(m.commandJumpAt) > 1200*time.Millisecond {
		m.commandJump = ""
	}
	m.commandJump += strings.ToLower(text)
	m.commandJumpAt = now
	m.jumpCommandSelection(m.commandJump)
}

func (m *devBubbleModel) jumpCommandSelection(prefix string) {
	if prefix == "" || len(m.commands) == 0 {
		return
	}
	prefix = strings.ToLower(prefix)
	start := m.commandIndex
	for offset := 0; offset < len(m.commands); offset++ {
		idx := (start + offset) % len(m.commands)
		if strings.HasPrefix(strings.ToLower(m.commands[idx].Name), prefix) {
			m.commandIndex = idx
			m.commandArgs = ""
			return
		}
	}
}

func (m devBubbleModel) selectedCommandAcceptsArgs() bool {
	if len(m.commands) == 0 || m.commandIndex < 0 || m.commandIndex >= len(m.commands) {
		return false
	}
	return m.commands[m.commandIndex].AcceptsArgs
}

func (m devBubbleModel) selectedCommandRequest() devShellCommandRequest {
	if len(m.commands) == 0 || m.commandIndex < 0 || m.commandIndex >= len(m.commands) {
		return devShellCommandRequest{}
	}
	selected := m.commands[m.commandIndex]
	args := strings.TrimSpace(m.commandArgs)
	appBinary := activeDevAppBinaryPath()
	shellCommand := appBinary + " " + selected.Name
	display := appBinary + " " + selected.Name
	if args != "" {
		shellCommand += " " + args
		display += " " + args
	}
	return devShellCommandRequest{
		DisplayName:  display,
		ShellCommand: shellCommand,
	}
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

func renderDevBubbleOverlay(base, overlay string, width, height int) string {
	lines := strings.Split(overlay, "\n")
	panelWidth := 0
	for _, line := range lines {
		if w := lipgloss.Width(line); w > panelWidth {
			panelWidth = w
		}
	}
	panelHeight := len(lines)
	if panelWidth <= 0 || panelHeight == 0 {
		return base
	}
	top := (height - panelHeight) / 2
	left := (width - panelWidth) / 2
	if top < 0 {
		top = 0
	}
	if left < 0 {
		left = 0
	}
	baseLines := strings.Split(base, "\n")
	if len(baseLines) < height {
		baseLines = append(baseLines, make([]string, height-len(baseLines))...)
	}
	for i, line := range lines {
		row := top + i
		if row < 0 || row >= len(baseLines) {
			continue
		}
		baseLines[row] = overlayDevBubbleLine(baseLines[row], line, left, panelWidth)
	}
	return strings.Join(baseLines, "\n")
}

func overlayDevBubbleLine(base, overlay string, left, overlayWidth int) string {
	if left < 0 {
		left = 0
	}
	if overlayWidth < 0 {
		overlayWidth = 0
	}
	leftSegment := charmansi.Cut(base, 0, left)
	leftPad := left - charmansi.StringWidth(leftSegment)
	if leftPad < 0 {
		leftPad = 0
	}
	overlayPad := overlayWidth - charmansi.StringWidth(overlay)
	if overlayPad < 0 {
		overlayPad = 0
	}
	baseWidth := charmansi.StringWidth(base)
	rightSegment := ""
	if baseWidth > left+overlayWidth {
		rightSegment = charmansi.Cut(base, left+overlayWidth, baseWidth)
	}
	return leftSegment + strings.Repeat(" ", leftPad) + overlay + strings.Repeat(" ", overlayPad) + rightSegment
}

func decorateDevSearchMatches(lines []string, viewportStart int, matches []int, currentMatch int) []string {
	if len(lines) == 0 || len(matches) == 0 {
		return lines
	}
	currentLine := -1
	if currentMatch >= 0 && currentMatch < len(matches) {
		currentLine = matches[currentMatch]
	}
	highlighted := append([]string(nil), lines...)
	start := sort.Search(len(matches), func(i int) bool { return matches[i] >= viewportStart })
	end := sort.Search(len(matches), func(i int) bool { return matches[i] >= viewportStart+len(lines) })
	if start >= end {
		return lines
	}
	matchSet := make(map[int]struct{}, end-start)
	for _, match := range matches[start:end] {
		matchSet[match] = struct{}{}
	}
	for i, line := range highlighted {
		global := viewportStart + i
		if _, ok := matchSet[global]; !ok {
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
	const fieldSeparator = " · "

	if width <= 0 {
		return []string{line}
	}

	if prefix, metadata, ok := splitDevTranscriptMetadataBoundary(line); ok {
		prefixWidth := charmansi.StringWidth(prefix)
		indentWidth := charmansi.StringWidth(continuationPrefix)
		if prefixWidth > 0 && prefixWidth < width && indentWidth < width {
			firstWidth := width - prefixWidth
			continuationWidth := width - indentWidth
			if continuationWidth < 1 {
				continuationWidth = width
			}
			lines := wrapDevMetadataFields(prefix, firstWidth, continuationPrefix, continuationWidth, metadata)
			if len(lines) > 0 {
				return lines
			}
		}
	}

	return strings.Split(charmansi.Wrap(line, width, ""), "\n")
}

func wrapDevMetadataFields(firstPrefix string, firstWidth int, continuationPrefix string, continuationWidth int, metadata string) []string {
	const fieldSeparator = " · "

	fields := splitDevMetadataFields(metadata)
	if len(fields) == 0 {
		return nil
	}

	lines := make([]string, 0, len(fields))
	currentPrefix := firstPrefix
	currentWidth := firstWidth
	current := ""

	flushCurrent := func() {
		if current == "" {
			return
		}
		lines = append(lines, currentPrefix+current)
		current = ""
		currentPrefix = continuationPrefix
		currentWidth = continuationWidth
	}

	flushSingleField := func(field string) {
		if field == "" {
			return
		}
		wrappedField := strings.Split(charmansi.Wrap(field, currentWidth, ""), "\n")
		if len(wrappedField) == 0 {
			return
		}
		for _, continuation := range wrappedField {
			if strings.TrimSpace(continuation) == "" {
				continue
			}
			lines = append(lines, currentPrefix+continuation)
			currentPrefix = continuationPrefix
			currentWidth = continuationWidth
		}
	}

	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		candidate := field
		if current != "" {
			candidate = current + fieldSeparator + field
		}
		if current == "" && charmansi.StringWidth(field) > currentWidth {
			flushSingleField(field)
			continue
		}
		if charmansi.StringWidth(candidate) <= currentWidth {
			current = candidate
			continue
		}
		flushCurrent()
		if charmansi.StringWidth(field) > currentWidth {
			flushSingleField(field)
			continue
		}
		current = field
	}
	flushCurrent()
	return lines
}

func splitDevMetadataFields(metadata string) []string {
	const fieldSeparator = " · "

	indices := devMetadataSeparatorPattern.FindAllStringIndex(metadata, -1)
	if len(indices) == 0 {
		return strings.Split(metadata, fieldSeparator)
	}

	fields := make([]string, 0, len(indices)+1)
	start := 0
	for _, idx := range indices {
		field := strings.TrimSpace(metadata[start:idx[0]])
		if field != "" {
			fields = append(fields, field)
		}
		start = idx[1]
	}
	if tail := strings.TrimSpace(metadata[start:]); tail != "" {
		fields = append(fields, tail)
	}
	return fields
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

func parseDevAppHelpCommands(help string) []devAppCommandOption {
	plain := stripANSIForSearch(help)
	lines := strings.Split(plain, "\n")
	commands := make([]devAppCommandOption, 0, len(lines))
	seen := map[string]bool{}
	for _, line := range lines {
		matches := devAppCommandLinePattern.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}
		name := strings.TrimSpace(matches[1])
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		commands = append(commands, devAppCommandOption{
			Name: name,
			Help: strings.TrimSpace(matches[2]),
		})
	}
	return commands
}

func parseDevAppCommandAcceptsArgs(help string) bool {
	plain := stripANSIForSearch(help)
	for _, line := range strings.Split(plain, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(trimmed, "› ") {
			return true
		}
	}
	return false
}

// buildDevOutputSessionBubble couples both terminal streams to one Bubble Tea lifecycle.
func buildDevOutputSessionBubble(config *project.Config, requestRestart func(), requestRender func(), requestCommand func(devShellCommandRequest)) devOutputSession {
	writer := newDevBubbleWriter(config, requestRestart, requestRender, requestCommand)
	return devOutputSession{
		stdout:   writer,
		stderr:   writer,
		shutdown: func() { _ = writer.Close() },
		refresh:  func() { writer.RefreshEnv(config) },
	}
}
