package forj

import (
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

func TestResolveLighthouseUIURL(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "disabled",
			env:  map[string]string{"LIGHTHOUSE_ENABLED": "false"},
			want: "",
		},
		{
			name: "default",
			env:  map[string]string{},
			want: "http://localhost:3000/lighthouse",
		},
		{
			name: "from ws url",
			env:  map[string]string{"LIGHTHOUSE_URL": "ws://127.0.0.1:7777/lighthouse/ws/agent"},
			want: "http://127.0.0.1:7777/lighthouse",
		},
		{
			name: "from wss url",
			env:  map[string]string{"LIGHTHOUSE_URL": "wss://example.com/lighthouse/ws/agent"},
			want: "https://example.com/lighthouse",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveLighthouseUIURL(tc.env)
			if got != tc.want {
				t.Fatalf("resolveLighthouseUIURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveLighthouseOpenURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "empty",
			raw:  "",
			want: "",
		},
		{
			name: "default",
			raw:  "http://localhost:3000/lighthouse",
			want: "http://localhost:3000/lighthouse/auth/dev-session",
		},
		{
			name: "trailing slash",
			raw:  "http://localhost:3000/lighthouse/",
			want: "http://localhost:3000/lighthouse/auth/dev-session",
		},
		{
			name: "invalid url falls back",
			raw:  "://bad",
			want: "://bad",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveLighthouseOpenURL(tc.raw)
			if got != tc.want {
				t.Fatalf("resolveLighthouseOpenURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAbsolutizeLighthouseURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		raw     string
		want    string
	}{
		{
			name:    "relative path",
			baseURL: "http://localhost:3000/lighthouse",
			raw:     "/lighthouse/auth/dev/abc123",
			want:    "http://localhost:3000/lighthouse/auth/dev/abc123",
		},
		{
			name:    "absolute url",
			baseURL: "http://localhost:3000/lighthouse",
			raw:     "http://127.0.0.1:3000/lighthouse/auth/dev/abc123",
			want:    "http://127.0.0.1:3000/lighthouse/auth/dev/abc123",
		},
		{
			name:    "invalid base",
			baseURL: "://bad",
			raw:     "/lighthouse/auth/dev/abc123",
			want:    "/lighthouse/auth/dev/abc123",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := absolutizeLighthouseURL(tc.baseURL, tc.raw)
			if got != tc.want {
				t.Fatalf("absolutizeLighthouseURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveAPIURL(t *testing.T) {
	if got := resolveAPIURL(map[string]string{"APP_URL": "http://127.0.0.1:8080"}); got != "http://127.0.0.1:8080" {
		t.Fatalf("resolveAPIURL(APP_URL) = %q", got)
	}
	if got := resolveAPIURL(map[string]string{}); got != "http://localhost:3000" {
		t.Fatalf("resolveAPIURL(default) = %q", got)
	}
}

func TestCollectDevToolLinks(t *testing.T) {
	config := &project.Config{}
	config.Render.Components.WebAPI = true
	config.Render.Components.Mail = true
	config.Render.Components.Docker = true
	config.Render.Components.Observability = true
	config.Render.Components.Grafana = true

	env := map[string]string{
		"APP_URL":               "http://127.0.0.1:8080",
		"LIGHTHOUSE_URL":        "ws://127.0.0.1:7777/lighthouse/ws/agent",
		"API_SWAGGER_ENABLED":   "true",
		"MAILPIT_HTTP_PORT":     "18025",
		"OBSERVABILITY_VM_PORT": "18428",
		"GRAFANA_PORT":          "13001",
		"GRAFANA_ADMIN_USER":    "ops",
		"COMPOSE_PROFILES":      "mailpit,victoriametrics,grafana",
	}

	got := collectDevToolLinks(config, env)
	if len(got) != 6 {
		t.Fatalf("collectDevToolLinks() len = %d, want 6", len(got))
	}

	wantURLs := map[string]string{
		"App":             "http://127.0.0.1:8080",
		"Lighthouse":      "http://127.0.0.1:7777/lighthouse",
		"Swagger":         "http://127.0.0.1:8080/swagger",
		"Mailpit":         "http://localhost:18025",
		"VictoriaMetrics": "http://localhost:18428",
		"Grafana":         "http://localhost:13001",
	}

	for _, tool := range got {
		wantURL, ok := wantURLs[tool.Label]
		if !ok {
			t.Fatalf("unexpected tool label %q", tool.Label)
		}
		if tool.URL != wantURL {
			t.Fatalf("%s URL = %q, want %q", tool.Label, tool.URL, wantURL)
		}
	}
}

func TestBuildDevReadySummaryLines(t *testing.T) {
	config := &project.Config{}
	config.Render.Components.Mail = true
	config.Render.Components.Docker = true

	lines := buildDevReadySummaryLines(config, map[string]string{
		"APP_URL":           "http://localhost:9000",
		"MAILPIT_HTTP_PORT": "18025",
		"COMPOSE_PROFILES":  "mailpit",
	})
	if len(lines) < 4 {
		t.Fatalf("buildDevReadySummaryLines() len = %d, want at least 4", len(lines))
	}

	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"Dev ready",
		"App",
		"http://localhost:9000",
		"Mailpit",
		"http://localhost:18025",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("summary missing %q\n%s", want, joined)
		}
	}
}
