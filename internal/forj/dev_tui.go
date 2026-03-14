package forj

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/goforj/goforj/internal/console"
	"golang.org/x/term"
)

type devFooterWriter struct {
	mu         sync.Mutex
	out        io.Writer
	separator  string
	footerLine string
	partial    string
	shown      bool
	ansiTail   string
	disabled   bool
}

type devFooterController struct {
	writer         *devFooterWriter
	apiURL         string
	lighthouseURL  string
	requestRestart func()
	requestRender  func()
	dbQueryLogging bool
	appDebug       string
	tty            *os.File
	restoreTTY     func()
}

var ansiCSI = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func newDevFooterWriter(out io.Writer, separator, footerLine string) *devFooterWriter {
	return &devFooterWriter{out: out, separator: separator, footerLine: footerLine}
}

func (w *devFooterWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	raw := w.ansiTail + string(p)
	cleanRaw, tail := splitANSITail(raw)
	w.ansiTail = tail
	cleanRaw = sanitizeCSI(cleanRaw)
	cleanRaw = strings.ReplaceAll(cleanRaw, "\r", "")
	if w.disabled {
		if cleanRaw == "" {
			return len(p), nil
		}
		_, err := io.WriteString(w.out, cleanRaw)
		return len(p), err
	}
	input := w.partial + cleanRaw
	lines := strings.Split(input, "\n")
	w.partial = lines[len(lines)-1]

	for _, line := range lines[:len(lines)-1] {
		if w.shown {
			if _, err := io.WriteString(w.out, "\x1b[1A\r\x1b[2K\r\x1b[1A\r\x1b[2K\r"); err != nil {
				return 0, err
			}
		}
		if _, err := io.WriteString(w.out, line+"\n"); err != nil {
			return 0, err
		}
		if _, err := io.WriteString(w.out, w.separator+"\n"); err != nil {
			return 0, err
		}
		if _, err := io.WriteString(w.out, w.footerLine+"\n"); err != nil {
			return 0, err
		}
		w.shown = true
	}

	return len(p), nil
}

func (w *devFooterWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.shown {
		_, _ = io.WriteString(w.out, "\x1b[1A\r\x1b[2K\r\x1b[1A\r\x1b[2K\r")
		w.shown = false
	}
	if w.partial != "" {
		_, _ = io.WriteString(w.out, w.partial+"\n")
		w.partial = ""
	}
	w.ansiTail = ""
	return nil
}

func (w *devFooterWriter) DisableFooter() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.disabled = true
	if w.shown {
		_, _ = io.WriteString(w.out, "\x1b[1A\r\x1b[2K\r\x1b[1A\r\x1b[2K\r")
		w.shown = false
	}
}

func (w *devFooterWriter) EnableFooter() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.disabled = false
}

func (w *devFooterWriter) SetFooterLine(line string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.footerLine = line
}

// ClearBuffer clears the terminal and redraws the sticky footer.
func (w *devFooterWriter) ClearBuffer() {
	w.mu.Lock()
	defer w.mu.Unlock()

	_, _ = io.WriteString(w.out, "\x1b[2J\x1b[H")
	w.partial = ""
	w.ansiTail = ""
	if w.disabled {
		w.shown = false
		return
	}
	_, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || height <= 0 {
		height = 24
	}
	// Reserve the last two lines for separator + footer.
	fillerLines := height - 3
	if fillerLines < 0 {
		fillerLines = 0
	}
	if fillerLines > 0 {
		_, _ = io.WriteString(w.out, strings.Repeat("\n", fillerLines))
	}
	_, _ = io.WriteString(w.out, w.separator+"\n")
	_, _ = io.WriteString(w.out, w.footerLine+"\n")
	w.shown = true
}

func disableDevFooter(writer io.Writer) {
	footerWriter, ok := writer.(*devFooterWriter)
	if !ok || footerWriter == nil {
		return
	}
	footerWriter.DisableFooter()
}

func enableDevFooter(writer io.Writer) {
	footerWriter, ok := writer.(*devFooterWriter)
	if !ok || footerWriter == nil {
		return
	}
	footerWriter.EnableFooter()
}

func splitANSITail(raw string) (string, string) {
	last := strings.LastIndex(raw, "\x1b[")
	if last == -1 {
		return raw, ""
	}
	seq := raw[last:]
	for i := 2; i < len(seq); i++ {
		b := seq[i]
		if b >= 0x40 && b <= 0x7e {
			return raw, ""
		}
	}
	return raw[:last], seq
}

