package forj

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/goforj/console"
	"github.com/goforj/goforj/project"
	"github.com/goforj/str/v2"
	"github.com/gorilla/websocket"
)

// devwatchLine represents a watcher output line.
type devwatchLine struct {
	ID        int64     `json:"id"`
	Line      string    `json:"line"`
	Stream    string    `json:"stream"`
	Timestamp time.Time `json:"timestamp"`
	Watcher   string    `json:"watcher,omitempty"`
}

// devwatchStreamer ships watcher output to the dev console websocket.
type devwatchStreamer struct {
	url               string
	header            http.Header
	mu                sync.Mutex
	conn              *websocket.Conn
	writeMu           sync.Mutex
	connected         bool
	ch                chan devwatchLine
	done              chan struct{}
	once              sync.Once
	lastWarn          time.Time
	lastAttempt       time.Time
	lastReconnectLog  time.Time
	reconnectAttempts int
	pending           []devwatchLine
	startAt           time.Time
	startDelay        time.Duration
	restartCh         chan struct{}
	renderCh          chan struct{}
	closing           bool
	lastPing          time.Time
	waitForServer     bool
	nextDialAllowedAt time.Time
}

// newDevwatchStreamerFromEnv creates a streamer when lighthouse env is configured.
func newDevwatchStreamerFromEnv() *devwatchStreamer {
	if !Enabled() {
		console.Debugf("devwatch disabled: LIGHTHOUSE_ENABLED is false")
		return nil
	}
	token := str.Of(resolveLighthouseSecret(nil)).Trim().String()
	if token == "" {
		console.Debugf("devwatch disabled: LIGHTHOUSE_SECRET is empty")
		return nil
	}
	rawURL := str.Of(getEnv("LIGHTHOUSE_URL")).Trim().String()
	if rawURL == "" {
		rawURL = "ws://localhost:3000/lighthouse/ws/devwatch"
	}
	return newDevwatchStreamer(rawURL, token)
}

// resolveLighthouseSecret centralizes resolve lighthouse secret lookup for the surrounding workflow.
func resolveLighthouseSecret(env map[string]string) string {
	if env != nil {
		if value := strings.TrimSpace(env["LIGHTHOUSE_SECRET"]); value != "" {
			return value
		}
	}
	return strings.TrimSpace(getEnv("LIGHTHOUSE_SECRET"))
}

// newDevwatchStreamer centralizes new devwatch streamer behavior so callers follow the same contract.
func newDevwatchStreamer(rawURL string, token string) *devwatchStreamer {
	token = str.Of(token).Trim().String()
	if token == "" {
		console.Debugf("devwatch disabled: token is empty")
		return nil
	}
	wsURL := normalizeDevwatchWSURL(rawURL)
	console.Debugf("devwatch configured: url=%s", wsURL)

	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	streamer := &devwatchStreamer{
		url:        wsURL,
		header:     header,
		ch:         make(chan devwatchLine, 256),
		done:       make(chan struct{}),
		startAt:    time.Now(),
		startDelay: 2 * time.Second,
	}
	go streamer.run()
	go streamer.readLoop()
	go streamer.reconnectLoop()
	return streamer
}

// normalizeDevwatchWSURL keeps normalize devwatch wsurl handling consistent across callers.
func normalizeDevwatchWSURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err == nil && parsed != nil {
		// Always target the devwatch endpoint regardless of whether LIGHTHOUSE_URL
		// points to /lighthouse/ws/agent.
		switch parsed.Path {
		case "/lighthouse/ws/agent", "/lighthouse/ws/devwatch":
			parsed.Path = "/lighthouse/ws/devwatch"
		default:
			if strings.Contains(parsed.Path, "/ws/agent") || strings.Contains(parsed.Path, "/ws/devwatch") {
				parsed.Path = "/lighthouse/ws/devwatch"
			}
		}
		q := parsed.Query()
		q.Set("role", "source")
		parsed.RawQuery = q.Encode()
		return parsed.String()
	}

	// Fallback for malformed values.
	wsURL := strings.Replace(raw, "/lighthouse/ws/agent", "/lighthouse/ws/devwatch", 1)
	if strings.Contains(wsURL, "?") {
		if !strings.Contains(wsURL, "role=") {
			wsURL += "&role=source"
		}
	} else {
		wsURL += "?role=source"
	}
	return wsURL
}

