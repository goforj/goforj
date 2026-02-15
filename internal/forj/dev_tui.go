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
	devconsoleURL  string
	requestRestart func()
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

func (w *devFooterWriter) SetFooterLine(line string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.footerLine = line
}

func disableDevFooter(writer io.Writer) {
	footerWriter, ok := writer.(*devFooterWriter)
	if !ok || footerWriter == nil {
		return
	}
	footerWriter.DisableFooter()
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

func buildDevOutputWriters(env map[string]string, requestRestart func()) (io.Writer, io.Writer, func()) {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return os.Stdout, os.Stderr, func() {}
	}
	if strings.TrimSpace(os.Getenv("FORJ_DEV_PLAIN")) == "1" {
		return os.Stdout, os.Stderr, func() {}
	}

	apiURL := resolveAPIURL(env)
	devconsoleURL := resolveDevconsoleUIURL(env)
	dbQueryLogging, appDebug := loadDevRuntimeSettings()
	footer := buildDevFooterLineWithState(apiURL, devconsoleURL, dbQueryLogging, appDebug)
	if footer == "" {
		return os.Stdout, os.Stderr, func() {}
	}

	writer := newDevFooterWriter(os.Stdout, buildDevFooterSeparatorLine(), footer)
	controller := &devFooterController{
		writer:         writer,
		apiURL:         apiURL,
		devconsoleURL:  devconsoleURL,
		requestRestart: requestRestart,
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
	return buildDevFooterLineWithState(resolveAPIURL(env), resolveDevconsoleUIURL(env), dbQueryLogging, appDebug)
}

func buildDevFooterLineWithURLs(apiURL, devconsoleURL string) string {
	dbQueryLogging, appDebug := loadDevRuntimeSettings()
	return buildDevFooterLineWithState(apiURL, devconsoleURL, dbQueryLogging, appDebug)
}

func buildDevFooterLineWithState(apiURL, devconsoleURL string, dbQueryLogging bool, appDebug string) string {
	if apiURL == "" && devconsoleURL == "" {
		return ""
	}
	parts := []string{"hotkeys: ? help"}
	if devconsoleURL != "" {
		parts = append(parts, "o devconsole")
	}
	if apiURL != "" {
		parts = append(parts, "a api")
	}
	if requestRestartEnabled(devconsoleURL, apiURL) {
		parts = append(parts, "r restart")
		queryState := "off"
		if dbQueryLogging {
			queryState = "on"
		}
		parts = append(parts, "q query logging:"+queryState)
		parts = append(parts, "0/1/2/3 app debug:"+appDebug)
	}
	footerText := strings.Join(parts, " · ")
	if bannerColorsEnabled() {
		footerText = colorizeGradientLine(footerText, false)
	}
	return fmt.Sprintf(
		"%s %s",
		console.Colorize(console.ColorYellow, "•"),
		footerText,
	)
}

func requestRestartEnabled(devconsoleURL, apiURL string) bool {
	// Footer hotkeys are only displayed when either API/devconsole is available.
	// Restart is always available in forj dev when footer is active.
	return devconsoleURL != "" || apiURL != ""
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
		switch strings.ToLower(string(ch)) {
		case "o":
			if c.devconsoleURL == "" {
				continue
			}
			if err := openURL(c.devconsoleURL); err != nil {
				_, _ = c.writer.Write([]byte(fmt.Sprintf("%s Failed to open devconsole: %v\n", console.ErrorMark(), err)))
			}
		case "a":
			if c.apiURL == "" {
				continue
			}
			if err := openURL(c.apiURL); err != nil {
				_, _ = c.writer.Write([]byte(fmt.Sprintf("%s Failed to open API URL: %v\n", console.ErrorMark(), err)))
			}
		case "?", "h":
			parts := []string{fmt.Sprintf("%s Dev hotkeys:", console.InfoMark())}
			if c.devconsoleURL != "" {
				parts = append(parts, fmt.Sprintf("  o  open devconsole (%s)", c.devconsoleURL))
			}
			if c.apiURL != "" {
				parts = append(parts, fmt.Sprintf("  a  open api (%s)", c.apiURL))
			}
			parts = append(parts, "  r  restart watchers")
			parts = append(parts, "  q  toggle DB_QUERY_LOGGING and restart")
			parts = append(parts, "  0/1/2/3  set APP_DEBUG and restart")
			parts = append(parts, "  ?  show this help")
			_, _ = c.writer.Write([]byte(strings.Join(parts, "\n") + "\n"))
		case "r":
			if c.requestRestart == nil {
				continue
			}
			c.requestRestart()
			_, _ = c.writer.Write([]byte(fmt.Sprintf("%s Restart requested\n", console.ActionMark())))
		case "q":
			if err := c.toggleDBQueryLogging(); err != nil {
				_, _ = c.writer.Write([]byte(fmt.Sprintf("%s %v\n", console.ErrorMark(), err)))
				continue
			}
		case "0", "1", "2", "3":
			if err := c.setAppDebugLevel(string(ch)); err != nil {
				_, _ = c.writer.Write([]byte(fmt.Sprintf("%s %v\n", console.ErrorMark(), err)))
				continue
			}
		}
	}
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
		buildDevFooterLineWithState(c.apiURL, c.devconsoleURL, c.dbQueryLogging, c.appDebug),
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