func sanitizeCSI(input string) string {
	return ansiCSI.ReplaceAllStringFunc(input, func(seq string) string {
		if strings.HasSuffix(seq, "m") {
			return seq
		}
		return ""
	})
}

func buildDevOutputWriters(env map[string]string, requestRestart func(), requestRender func()) (io.Writer, io.Writer, func()) {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return os.Stdout, os.Stderr, func() {}
	}
	if strings.TrimSpace(os.Getenv("FORJ_DEV_PLAIN")) == "1" {
		return os.Stdout, os.Stderr, func() {}
	}

	apiURL := resolveAPIURL(env)
	lighthouseURL := resolveLighthouseUIURL(env)
	dbQueryLogging, appDebug := loadDevRuntimeSettings()
	footer := buildDevFooterLineWithState(apiURL, lighthouseURL, dbQueryLogging, appDebug)
	if footer == "" {
		return os.Stdout, os.Stderr, func() {}
	}

	writer := newDevFooterWriter(os.Stdout, buildDevFooterSeparatorLine(), footer)
	controller := &devFooterController{
		writer:         writer,
		apiURL:         apiURL,
		lighthouseURL:  lighthouseURL,
		requestRestart: requestRestart,
		requestRender:  requestRender,
		dbQueryLogging: dbQueryLogging,
		appDebug:       appDebug,
	}
	controller.startHotkeys()
	return writer, writer, controller.shutdown
}

func buildDevFooterSeparatorLine() string {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		width = 120
	}
	if width < 20 {
		width = 20
	}
	borderColor := lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}
	return lipgloss.NewStyle().Foreground(borderColor).Render(strings.Repeat("─", width))
}

func buildDevFooterLine(env map[string]string) string {
	dbQueryLogging, appDebug := loadDevRuntimeSettings()
	return buildDevFooterLineWithState(resolveAPIURL(env), resolveLighthouseUIURL(env), dbQueryLogging, appDebug)
}

func buildDevFooterLineWithURLs(apiURL, lighthouseURL string) string {
	dbQueryLogging, appDebug := loadDevRuntimeSettings()
	return buildDevFooterLineWithState(apiURL, lighthouseURL, dbQueryLogging, appDebug)
}

func buildDevFooterLineWithState(apiURL, lighthouseURL string, dbQueryLogging bool, appDebug string) string {
	if apiURL == "" && lighthouseURL == "" {
		return ""
	}
	type footerHotkey struct {
		key   string
		label string
		state string
	}
	parts := []string{"keys"}
	entries := []footerHotkey{
		{key: "?", label: "help"},
	}
	if lighthouseURL != "" {
		entries = append(entries, footerHotkey{key: "o", label: "lighthouse"})
	}
	if apiURL != "" {
		entries = append(entries, footerHotkey{key: "a", label: "api"})
	}
	if requestRestartEnabled(lighthouseURL, apiURL) {
		entries = append(entries, footerHotkey{key: "r", label: "restart"})
		entries = append(entries, footerHotkey{key: "c", label: "clear"})
		queryState := "off"
		if dbQueryLogging {
			queryState = "on"
		}
		entries = append(entries, footerHotkey{key: "q", label: "query", state: queryState})
		entries = append(entries, footerHotkey{key: "Shift+0/1/2/3", label: "debug", state: appDebug})
	}
	for _, entry := range entries {
		parts = append(parts, formatFooterHotkeyEntry(entry.key, entry.label, entry.state))
	}
	footerText := strings.Join(parts, " · ")
	return fmt.Sprintf(
		"%s %s",
		console.Colorize(console.ColorYellow, "•"),
		footerText,
	)
}

func formatFooterHotkeyEntry(key, label, state string) string {
	keyBlock := fmt.Sprintf("[ %s ]", key)
	keyBlock = console.Colorize(console.ColorGray, keyBlock)
	labelText := console.Colorize(console.ColorBoldWhite, label)
	if strings.TrimSpace(state) == "" {
		return keyBlock + " " + labelText
	}
	stateText := console.Colorize(console.ColorBoldWhite, state)
	return keyBlock + " " + labelText + ":" + stateText
}

func requestRestartEnabled(lighthouseURL, apiURL string) bool {
	// Footer hotkeys are only displayed when either API/lighthouse is available.
	// Restart is always available in forj dev when footer is active.
	return lighthouseURL != "" || apiURL != ""
}

