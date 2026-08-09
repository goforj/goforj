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

const (
	devBubbleFlushDelay      = 20 * time.Millisecond
	devBubbleSpinnerInterval = 80 * time.Millisecond
	devLifecycleBufferLines  = 400
	devLifecycleVisibleLines = 8
	devTerminalProgressClear = "\x1b]9;4;0;0\x07"
	devTerminalProgressBusy  = "\x1b]9;4;3;0\x07"
)

var devBubbleSpinnerFrames = console.DefaultMarks().SpinnerFrames

type devBubbleWriter struct {
	mu                 sync.Mutex
	program            *tea.Program
	done               chan struct{}
	partial            string
	ansiTail           string
	pending            []string
	flushTimer         *time.Timer
	disabled           bool
	footerLine         string
	defaultLine        string
	statusLine         string
	transitions        map[string]devBubbleTransition
	transitionSequence uint64
	lifecycle          *devBubbleLifecycleTransaction
	inputState         *term.State
	outputState        *term.State
	startupTranscript  []string
	closed             bool
}

type devBubbleModel struct {
	width             int
	height            int
	lines             []string
	tools             []devToolLink
	commands          []devAppCommandOption
	commandError      string
	footerLine        string
	statusLine        string
	lifecycleActive   bool
	lifecycleLines    []string
	spinnerFrame      int
	spinnerGeneration uint64
	footerEnabled     bool
	helpVisible       bool
	apiURL            string
	lighthouseURL     string
	dbQuery           bool
	appDebug          string
	followMode        bool
	viewportTop       int
	unreadCount       int
	filterVisible     bool
	commandVisible    bool
	commandIndex      int
	commandArgs       string
	commandArgsFocus  bool
	commandJump       string
	commandJumpAt     time.Time
	componentShown    map[string]bool
	searchMode        bool
	searchQuery       string
	searchMatches     []int
	searchIndex       int
	cachedLines       []string
	cacheWidth        int
	cacheHasHeader    bool
	cacheValid        bool
	requestRestart    func()
	requestRender     func()
	requestCommand    func(devShellCommandRequest)
}

type devAppendLinesMsg struct{ lines []string }
type devSetLifecycleLinesMsg struct {
	active bool
	lines  []string
}

// devResetFooterMsg carries the refreshed default into Bubble's independently owned model state.
type devResetFooterMsg struct{ line string }
type devSetStatusMsg struct{ line string }
type devClearStatusMsg struct{}
type devMarkStatusDoneMsg struct{}
type devSpinnerTickMsg struct{ generation uint64 }
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

type devBubbleTransition struct {
	line     string
	sequence uint64
}

type devBubbleLifecycleTransaction struct {
	transaction devLifecycleTransaction
	lines       []string
}

// streamsGroupedOutput identifies restart work that can move directly into the durable transcript.
func (t *devBubbleLifecycleTransaction) streamsGroupedOutput() bool {
	return t != nil && t.transaction.Kind == devLifecycleRestart && !t.transaction.Detailed
}

// headingLine opens one durable lifecycle block before its child output begins.
func (t *devBubbleLifecycleTransaction) headingLine() string {
	if t == nil {
		return ""
	}
	heading := "App startup"
	if t.transaction.Kind == devLifecycleRestart {
		heading = "App restart"
	}
	return console.Colorize(console.ColorGray, "┏") + " " + joinDevLifecycleFields(heading, t.transaction.Watchers...)
}

// groupedLines associates streamed lifecycle output with its already-open transaction block.
func (t *devBubbleLifecycleTransaction) groupedLines(lines []string) []string {
	rail := console.Colorize(console.ColorGray, "┃")
	grouped := make([]string, 0, len(lines))
	for _, line := range lines {
		grouped = append(grouped, rail+" "+line)
	}
	return grouped
}

// retain captures initial startup output until readiness is known and the terminal story can be committed.
func (t *devBubbleLifecycleTransaction) retain(lines []string) bool {
	if t == nil || t.transaction.Detailed || t.transaction.Kind != devLifecycleStartup {
		return false
	}
	t.lines = append(t.lines, lines...)
	if overflow := len(t.lines) - devLifecycleBufferLines; overflow > 0 {
		t.lines = append([]string(nil), t.lines[overflow:]...)
	}
	return true
}

