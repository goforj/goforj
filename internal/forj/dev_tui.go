package forj

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/goforj/goforj/internal/console"
	"golang.org/x/term"
)

type devFooterWriter struct {
	mu          sync.Mutex
	out         io.Writer
	separator   string
	footerLine  string
	defaultLine string
	statusLine  string
	statusShown bool
	statusFrame int
	statusStop  chan struct{}
	partial     string
	shown       bool
	ansiTail    string
	disabled    bool
}

type devFooterController struct {
	writer          *devFooterWriter
	apiURL          string
	lighthouseURL   string
	lighthouseToken string
	requestRestart  func()
	requestRender   func()
	dbQueryLogging  bool
	appDebug        string
	tty             *os.File
	restoreTTY      func()
}

var ansiCSI = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func newDevFooterWriter(out io.Writer, separator, footerLine string) *devFooterWriter {
	return &devFooterWriter{out: out, separator: separator, footerLine: footerLine, defaultLine: footerLine}
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
		if err := w.clearManagedStatusLocked(); err != nil {
			return 0, err
		}
		if err := w.clearFooterStackLocked(); err != nil {
			return 0, err
		}
		if _, err := io.WriteString(w.out, line+"\n"); err != nil {
			return 0, err
		}
		if err := w.drawFooterStackLocked(); err != nil {
			return 0, err
		}
		w.shown = true
	}

	return len(p), nil
}

func (w *devFooterWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	_ = w.clearFooterStackLocked()
	if w.partial != "" {
		_, _ = io.WriteString(w.out, w.partial+"\n")
		w.partial = ""
	}
	w.stopStatusAnimationLocked()
	w.ansiTail = ""
	return nil
}

func (w *devFooterWriter) DisableFooter() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.disabled = true
	w.stopStatusAnimationLocked()
	_ = w.clearFooterStackLocked()
}

func (w *devFooterWriter) EnableFooter() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.disabled = false
	if strings.TrimSpace(w.statusLine) != "" {
		w.ensureStatusAnimationLocked()
	}
}

func (w *devFooterWriter) SetFooterLine(line string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.footerLine = line
	w.redrawFooterStackLocked()
}

func (w *devFooterWriter) ResetFooterLine() {
	w.SetFooterLine(w.defaultLine)
}

func (w *devFooterWriter) SetStatusLine(line string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.statusLine = line
	w.ensureStatusAnimationLocked()
	w.redrawManagedStatusLocked()
}

func (w *devFooterWriter) MarkStatusDone() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if strings.TrimSpace(w.statusLine) == "" {
		return
	}
	line := w.statusLine
	w.stopStatusAnimationLocked()
	_ = w.clearFooterStackLocked()
	_ = w.clearManagedStatusLocked()
	_, _ = io.WriteString(w.out, console.SuccessMark()+" "+line+"\n")
	w.statusLine = ""
	w.statusShown = false
	_ = w.drawFooterStackLocked()
}

func (w *devFooterWriter) ClearStatusLine() {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.clearFooterStackLocked()
	_ = w.clearManagedStatusLocked()
	w.statusLine = ""
	w.statusShown = false
	w.stopStatusAnimationLocked()
	_ = w.drawFooterStackLocked()
}

func (w *devFooterWriter) HasStatusLine() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.TrimSpace(w.statusLine) != ""
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
	_ = w.drawFooterStackLocked()
	w.shown = true
}

func (w *devFooterWriter) clearFooterStackLocked() error {
	if !w.shown {
		return nil
	}
	for i := 0; i < 2; i++ {
		if _, err := io.WriteString(w.out, "\x1b[1A\r\x1b[2K\r"); err != nil {
			return err
		}
	}
	w.shown = false
	return nil
}

func (w *devFooterWriter) drawFooterStackLocked() error {
	if w.disabled {
		return nil
	}
	if _, err := io.WriteString(w.out, w.separator+"\n"); err != nil {
		return err
	}
	if _, err := io.WriteString(w.out, w.footerLine+"\n"); err != nil {
		return err
	}
	w.shown = true
	return nil
}

