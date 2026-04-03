package forj

import "testing"

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