// transientLines limits redraw payloads while the complete buffer remains available for failure diagnostics.
func (t *devBubbleLifecycleTransaction) transientLines() []string {
	if t == nil || len(t.lines) == 0 {
		return nil
	}
	lines := t.lines
	if len(lines) > devLifecycleVisibleLines {
		lines = lines[len(lines)-devLifecycleVisibleLines:]
	}
	return append([]string(nil), lines...)
}

// successLines closes streamed restarts or commits buffered startup as one bounded lifecycle block.
func (t *devBubbleLifecycleTransaction) successLines(elapsed time.Duration, summary devLifecycleTransactionSummary) []string {
	if t == nil {
		return nil
	}
	bottom := console.Colorize(console.ColorGray, "┗")
	if t.streamsGroupedOutput() {
		return []string{bottom + " " + t.transaction.successLine(elapsed, devLifecycleTransactionSummary{})}
	}
	lines := make([]string, 0, len(t.lines)+2)
	if !t.transaction.Detailed && len(t.lines) > 0 {
		lines = append(lines, t.headingLine())
		lines = append(lines, t.groupedLines(t.lines)...)
		return append(lines, bottom+" "+t.transaction.successLine(elapsed, summary))
	}
	return append(lines, t.transaction.successLine(elapsed, summary))
}

// failureLines expands retained output beneath the failed transaction summary.
func (t *devBubbleLifecycleTransaction) failureLines(elapsed time.Duration, err error) []string {
	if t == nil {
		return nil
	}
	if t.streamsGroupedOutput() {
		lines := []string{}
		if detail := strings.TrimSpace(formatDevLifecycleFailure(err)); detail != "" {
			lines = append(lines, t.groupedLines([]string{detail})...)
		}
		bottom := console.Colorize(console.ColorGray, "┗")
		return append(lines, bottom+" "+t.transaction.failureLine(elapsed))
	}
	lines := []string{t.transaction.failureLine(elapsed)}
	if len(t.lines) > 0 {
		lines = append(lines, "")
		lines = append(lines, t.lines...)
	}
	if detail := formatDevLifecycleFailure(err); detail != "" && !devLifecycleLinesContain(lines, err.Error()) {
		lines = append(lines, detail)
	}
	return lines
}

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

// newDevBubbleWriter centralizes new dev bubble writer behavior so callers follow the same contract.
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
		transitions: make(map[string]devBubbleTransition),
		inputState:  inputState,
		outputState: outputState,
	}
}

// Write accepts output through Go's standard writer contract.
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

// Close releases resources owned by the receiver.
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
	startupTranscript := append([]string(nil), w.startupTranscript...)
	w.mu.Unlock()
	w.program.Send(devQuitMsg{})
	select {
	case <-w.done:
	case <-time.After(500 * time.Millisecond):
		w.program.Kill()
		<-w.done
	}
	restoreDevTerminalState(w.inputState, w.outputState)
	if len(startupTranscript) == 0 {
		return nil
	}
	_, err := io.WriteString(os.Stdout, strings.Join(startupTranscript, "\n")+"\n\n")
	return err
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

// devTerminalModeResetSequence releases terminal modes without advancing past Bubble Tea's restored cursor row.
const devTerminalModeResetSequence = devTerminalProgressClear + "\x1b[>u\x1b[<u\x1b[?1004l\x1b[?2004l\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?1049l\x1b[?25h\r\x1b[2K"

// scheduleFlushLocked centralizes schedule flush locked behavior so callers follow the same contract.
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

// stopFlushTimerLocked centralizes stop flush timer locked behavior so callers follow the same contract.
func (w *devBubbleWriter) stopFlushTimerLocked() {
	if w.flushTimer == nil {
		return
	}
	w.flushTimer.Stop()
	w.flushTimer = nil
}

// flushPending centralizes flush pending behavior so callers follow the same contract.
func (w *devBubbleWriter) flushPending() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.flushPendingLocked()
}