func (w *devFooterWriter) redrawFooterStackLocked() {
	if w.disabled || !w.shown {
		return
	}
	_ = w.clearFooterStackLocked()
	_ = w.drawFooterStackLocked()
}

func (w *devFooterWriter) currentStatusLineLocked() string {
	if strings.TrimSpace(w.statusLine) == "" {
		return ""
	}
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinner := console.Colorize(console.ColorGreen, frames[w.statusFrame%len(frames)])
	return spinner + " " + w.statusLine
}

func (w *devFooterWriter) clearManagedStatusLocked() error {
	if !w.statusShown {
		return nil
	}
	if _, err := io.WriteString(w.out, "\x1b[1A\r\x1b[2K\r"); err != nil {
		return err
	}
	w.statusShown = false
	return nil
}

func (w *devFooterWriter) drawManagedStatusLocked() error {
	if w.disabled || strings.TrimSpace(w.statusLine) == "" {
		return nil
	}
	if _, err := io.WriteString(w.out, w.currentStatusLineLocked()+"\n"); err != nil {
		return err
	}
	w.statusShown = true
	return nil
}

func (w *devFooterWriter) redrawManagedStatusLocked() {
	if w.disabled {
		return
	}
	_ = w.clearFooterStackLocked()
	_ = w.clearManagedStatusLocked()
	_ = w.drawManagedStatusLocked()
	_ = w.drawFooterStackLocked()
}

func (w *devFooterWriter) ensureStatusAnimationLocked() {
	if strings.TrimSpace(w.statusLine) == "" || w.statusStop != nil {
		return
	}
	stop := make(chan struct{})
	w.statusStop = stop
	go w.runStatusAnimation(stop)
}

func (w *devFooterWriter) stopStatusAnimationLocked() {
	if w.statusStop == nil {
		return
	}
	close(w.statusStop)
	w.statusStop = nil
	w.statusFrame = 0
}

func (w *devFooterWriter) runStatusAnimation(stop <-chan struct{}) {
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			w.mu.Lock()
			if w.disabled || strings.TrimSpace(w.statusLine) == "" || w.statusStop != stop {
				w.mu.Unlock()
				return
			}
			w.statusFrame++
			w.redrawManagedStatusLocked()
			w.mu.Unlock()
		}
	}
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

func setDevFooterLine(writer io.Writer, line string) {
	footerWriter, ok := writer.(*devFooterWriter)
	if !ok || footerWriter == nil {
		return
	}
	footerWriter.SetFooterLine(line)
}

func resetDevFooterLine(writer io.Writer) {
	footerWriter, ok := writer.(*devFooterWriter)
	if !ok || footerWriter == nil {
		return
	}
	footerWriter.ResetFooterLine()
}

func setDevStatusLine(writer io.Writer, line string) {
	footerWriter, ok := writer.(*devFooterWriter)
	if !ok || footerWriter == nil {
		return
	}
	footerWriter.SetStatusLine(line)
}

func markDevStatusDone(writer io.Writer) {
	footerWriter, ok := writer.(*devFooterWriter)
	if !ok || footerWriter == nil {
		return
	}
	footerWriter.MarkStatusDone()
}

func clearDevStatusLine(writer io.Writer) {
	footerWriter, ok := writer.(*devFooterWriter)
	if !ok || footerWriter == nil {
		return
	}
	footerWriter.ClearStatusLine()
}