// Close stops the streamer and closes the websocket.
func (s *devwatchStreamer) Close() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		s.mu.Lock()
		s.closing = true
		s.mu.Unlock()
		close(s.done)
		s.closeConn()
	})
}

// SetRestartChannel registers a restart notification channel.
func (s *devwatchStreamer) SetRestartChannel(ch chan struct{}) {
	if s == nil {
		return
	}
	s.restartCh = ch
}

// SetRenderChannel registers a render notification channel.
func (s *devwatchStreamer) SetRenderChannel(ch chan struct{}) {
	if s == nil {
		return
	}
	s.renderCh = ch
}

// Send enqueues a line for streaming.
func (s *devwatchStreamer) Send(line devwatchLine) {
	if s == nil {
		return
	}
	select {
	case s.ch <- line:
	default:
	}
}

// run processes queued lines and streams them via the dev console websocket.
func (s *devwatchStreamer) run() {
	for {
		select {
		case line := <-s.ch:
			if !s.ensureConn() {
				s.buffer(line)
				continue
			}
			if err := s.writeLine(line); err != nil {
				s.closeConn()
				s.buffer(line)
				continue
			}
			s.flushPending()
		case <-s.done:
			return
		}
	}
}

// ensureConn lazily dials the websocket when allowed by the configured delay.
func (s *devwatchStreamer) ensureConn() bool {
	s.mu.Lock()
	if s.conn != nil {
		s.mu.Unlock()
		return true
	}
	if s.closing {
		s.mu.Unlock()
		return false
	}
	if time.Since(s.startAt) < s.startDelay {
		console.Debugf("devwatch ensureConn delayed: startAt=%v startDelay=%v", s.startAt, s.startDelay)
		s.mu.Unlock()
		return false
	}
	now := time.Now()
	if now.Before(s.nextDialAllowedAt) {
		s.mu.Unlock()
		return false
	}
	if s.waitForServer {
		s.mu.Unlock()
		return false
	}
	s.lastAttempt = now
	s.reconnectAttempts++
	console.Debugf("devwatch ensureConn dialing %s (attempt %d)", s.url, s.reconnectAttempts)
	url := s.url
	header := s.header
	wasConnected := s.connected
	s.mu.Unlock()

	conn, _, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		console.Debugf("lighthouse watcher dial failed: %v", err)
		s.maybeWarn(err)
		s.mu.Lock()
		s.nextDialAllowedAt = now.Add(time.Second)
		s.mu.Unlock()
		return false
	}
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		_ = conn.Close()
		return false
	}
	s.conn = conn
	s.connected = true
	s.startDelay = 0
	s.reconnectAttempts = 0
	s.nextDialAllowedAt = time.Time{}
	s.lastPing = time.Time{}
	s.mu.Unlock()
	_ = conn.SetReadDeadline(time.Now().Add(20 * time.Second))
	conn.SetPongHandler(func(string) error {
		s.mu.Lock()
		active := s.conn
		s.mu.Unlock()
		if active == nil {
			return nil
		}
		return active.SetReadDeadline(time.Now().Add(20 * time.Second))
	})
	conn.SetPingHandler(func(appData string) error {
		s.mu.Lock()
		active := s.conn
		s.mu.Unlock()
		if active == nil {
			return nil
		}
		_ = active.SetReadDeadline(time.Now().Add(20 * time.Second))
		s.writeMu.Lock()
		defer s.writeMu.Unlock()
		return active.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(5*time.Second))
	})
	console.Debugf("devwatch connected (%s)", url)
	if !wasConnected {
		console.Debugf("Devwatch stream reconnected to %s", url)
	}
	return true
}

// closeConn closes the current websocket connection, if any.
func (s *devwatchStreamer) closeConn() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return
	}
	console.Debugf("devwatch closing connection")
	_ = s.conn.Close()
	s.conn = nil
	s.connected = false
	// Prevent tight reconnect loops after abrupt disconnects.
	if s.nextDialAllowedAt.Before(time.Now().Add(time.Second)) {
		s.nextDialAllowedAt = time.Now().Add(time.Second)
	}
	s.lastPing = time.Time{}
}