// flushPendingLocked centralizes flush pending locked behavior so callers follow the same contract.
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
	if w.lifecycle != nil && w.lifecycle.streamsGroupedOutput() {
		w.program.Send(devAppendLinesMsg{lines: w.lifecycle.groupedLines(payload)})
		return
	}
	if w.lifecycle != nil && w.lifecycle.retain(payload) {
		w.program.Send(devSetLifecycleLinesMsg{active: true, lines: w.lifecycle.transientLines()})
		return
	}
	w.program.Send(devAppendLinesMsg{lines: payload})
}

// BeginLifecycleTransaction opens restart output immediately while reserving startup output until readiness is known.
func (w *devBubbleWriter) BeginLifecycleTransaction(transaction devLifecycleTransaction) {
	if strings.TrimSpace(transaction.Key) == "" {
		return
	}
	w.mu.Lock()
	w.flushPendingLocked()
	if w.partial != "" {
		w.program.Send(devAppendLinesMsg{lines: []string{w.partial}})
		w.partial = ""
	}
	w.lifecycle = &devBubbleLifecycleTransaction{transaction: transaction}
	if w.lifecycle.streamsGroupedOutput() {
		w.program.Send(devAppendLinesMsg{lines: []string{w.lifecycle.headingLine()}})
	} else {
		w.program.Send(devSetLifecycleLinesMsg{active: true})
	}
	w.setTransitionLocked(transaction.Key, transaction.inProgressLine())
	w.mu.Unlock()
}

// compactLifecycleTransactionActive reports whether the active structured boundary replaces typed runner narration.
func (w *devBubbleWriter) compactLifecycleTransactionActive() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lifecycle != nil && !w.lifecycle.transaction.Detailed
}

// CompleteLifecycleTransaction commits retained lifecycle work as one bounded success block.
func (w *devBubbleWriter) CompleteLifecycleTransaction(key string, elapsed time.Duration, summary devLifecycleTransactionSummary) {
	w.mu.Lock()
	if w.lifecycle == nil || w.lifecycle.transaction.Key != strings.TrimSpace(key) {
		w.mu.Unlock()
		return
	}
	w.flushPendingLocked()
	transaction := w.lifecycle
	if w.partial != "" {
		if transaction.streamsGroupedOutput() {
			w.program.Send(devAppendLinesMsg{lines: transaction.groupedLines([]string{w.partial})})
		}
		w.partial = ""
	}
	w.lifecycle = nil
	w.program.Send(devSetLifecycleLinesMsg{})
	successLines := transaction.successLines(elapsed, summary)
	if transaction.transaction.Kind == devLifecycleStartup {
		w.startupTranscript = append([]string(nil), successLines...)
	}
	w.program.Send(devAppendLinesMsg{lines: successLines})
	w.clearTransitionLocked(key)
	w.mu.Unlock()
}

// FailLifecycleTransaction closes streamed restarts or expands retained startup context with its failure.
func (w *devBubbleWriter) FailLifecycleTransaction(key string, elapsed time.Duration, err error) {
	w.mu.Lock()
	if w.lifecycle == nil || w.lifecycle.transaction.Key != strings.TrimSpace(key) {
		w.mu.Unlock()
		return
	}
	w.flushPendingLocked()
	if w.partial != "" {
		if w.lifecycle.streamsGroupedOutput() {
			w.program.Send(devAppendLinesMsg{lines: w.lifecycle.groupedLines([]string{w.partial})})
		} else {
			w.lifecycle.lines = append(w.lifecycle.lines, w.partial)
		}
		w.partial = ""
	}
	lines := w.lifecycle.failureLines(elapsed, err)
	w.lifecycle = nil
	w.program.Send(devSetLifecycleLinesMsg{})
	w.program.Send(devAppendLinesMsg{lines: lines})
	w.clearTransitionLocked(key)
	w.mu.Unlock()
}

// devLifecycleLinesContain avoids repeating an error already captured from the failing child process.
func devLifecycleLinesContain(lines []string, value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, line := range lines {
		if strings.Contains(stripANSIForSearch(line), value) {
			return true
		}
	}
	return false
}

