package forj

import (
	"io"
	"os"
	"strings"
	"sync"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
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
		m.lines = append(m.lines, msg.lines...)
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
		switch msg.String() {
		case "ctrl+c":
			return m, devForwardInterruptCmd()
		case "?":
			m.helpVisible = !m.helpVisible
		case "esc":
			if m.helpVisible {
				m.helpVisible = false
			}
		case "h":
			m.helpVisible = !m.helpVisible
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
	status := ""
	statusLines := 0
	if strings.TrimSpace(m.statusLine) != "" {
		status = console.Colorize(console.ColorGreen, "•") + " " + m.statusLine
		statusLines = 1
	}
	bodyHeight := height - footerLines - statusLines - headerLines
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	lines := wrapDevTranscriptLines(m.lines, width)
	lines = normalizeDevTranscriptLines(lines, header != "")
	if len(lines) > bodyHeight {
		lines = lines[len(lines)-bodyHeight:]
	}
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
	if status != "" {
		parts = append(parts, status)
	}
	if footer != "" {
		parts = append(parts, footer)
	}
	return strings.Join(parts, "\n")
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
	const continuationIndent = "  "

	if width <= 0 {
		return []string{line}
	}

	if prefix, metadata, ok := splitDevTranscriptMetadataBoundary(line); ok {
		prefixWidth := charmansi.StringWidth(prefix)
		indentWidth := charmansi.StringWidth(continuationIndent)
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
						lines = append(lines, continuationIndent+continuation)
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