// closeConnIfCurrent centralizes close conn if current behavior so callers follow the same contract.
func (s *devwatchStreamer) closeConnIfCurrent(conn *websocket.Conn) {
	s.mu.Lock()
	if s.conn != conn {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	s.closeConn()
}

// maybeWarn throttles warning logs about websocket failures.
func (s *devwatchStreamer) maybeWarn(err error) {
	select {
	case <-s.done:
		return
	default:
	}
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	if time.Since(s.lastWarn) < 10*time.Second {
		return
	}
	if shouldSuppressWarn(err) {
		return
	}
	s.lastWarn = time.Now()
	console.Debugf("lighthouse watcher stream unavailable: %v", err)
}

// shouldSuppressWarn determines if a warning should be suppressed based on the error type.
func shouldSuppressWarn(err error) bool {
	return strings.Contains(err.Error(), "bad handshake")
}

// readLoop listens for restart requests from the backend control channel.
func (s *devwatchStreamer) readLoop() {
	for {
		select {
		case <-s.done:
			return
		default:
		}
		if !s.ensureConn() {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		s.mu.Lock()
		conn := s.conn
		s.mu.Unlock()
		if conn == nil {
			continue
		}
		var msg map[string]any
		if err := conn.ReadJSON(&msg); err != nil {
			console.Debugf("devwatch readLoop: read error: %v", err)
			s.closeConnIfCurrent(conn)
			continue
		}
		if msg["type"] != "devwatch-control" {
			continue
		}
		payload, ok := msg["payload"].(map[string]any)
		if !ok {
			continue
		}
		action := fmt.Sprint(payload["action"])
		switch action {
		case "restart":
			if s.restartCh == nil {
				continue
			}
			select {
			case s.restartCh <- struct{}{}:
			default:
			}
			// Restart the websocket connection so the devwatch stream re-establishes
			// once the watchers come back online.
			console.Debugf("Devwatch restart requested; closing connection to reconnect")
			s.mu.Lock()
			s.startAt = time.Now()
			s.startDelay = 2 * time.Second
			s.waitForServer = true
			s.mu.Unlock()
			s.closeConn()
		case "render":
			if s.renderCh == nil {
				continue
			}
			select {
			case s.renderCh <- struct{}{}:
			default:
			}
		}
	}
}

// reconnectLoop centralizes reconnect loop behavior so callers follow the same contract.
func (s *devwatchStreamer) reconnectLoop() {
	console.Debugf("devwatch reconnect loop started")
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
		}
		s.mu.Lock()
		hasConn := s.conn != nil
		closing := s.closing
		conn := s.conn
		shouldPing := hasConn && (s.lastPing.IsZero() || time.Since(s.lastPing) > 10*time.Second)
		shouldLog := time.Since(s.lastReconnectLog) > 2*time.Second
		waitForServer := s.waitForServer
		if !hasConn && !closing && shouldLog {
			s.lastReconnectLog = time.Now()
		}
		s.mu.Unlock()
		if hasConn {
			if shouldPing && conn != nil {
				s.writeMu.Lock()
				err := conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(5*time.Second))
				s.writeMu.Unlock()
				if err != nil {
					console.Debugf("devwatch ping failed: %v", err)
					s.closeConn()
					continue
				}
				s.mu.Lock()
				s.lastPing = time.Now()
				s.mu.Unlock()
			}
			continue
		}
		if waitForServer {
			if s.checkServerReady() {
				s.mu.Lock()
				s.waitForServer = false
				s.mu.Unlock()
			} else {
				if shouldLog {
					console.Debugf("devwatch reconnect loop: waiting for server readiness")
				}
				continue
			}
		}
		if !closing && shouldLog {
			console.Debugf("devwatch reconnect loop: no connection, attempt=%d", s.reconnectAttempts+1)
		}
		_ = s.ensureConn()
	}
}

// checkServerReady centralizes check server ready behavior so callers follow the same contract.
func (s *devwatchStreamer) checkServerReady() bool {
	raw := s.url
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	scheme := "http"
	if parsed.Scheme == "wss" {
		scheme = "https"
	}
	parsed.Scheme = scheme
	parsed.Path = "/lighthouse/api/agents"
	parsed.RawQuery = ""
	req, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", s.header.Get("Authorization"))
	client := &http.Client{Timeout: 750 * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}

// buffer queues a line locally while the websocket is unavailable.
func (s *devwatchStreamer) buffer(line devwatchLine) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = append(s.pending, line)
	if len(s.pending) > 5000 {
		s.pending = s.pending[len(s.pending)-5000:]
	}
}