// DisableFooter centralizes disable footer behavior so callers follow the same contract.
func (w *devBubbleWriter) DisableFooter() {
	w.mu.Lock()
	w.disabled = true
	w.mu.Unlock()
	w.program.Send(devSetFooterEnabledMsg{enabled: false})
}

// EnableFooter centralizes enable footer behavior so callers follow the same contract.
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

// SetStatusLine centralizes set status line behavior so callers follow the same contract.
func (w *devBubbleWriter) SetStatusLine(line string) {
	w.SetTransition(devDefaultTransitionKey, line)
}

// MarkStatusDone centralizes mark status done behavior so callers follow the same contract.
func (w *devBubbleWriter) MarkStatusDone() {
	w.mu.Lock()
	line := strings.TrimSpace(w.transitions[devDefaultTransitionKey].line)
	delete(w.transitions, devDefaultTransitionKey)
	next := w.activeTransitionLineLocked()
	w.statusLine = next
	if line != "" {
		w.program.Send(devAppendLinesMsg{lines: []string{console.SuccessMark() + " " + line}})
	}
	if next == "" {
		w.program.Send(devMarkStatusDoneMsg{})
		w.mu.Unlock()
		return
	}
	w.program.Send(devSetStatusMsg{line: next})
	w.mu.Unlock()
}

// ClearStatusLine centralizes clear status line behavior so callers follow the same contract.
func (w *devBubbleWriter) ClearStatusLine() {
	w.ClearTransition(devDefaultTransitionKey)
}

// HasStatusLine centralizes has status line behavior so callers follow the same contract.
func (w *devBubbleWriter) HasStatusLine() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.TrimSpace(w.statusLine) != ""
}

// SetTransition publishes keyed lifecycle work so concurrent operations cannot clear each other's status.
func (w *devBubbleWriter) SetTransition(key string, line string) {
	key = strings.TrimSpace(key)
	line = strings.TrimSpace(line)
	if key == "" || line == "" {
		return
	}
	w.mu.Lock()
	w.setTransitionLocked(key, line)
	w.mu.Unlock()
}

// setTransitionLocked updates a lifecycle owner while keeping an outer transaction visually dominant.
func (w *devBubbleWriter) setTransitionLocked(key string, line string) {
	if w.transitions == nil {
		w.transitions = make(map[string]devBubbleTransition)
	}
	w.transitionSequence++
	w.transitions[key] = devBubbleTransition{line: line, sequence: w.transitionSequence}
	w.statusLine = w.activeTransitionLineLocked()
	w.program.Send(devSetStatusMsg{line: w.statusLine})
}

// ClearTransition removes one lifecycle owner while preserving any other active operation.
func (w *devBubbleWriter) ClearTransition(key string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	w.mu.Lock()
	w.clearTransitionLocked(key)
	w.mu.Unlock()
}

// clearTransitionLocked releases one status owner without disturbing a surrounding transaction.
func (w *devBubbleWriter) clearTransitionLocked(key string) {
	delete(w.transitions, key)
	next := w.activeTransitionLineLocked()
	w.statusLine = next
	if next == "" {
		w.program.Send(devClearStatusMsg{})
		return
	}
	w.program.Send(devSetStatusMsg{line: next})
}

// activeTransitionLineLocked selects the most recently updated active operation for the single reserved status row.
func (w *devBubbleWriter) activeTransitionLineLocked() string {
	if w.lifecycle != nil {
		if transaction, ok := w.transitions[w.lifecycle.transaction.Key]; ok {
			return transaction.line
		}
	}
	var active devBubbleTransition
	for _, transition := range w.transitions {
		if transition.sequence > active.sequence {
			active = transition
		}
	}
	return active.line
}

// RefreshEnv centralizes refresh env behavior so callers follow the same contract.
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

// loadDevBubbleRuntimeState centralizes load dev bubble runtime state lookup for the surrounding workflow.
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

// Init prepares the receiver for use by its caller.
func (m devBubbleModel) Init() tea.Cmd { return nil }