func (c *devFooterController) startHotkeys() {
	if runtime.GOOS == "windows" {
		return
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return
	}
	if !term.IsTerminal(int(tty.Fd())) {
		_ = tty.Close()
		return
	}
	restore, err := setTTYSingleKeyMode(int(tty.Fd()))
	if err != nil {
		_ = tty.Close()
		return
	}
	drainTTYInput(int(tty.Fd()))
	c.tty = tty
	c.restoreTTY = restore
	go c.listenHotkeys()
}

func (c *devFooterController) listenHotkeys() {
	reader := bufio.NewReader(c.tty)
	for {
		ch, err := reader.ReadByte()
		if err != nil {
			return
		}
		if ch == 0x1b {
			discardEscapeSequence(reader)
			continue
		}
		// Ctrl+R emits byte 0x12 in terminal raw mode.
		if ch == 0x12 {
			if c.requestRender != nil {
				c.requestRender()
				_, _ = c.writer.Write([]byte(fmt.Sprintf("%s Render requested\n", console.ActionMark())))
			}
			continue
		}
		if c.handleHotkeyCtrlByte(ch) {
			continue
		}
		if ch < 0x20 || ch == 0x7f {
			continue
		}
		c.handleHotkeyByte(ch)
	}
}

func (c *devFooterController) handleHotkeyByte(ch byte) {
	switch string(ch) {
	case "o":
		if c.lighthouseURL == "" {
			return
		}
		if err := openURL(c.lighthouseURL); err != nil {
			_, _ = c.writer.Write([]byte(fmt.Sprintf("%s Failed to open lighthouse: %v\n", console.ErrorMark(), err)))
		}
	case "a":
		if c.apiURL == "" {
			return
		}
		if err := openURL(c.apiURL); err != nil {
			_, _ = c.writer.Write([]byte(fmt.Sprintf("%s Failed to open API URL: %v\n", console.ErrorMark(), err)))
		}
	case "?", "h":
		parts := []string{fmt.Sprintf("%s Dev hotkeys:", console.InfoMark())}
		if c.lighthouseURL != "" {
			parts = append(parts, fmt.Sprintf("  o  open lighthouse (%s)", c.lighthouseURL))
		}
		if c.apiURL != "" {
			parts = append(parts, fmt.Sprintf("  a  open api (%s)", c.apiURL))
		}
		parts = append(parts, "  r  restart watchers")
		parts = append(parts, "  c  clear output")
		parts = append(parts, "  q  toggle DB_QUERY_LOGGING and restart")
		parts = append(parts, "  Shift+0 / Shift+1 / Shift+2 / Shift+3  set APP_DEBUG to 0 / 1 / 2 / 3 and restart")
		parts = append(parts, "  Ctrl+R  render project")
		parts = append(parts, "  ?  show this help")
		_, _ = c.writer.Write([]byte(strings.Join(parts, "\n") + "\n"))
	case "r":
		if c.requestRestart == nil {
			return
		}
		c.requestRestart()
		_, _ = c.writer.Write([]byte(fmt.Sprintf("%s Restart requested\n", console.ActionMark())))
	case "c":
		c.writer.ClearBuffer()
	case "q":
		if err := c.toggleDBQueryLogging(); err != nil {
			_, _ = c.writer.Write([]byte(fmt.Sprintf("%s %v\n", console.ErrorMark(), err)))
		}
	case ")", "!", "@", "#":
		level := map[byte]string{
			')': "0",
			'!': "1",
			'@': "2",
			'#': "3",
		}[ch]
		if err := c.setAppDebugLevel(level); err != nil {
			_, _ = c.writer.Write([]byte(fmt.Sprintf("%s %v\n", console.ErrorMark(), err)))
		}
	}
}

func (c *devFooterController) handleHotkeyCtrlByte(ch byte) bool {
	return false
}

func (c *devFooterController) shutdown() {
	if c.restoreTTY != nil {
		c.restoreTTY()
	}
	if c.tty != nil {
		_ = c.tty.Close()
	}
	_ = c.writer.Close()
}

func openURL(raw string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", raw).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", "", raw).Start()
	default:
		return exec.Command("xdg-open", raw).Start()
	}
}