// flushPending drains buffered lines once the connection recovers.
func (s *devwatchStreamer) flushPending() {
	s.mu.Lock()
	pending := append([]devwatchLine{}, s.pending...)
	s.pending = nil
	conn := s.conn
	s.mu.Unlock()
	if conn == nil || len(pending) == 0 {
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	for _, line := range pending {
		if err := conn.WriteJSON(map[string]any{
			"type": "devwatch",
			"payload": map[string]any{
				"line": line,
			},
		}); err != nil {
			s.closeConn()
			s.buffer(line)
			return
		}
	}
}

// writeLine sends a single line via the websocket connection.
func (s *devwatchStreamer) writeLine(line devwatchLine) error {
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("no websocket connection")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return conn.WriteJSON(map[string]any{
		"type": "devwatch",
		"payload": map[string]any{
			"line": line,
		},
	})
}

// devwatchWriter tees output to the console and devwatch streamer.
type devwatchWriter struct {
	out                   io.Writer
	stream                string
	watcher               string
	command               string
	appName               string
	appNameWidth          int
	showAppColumn         bool
	lifecycle             *devwatchLifecycleState
	onTrigger             func()
	skipBlankAfterTrigger bool
	buf                   bytes.Buffer
	streamer              *devwatchStreamer
	mu                    sync.Mutex
}

const watcherTriggerMarker = "__FORJ_WATCHER_TRIGGER__"
const buildProgressMarker = "__FORJ_BUILD_PROGRESS__"
const devDefaultAppColor = "\033[38;5;109m"

var devLogTimestampPrefixPattern = regexp.MustCompile(`^((?:\x1b\[[0-9;?]*[ -/]*[@-~])*\d{2}:\d{2}:\d{2}\.\d{3}(?:\x1b\[[0-9;?]*[ -/]*[@-~])*)(\s+)`)

type devwatchLifecycleState struct {
	mu              sync.Mutex
	startupExpected int
	startupSeen     int
	startupEmitted  bool
	restartExpected map[string]struct{}
	restartSeen     map[string]struct{}
	restartShutdown bool
	separators      bool
}

// newDevwatchLifecycleState centralizes new devwatch lifecycle state behavior so callers follow the same contract.
func newDevwatchLifecycleState(startupExpected int, restartWatches []string) *devwatchLifecycleState {
	restartExpected := make(map[string]struct{}, len(restartWatches))
	for _, watch := range restartWatches {
		if watch = str.Of(watch).Trim().String(); watch != "" {
			restartExpected[watch] = struct{}{}
		}
	}
	return &devwatchLifecycleState{
		startupExpected: startupExpected,
		restartExpected: restartExpected,
		restartSeen:     map[string]struct{}{},
		separators:      true,
	}
}

// separatorsEnabled keeps decorative boundaries on legacy streams while the TUI uses lifecycle transactions.
func (s *devwatchLifecycleState) separatorsEnabled() bool {
	return s != nil && s.separators
}

// noteStartupTrigger centralizes note startup trigger behavior so callers follow the same contract.
func (s *devwatchLifecycleState) noteStartupTrigger() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.startupEmitted {
		return false
	}
	s.startupSeen++
	// Emit the closing separator exactly once after the last initial startup
	// trigger so the started/ready/starting block is visually bracketed.
	if s.startupSeen < s.startupExpected {
		return false
	}
	s.startupEmitted = true
	return true
}

// startupEmittedAlready centralizes startup emitted already behavior so callers follow the same contract.
func (s *devwatchLifecycleState) startupEmittedAlready() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startupEmitted
}

// noteRestartShutdown centralizes note restart shutdown behavior so callers follow the same contract.
func (s *devwatchLifecycleState) noteRestartShutdown(watcher string, line string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.restartExpected[watcher]; !ok {
		return false
	}
	if !isRuntimeShutdownLine(line) || s.restartShutdown {
		return false
	}
	s.restartShutdown = true
	return true
}

