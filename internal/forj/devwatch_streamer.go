package forj

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"net/url"

	"github.com/goforj/goforj/internal/console"
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
	url         string
	header      http.Header
	mu          sync.Mutex
	conn        *websocket.Conn
	writeMu     sync.Mutex
	connected   bool
	ch          chan devwatchLine
	done        chan struct{}
	once        sync.Once
	lastWarn    time.Time
	lastAttempt time.Time
	lastReconnectLog time.Time
	reconnectAttempts int
	pending     []devwatchLine
	startAt     time.Time
	startDelay  time.Duration
	restartCh   chan struct{}
	closing     bool
	lastPing    time.Time
	waitForServer bool
}

// newDevwatchStreamerFromEnv creates a streamer when devconsole env is configured.
func newDevwatchStreamerFromEnv() *devwatchStreamer {
	if !Enabled() {
		console.Debugf("devwatch disabled: DEVCONSOLE_ENABLED is false")
		return nil
	}
	token := strings.TrimSpace(getEnv("DEVCONSOLE_TOKEN"))
	if token == "" {
		console.Debugf("devwatch disabled: DEVCONSOLE_TOKEN is empty")
		return nil
	}
	rawURL := strings.TrimSpace(getEnv("DEVCONSOLE_URL"))
	if rawURL == "" {
		rawURL = "ws://localhost:3000/__devconsole/ws/devwatch"
	}
	wsURL := strings.Replace(rawURL, "/__devconsole/ws/agent", "/__devconsole/ws/devwatch", 1)
	if strings.Contains(wsURL, "?") {
		wsURL += "&role=source"
	} else {
		wsURL += "?role=source"
	}
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
	defer s.mu.Unlock()
	if s.conn != nil {
		return true
	}
	if s.closing {
		return false
	}
	if time.Since(s.startAt) < s.startDelay {
		console.Debugf("devwatch ensureConn delayed: startAt=%v startDelay=%v", s.startAt, s.startDelay)
		return false
	}
	if time.Since(s.lastAttempt) < time.Second {
		console.Debugf("devwatch ensureConn throttle: lastAttempt=%v", s.lastAttempt)
		return false
	}
	if s.waitForServer {
		return false
	}
	s.lastAttempt = time.Now()
	s.reconnectAttempts++
	console.Debugf("devwatch ensureConn dialing %s (attempt %d)", s.url, s.reconnectAttempts)
	conn, _, err := websocket.DefaultDialer.Dial(s.url, s.header)
	if err != nil {
		console.Debugf("devconsole watcher dial failed: %v", err)
		s.maybeWarn(err)
		s.lastAttempt = time.Time{}
		s.startAt = time.Now()
		s.startDelay = 0
		return false
	}
	wasConnected := s.connected
	s.conn = conn
	s.connected = true
	s.startDelay = 0
	s.reconnectAttempts = 0
	s.lastPing = time.Time{}
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
	console.Debugf("devwatch connected (%s)", s.url)
	if !wasConnected {
		console.Debugf("Devwatch stream reconnected to %s", s.url)
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
	s.lastAttempt = time.Time{}
	s.startDelay = 0
	s.lastPing = time.Time{}
}

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
	console.Warnf("devconsole watcher stream unavailable: %v", err)
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
		if fmt.Sprint(payload["action"]) != "restart" {
			continue
		}
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
	}
}

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
	parsed.Path = "/__devconsole/api/agents"
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
	out      io.Writer
	stream   string
	watcher  string
	buf      bytes.Buffer
	streamer *devwatchStreamer
	mu       sync.Mutex
}

// newDevwatchWriter creates a writer that mirrors output to the devwatch websocket while still writing to the original writer.
func newDevwatchWriter(out io.Writer, streamer *devwatchStreamer, stream string, watcher string) io.Writer {
	if out == nil || streamer == nil {
		return out
	}
	return &devwatchWriter{out: out, streamer: streamer, stream: stream, watcher: watcher}
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
	for {
		data := w.buf.Bytes()
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			break
		}
		line := data[:idx]
		w.buf.Next(idx + 1)
		timestamp := time.Now()
		w.streamer.Send(devwatchLine{
			Line:      string(bytes.TrimSuffix(line, []byte{'\r'})),
			Stream:    w.stream,
			Timestamp: timestamp,
			ID:        timestamp.UnixMilli(),
			Watcher:   w.watcher,
		})
		if _, err := w.out.Write(line); err != nil {
			return 0, err
		}
		if _, err := w.out.Write([]byte{'\n'}); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

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
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		name, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(name) != key {
			continue
		}
		return strings.Trim(strings.TrimSpace(value), `"`), true
	}
	return "", false
}
