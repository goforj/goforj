//go:build integration

package forj

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
	"github.com/gorilla/websocket"
)

type testAgentInfo struct {
	Source string `json:"source"`
}

type procHandle struct {
	name   string
	cmd    *exec.Cmd
	cancel context.CancelFunc
	done   chan error
	stdout bytes.Buffer
	stderr bytes.Buffer
}

var (
	sharedAppOnce    sync.Once
	sharedAppErr     error
	sharedProjectDir string
	sharedBinPath    string
	sharedCleanup    func()
)

func TestMain(m *testing.M) {
	code := m.Run()
	if sharedCleanup != nil {
		sharedCleanup()
	}
	os.Exit(code)
}

func (p *procHandle) Output() string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("stdout:\n%s\nstderr:\n%s", p.stdout.String(), p.stderr.String())
}

func (p *procHandle) Start() error {
	if p == nil || p.cmd == nil {
		return fmt.Errorf("invalid process")
	}
	if err := p.cmd.Start(); err != nil {
		return err
	}
	p.done = make(chan error, 1)
	go func() {
		p.done <- p.cmd.Wait()
	}()
	return nil
}

func (p *procHandle) Stop() {
	if p == nil {
		return
	}
	if p.cancel != nil {
		p.cancel()
	}
	if p.done == nil {
		return
	}
	select {
	case <-p.done:
		return
	case <-time.After(300 * time.Millisecond):
	}
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	select {
	case <-p.done:
	case <-time.After(300 * time.Millisecond):
	}
}

func (p *procHandle) ExitError() error {
	if p == nil || p.done == nil {
		return nil
	}
	select {
	case err := <-p.done:
		if err != nil {
			return fmt.Errorf("%s exited: %v\nstdout:\n%s\nstderr:\n%s", p.name, err, p.stdout.String(), p.stderr.String())
		}
		return fmt.Errorf("%s exited unexpectedly\nstdout:\n%s\nstderr:\n%s", p.name, p.stdout.String(), p.stderr.String())
	default:
		return nil
	}
}

func configureWebsocketDialer(t *testing.T) {
	t.Helper()
	origHandshake := websocket.DefaultDialer.HandshakeTimeout
	origNetDial := websocket.DefaultDialer.NetDialContext
	websocket.DefaultDialer.HandshakeTimeout = 200 * time.Millisecond
	websocket.DefaultDialer.NetDialContext = (&net.Dialer{
		Timeout:   200 * time.Millisecond,
		KeepAlive: 200 * time.Millisecond,
	}).DialContext
	t.Cleanup(func() {
		websocket.DefaultDialer.HandshakeTimeout = origHandshake
		websocket.DefaultDialer.NetDialContext = origNetDial
	})
}

func stopProcAsync(t *testing.T, label string, proc *procHandle, timeout time.Duration) {
	t.Helper()
	if proc == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		proc.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Logf("stop timeout for %s", label)
	}
}

func startAppServer(t *testing.T, projectDir, binPath, port, token string) (*procHandle, string) {
	t.Helper()

	if _, err := os.Stat(binPath); err != nil {
		t.Fatalf("missing app binary: %v", err)
	}
	writeLighthouseEnv(t, projectDir, token, port)

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binPath, "http:serve", "--port", port)
	cmd.Dir = projectDir
	cmd.Env = append(os.Environ(),
		"LIGHTHOUSE_ENABLED=true",
		"LIGHTHOUSE_TOKEN="+token,
		"LIGHTHOUSE_URL=ws://127.0.0.1:"+port+"/lighthouse/ws/agent",
		"LIGHTHOUSE_AGENT_RETRY_MS=100",
	)
	handle := &procHandle{
		name:   "api",
		cmd:    cmd,
		cancel: cancel,
	}
	cmd.Stdout = &handle.stdout
	cmd.Stderr = &handle.stderr
	if err := handle.Start(); err != nil {
		cancel()
		t.Fatalf("failed to start api server: %v", err)
	}

	baseURL := "http://127.0.0.1:" + port
	return handle, baseURL
}

func buildAgentEnv(baseURL, token string) []string {
	agentURL := "ws://" + strings.TrimPrefix(baseURL, "http://") + "/lighthouse/ws/agent"
	return append(os.Environ(),
		"LIGHTHOUSE_ENABLED=true",
		"LIGHTHOUSE_TOKEN="+token,
		"LIGHTHOUSE_URL="+agentURL,
		"LIGHTHOUSE_AGENT_RETRY_MS=50",
	)
}

func startProcess(t *testing.T, name, projectDir, binPath string, env []string, args ...string) *procHandle {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.Dir = projectDir
	cmd.Env = env
	handle := &procHandle{
		name:   name,
		cmd:    cmd,
		cancel: cancel,
	}
	cmd.Stdout = &handle.stdout
	cmd.Stderr = &handle.stderr
	if err := handle.Start(); err != nil {
		cancel()
		t.Fatalf("failed to start %s: %v", name, err)
	}
	return handle
}