// noteRestartTrigger centralizes note restart trigger behavior so callers follow the same contract.
func (s *devwatchLifecycleState) noteRestartTrigger(watcher string) string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.restartExpected[watcher]; !ok {
		return ""
	}
	emit := len(s.restartSeen) == 0
	label := ""
	if emit && s.restartShutdown {
		label = "Start"
	}
	s.restartSeen[watcher] = struct{}{}
	if len(s.restartSeen) >= len(s.restartExpected) {
		s.restartSeen = map[string]struct{}{}
		s.restartShutdown = false
	}
	if !emit {
		return ""
	}
	return label
}

// legacyRestartExpected centralizes legacy restart expected behavior so callers follow the same contract.
func legacyRestartExpected(watches []string) map[string]struct{} {
	expected := make(map[string]struct{}, len(watches))
	for _, watch := range watches {
		if watch = str.Of(watch).Trim().String(); watch != "" {
			expected[watch] = struct{}{}
		}
	}
	return expected
}

// Serializes writes across all watcher writers to avoid interleaved terminal lines.
var devwatchOutputMu sync.Mutex

// newDevwatchWriter creates a writer that mirrors output to the devwatch websocket while still writing to the original writer.
func newDevwatchWriter(out io.Writer, streamer *devwatchStreamer, stream string, watcher string, command string, lifecycle *devwatchLifecycleState) io.Writer {
	return newDevwatchWriterForApp(out, streamer, stream, watcher, command, "", 0, false, lifecycle, nil)
}

// newDevwatchWriterForApp creates a writer that can add dev-only app context to runtime logs.
func newDevwatchWriterForApp(
	out io.Writer,
	streamer *devwatchStreamer,
	stream string,
	watcher string,
	command string,
	appName string,
	appNameWidth int,
	showAppColumn bool,
	lifecycle *devwatchLifecycleState,
	onTrigger func(),
) io.Writer {
	if out == nil {
		return out
	}
	return &devwatchWriter{
		out:           out,
		streamer:      streamer,
		stream:        stream,
		watcher:       watcher,
		command:       command,
		appName:       appName,
		appNameWidth:  appNameWidth,
		showAppColumn: showAppColumn,
		lifecycle:     lifecycle,
		onTrigger:     onTrigger,
	}
}

// Write streams each complete line to both the wrapped writer and the dev console websocket for buffering/tracking.
func (w *devwatchWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(p) == 0 {
		return 0, nil
	}
	if _, err := w.buf.Write(p); err != nil {
		return 0, err
	}
	triggered := false
	for {
		data := w.buf.Bytes()
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			break
		}
		line := data[:idx]
		w.buf.Next(idx + 1)
		timestamp := time.Now()
		rawLine := finalDevwatchTerminalFrame(string(line))
		if w.skipBlankAfterTrigger {
			w.skipBlankAfterTrigger = false
			if rawLine == "" {
				continue
			}
		}
		if isWatcherTriggerLine(rawLine) {
			w.skipBlankAfterTrigger = true
			triggered = true
		}
		if handled := handleBuildProgressLine(w.out, w.watcher, rawLine); handled {
			continue
		}
		if isDevBuildWatcher(w.watcher) && hasDevStatusLine(w.out) {
			clearDevStatusLine(w.out)
		}
		outLine := decorateWatcherLine(rawLine, w.watcher, w.command)
		outLine = decorateDevAppLogAppColumn(outLine, w.appName, w.appNameWidth, w.showAppColumn)
		restartSeparator := ""
		shutdownSeparator := false
		if w.lifecycle.separatorsEnabled() && w.lifecycle.startupEmittedAlready() {
			shutdownSeparator = w.lifecycle.noteRestartShutdown(w.watcher, rawLine)
		}
		if w.lifecycle.separatorsEnabled() && isWatcherTriggerLine(rawLine) && w.lifecycle.startupEmittedAlready() {
			restartSeparator = w.lifecycle.noteRestartTrigger(w.watcher)
		}
		if w.streamer != nil {
			w.streamer.Send(devwatchLine{
				Line:      outLine,
				Stream:    w.stream,
				Timestamp: timestamp,
				ID:        timestamp.UnixMilli(),
				Watcher:   w.watcher,
			})
		}
		devwatchOutputMu.Lock()
		if shutdownSeparator {
			separator := buildDevShutdownSeparatorLine()
			if err := writeDevwatchSeparator(w.out, w.streamer, w.stream, w.watcher, timestamp, separator); err != nil {
				devwatchOutputMu.Unlock()
				return 0, err
			}
		}
		if restartSeparator != "" {
			separator := buildDevSectionSeparatorLine(restartSeparator)
			if err := writeDevwatchSeparator(w.out, w.streamer, w.stream, w.watcher, timestamp, separator); err != nil {
				devwatchOutputMu.Unlock()
				return 0, err
			}
		}
		if _, err := io.WriteString(w.out, outLine); err != nil {
			devwatchOutputMu.Unlock()
			return 0, err
		}
		if _, err := w.out.Write([]byte{'\n'}); err != nil {
			devwatchOutputMu.Unlock()
			return 0, err
		}
		if w.lifecycle.separatorsEnabled() && isWatcherTriggerLine(rawLine) && w.lifecycle.noteStartupTrigger() {
			separator := buildDevStartupSeparatorLine()
			if w.streamer != nil {
				w.streamer.Send(devwatchLine{
					Line:      separator,
					Stream:    w.stream,
					Timestamp: timestamp,
					ID:        timestamp.UnixMilli(),
					Watcher:   w.watcher,
				})
			}
			if _, err := io.WriteString(w.out, separator); err != nil {
				devwatchOutputMu.Unlock()
				return 0, err
			}
			if _, err := w.out.Write([]byte{'\n'}); err != nil {
				devwatchOutputMu.Unlock()
				return 0, err
			}
		}
		devwatchOutputMu.Unlock()
	}
	// Report the launch after the whole pipe read is persisted so adjacent startup output cannot overtake the transaction.
	if triggered && w.onTrigger != nil {
		w.onTrigger()
	}
	return len(p), nil
}