// Update serializes terminal events through Bubble Tea so transcript and control state stay synchronized.
func (m devBubbleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.footerLine = buildDevFooterLineWithStateAtWidth(m.apiURL, m.lighthouseURL, m.dbQuery, m.appDebug, msg.Width)
		m.invalidateVisibleTranscriptCache()
	case devAppendLinesMsg:
		if !m.followMode {
			m.unreadCount += countDevVisibleLines(msg.lines, m.componentShown)
		}
		m.lines = append(m.lines, msg.lines...)
		m.invalidateVisibleTranscriptCache()
		m.updateSearchMatches()
	case devSetLifecycleLinesMsg:
		m.lifecycleActive = msg.active
		m.lifecycleLines = append([]string(nil), msg.lines...)
	case devResetFooterMsg:
		m.footerLine = msg.line
	case devSetStatusMsg:
		wasIdle := strings.TrimSpace(m.statusLine) == ""
		m.statusLine = msg.line
		if wasIdle && strings.TrimSpace(msg.line) != "" {
			m.spinnerFrame = 0
			m.spinnerGeneration++
			return m, devBubbleSpinnerTick(m.spinnerGeneration)
		}
	case devClearStatusMsg, devMarkStatusDoneMsg:
		m.statusLine = ""
		m.spinnerGeneration++
	case devSpinnerTickMsg:
		if msg.generation != m.spinnerGeneration || strings.TrimSpace(m.statusLine) == "" {
			return m, nil
		}
		m.spinnerFrame = (m.spinnerFrame + 1) % len(devBubbleSpinnerFrames)
		return m, devBubbleSpinnerTick(msg.generation)
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
		m.footerLine = buildDevFooterLineWithStateAtWidth(msg.apiURL, msg.lighthouseURL, msg.dbQuery, msg.appDebug, m.width)
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

// View renders the receiver's current terminal state.
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
	header = buildDevResourceHeaderLineAtWidth(m.tools, width) + "\n" + buildDevFooterSeparatorLineAtWidth(width)
	headerLines = 2
	if m.footerEnabled {
		footer = buildDevFooterSeparatorLineAtWidth(width) + "\n" + m.footerLine
		footerLines = 2
	}
	status := m.contextStatusLineAtWidth(width)
	statusDecorated := ""
	statusLines := 0
	contentHeight := height - footerLines - headerLines
	lifecycleShelfActive := m.lifecycleShelfActive(contentHeight)
	if strings.TrimSpace(status) != "" {
		if strings.TrimSpace(m.statusLine) != "" {
			frame := devBubbleSpinnerFrames[m.spinnerFrame%len(devBubbleSpinnerFrames)]
			statusDecorated = console.Colorize(console.ColorGreen, frame) + " " + status
		} else {
			statusDecorated = status
		}
		if !lifecycleShelfActive {
			statusLines = 1
		}
	}
	availableBodyHeight := height - footerLines - statusLines - headerLines
	if availableBodyHeight < 1 {
		availableBodyHeight = 1
	}
	lifecycleShelf := m.lifecycleShelfLines(width, availableBodyHeight-1, statusDecorated)
	bodyHeight := availableBodyHeight - len(lifecycleShelf)
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
	if len(lifecycleShelf) > 0 {
		if body != "" {
			body += "\n"
		}
		body += strings.Join(lifecycleShelf, "\n")
	}
	parts := make([]string, 0, 4)
	if header != "" {
		parts = append(parts, header)
	}
	parts = append(parts, body)
	if statusDecorated != "" && !lifecycleShelfActive {
		parts = append(parts, statusDecorated)
	}
	if footer != "" {
		parts = append(parts, footer)
	}
	base := strings.Join(parts, "\n")
	overlay := m.currentOverlay()
	if overlay == "" {
		return m.terminalProgressSequence() + base
	}
	return m.terminalProgressSequence() + renderDevBubbleOverlay(base, overlay, width, height)
}

// terminalProgressSequence keeps terminal-owned progress aligned with the TUI's active lifecycle row.
func (m devBubbleModel) terminalProgressSequence() string {
	if strings.TrimSpace(m.statusLine) != "" {
		return devTerminalProgressBusy
	}
	return devTerminalProgressClear
}

// devBubbleSpinnerTick schedules one TUI-owned animation frame without writing around Bubble Tea.
func devBubbleSpinnerTick(generation uint64) tea.Cmd {
	return tea.Tick(devBubbleSpinnerInterval, func(time.Time) tea.Msg {
		return devSpinnerTickMsg{generation: generation}
	})
}