func waitForTCP(t *testing.T, addr string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func sendConsoleCommand(t *testing.T, conn *websocket.Conn, target, name string, params map[string]any, timeout time.Duration) (map[string]any, error) {
	t.Helper()
	id := fmt.Sprintf("cmd-%d", time.Now().UnixNano())
	payload, _ := json.Marshal(map[string]any{
		"name":   name,
		"params": params,
	})
	if err := conn.WriteJSON(map[string]any{
		"type":    "command",
		"id":      id,
		"target":  target,
		"payload": json.RawMessage(payload),
	}); err != nil {
		return nil, fmt.Errorf("send command: %w", err)
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn.SetReadDeadline(deadline)
		var msg struct {
			Type    string          `json:"type"`
			ReplyTo string          `json:"reply_to"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := conn.ReadJSON(&msg); err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		if msg.Type != "response" || msg.ReplyTo != id {
			continue
		}
		var resp struct {
			Ok   bool                   `json:"ok"`
			Data map[string]interface{} `json:"data"`
		}
		if err := json.Unmarshal(msg.Payload, &resp); err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}
		if !resp.Ok {
			return nil, fmt.Errorf("response not ok: %s", string(msg.Payload))
		}
		return resp.Data, nil
	}
	return nil, fmt.Errorf("response timeout for %s", name)
}

type testLighthouseServer struct {
	token  string
	ln     net.Listener
	server *http.Server

	mu               sync.Mutex
	agents           map[string]testAgentInfo
	conns            map[*websocket.Conn]struct{}
	agentConnSources map[*websocket.Conn]string
}

func newTestLighthouseServer(t *testing.T, addr, token string) *testLighthouseServer {
	t.Helper()

	ln, err := listenWithRetry(addr, 20, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	s := &testLighthouseServer{
		token:            token,
		ln:               ln,
		agents:           map[string]testAgentInfo{},
		conns:            map[*websocket.Conn]struct{}{},
		agentConnSources: map[*websocket.Conn]string{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/lighthouse/api/agents", s.handleAgents)
	mux.HandleFunc("/lighthouse/ws/devwatch", s.handleDevwatch)
	mux.HandleFunc("/lighthouse/ws/agent", s.handleAgent)

	s.server = &http.Server{Handler: mux}
	go func() {
		_ = s.server.Serve(ln)
	}()

	return s
}

func listenWithRetry(addr string, attempts int, delay time.Duration) (net.Listener, error) {
	var lastErr error
	for i := 0; i < attempts; i++ {
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			return ln, nil
		}
		lastErr = err
		if !isAddrInUse(err) {
			return nil, err
		}
		time.Sleep(delay)
	}
	return nil, lastErr
}

func isAddrInUse(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "address already in use")
}

func findFreeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func dialWS(t *testing.T, baseURL, path, token string) *websocket.Conn {
	t.Helper()
	wsURL := "ws://" + strings.TrimPrefix(baseURL, "http://") + path
	header := http.Header{}
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("dial websocket %s: %v", wsURL, err)
	}
	return conn
}

func waitForAgentMissing(ctx context.Context, baseURL, token, source string, timeout time.Duration) error {
	client := &http.Client{Timeout: 200 * time.Millisecond}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		req, err := http.NewRequest(http.MethodGet, baseURL+"/lighthouse/api/agents", nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := client.Do(req)
		if err == nil && resp.Body != nil {
			var agents []testAgentInfo
			_ = json.NewDecoder(resp.Body).Decode(&agents)
			_ = resp.Body.Close()
			found := false
			for _, agent := range agents {
				if agent.Source == source {
					found = true
					break
				}
			}
			if !found {
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("agent %s still present after %s", source, timeout)
}

func (s *testLighthouseServer) Close() {
	if s == nil || s.server == nil {
		return
	}
	s.mu.Lock()
	for conn := range s.conns {
		_ = conn.Close()
	}
	s.conns = map[*websocket.Conn]struct{}{}
	s.agents = map[string]testAgentInfo{}
	s.agentConnSources = map[*websocket.Conn]string{}
	s.mu.Unlock()
	_ = s.server.Close()
}

func (s *testLighthouseServer) Addr() string {
	return s.ln.Addr().String()
}

func (s *testLighthouseServer) handleAgents(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+s.token {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	s.mu.Lock()
	list := make([]testAgentInfo, 0, len(s.agents))
	for _, agent := range s.agents {
		list = append(list, agent)
	}
	s.mu.Unlock()
	_ = json.NewEncoder(w).Encode(list)
}

func (s *testLighthouseServer) handleDevwatch(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+s.token {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	upgrader := websocket.Upgrader{}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	isSource := r.URL.Query().Get("role") == "source"
	if isSource {
		s.mu.Lock()
		s.agents["dev"] = testAgentInfo{Source: "dev"}
		s.mu.Unlock()
	}
	s.mu.Lock()
	s.conns[conn] = struct{}{}
	s.mu.Unlock()

	go func() {
		defer func() {
			if isSource {
				s.mu.Lock()
				delete(s.agents, "dev")
				s.mu.Unlock()
			}
			s.mu.Lock()
			delete(s.conns, conn)
			s.mu.Unlock()
			_ = conn.Close()
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
}

func (s *testLighthouseServer) handleAgent(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+s.token {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	upgrader := websocket.Upgrader{}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	s.mu.Lock()
	s.conns[conn] = struct{}{}
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			if source, ok := s.agentConnSources[conn]; ok {
				delete(s.agents, source)
				delete(s.agentConnSources, conn)
			}
			delete(s.conns, conn)
			s.mu.Unlock()
			_ = conn.Close()
		}()
		for {
			var msg struct {
				Type    string          `json:"type"`
				Source  string          `json:"source"`
				Payload json.RawMessage `json:"payload"`
			}
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			if msg.Type != "register" {
				continue
			}
			if msg.Source == "" {
				continue
			}
			s.mu.Lock()
			s.agents[msg.Source] = testAgentInfo{Source: msg.Source}
			s.agentConnSources[conn] = msg.Source
			s.mu.Unlock()
		}
	}()
}

func waitForDevAgent(ctx context.Context, baseURL, token string, timeout time.Duration) error {
	client := &http.Client{Timeout: 200 * time.Millisecond}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		req, err := http.NewRequest(http.MethodGet, baseURL+"/lighthouse/api/agents", nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := client.Do(req)
		if err == nil && resp.Body != nil {
			var agents []testAgentInfo
			_ = json.NewDecoder(resp.Body).Decode(&agents)
			_ = resp.Body.Close()
			for _, agent := range agents {
				if agent.Source == "dev" {
					return nil
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("dev agent not observed within %s", timeout)
}

func waitForServerReady(ctx context.Context, baseURL, token string, timeout time.Duration) error {
	client := &http.Client{Timeout: 200 * time.Millisecond}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		req, err := http.NewRequest(http.MethodGet, baseURL+"/lighthouse/api/agents", nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := client.Do(req)
		if err == nil && resp != nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("server not ready within %s", timeout)
}

func waitForAgents(ctx context.Context, baseURL, token string, sources []string, timeout time.Duration) error {
	if len(sources) == 0 {
		return nil
	}
	client := &http.Client{Timeout: 200 * time.Millisecond}
	deadline := time.Now().Add(timeout)
	required := map[string]struct{}{}
	for _, source := range sources {
		required[source] = struct{}{}
	}
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		req, err := http.NewRequest(http.MethodGet, baseURL+"/lighthouse/api/agents", nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := client.Do(req)
		if err == nil && resp.Body != nil {
			var agents []testAgentInfo
			_ = json.NewDecoder(resp.Body).Decode(&agents)
			_ = resp.Body.Close()
			seen := map[string]struct{}{}
			for _, agent := range agents {
				seen[agent.Source] = struct{}{}
			}
			missing := false
			for source := range required {
				if _, ok := seen[source]; !ok {
					missing = true
					break
				}
			}
			if !missing {
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("agents not observed within %s: %v", timeout, sources)
}

func getSharedApp(t *testing.T) (string, string) {
	t.Helper()
	sharedAppOnce.Do(func() {
		projectDir, err := os.MkdirTemp("", "forj-lighthouse-app-*")
		if err != nil {
			sharedAppErr = fmt.Errorf("create temp app dir: %w", err)
			return
		}
		renderAppAtDir(t, projectDir)
		binDir, err := os.MkdirTemp("", "forj-lighthouse-bin-*")
		if err != nil {
			sharedAppErr = fmt.Errorf("create temp bin dir: %w", err)
			_ = os.RemoveAll(projectDir)
			return
		}
		binPath := filepath.Join(binDir, "app")
		cmd := exec.Command("go", "build", "-o", binPath, ".")
		cmd.Dir = projectDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			sharedAppErr = fmt.Errorf("failed to build temp binary: %w", err)
			_ = os.RemoveAll(projectDir)
			_ = os.RemoveAll(binDir)
			return
		}
		sharedProjectDir = projectDir
		sharedBinPath = binPath
		sharedCleanup = func() {
			_ = os.RemoveAll(projectDir)
			_ = os.RemoveAll(binDir)
		}
	})
	if sharedAppErr != nil {
		t.Fatalf("shared app setup failed: %v", sharedAppErr)
	}
	return sharedProjectDir, sharedBinPath
}

func verifyBinaryHasCommand(t *testing.T, binPath, command string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binPath, "--help")
	out, err := cmd.CombinedOutput()
	if err != nil && ctx.Err() != nil {
		t.Fatalf("timed out running %s --help", binPath)
	}
	if !strings.Contains(string(out), command) {
		t.Fatalf("binary %s does not expose %q command; output:\n%s", binPath, command, string(out))
	}
}

func writeLighthouseEnv(t *testing.T, projectDir, token, port string) {
	t.Helper()
	content := fmt.Sprintf(`APP_ENV=local
APP_NAME=TestApp
LIGHTHOUSE_ENABLED=true
LIGHTHOUSE_TOKEN=%s
LIGHTHOUSE_URL=ws://127.0.0.1:%s/lighthouse/ws/agent
`, token, port)
	if err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte(content), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
}

func writeProjectConfig(t *testing.T, dir string) {
	t.Helper()
	writeProjectConfigFile(t, dir, project.Config{
		ProjectName:  "TestApp",
		GoModuleName: "example.com/testapp",
		UpdatedAt:    "2026-01-01 00:00:00 UTC",
		Render: project.RenderConfig{
			QueueDriver: "redis",
			Components: project.Components{
				WebAPI:    true,
				WebUI:     true,
				Scheduler: true,
				Jobs:      true,
			},
		},
	})
}

func renderAppAtDir(t *testing.T, dir string) {
	t.Helper()
	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(orig) }()
	writeProjectConfig(t, dir)
	renderer := NewProjectRenderer(logger.NewSilentLogger())
	if err := renderer.Render(ComponentRenderInput{renderAll: true}); err != nil {
		t.Fatalf("render temp app: %v", err)
	}
}

func startRealProcesses(t *testing.T, baseURL, token, projectDir, binPath string, components project.Components) ([]*procHandle, []string) {
	t.Helper()

	env := buildAgentEnv(baseURL, token)

	var procs []*procHandle
	var sources []string

	startProc := func(name string, args ...string) {
		handle := startProcess(t, name, projectDir, binPath, env, args...)
		procs = append(procs, handle)
		sources = append(sources, strings.ToLower(name))
	}

	if components.Scheduler {
		startProc("scheduler", "schedule:run")
	}
	if components.Jobs {
		startProc("jobs", "queue:work")
	}

	return procs, sources
}

func forceReconnect(streamer *devwatchStreamer, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		streamer.mu.Lock()
		streamer.lastAttempt = time.Time{}
		streamer.mu.Unlock()
		resultCh := make(chan bool, 1)
		go func() {
			resultCh <- streamer.ensureConn()
		}()
		select {
		case ok := <-resultCh:
			if ok {
				return nil
			}
		case <-time.After(100 * time.Millisecond):
			return fmt.Errorf("ensureConn timed out")
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("force reconnect timed out after %s", timeout)
}

func streamerConnected(streamer *devwatchStreamer) bool {
	streamer.mu.Lock()
	defer streamer.mu.Unlock()
	return streamer.conn != nil
}

func TestLighthouseReconnectIntegration(t *testing.T) {
	configureWebsocketDialer(t)

	token := "test-token"
	envs := map[string]string{
		"LIGHTHOUSE_ENABLED": "true",
		"LIGHTHOUSE_TOKEN":   token,
	}
	for key, value := range envs {
		t.Setenv(key, value)
	}

	addr := os.Getenv("LIGHTHOUSE_TEST_ADDR")
	if addr == "" {
		addr = findFreeAddr(t)
	}
	projectDir, binPath := getSharedApp(t)
	realMode := true

	components := project.Components{
		WebAPI:    true,
		WebUI:     true,
		Scheduler: true,
		Jobs:      true,
	}

	t.Log("using shared temp binary")
	verifyBinaryHasCommand(t, binPath, "http:serve")

	ctxTimeout := 1500 * time.Millisecond
	serverReadyWait := 800 * time.Millisecond
	initialWait := 800 * time.Millisecond
	reconnectWait := 800 * time.Millisecond
	if realMode {
		ctxTimeout = 4 * time.Second
		serverReadyWait = 1 * time.Second
		initialWait = 1 * time.Second
		reconnectWait = 1 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), ctxTimeout)
	defer cancel()

	var serverProc *procHandle
	var baseURL string
	var controlPort string

	if realMode {
		_, port, _ := net.SplitHostPort(addr)
		controlPort = port
		serverProc, baseURL = startAppServer(t, projectDir, binPath, controlPort, token)
		defer stopProcAsync(t, "server", serverProc, 1*time.Second)
	} else {
		t.Fatalf("realMode is required; refusing to use fallback server")
	}

	wsURL := url.URL{Scheme: "ws", Host: "127.0.0.1:" + controlPort, Path: "/lighthouse/ws/devwatch"}
	t.Setenv("LIGHTHOUSE_URL", wsURL.String())

	streamer := newDevwatchStreamerFromEnv()
	if streamer == nil {
		t.Fatal("expected devwatch streamer to be initialized")
	}
	streamer.mu.Lock()
	streamer.startDelay = 0
	streamer.startAt = time.Now().Add(-time.Second)
	streamer.waitForServer = false
	streamer.lastAttempt = time.Time{}
	streamer.mu.Unlock()
	defer streamer.Close()
	t.Logf("assertion: control plane responds to /lighthouse/api/agents")
	if err := waitForServerReady(ctx, baseURL, token, serverReadyWait); err != nil {
		if exitErr := serverProc.ExitError(); exitErr != nil {
			t.Fatalf("control plane failed to start: %v", exitErr)
		}
		t.Fatalf("control plane not ready: %v\n%s", err, serverProc.Output())
	}
	t.Log("assertion: devwatch source registers dev agent after startup")
	t.Logf("waiting for initial dev agent at %s", baseURL)
	if err := waitForDevAgent(ctx, baseURL, token, initialWait); err != nil {
		if exitErr := serverProc.ExitError(); exitErr != nil {
			t.Fatalf("control plane exited while waiting for dev agent: %v", exitErr)
		}
		t.Fatalf("initial dev agent missing: %v\n%s", err, serverProc.Output())
	}

	var processes []*procHandle
	var expectedSources []string
	if realMode {
		processes, expectedSources = startRealProcesses(t, baseURL, token, projectDir, binPath, components)
		if components.WebAPI || components.WebUI {
			expectedSources = append(expectedSources, "api")
		}
	}
	for _, proc := range processes {
		defer stopProcAsync(t, proc.name, proc, 1*time.Second)
	}
	if len(expectedSources) > 0 {
		t.Logf("assertion: agents register: %v", expectedSources)
		if err := waitForAgents(context.Background(), baseURL, token, expectedSources, 3*time.Second); err != nil {
			for _, proc := range processes {
				if exitErr := proc.ExitError(); exitErr != nil {
					t.Fatalf("agents did not register: %v", exitErr)
				}
			}
			t.Fatalf("agents did not register: %v", err)
		}
	}

	if realMode {
		stopProcAsync(t, "server", serverProc, 1*time.Second)
		for _, proc := range processes {
			stopProcAsync(t, proc.name, proc, 1*time.Second)
		}
	} else {
		t.Fatalf("realMode is required; refusing to use fallback server")
	}

	time.Sleep(200 * time.Millisecond)

	if realMode {
		serverProc, baseURL = startAppServer(t, projectDir, binPath, controlPort, token)
		defer stopProcAsync(t, "server", serverProc, 1*time.Second)
		processes, expectedSources = startRealProcesses(t, baseURL, token, projectDir, binPath, components)
		if components.WebAPI || components.WebUI {
			expectedSources = append(expectedSources, "api")
		}
		for _, proc := range processes {
			defer stopProcAsync(t, proc.name, proc, 1*time.Second)
		}
	} else {
		t.Fatalf("realMode is required; refusing to use fallback server")
	}

	t.Log("restarting lighthouse server")
	t.Logf("streamer connected before reconnect: %v", streamerConnected(streamer))
	if err := forceReconnect(streamer, 2*time.Second); err != nil {
		t.Fatalf("force reconnect failed: %v", err)
	}
	t.Logf("streamer connected after reconnect: %v", streamerConnected(streamer))
	t.Log("assertion: dev agent reconnects after server restart")
	ready := make(chan error, 1)
	go func() {
		ready <- waitForDevAgent(ctx, baseURL, token, reconnectWait)
	}()
	select {
	case <-ctx.Done():
		t.Fatal("dev agent did not reconnect after server restart")
	case err := <-ready:
		if err != nil {
			t.Fatalf("dev agent did not reconnect after server restart: %v", err)
		}
	}

	if len(expectedSources) > 0 {
		t.Logf("assertion: agents reconnect after server restart: %v", expectedSources)
		if err := waitForAgents(context.Background(), baseURL, token, expectedSources, 3*time.Second); err != nil {
			for _, proc := range processes {
				if exitErr := proc.ExitError(); exitErr != nil {
					t.Fatalf("agents did not reconnect after server restart: %v", exitErr)
				}
			}
			t.Fatalf("agents did not reconnect after server restart: %v", err)
		}
	}
}

func TestLighthouseAuthBootIntegration(t *testing.T) {
	configureWebsocketDialer(t)

	token := "auth-token"
	t.Setenv("LIGHTHOUSE_ENABLED", "true")
	t.Setenv("LIGHTHOUSE_TOKEN", token)

	projectDir, binPath := getSharedApp(t)

	addr := findFreeAddr(t)
	_, port, _ := net.SplitHostPort(addr)
	serverProc, baseURL := startAppServer(t, projectDir, binPath, port, token)
	defer stopProcAsync(t, "server", serverProc, 1*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	if err := waitForServerReady(ctx, baseURL, token, 2*time.Second); err != nil {
		t.Fatalf("control plane not ready: %v", err)
	}

	t.Log("assertion: control plane responds with 401 for invalid token")
	req, err := http.NewRequest(http.MethodGet, baseURL+"/lighthouse/api/agents", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer bad-token")
	resp, err := (&http.Client{Timeout: 200 * time.Millisecond}).Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}

	t.Log("assertion: devwatch source registers dev agent after startup")
	wsURL := url.URL{Scheme: "ws", Host: "127.0.0.1:" + port, Path: "/lighthouse/ws/devwatch"}
	t.Setenv("LIGHTHOUSE_URL", wsURL.String())
	streamer := newDevwatchStreamerFromEnv()
	if streamer == nil {
		t.Fatal("expected devwatch streamer to be initialized")
	}
	streamer.mu.Lock()
	streamer.startDelay = 0
	streamer.startAt = time.Now().Add(-time.Second)
	streamer.waitForServer = false
	streamer.lastAttempt = time.Time{}
	streamer.mu.Unlock()
	defer streamer.Close()

	if err := forceReconnect(streamer, 300*time.Millisecond); err != nil {
		t.Fatalf("devwatch connect failed: %v", err)
	}
	if err := waitForDevAgent(ctx, baseURL, token, 1*time.Second); err != nil {
		t.Fatalf("dev agent missing: %v", err)
	}

	t.Log("assertion: agents register")
	processes, expectedSources := startRealProcesses(t, baseURL, token, projectDir, binPath, project.Components{
		WebAPI:    true,
		WebUI:     true,
		Scheduler: true,
		Jobs:      true,
	})
	expectedSources = append(expectedSources, "api")
	for _, proc := range processes {
		defer stopProcAsync(t, proc.name, proc, 1*time.Second)
	}
	if err := waitForAgents(ctx, baseURL, token, expectedSources, 5*time.Second); err != nil {
		for _, proc := range processes {
			if exitErr := proc.ExitError(); exitErr != nil {
				t.Fatalf("agents did not register: %v", exitErr)
			}
		}
		t.Fatalf("agents did not register: %v", err)
	}
}

func TestLighthouseOutOfOrderIntegration(t *testing.T) {
	configureWebsocketDialer(t)

	token := "retry-token"
	t.Setenv("LIGHTHOUSE_ENABLED", "true")
	t.Setenv("LIGHTHOUSE_TOKEN", token)

	projectDir, binPath := getSharedApp(t)

	addr := findFreeAddr(t)
	_, port, _ := net.SplitHostPort(addr)
	baseURL := "http://127.0.0.1:" + port

	t.Log("assertion: devwatch reconnects when server comes up")
	wsURL := url.URL{Scheme: "ws", Host: "127.0.0.1:" + port, Path: "/lighthouse/ws/devwatch"}
	t.Setenv("LIGHTHOUSE_URL", wsURL.String())
	streamer := newDevwatchStreamerFromEnv()
	if streamer == nil {
		t.Fatal("expected devwatch streamer to be initialized")
	}
	streamer.mu.Lock()
	streamer.startDelay = 0
	streamer.startAt = time.Now().Add(-time.Second)
	streamer.waitForServer = false
	streamer.lastAttempt = time.Time{}
	streamer.mu.Unlock()
	defer streamer.Close()

	serverProc, _ := startAppServer(t, projectDir, binPath, port, token)
	defer stopProcAsync(t, "server", serverProc, 1*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	if err := waitForServerReady(ctx, baseURL, token, 2*time.Second); err != nil {
		t.Fatalf("control plane not ready: %v", err)
	}
	if err := waitForDevAgent(ctx, baseURL, token, 5*time.Second); err != nil {
		t.Fatalf("dev agent did not register after server start: %v", err)
	}
}

func TestLighthousePartialRestartIntegration(t *testing.T) {
	configureWebsocketDialer(t)

	token := "partial-token"
	t.Setenv("LIGHTHOUSE_ENABLED", "true")
	t.Setenv("LIGHTHOUSE_TOKEN", token)

	projectDir, binPath := getSharedApp(t)

	addr := findFreeAddr(t)
	_, port, _ := net.SplitHostPort(addr)
	serverProc, baseURL := startAppServer(t, projectDir, binPath, port, token)
	defer stopProcAsync(t, "server", serverProc, 1*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := waitForServerReady(ctx, baseURL, token, 2*time.Second); err != nil {
		t.Fatalf("control plane not ready: %v", err)
	}

	processes, expectedSources := startRealProcesses(t, baseURL, token, projectDir, binPath, project.Components{
		Scheduler: true,
		Jobs:      true,
	})
	for _, proc := range processes {
		defer stopProcAsync(t, proc.name, proc, 1*time.Second)
	}
	if err := waitForAgents(ctx, baseURL, token, expectedSources, 3*time.Second); err != nil {
		t.Fatalf("agents did not register: %v", err)
	}

	var schedulerProc *procHandle
	for _, proc := range processes {
		if proc.name == "scheduler" {
			schedulerProc = proc
			break
		}
	}
	if schedulerProc == nil {
		t.Fatal("scheduler process not started")
	}
	stopProcAsync(t, "scheduler", schedulerProc, 1*time.Second)

	t.Log("assertion: scheduler agent disappears while others remain")
	if err := waitForAgentMissing(ctx, baseURL, token, "scheduler", 1*time.Second); err != nil {
		t.Fatalf("scheduler did not disappear: %v", err)
	}

	env := buildAgentEnv(baseURL, token)
	restarted := startProcess(t, "scheduler", projectDir, binPath, env, "schedule:run")
	defer stopProcAsync(t, "scheduler-restart", restarted, 1*time.Second)

	t.Log("assertion: scheduler agent reconnects")
	if err := waitForAgents(ctx, baseURL, token, []string{"scheduler"}, 3*time.Second); err != nil {
		t.Fatalf("scheduler did not reconnect: %v", err)
	}

	consoleConn := dialWS(t, baseURL, "/lighthouse/ws/console", token)
	defer consoleConn.Close()
	t.Log("assertion: schedule:list command works after restart")
	resp, err := sendConsoleCommand(t, consoleConn, "scheduler", "schedule:list", map[string]any{}, 1*time.Second)
	if err != nil {
		t.Fatalf("schedule:list command failed: %v", err)
	}
	if _, ok := resp["schedules"]; !ok {
		t.Fatalf("expected schedules payload, got %v", resp)
	}
}

func TestDevwatchStreamIntegration(t *testing.T) {
	configureWebsocketDialer(t)

	token := "devwatch-token"
	t.Setenv("LIGHTHOUSE_ENABLED", "true")
	t.Setenv("LIGHTHOUSE_TOKEN", token)

	projectDir, binPath := getSharedApp(t)

	addr := findFreeAddr(t)
	_, port, _ := net.SplitHostPort(addr)
	serverProc, baseURL := startAppServer(t, projectDir, binPath, port, token)
	defer stopProcAsync(t, "server", serverProc, 1*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := waitForServerReady(ctx, baseURL, token, 1*time.Second); err != nil {
		t.Fatalf("control plane not ready: %v", err)
	}

	wsURL := url.URL{Scheme: "ws", Host: "127.0.0.1:" + port, Path: "/lighthouse/ws/devwatch"}
	t.Setenv("LIGHTHOUSE_URL", wsURL.String())
	streamer := newDevwatchStreamerFromEnv()
	if streamer == nil {
		t.Fatal("expected devwatch streamer to be initialized")
	}
	streamer.mu.Lock()
	streamer.startDelay = 0
	streamer.startAt = time.Now().Add(-time.Second)
	streamer.waitForServer = false
	streamer.lastAttempt = time.Time{}
	streamer.mu.Unlock()
	defer streamer.Close()

	if err := forceReconnect(streamer, 300*time.Millisecond); err != nil {
		t.Fatalf("devwatch connect failed: %v", err)
	}

	consoleConn := dialWS(t, baseURL, "/lighthouse/ws/devwatch", token)
	defer consoleConn.Close()

	writer := newDevwatchWriter(io.Discard, streamer, "stdout", "API", "go test ./...")
	if _, err := writer.Write([]byte("Test > API > Hello\n")); err != nil {
		t.Fatalf("write devwatch line: %v", err)
	}

	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		consoleConn.SetReadDeadline(deadline)
		var msg struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := consoleConn.ReadJSON(&msg); err != nil {
			t.Fatalf("read devwatch message: %v", err)
		}
		if msg.Type != "devwatch" {
			continue
		}
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			t.Fatalf("decode devwatch payload: %v", err)
		}
		linePayload, ok := payload["line"]
		if !ok {
			continue
		}
		var line devwatchLine
		if err := json.Unmarshal(linePayload, &line); err != nil {
			t.Fatalf("decode devwatch line: %v", err)
		}
		if !strings.Contains(line.Line, "Test") {
			continue
		}
		if line.Watcher != "API" {
			t.Fatalf("expected watcher API, got %q", line.Watcher)
		}
		return
	}
	t.Fatal("devwatch message not received")
}

func TestLighthouseCommandRoutingIntegration(t *testing.T) {
	configureWebsocketDialer(t)

	token := "command-token"
	t.Setenv("LIGHTHOUSE_ENABLED", "true")
	t.Setenv("LIGHTHOUSE_TOKEN", token)

	projectDir, binPath := getSharedApp(t)

	addr := findFreeAddr(t)
	_, port, _ := net.SplitHostPort(addr)
	serverProc, baseURL := startAppServer(t, projectDir, binPath, port, token)
	defer stopProcAsync(t, "server", serverProc, 1*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := waitForServerReady(ctx, baseURL, token, 1*time.Second); err != nil {
		t.Fatalf("control plane not ready: %v", err)
	}

	agentConn := dialWS(t, baseURL, "/lighthouse/ws/agent", token)
	defer agentConn.Close()
	registerPayload, _ := json.Marshal(map[string]any{
		"id":            "agent-1",
		"source":        "api",
		"app":           "TestApp",
		"version":       "test",
		"env":           "local",
		"capabilities":  []string{"routes"},
		"instance_id":   "instance-1",
		"instance_kind": "test",
		"host":          "localhost",
		"started_at":    time.Now(),
	})
	if err := agentConn.WriteJSON(map[string]any{
		"type":    "register",
		"id":      "reg-1",
		"source":  "api",
		"payload": json.RawMessage(registerPayload),
	}); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	if err := waitForAgents(ctx, baseURL, token, []string{"api"}, 1*time.Second); err != nil {
		t.Fatalf("agent did not register: %v", err)
	}

	consoleConn := dialWS(t, baseURL, "/lighthouse/ws/console", token)
	defer consoleConn.Close()

	commandPayload, _ := json.Marshal(map[string]any{
		"name":   "routes:list",
		"params": map[string]any{},
	})
	if err := consoleConn.WriteJSON(map[string]any{
		"type":    "command",
		"id":      "cmd-1",
		"target":  "api",
		"payload": json.RawMessage(commandPayload),
	}); err != nil {
		t.Fatalf("send command: %v", err)
	}

	readDeadline := time.Now().Add(1 * time.Second)
	var gotCommand bool
	for time.Now().Before(readDeadline) {
		agentConn.SetReadDeadline(readDeadline)
		var msg struct {
			Type    string          `json:"type"`
			ID      string          `json:"id"`
			Target  string          `json:"target"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := agentConn.ReadJSON(&msg); err != nil {
			t.Fatalf("agent read: %v", err)
		}
		if msg.Type != "command" || msg.Target != "api" {
			continue
		}
		gotCommand = true
		responsePayload, _ := json.Marshal(map[string]any{
			"ok":   true,
			"data": map[string]any{"value": "ok"},
		})
		if err := agentConn.WriteJSON(map[string]any{
			"type":     "response",
			"id":       "resp-1",
			"reply_to": msg.ID,
			"source":   "api",
			"payload":  json.RawMessage(responsePayload),
		}); err != nil {
			t.Fatalf("agent response: %v", err)
		}
		break
	}
	if !gotCommand {
		t.Fatal("agent did not receive command")
	}

	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		consoleConn.SetReadDeadline(deadline)
		var msg struct {
			Type    string          `json:"type"`
			ReplyTo string          `json:"reply_to"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := consoleConn.ReadJSON(&msg); err != nil {
			t.Fatalf("console read: %v", err)
		}
		if msg.Type != "response" || msg.ReplyTo != "cmd-1" {
			continue
		}
		var resp struct {
			Ok   bool            `json:"ok"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(msg.Payload, &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if !resp.Ok {
			t.Fatalf("response not ok: %s", string(msg.Payload))
		}
		return
	}
	t.Fatal("console did not receive response")
}

func TestLighthouseJobsQueueHealthIntegration(t *testing.T) {
	configureWebsocketDialer(t)

	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = "127.0.0.1"
	}
	redisPort := os.Getenv("REDIS_PORT")
	if redisPort == "" {
		redisPort = "6379"
	}
	redisAddr := net.JoinHostPort(redisHost, redisPort)
	if !waitForTCP(t, redisAddr, 1*time.Second) {
		t.Skipf("redis not reachable at %s", redisAddr)
	}

	token := "jobs-token"
	t.Setenv("LIGHTHOUSE_ENABLED", "true")
	t.Setenv("LIGHTHOUSE_TOKEN", token)
	t.Setenv("REDIS_HOST", redisHost)
	t.Setenv("REDIS_PORT", redisPort)

	projectDir, binPath := getSharedApp(t)
	addr := findFreeAddr(t)
	_, port, _ := net.SplitHostPort(addr)
	serverProc, baseURL := startAppServer(t, projectDir, binPath, port, token)
	defer stopProcAsync(t, "server", serverProc, 1*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if err := waitForServerReady(ctx, baseURL, token, 1*time.Second); err != nil {
		t.Fatalf("control plane not ready: %v", err)
	}

	processes, _ := startRealProcesses(t, baseURL, token, projectDir, binPath, project.Components{Jobs: true})
	for _, proc := range processes {
		defer stopProcAsync(t, proc.name, proc, 1*time.Second)
	}
	if err := waitForAgents(ctx, baseURL, token, []string{"jobs"}, 2*time.Second); err != nil {
		t.Fatalf("jobs agent did not register: %v", err)
	}

	consoleConn := dialWS(t, baseURL, "/lighthouse/ws/console", token)
	defer consoleConn.Close()

	if _, err := sendConsoleCommand(t, consoleConn, "jobs", "asynq:queue:pause", map[string]any{"queue": "default"}, 1*time.Second); err != nil {
		t.Fatalf("pause queue failed: %v", err)
	}
	if _, err := sendConsoleCommand(t, consoleConn, "jobs", "cli:run", map[string]any{"args": []string{"queue:hello-test"}}, 2*time.Second); err != nil {
		t.Fatalf("enqueue jobs failed: %v", err)
	}

	foundPending := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := sendConsoleCommand(t, consoleConn, "jobs", "asynq:queues", map[string]any{}, 1*time.Second)
		if err != nil {
			t.Fatalf("fetch queues failed: %v", err)
		}
		queues, ok := resp["queues"].([]interface{})
		if !ok {
			t.Fatalf("unexpected queues payload: %v", resp)
		}
		for _, entry := range queues {
			queue, ok := entry.(map[string]interface{})
			if !ok {
				continue
			}
			if queue["name"] != "default" {
				continue
			}
			if pending, ok := queue["pending"].(float64); ok && pending > 0 {
				foundPending = true
				break
			}
		}
		if foundPending {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !foundPending {
		t.Fatal("expected pending jobs after enqueue")
	}

	if _, err := sendConsoleCommand(t, consoleConn, "jobs", "asynq:queue:resume", map[string]any{"queue": "default"}, 1*time.Second); err != nil {
		t.Fatalf("resume queue failed: %v", err)
	}

	drained := false
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := sendConsoleCommand(t, consoleConn, "jobs", "asynq:queues", map[string]any{}, 1*time.Second)
		if err != nil {
			t.Fatalf("fetch queues failed: %v", err)
		}
		queues, ok := resp["queues"].([]interface{})
		if !ok {
			t.Fatalf("unexpected queues payload: %v", resp)
		}
		for _, entry := range queues {
			queue, ok := entry.(map[string]interface{})
			if !ok {
				continue
			}
			if queue["name"] != "default" {
				continue
			}
			if pending, ok := queue["pending"].(float64); ok && pending == 0 {
				drained = true
				break
			}
		}
		if drained {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if !drained {
		t.Fatal("expected queue to drain after resume")
	}
}