// finalDevwatchTerminalFrame retains the last redraw frame because transcript lines cannot reproduce in-place terminal updates.
func finalDevwatchTerminalFrame(line string) string {
	line = strings.TrimRight(line, "\r")
	if index := strings.LastIndexByte(line, '\r'); index >= 0 {
		return line[index+1:]
	}
	return line
}

// decorateDevAppLogAppColumn adds a dev-only app column to timestamped runtime log lines.
func decorateDevAppLogAppColumn(line string, appName string, width int, enabled bool) string {
	appName = str.Of(appName).Trim().String()
	if !enabled || appName == "" || width <= 0 {
		return line
	}
	match := devLogTimestampPrefixPattern.FindStringSubmatchIndex(line)
	if len(match) < 6 {
		return line
	}
	targetLabel := truncateDevApp(appName, width)
	column := fmt.Sprintf(" %-*s ", width, targetLabel)
	column = colorizeDevAppColumn(column, appName)
	return line[:match[3]] + column + line[match[5]:]
}

// colorizeDevAppColumn keeps the default app quiet and gives additional apps stable accents.
func colorizeDevAppColumn(column string, appName string) string {
	if appName == project.DefaultAppName {
		return console.Colorize(devDefaultAppColor, column)
	}
	return console.Colorize(devAppColor(appName), column)
}

// devAppColor maps app names to a stable restrained ANSI palette.
func devAppColor(appName string) string {
	palette := []string{
		console.ColorCyan,
		console.ColorGreen,
		console.ColorYellow,
		"\033[34m",
		"\033[35m",
		"\033[96m",
	}
	hash := uint32(2166136261)
	for _, r := range strings.ToLower(appName) {
		hash ^= uint32(r)
		hash *= 16777619
	}
	return palette[int(hash%uint32(len(palette)))]
}

// truncateDevApp keeps the app column stable when a long app name appears in dev logs.
func truncateDevApp(appName string, width int) string {
	if width <= 0 || len(appName) <= width {
		return appName
	}
	if width == 1 {
		return appName[:1]
	}
	return appName[:width-1] + "~"
}

// decorateWatcherLine centralizes decorate watcher line behavior so callers follow the same contract.
func decorateWatcherLine(line, watcher string, command string) string {
	if watcher == "" {
		return line
	}
	if isWatcherTriggerLine(line) {
		cmd := str.Of(command).Trim().String()
		if cmd == "" {
			cmd = "(unknown command)"
		}
		return fmt.Sprintf(
			"%s %s %s - %s",
			console.ActionMark(),
			console.Colorize(console.ColorBoldWhite, "Starting"),
			console.Colorize(console.ColorBoldWhite, watcher),
			console.Colorize(console.ColorGray, cmd),
		)
	}
	if strings.Contains(line, "Starting ") {
		return line
	}
	return line
}