// currentOverlay centralizes current overlay behavior so callers follow the same contract.
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

// bodyHeight centralizes body height behavior so callers follow the same contract.
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
	contentHeight := height - footerLines - headerLines
	lifecycleShelfActive := m.lifecycleShelfActive(contentHeight)
	if strings.TrimSpace(m.contextStatusLine()) != "" && !lifecycleShelfActive {
		statusLines = 1
	}
	bodyHeight := height - footerLines - headerLines - statusLines
	if bodyHeight < 1 {
		return 1
	}
	return bodyHeight - len(m.lifecycleShelfLines(width, bodyHeight-1, ""))
}

// lifecycleShelfActive keeps the transient pane distinct while preserving a status row in extremely short terminals.
func (m devBubbleModel) lifecycleShelfActive(contentHeight int) bool {
	return m.lifecycleActive && strings.TrimSpace(m.statusLine) != "" && contentHeight >= 2
}

// lifecycleShelfLines gives transaction output a visually owned region instead of mixing it into App scrollback.
func (m devBubbleModel) lifecycleShelfLines(width int, available int, heading string) []string {
	if !m.lifecycleActive || strings.TrimSpace(m.statusLine) == "" || available <= 0 {
		return nil
	}
	rail := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#64748B", Dark: "#64748B"}).
		Render("┃")
	lines := []string{rail + " " + heading}
	for _, line := range m.visibleLifecycleLines(width-2, available-1) {
		lines = append(lines, rail+" "+line)
	}
	return lines
}

// visibleLifecycleLines keeps current transaction output visible without adding it to the durable transcript.
func (m devBubbleModel) visibleLifecycleLines(width int, available int) []string {
	if available <= 0 || len(m.lifecycleLines) == 0 {
		return nil
	}
	lines := wrapDevTranscriptLines(m.lifecycleLines, width)
	limit := min(devLifecycleVisibleLines, available)
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return lines
}

// applyRuntimeSettingChange restarts the App so the process and newly persisted footer state agree.
func (m *devBubbleModel) applyRuntimeSettingChange(successLine string) {
	m.dbQuery, m.appDebug = loadDevRuntimeSettings()
	m.footerLine = buildDevFooterLineWithStateAtWidth(m.apiURL, m.lighthouseURL, m.dbQuery, m.appDebug, m.width)
	m.requestRestart()
	m.lines = append(m.lines, successLine)
}

// visibleTranscriptLines centralizes visible transcript lines behavior so callers follow the same contract.
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

// invalidateVisibleTranscriptCache centralizes invalidate visible transcript cache behavior so callers follow the same contract.
func (m *devBubbleModel) invalidateVisibleTranscriptCache() {
	m.cachedLines = nil
	m.cacheWidth = 0
	m.cacheHasHeader = false
	m.cacheValid = false
}

// viewportLines centralizes viewport lines behavior so callers follow the same contract.
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

// scrollUp centralizes scroll up behavior so callers follow the same contract.
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

// scrollDown centralizes scroll down behavior so callers follow the same contract.
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

// scrollToTop centralizes scroll to top behavior so callers follow the same contract.
func (m *devBubbleModel) scrollToTop() {
	m.followMode = false
	m.viewportTop = 0
}

// scrollToBottom centralizes scroll to bottom behavior so callers follow the same contract.
func (m *devBubbleModel) scrollToBottom() {
	m.followMode = true
	m.viewportTop = 0
	m.unreadCount = 0
}

// updateSearchMatches centralizes update search matches behavior so callers follow the same contract.
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

// jumpSearch centralizes jump search behavior so callers follow the same contract.
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

// jumpToCurrentSearchMatch centralizes jump to current search match behavior so callers follow the same contract.
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

// contextStatusLine centralizes context status line behavior so callers follow the same contract.
func (m devBubbleModel) contextStatusLine() string {
	return m.contextStatusLineAtWidth(120)
}

