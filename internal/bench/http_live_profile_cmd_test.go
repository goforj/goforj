package bench

import (
	"strings"
	"testing"
)

func TestRawHTTPHealthHandlerJSON(t *testing.T) {
	cmd := &HTTPLiveProfileCmd{HealthMode: "json"}
	handler := cmd.rawHTTPHealthHandler()
	if !strings.Contains(handler, `json.NewEncoder(w).Encode(map[string]string{"status": "ok"})`) {
		t.Fatalf("expected json body in handler, got %q", handler)
	}
	if !strings.Contains(handler, "application/json") {
		t.Fatalf("expected json content type in handler, got %q", handler)
	}
}

func TestRawHTTPHealthHandlerText(t *testing.T) {
	cmd := &HTTPLiveProfileCmd{HealthMode: "text"}
	handler := cmd.rawHTTPHealthHandler()
	if !strings.Contains(handler, `[]byte("ok")`) {
		t.Fatalf("expected text body in handler, got %q", handler)
	}
	if !strings.Contains(handler, "text/plain") {
		t.Fatalf("expected text content type in handler, got %q", handler)
	}
}

func TestRawHTTPHealthHandlerNoContent(t *testing.T) {
	cmd := &HTTPLiveProfileCmd{HealthMode: "nocontent"}
	handler := cmd.rawHTTPHealthHandler()
	if strings.Contains(handler, "[]byte(") {
		t.Fatalf("did not expect body write in nocontent handler, got %q", handler)
	}
	if !strings.Contains(handler, "http.StatusNoContent") {
		t.Fatalf("expected nocontent status in handler, got %q", handler)
	}
}

func TestEchoHTTPHealthHandlerJSON(t *testing.T) {
	cmd := &HTTPLiveProfileCmd{HealthMode: "json"}
	handler := cmd.echoHTTPHealthHandler()
	if !strings.Contains(handler, `c.JSON(http.StatusOK`) {
		t.Fatalf("expected echo json handler, got %q", handler)
	}
}

func TestStandaloneHTTPProfileSourceIncludesEchoModule(t *testing.T) {
	cmd := &HTTPLiveProfileCmd{ServerStack: "echonative"}
	source := cmd.standaloneHTTPProfileSource()
	if !strings.Contains(source, `echo "github.com/labstack/echo/v5"`) {
		t.Fatalf("expected echo import in standalone source, got %q", source)
	}
}

func TestStandaloneHTTPProfileSourceIncludesJSONImportForRawModes(t *testing.T) {
	cmd := &HTTPLiveProfileCmd{ServerStack: "rawnethttp", HealthMode: "json"}
	source := cmd.standaloneHTTPProfileSource()
	if !strings.Contains(source, `"encoding/json"`) {
		t.Fatalf("expected encoding/json import in raw source, got %q", source)
	}
}

func TestStandaloneHTTPProfileSourceOmitsJSONImportForNoContent(t *testing.T) {
	cmd := &HTTPLiveProfileCmd{ServerStack: "rawnethttp", HealthMode: "nocontent"}
	source := cmd.standaloneHTTPProfileSource()
	if strings.Contains(source, `"encoding/json"`) {
		t.Fatalf("did not expect encoding/json import in nocontent raw source, got %q", source)
	}
}