// isWatcherTriggerLine centralizes the is watcher trigger line decision for its callers.
func isWatcherTriggerLine(line string) bool {
	line = normalizeDevwatchProtocolLine(line)
	return line == watcherTriggerMarker || strings.Contains(line, watcherTriggerMarker)
}

// handleBuildProgressLine centralizes handle build progress line behavior so callers follow the same contract.
func handleBuildProgressLine(out io.Writer, watcher string, line string) bool {
	if !isDevBuildWatcher(watcher) {
		return false
	}
	line = normalizeDevwatchProtocolLine(line)
	markerIndex := strings.Index(line, buildProgressMarker)
	if markerIndex < 0 {
		return false
	}
	line = line[markerIndex:]
	payload := str.Of(line).TrimPrefix(buildProgressMarker).Trim().String()
	switch {
	case strings.HasPrefix(payload, "step "):
		parts := strings.Fields(str.Of(payload).TrimPrefix("step ").Trim().String())
		if len(parts) < 2 {
			return true
		}
		stepNumber := parts[0]
		stepName := strings.Join(parts[1:], " ")
		setDevStatusLine(out, formatBuildProgressStatus(stepNumber, stepName))
	case payload == "done":
		markDevStatusDone(out)
	case payload == "failed":
		clearDevStatusLine(out)
	default:
		clearDevStatusLine(out)
	}
	return true
}

// normalizeDevwatchProtocolLine keeps normalize devwatch protocol line handling consistent across callers.
func normalizeDevwatchProtocolLine(line string) string {
	line = ansiCSI.ReplaceAllString(line, "")
	if index := strings.LastIndex(line, "\r"); index >= 0 {
		line = line[index+1:]
	}
	return str.Of(line).Trim().String()
}

// formatBuildProgressStatus keeps the format build progress status representation consistent.
func formatBuildProgressStatus(stepNumber string, stepName string) string {
	stepLabel := console.Colorize(console.ColorBoldWhite, strings.TrimSpace(stepName))
	stepCount := console.Colorize(console.ColorGray, strings.TrimSpace(stepNumber))
	return fmt.Sprintf("%s %s", stepCount, stepLabel)
}

// isRuntimeShutdownLine centralizes the is runtime shutdown line decision for its callers.
func isRuntimeShutdownLine(line string) bool {
	line = str.Of(strings.ReplaceAll(line, "\r", "")).Trim().String()
	if line == "" {
		return false
	}
	for _, pattern := range []string{
		"Shutting down HTTP server",
		"HTTP server shut down",
		"Shutting down scheduler",
		"Scheduler shut down",
		"Shutting down queue worker",
		"Queue worker shut down",
		"INFO: Starting graceful shutdown",
		"INFO: Waiting for all workers to finish",
		"INFO: All workers have finished",
		"INFO: Exiting",
	} {
		if strings.Contains(line, pattern) {
			return true
		}
	}
	return false
}

// writeDevwatchSeparator centralizes write devwatch separator persistence for the surrounding workflow.
func writeDevwatchSeparator(out io.Writer, streamer *devwatchStreamer, stream string, watcher string, timestamp time.Time, separator string) error {
	if streamer != nil {
		streamer.Send(devwatchLine{
			Line:      separator,
			Stream:    stream,
			Timestamp: timestamp,
			ID:        timestamp.UnixMilli(),
			Watcher:   watcher,
		})
	}
	if _, err := io.WriteString(out, separator); err != nil {
		return err
	}
	if _, err := out.Write([]byte{'\n'}); err != nil {
		return err
	}
	return nil
}

// getEnv centralizes get env behavior so callers follow the same contract.
func getEnv(key string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	if value, ok := readDotEnvValue(key); ok {
		return value
	}
	return ""
}

// readDotEnvValue parses the local .env file to find a single key value.
func readDotEnvValue(key string) (string, bool) {
	data, err := os.ReadFile(".env")
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := str.Of(line).Trim().String()
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		name, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		if str.Of(name).Trim().String() != key {
			continue
		}
		return str.Of(value).Trim().TrimChars(`"`).String(), true
	}
	return "", false
}