// contextStatusLineAtWidth renders transient view state with the same responsive control grammar as the persistent footer.
func (m devBubbleModel) contextStatusLineAtWidth(width int) string {
	if strings.TrimSpace(m.statusLine) != "" {
		return m.statusLine
	}
	if m.commandVisible {
		return ""
	}
	if !m.followMode && strings.TrimSpace(m.searchQuery) == "" && len(activeDevComponentFilters(m.componentShown)) == 0 {
		return renderDevPausedStatusLine(m.unreadCount, width)
	}
	states := make([]string, 0, 3)
	actions := make([]string, 0, 3)
	if m.searchMode {
		states = append(states, renderDevFindState(m.searchQuery, ""))
		actions = append(actions, renderDevFooterShortcut("Enter", "Apply"), renderDevFooterShortcut("Esc", "Clear"))
	} else if strings.TrimSpace(m.searchQuery) != "" {
		matchState := "0/0"
		if len(m.searchMatches) > 0 && m.searchIndex >= 0 {
			matchState = fmt.Sprintf("%d/%d", m.searchIndex+1, len(m.searchMatches))
		}
		states = append(states, renderDevFindState(m.searchQuery, matchState))
		actions = append(actions, renderDevFooterShortcut("Tab/Shift+Tab", "Navigate"), renderDevFooterShortcut("Esc", "Clear"))
	}
	if !m.followMode {
		paused := renderDevFooterBooleanStatus("Paused", false)
		if m.unreadCount > 0 {
			paused += " " + devMutedForegroundStyle().Render(fmt.Sprintf("· %d new", m.unreadCount))
		}
		states = append(states, paused)
		if strings.TrimSpace(m.searchQuery) == "" {
			actions = append(actions, renderDevFooterShortcut("PgUp/PgDn", "Scroll"))
		}
		actions = append(actions, renderDevFooterShortcut("l", "Live"))
	}
	if active := activeDevComponentFilters(m.componentShown); len(active) > 0 {
		states = append(states, renderDevFooterValueStatus("Filters", strings.Join(active, ", ")))
	}
	return layoutDevContextStatus(states, actions, width)
}

// renderDevFindState presents the query as normal content and its match position as metadata.
func renderDevFindState(query string, matches string) string {
	state := renderDevFooterShortcut("/", "Find")
	if query = strings.TrimSpace(query); query != "" {
		state += " " + devNormalForegroundStyle().Render(query)
	}
	if matches = strings.TrimSpace(matches); matches != "" {
		state += " " + devMutedForegroundStyle().Render("· "+matches)
	}
	return state
}

// layoutDevContextStatus keeps view state left-aligned and its immediate actions right-aligned without wrapping.
func layoutDevContextStatus(states []string, actions []string, width int) string {
	if width <= 0 {
		width = 120
	}
	lastAction := actions
	if len(lastAction) > 0 {
		lastAction = lastAction[len(lastAction)-1:]
	}
	for _, layout := range []struct {
		actions []string
		gap     int
	}{
		{actions: actions, gap: 5},
		{actions: actions, gap: 2},
		{actions: lastAction, gap: 2},
		{gap: 2},
	} {
		if line, ok := layoutDevFooterGroups(states, layout.actions, layout.gap, width); ok {
			return line
		}
	}
	return charmansi.Truncate(strings.Join(states, "  "), width, "")
}

// renderDevPausedStatusLine keeps the paused state useful without restoring the footer's full control inventory.
func renderDevPausedStatusLine(unread int, width int) string {
	state := renderDevFooterBooleanStatus("Paused", false)
	if unread > 0 {
		state += " " + devMutedForegroundStyle().Render(fmt.Sprintf("· %d new", unread))
	}
	actions := []string{
		renderDevFooterShortcut("PgUp/PgDn", "Scroll"),
		renderDevFooterShortcut("l", "Live"),
	}
	for _, visible := range [][]string{actions, actions[1:], nil} {
		line := state
		if len(visible) > 0 {
			line += "     " + strings.Join(visible, "     ")
		}
		if width <= 0 || lipgloss.Width(line) <= width {
			return line
		}
	}
	return charmansi.Truncate(state, width, "")
}

// moveCommandSelection centralizes move command selection behavior so callers follow the same contract.
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

// handleCommandTextInput centralizes handle command text input behavior so callers follow the same contract.
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

