//go:build integration

package forj

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/str/v2"
	"github.com/gorilla/websocket"
)

type renderedTraceSummary struct {
	TraceID string            `json:"trace_id"`
	Source  string            `json:"source"`
	Name    string            `json:"name"`
	Status  string            `json:"status"`
	Labels  map[string]string `json:"labels"`
}

type renderedAgentInfo struct {
	Source string `json:"source"`
}

type renderedTraceEvent struct {
	Kind       string                `json:"kind"`
	Name       string                `json:"name"`
	Message    string                `json:"message"`
	Status     string                `json:"status"`
	HTTP       *renderedHTTPExchange `json:"http"`
	Attributes map[string]any        `json:"attributes"`
}

type renderedHTTPExchange struct {
	Method         string `json:"method"`
	URI            string `json:"uri"`
	ResponseStatus int    `json:"response_status"`
	ResponseBody   string `json:"response_body"`
}

type renderedTraceRecord struct {
	Summary renderedTraceSummary `json:"summary"`
	Events  []renderedTraceEvent `json:"events"`
}

func TestRenderedLighthouseTraceEndpoints(t *testing.T) {
	projectDir := t.TempDir()
	renderMetricsTestApp(t, projectDir)

	binPath := buildRenderedDefaultApp(t, projectDir, nil, "build rendered app")

	httpAddr := findFreeAddr(t)
	_, httpPort, err := net.SplitHostPort(httpAddr)
	if err != nil {
		t.Fatalf("split http addr: %v", err)
	}
	metricsAddr := findFreeAddr(t)
	_, metricsPort, err := net.SplitHostPort(metricsAddr)
	if err != nil {
		t.Fatalf("split metrics addr: %v", err)
	}
	setRenderedEnvValue(t, projectDir, "APP_URL", "http://127.0.0.1:"+httpPort)
	setRenderedEnvValue(t, projectDir, "LIGHTHOUSE_URL", "ws://127.0.0.1:"+httpPort+"/lighthouse/ws/agent")
	setRenderedEnvValue(t, projectDir, "METRICS_API_PORT", metricsPort)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, "http:serve", "--port", httpPort)
	cmd.Dir = projectDir
	cmd.Env = testkit.IntegrationProcessEnv(t, map[string]string{
		"LIGHTHOUSE_AGENT_RETRY_MS":           "100",
		"LIGHTHOUSE_INSPECT_ENABLED":          "true",
		"LIGHTHOUSE_INSPECT_FLUSH_BATCH_SIZE": "1",
		"LIGHTHOUSE_INSPECT_FLUSH_INTERVAL":   "100ms",
	})
	handle := &procHandle{
		name:   "http",
		cmd:    cmd,
		cancel: cancel,
	}
	cmd.Stdout = &handle.stdout
	cmd.Stderr = &handle.stderr
	if err := handle.Start(); err != nil {
		t.Fatalf("start rendered app: %v", err)
	}
	defer stopProcAsync(t, "trace-server", handle, time.Second)

	baseURL := "http://127.0.0.1:" + httpPort
	if !waitForTCP(t, "127.0.0.1:"+httpPort, 3*time.Second) {
		t.Fatalf("server did not accept TCP connections before timeout\n%s", handle.Output())
	}

	token := renderedEnvValue(t, projectDir, "LIGHTHOUSE_SECRET")
	if token == "" {
		t.Fatal("LIGHTHOUSE_SECRET missing from rendered env")
	}
	if err := waitForRenderedAgents(ctx, baseURL, token, []string{"http"}, 5*time.Second); err != nil {
		t.Fatalf("http lighthouse agent did not register: %v\n%s", err, handle.Output())
	}
	consoleConn := dialRenderedConsoleWS(t, baseURL, token)
	defer consoleConn.Close()
	time.Sleep(200 * time.Millisecond)

	helloResp, err := http.Get(baseURL + "/api/v1/hello")
	if err != nil {
		t.Fatalf("get hello endpoint: %v\n%s", err, handle.Output())
	}
	_ = helloResp.Body.Close()
	if helloResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/hello status = %d, want %d\n%s", helloResp.StatusCode, http.StatusOK, handle.Output())
	}

	var summaries []renderedTraceSummary
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		summaries = fetchRenderedTraceSummaries(t, baseURL, token)
		for _, summary := range summaries {
			if summary.Source == "http" && summary.Name == "GET /api/v1/hello" {
				record := fetchRenderedTraceRecord(t, baseURL, token, summary.TraceID)
				if record.Summary.TraceID != summary.TraceID {
					t.Fatalf("trace detail id = %q, want %q", record.Summary.TraceID, summary.TraceID)
				}
				if record.Summary.Source != "http" {
					t.Fatalf("trace detail source = %q, want %q", record.Summary.Source, "http")
				}
				if len(record.Events) == 0 {
					t.Fatalf("trace detail for %q had no events", summary.TraceID)
				}
				httpExchange := findRenderedTraceEvent(record.Events, "http", "http_exchange")
				if httpExchange == nil {
					t.Fatalf("trace detail for %q missing http exchange event", summary.TraceID)
				}
				if httpExchange.HTTP == nil {
					t.Fatalf("trace detail for %q missing http exchange payload", summary.TraceID)
				}
				if got := httpExchange.HTTP.Method; got != http.MethodGet {
					t.Fatalf("http exchange method = %q, want %q", got, http.MethodGet)
				}
				if got := httpExchange.HTTP.URI; got != "/api/v1/hello" {
					t.Fatalf("http exchange uri = %q, want %q", got, "/api/v1/hello")
				}
				if got := httpExchange.HTTP.ResponseStatus; got != http.StatusOK {
					t.Fatalf("http exchange response_status = %d, want %d", got, http.StatusOK)
				}
				if got := httpExchange.HTTP.ResponseBody; got == "" {
					t.Fatalf("http exchange response_body was empty for %q", summary.TraceID)
				}
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	t.Fatalf("recent trace list missing GET /api/v1/hello http trace\nsummaries=%+v\n%s", summaries, handle.Output())
}

func dialRenderedConsoleWS(t *testing.T, baseURL, token string) *websocket.Conn {
	t.Helper()

	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/lighthouse/ws/console"
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("dial rendered console websocket %s: %v", wsURL, err)
	}
	return conn
}

