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
	ch          chan devwatchLine
	done        chan struct{}
	once        sync.Once
	lastWarn    time.Time
	lastAttempt time.Time
	pending     []devwatchLine
	startAt     time.Time
	startDelay  time.Duration
	restartCh   chan struct{}
}

// newDevwatchStreamerFromEnv creates a streamer when devconsole env is configured.
func newDevwatchStreamerFromEnv() *devwatchStreamer {
	token := strings.TrimSpace(getEnv("DEVCONSOLE_TOKEN"))
	if token == "" {
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
	return streamer
}

// Close stops the streamer and closes the websocket.
func (s *devwatchStreamer) Close() {
	if s == nil {
		return
	}
	s.once.Do(func() {
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

func (s *devwatchStreamer) run() {
	for {
		select {
		case line := <-s.ch:
			if !s.ensureConn() {
				s.buffer(line)
				continue
			}
			if err := s.conn.WriteJSON(map[string]any{
				"type": "devwatch",
				"payload": map[string]any{
					"line": line,
				},
			}); err != nil {
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

func (s *devwatchStreamer) ensureConn() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil {
		return true
	}
	if time.Since(s.startAt) < s.startDelay {
		return false
	}
	if time.Since(s.lastAttempt) < time.Second {
		return false
	}
	s.lastAttempt = time.Now()
	conn, _, err := websocket.DefaultDialer.Dial(s.url, s.header)
	if err != nil {
		s.maybeWarn(err)
		return false
	}
	s.conn = conn
	return true
}

func (s *devwatchStreamer) closeConn() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return
	}
	_ = s.conn.Close()
	s.conn = nil
}

func (s *devwatchStreamer) maybeWarn(err error) {
	if time.Since(s.lastWarn) < 10*time.Second {
		return
	}
	s.lastWarn = time.Now()
	console.Warnf("devconsole watcher stream unavailable: %v", err)
}

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
		var msg map[string]any
		if err := s.conn.ReadJSON(&msg); err != nil {
			s.closeConn()
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
	}
}

func (s *devwatchStreamer) buffer(line devwatchLine) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = append(s.pending, line)
	if len(s.pending) > 5000 {
		s.pending = s.pending[len(s.pending)-5000:]
	}
}

func (s *devwatchStreamer) flushPending() {
	s.mu.Lock()
	pending := append([]devwatchLine{}, s.pending...)
	s.pending = nil
	conn := s.conn
	s.mu.Unlock()
	if conn == nil || len(pending) == 0 {
		return
	}
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

// devwatchWriter tees output to the console and devwatch streamer.
type devwatchWriter struct {
	out      io.Writer
	stream   string
	watcher  string
	buf      bytes.Buffer
	streamer *devwatchStreamer
}

// newDevwatchWriter creates a writer that mirrors output to devwatch.
func newDevwatchWriter(out io.Writer, streamer *devwatchStreamer, stream string, watcher string) io.Writer {
	if out == nil || streamer == nil {
		return out
	}
	return &devwatchWriter{out: out, streamer: streamer, stream: stream, watcher: watcher}
}

// Write streams full lines to the devwatch websocket.
func (w *devwatchWriter) Write(p []byte) (int, error) {
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

// getEnv returns an environment value by key.
func getEnv(key string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	if value, ok := readDotEnvValue(key); ok {
		return value
	}
	return ""
}

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