// jumpCommandSelection centralizes jump command selection behavior so callers follow the same contract.
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

// selectedCommandAcceptsArgs centralizes selected command accepts args behavior so callers follow the same contract.
func (m devBubbleModel) selectedCommandAcceptsArgs() bool {
	if len(m.commands) == 0 || m.commandIndex < 0 || m.commandIndex >= len(m.commands) {
		return false
	}
	return m.commands[m.commandIndex].AcceptsArgs
}

// selectedCommandRequest centralizes selected command request behavior so callers follow the same contract.
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

// defaultDevComponentShown centralizes default dev component shown behavior so callers follow the same contract.
func defaultDevComponentShown() map[string]bool {
	shown := make(map[string]bool, len(devComponentFilterOrder))
	for _, component := range devComponentFilterOrder {
		shown[component] = true
	}
	return shown
}

// countDevVisibleLines centralizes count dev visible lines behavior so callers follow the same contract.
func countDevVisibleLines(lines []string, shown map[string]bool) int {
	count := 0
	for _, line := range lines {
		if devTranscriptLineVisible(line, shown) {
			count++
		}
	}
	return count
}

// filterDevTranscriptLines centralizes filter dev transcript lines behavior so callers follow the same contract.
func filterDevTranscriptLines(lines []string, shown map[string]bool) []string {
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if devTranscriptLineVisible(line, shown) {
			filtered = append(filtered, line)
		}
	}
	return filtered
}

// devTranscriptLineVisible centralizes dev transcript line visible behavior so callers follow the same contract.
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

// devTranscriptComponent centralizes dev transcript component behavior so callers follow the same contract.
func devTranscriptComponent(line string) string {
	plain := stripANSIForSearch(line)
	matches := devTranscriptComponentPattern.FindStringSubmatch(plain)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

// activeDevComponentFilters centralizes active dev component filters behavior so callers follow the same contract.
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

// stripANSIForSearch centralizes strip ansifor search behavior so callers follow the same contract.
func stripANSIForSearch(s string) string {
	return ansiCSI.ReplaceAllString(s, "")
}

// renderDevBubbleOverlay keeps the render dev bubble overlay representation consistent.
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

// overlayDevBubbleLine centralizes overlay dev bubble line behavior so callers follow the same contract.
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

// decorateDevSearchMatches centralizes decorate dev search matches behavior so callers follow the same contract.
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

// renderDevSearchMatch keeps the render dev search match representation consistent.
func renderDevSearchMatch(line string) string {
	return applyDevLineHighlight(line, "\x1b[48;5;236m")
}

// renderDevCurrentSearchMatch keeps the render dev current search match representation consistent.
func renderDevCurrentSearchMatch(line string) string {
	return applyDevLineHighlight(line, "\x1b[48;5;24m\x1b[1m")
}

// applyDevLineHighlight centralizes apply dev line highlight behavior so callers follow the same contract.
func applyDevLineHighlight(line, prefix string) string {
	if line == "" {
		return prefix + "\x1b[0m"
	}
	reapplied := strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+prefix)
	return prefix + reapplied + "\x1b[0m"
}

// wrapDevTranscriptLines centralizes wrap dev transcript lines behavior so callers follow the same contract.
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

// wrapDevTranscriptLine centralizes wrap dev transcript line behavior so callers follow the same contract.
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

// wrapDevMetadataFields centralizes wrap dev metadata fields behavior so callers follow the same contract.
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

// splitDevMetadataFields centralizes split dev metadata fields behavior so callers follow the same contract.
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

// splitDevTranscriptMetadataBoundary centralizes split dev transcript metadata boundary behavior so callers follow the same contract.
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

// normalizeDevTranscriptLines keeps normalize dev transcript lines handling consistent across callers.
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

// parseDevAppHelpCommands keeps parse dev app help commands handling consistent across callers.
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

// parseDevAppCommandAcceptsArgs keeps parse dev app command accepts args handling consistent across callers.
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
		stdout:           writer,
		stderr:           writer,
		shutdown:         func() { _ = writer.Close() },
		refresh:          func() { writer.RefreshEnv(config) },
		restoresTerminal: true,
	}
}
