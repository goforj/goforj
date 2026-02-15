package forj

import "testing"

func TestResolveDevconsoleUIURL(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "disabled",
			env:  map[string]string{"DEVCONSOLE_ENABLED": "false"},
			want: "",
		},
		{
			name: "default",
			env:  map[string]string{},
			want: "http://localhost:3000/__devconsole",
		},
		{
			name: "from ws url",
			env:  map[string]string{"DEVCONSOLE_URL": "ws://127.0.0.1:7777/__devconsole/ws/agent"},
			want: "http://127.0.0.1:7777/__devconsole",
		},
		{
			name: "from wss url",
			env:  map[string]string{"DEVCONSOLE_URL": "wss://example.com/__devconsole/ws/agent"},
			want: "https://example.com/__devconsole",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveDevconsoleUIURL(tc.env)
			if got != tc.want {
				t.Fatalf("resolveDevconsoleUIURL() = %q, want %q", got, tc.want)
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
