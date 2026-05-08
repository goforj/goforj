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
)

type renderedTraceSummary struct {
	TraceID string            `json:"trace_id"`
	Source  string            `json:"source"`
	Name    string            `json:"name"`
	Status  string            `json:"status"`
	Labels  map[string]string `json:"labels"`
}

type renderedTraceEvent struct {
	Kind       string                 `json:"kind"`
	Name       string                 `json:"name"`
	Message    string                 `json:"message"`
	Status     string                 `json:"status"`
	Attributes map[string]any         `json:"attributes"`
}

type renderedTraceRecord struct {
	Summary renderedTraceSummary `json:"summary"`
	Events  []renderedTraceEvent `json:"events"`
}

func TestRenderedLighthouseTraceEndpoints(t *testing.T) {
	projectDir := t.TempDir()
	renderMetricsTestApp(t, projectDir)

	binPath := filepath.Join(t.TempDir(), "app")
	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	buildCmd.Dir = projectDir
	buildCmd.Env = testkit.IntegrationGoProcessEnv(t, nil)
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build rendered app: %v\n%s", err, out)
	}

	httpAddr := findFreeAddr(t)
	_, httpPort, err := net.SplitHostPort(httpAddr)
	if err != nil {
		t.Fatalf("split http addr: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, "http:serve", "--port", httpPort)
	cmd.Dir = projectDir
	cmd.Env = testkit.IntegrationProcessEnv(t, nil)
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

	helloResp, err := http.Get(baseURL + "/api/v1/hello")
	if err != nil {
		t.Fatalf("get hello endpoint: %v\n%s", err, handle.Output())
	}
	_ = helloResp.Body.Close()
	if helloResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/hello status = %d, want %d\n%s", helloResp.StatusCode, http.StatusOK, handle.Output())
	}

	token := renderedEnvValue(t, projectDir, "LIGHTHOUSE_SECRET")
	if token == "" {
		t.Fatal("LIGHTHOUSE_SECRET missing from rendered env")
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
				if got := renderedStringAttr(httpExchange.Attributes, "method"); got != http.MethodGet {
					t.Fatalf("http exchange method = %q, want %q", got, http.MethodGet)
				}
				if got := renderedStringAttr(httpExchange.Attributes, "uri"); got != "/api/v1/hello" {
					t.Fatalf("http exchange uri = %q, want %q", got, "/api/v1/hello")
				}
				if got := renderedIntAttr(httpExchange.Attributes, "response_status"); got != http.StatusOK {
					t.Fatalf("http exchange response_status = %d, want %d", got, http.StatusOK)
				}
				if got := renderedStringAttr(httpExchange.Attributes, "response_body"); got == "" {
					t.Fatalf("http exchange response_body was empty for %q", summary.TraceID)
				}
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	t.Fatalf("recent trace list missing GET /api/v1/hello http trace\nsummaries=%+v\n%s", summaries, handle.Output())
}

func findRenderedTraceEvent(events []renderedTraceEvent, kind, name string) *renderedTraceEvent {
	for i := range events {
		if events[i].Kind == kind && events[i].Name == name {
			return &events[i]
		}
	}
	return nil
}

func renderedStringAttr(attrs map[string]any, key string) string {
	if attrs == nil {
		return ""
	}
	raw, ok := attrs[key]
	if !ok || raw == nil {
		return ""
	}
	text, _ := raw.(string)
	return text
}

func renderedIntAttr(attrs map[string]any, key string) int {
	if attrs == nil {
		return 0
	}
	raw, ok := attrs[key]
	if !ok || raw == nil {
		return 0
	}
	switch value := raw.(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}

func fetchRenderedTraceSummaries(t *testing.T, baseURL, token string) []renderedTraceSummary {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/lighthouse/api/traces?limit=20", nil)
	if err != nil {
		t.Fatalf("new traces request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("fetch traces: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /lighthouse/api/traces status = %d, want %d\nbody:\n%s", resp.StatusCode, http.StatusOK, body)
	}
	var payload struct {
		Traces []renderedTraceSummary `json:"traces"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode traces response: %v", err)
	}
	return payload.Traces
}

func fetchRenderedTraceRecord(t *testing.T, baseURL, token, traceID string) renderedTraceRecord {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/lighthouse/api/traces/"+traceID, nil)
	if err != nil {
		t.Fatalf("new trace detail request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("fetch trace detail: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /lighthouse/api/traces/:id status = %d, want %d\nbody:\n%s", resp.StatusCode, http.StatusOK, body)
	}
	var record renderedTraceRecord
	if err := json.NewDecoder(resp.Body).Decode(&record); err != nil {
		t.Fatalf("decode trace detail: %v", err)
	}
	return record
}

func renderedEnvValue(t *testing.T, root, key string) string {
	t.Helper()

	prefix := strings.TrimSpace(key) + "="
	for _, line := range strings.Split(readRenderedFile(t, root, ".env"), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}