func findRenderedTraceEvent(events []renderedTraceEvent, kind, name string) *renderedTraceEvent {
	for i := range events {
		if events[i].Kind == kind && events[i].Name == name {
			return &events[i]
		}
	}
	return nil
}

func fetchRenderedTraceSummaries(t *testing.T, baseURL, token string) []renderedTraceSummary {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/lighthouse/api/inspect?limit=20", nil)
	if err != nil {
		t.Fatalf("new inspect request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("fetch inspect list: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /lighthouse/api/inspect status = %d, want %d\nbody:\n%s", resp.StatusCode, http.StatusOK, body)
	}
	var payload struct {
		Inspects []renderedTraceSummary `json:"inspects"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode inspect response: %v", err)
	}
	return payload.Inspects
}

func fetchRenderedTraceRecord(t *testing.T, baseURL, token, traceID string) renderedTraceRecord {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/lighthouse/api/inspect/"+traceID, nil)
	if err != nil {
		t.Fatalf("new inspect detail request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("fetch inspect detail: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /lighthouse/api/inspect/:id status = %d, want %d\nbody:\n%s", resp.StatusCode, http.StatusOK, body)
	}
	var record renderedTraceRecord
	if err := json.NewDecoder(resp.Body).Decode(&record); err != nil {
		t.Fatalf("decode inspect detail: %v", err)
	}
	return record
}

func waitForRenderedAgents(ctx context.Context, baseURL, token string, sources []string, timeout time.Duration) error {
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
			var agents []renderedAgentInfo
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
	return context.DeadlineExceeded
}

func renderedEnvValue(t *testing.T, root, key string) string {
	t.Helper()

	prefix := strings.TrimSpace(key) + "="
	for _, line := range strings.Split(readRenderedFile(t, root, ".env"), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return str.Of(line).TrimPrefix(prefix).Trim().String()
		}
	}
	return ""
}

func setRenderedEnvValue(t *testing.T, root, key, value string) {
	t.Helper()
	for _, envFile := range []string{".env", ".env.host", ".env.local"} {
		if err := testkit.ReplaceOrAppendEnvValue(filepath.Join(root, envFile), key, value); err != nil {
			t.Fatalf("set %s in %s: %v", key, envFile, err)
		}
	}
}
