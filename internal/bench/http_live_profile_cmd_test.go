package bench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHTTPHealthHandlers keeps raw and native comparison responses aligned with their selected health modes.
func TestHTTPHealthHandlers(t *testing.T) {
	tests := []struct {
		name  string
		stack string
		mode  string
		want  []string
		avoid []string
	}{
		{name: "raw json", stack: "raw", mode: "json", want: []string{`json.NewEncoder(w).Encode(map[string]string{"status": "ok"})`, "application/json"}},
		{name: "raw text", stack: "raw", mode: "text", want: []string{`[]byte("ok")`, "text/plain"}},
		{name: "raw no content", stack: "raw", mode: "nocontent", want: []string{"http.StatusNoContent"}, avoid: []string{"[]byte("}},
		{name: "echo json", stack: "echo", mode: "json", want: []string{`c.JSON(http.StatusOK`}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := &HTTPLiveProfileCmd{HealthMode: test.mode}
			handler := cmd.rawHTTPHealthHandler()
			if test.stack == "echo" {
				handler = cmd.echoHTTPHealthHandler()
			}
			for _, want := range test.want {
				if !strings.Contains(handler, want) {
					t.Fatalf("handler missing %q: %q", want, handler)
				}
			}
			for _, avoid := range test.avoid {
				if strings.Contains(handler, avoid) {
					t.Fatalf("handler unexpectedly contains %q: %q", avoid, handler)
				}
			}
		})
	}
}

// TestStandaloneHTTPProfileSourceDependencies keeps generated imports limited to the selected stack and response shape.
func TestStandaloneHTTPProfileSourceDependencies(t *testing.T) {
	tests := []struct {
		name    string
		stack   string
		mode    string
		token   string
		present bool
	}{
		{name: "Echo module", stack: "echonative", token: `echo "github.com/labstack/echo/v5"`, present: true},
		{name: "raw JSON import", stack: "rawnethttp", mode: "json", token: `"encoding/json"`, present: true},
		{name: "raw no-content import", stack: "rawnethttp", mode: "nocontent", token: `"encoding/json"`, present: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := &HTTPLiveProfileCmd{ServerStack: test.stack, HealthMode: test.mode}
			source := cmd.standaloneHTTPProfileSource()
			if present := strings.Contains(source, test.token); present != test.present {
				t.Fatalf("source contains %q = %t, want %t: %q", test.token, present, test.present, source)
			}
		})
	}
}

// TestStandaloneHTTPProfileSecurityMinimums keeps comparison Apps on the same fixed HTTP dependencies as rendered Apps.
func TestStandaloneHTTPProfileSecurityMinimums(t *testing.T) {
	for _, stack := range []string{"echonative", "rawnethttp", "rawdirect"} {
		t.Run(stack, func(t *testing.T) {
			dir := t.TempDir()
			cmd := &HTTPLiveProfileCmd{ServerStack: stack}
			if err := cmd.writeStandaloneHTTPProfileApp(dir); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
			if err != nil {
				t.Fatal(err)
			}
			text := string(data)
			if !strings.Contains(text, "go 1.25.0") {
				t.Fatalf("comparison App Go floor changed: %s", text)
			}
			if stack == "echonative" {
				for _, minimum := range []string{"github.com/labstack/echo/v5 v5.4.0", "golang.org/x/net v0.61.0"} {
					if !strings.Contains(text, minimum) {
						t.Fatalf("comparison App lacks security minimum %s: %s", minimum, text)
					}
				}
			} else if strings.Contains(text, "github.com/labstack/echo") {
				t.Fatalf("standard-library comparison unexpectedly selects Echo: %s", text)
			}
		})
	}
}