func hasDevStatusLine(writer io.Writer) bool {
	footerWriter, ok := writer.(*devFooterWriter)
	if !ok || footerWriter == nil {
		return false
	}
	return footerWriter.HasStatusLine()
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

func buildDevOutputWriters(requestRestart func(), requestRender func()) (io.Writer, io.Writer, func(), func()) {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return os.Stdout, os.Stderr, func() {}, func() {}
	}
	if strings.TrimSpace(os.Getenv("FORJ_DEV_PLAIN")) == "1" {
		return os.Stdout, os.Stderr, func() {}, func() {}
	}

	apiURL := resolveAPIURL(nil)
	lighthouseURL := resolveLighthouseUIURL(nil)
	lighthouseToken := strings.TrimSpace(os.Getenv("LIGHTHOUSE_TOKEN"))
	dbQueryLogging, appDebug := loadDevRuntimeSettings()
	footer := buildDevFooterLineWithState(apiURL, lighthouseURL, dbQueryLogging, appDebug)
	if footer == "" {
		return os.Stdout, os.Stderr, func() {}, func() {}
	}

	writer := newDevFooterWriter(os.Stdout, buildDevFooterSeparatorLine(), footer)
	controller := &devFooterController{
		writer:          writer,
		apiURL:          apiURL,
		lighthouseURL:   lighthouseURL,
		lighthouseToken: lighthouseToken,
		requestRestart:  requestRestart,
		requestRender:   requestRender,
		dbQueryLogging:  dbQueryLogging,
		appDebug:        appDebug,
	}
	controller.startHotkeys()
	return writer, writer, controller.shutdown, controller.applyEnv
}

func buildDevFooterSeparatorLine() string {
	return buildDevSectionSeparatorLine("")
}

func buildDevStartupSeparatorLine() string {
	success := lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
	return buildDevSectionSeparatorLine(success.Render(console.SuccessMark() + " Startup"))
}

func buildDevSectionSeparatorLine(label string) string {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		width = 120
	}
	if width < 20 {
		width = 20
	}
	borderColor := lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}
	if strings.TrimSpace(label) == "" {
		return lipgloss.NewStyle().Foreground(borderColor).Render(strings.Repeat("─", width))
	}
	ruleStyle := lipgloss.NewStyle().Foreground(borderColor)
	centerText := fmt.Sprintf(" %s ", strings.TrimSpace(label))
	centerWidth := lipgloss.Width(centerText)
	if width <= centerWidth {
		return centerText
	}
	left := 5
	if width-centerWidth-left < 0 {
		left = 0
	}
	right := width - centerWidth - left
	if right < 0 {
		right = 0
	}
	return ruleStyle.Render(strings.Repeat("─", left)) + centerText + ruleStyle.Render(strings.Repeat("─", right))
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
		targetURL, err := c.lighthouseOpenTarget()
		if err != nil {
			_, _ = c.writer.Write([]byte(fmt.Sprintf("%s Failed to prepare lighthouse login: %v\n", console.ErrorMark(), err)))
			return
		}
		if err := openURL(targetURL); err != nil {
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

func resolveLighthouseOpenURL(lighthouseURL string) string {
	raw := strings.TrimSpace(lighthouseURL)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	basePath := strings.TrimRight(u.Path, "/")
	if basePath == "" {
		basePath = "/lighthouse"
	}
	u.Path = basePath + "/auth/dev-session"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func (c *devFooterController) applyEnv() {
	c.apiURL = resolveAPIURL(nil)
	c.lighthouseURL = resolveLighthouseUIURL(nil)
	c.lighthouseToken = strings.TrimSpace(os.Getenv("LIGHTHOUSE_TOKEN"))
	c.dbQueryLogging, c.appDebug = loadDevRuntimeSettings()
	c.refreshFooter()
}

func (c *devFooterController) lighthouseOpenTarget() (string, error) {
	target := strings.TrimSpace(c.lighthouseURL)
	if target == "" {
		return "", fmt.Errorf("lighthouse URL is empty")
	}
	token := strings.TrimSpace(c.lighthouseToken)
	if token == "" {
		return target, nil
	}
	openURL := resolveLighthouseOpenURL(target)
	if strings.TrimSpace(openURL) == "" {
		return target, nil
	}
	payloadBody, err := json.Marshal(map[string]string{})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, openURL, bytes.NewReader(payloadBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return target, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		return target, nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	var payload struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.URL) == "" {
		return "", fmt.Errorf("empty dev session URL")
	}
	return absolutizeLighthouseURL(target, payload.URL), nil
}

func absolutizeLighthouseURL(baseURL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsedRaw, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if parsedRaw.IsAbs() {
		return parsedRaw.String()
	}
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return raw
	}
	return base.ResolveReference(parsedRaw).String()
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