func (c *devFooterController) toggleDBQueryLogging() error {
	content, err := os.ReadFile(".env")
	if err != nil {
		return fmt.Errorf("unable to read .env: %w", err)
	}
	current := strings.TrimSpace(readEnvKey(string(content), "DB_QUERY_LOGGING"))
	next := "true"
	if current == "1" || strings.EqualFold(current, "true") {
		next = "false"
	}
	updated := updateEnvKey(string(content), "DB_QUERY_LOGGING", next)
	if err := os.WriteFile(".env", []byte(updated), 0644); err != nil {
		return fmt.Errorf("unable to write .env: %w", err)
	}
	c.dbQueryLogging = strings.EqualFold(next, "true")
	c.requestWatcherRestart()
	c.refreshFooter()
	_, _ = c.writer.Write([]byte(fmt.Sprintf("%s DB_QUERY_LOGGING=%s\n", console.SuccessMark(), next)))
	return nil
}

func (c *devFooterController) setAppDebugLevel(level string) error {
	if level != "0" && level != "1" && level != "2" && level != "3" {
		return fmt.Errorf("invalid APP_DEBUG level: %s", level)
	}
	content, err := os.ReadFile(".env")
	if err != nil {
		return fmt.Errorf("unable to read .env: %w", err)
	}
	current := strings.TrimSpace(readEnvKey(string(content), "APP_DEBUG"))
	if current == level {
		return nil
	}
	updated := updateEnvKey(string(content), "APP_DEBUG", level)
	if err := os.WriteFile(".env", []byte(updated), 0644); err != nil {
		return fmt.Errorf("unable to write .env: %w", err)
	}
	c.appDebug = level
	c.requestWatcherRestart()
	c.refreshFooter()
	_, _ = c.writer.Write([]byte(fmt.Sprintf("%s APP_DEBUG=%s\n", console.SuccessMark(), level)))
	return nil
}

func (c *devFooterController) requestWatcherRestart() {
	if c.requestRestart != nil {
		c.requestRestart()
	}
}

func (c *devFooterController) refreshFooter() {
	c.writer.SetFooterLine(
		buildDevFooterLineWithState(c.apiURL, c.lighthouseURL, c.dbQueryLogging, c.appDebug),
	)
}

func readEnvKey(content, key string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		working := trimmed
		if strings.HasPrefix(working, "export ") {
			working = strings.TrimSpace(strings.TrimPrefix(working, "export "))
		}
		if strings.HasPrefix(working, key+"=") {
			return strings.TrimSpace(strings.TrimPrefix(working, key+"="))
		}
	}
	return ""
}

func discardEscapeSequence(reader *bufio.Reader) {
	first, err := reader.ReadByte()
	if err != nil {
		return
	}
	if first == '[' {
		for {
			next, err := reader.ReadByte()
			if err != nil {
				return
			}
			if next >= 0x40 && next <= 0x7e {
				return
			}
		}
	}
	if first == 'O' {
		_, _ = reader.ReadByte()
		return
	}
	if first == ']' {
		for {
			next, err := reader.ReadByte()
			if err != nil {
				return
			}
			if next == 0x07 {
				return
			}
			if next == 0x1b {
				peek, err := reader.ReadByte()
				if err != nil {
					return
				}
				if peek == '\\' {
					return
				}
			}
		}
	}
}

func updateEnvKey(content, key, value string) string {
	lines := strings.Split(content, "\n")
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		exportPrefix := ""
		working := trimmed
		if strings.HasPrefix(working, "export ") {
			exportPrefix = "export "
			working = strings.TrimSpace(strings.TrimPrefix(working, "export "))
		}
		if !strings.HasPrefix(working, key+"=") {
			continue
		}
		found = true
		comment := ""
		if idx := strings.Index(line, " #"); idx >= 0 {
			comment = line[idx:]
		}
		lines[i] = exportPrefix + key + "=" + value + comment
	}
	if !found {
		lines = append(lines, key+"="+value)
	}
	return strings.Join(lines, "\n")
}

func loadDevRuntimeSettings() (bool, string) {
	content, err := os.ReadFile(".env")
	if err != nil {
		return false, "1"
	}
	queryRaw := strings.TrimSpace(readEnvKey(string(content), "DB_QUERY_LOGGING"))
	debugRaw := strings.TrimSpace(readEnvKey(string(content), "APP_DEBUG"))
	queryOn := queryRaw == "1" || strings.EqualFold(queryRaw, "true")
	if debugRaw != "0" && debugRaw != "1" && debugRaw != "2" && debugRaw != "3" {
		debugRaw = "1"
	}
	return queryOn, debugRaw
}
