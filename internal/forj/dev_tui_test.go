package forj

import (
	"strings"
	"testing"
)

func TestBuildDevFooterLine(t *testing.T) {
	line := buildDevFooterLine(map[string]string{
		"APP_URL":            "http://127.0.0.1:3000",
		"DEVCONSOLE_URL":     "ws://127.0.0.1:3000/__devconsole/ws/agent",
		"DEVCONSOLE_ENABLED": "true",
	})
	if !strings.Contains(line, "keys") || !strings.Contains(line, "[ ? ] help") {
		t.Fatalf("expected hotkey help in footer line: %q", line)
	}
	if !strings.Contains(line, "[ o ] devconsole") || !strings.Contains(line, "[ a ] api") {
		t.Fatalf("expected action hotkeys in footer line: %q", line)
	}
}

func TestBuildDevFooterLineWithURLs(t *testing.T) {
	line := buildDevFooterLineWithState("http://localhost:3000", "http://localhost:3000/__devconsole", true, "2")
	if !strings.Contains(line, "[ o ] devconsole") || !strings.Contains(line, "[ a ] api") {
		t.Fatalf("expected compact hotkeys in line: %q", line)
	}
	if !strings.Contains(line, "[ r ] restart") || !strings.Contains(line, "[ c ] clear") || !strings.Contains(line, "[ q ] query:on") || !strings.Contains(line, "[ 0/1/2/3 ] debug:2") {
		t.Fatalf("expected env/restart hotkeys in line: %q", line)
	}
}

func TestSplitANSITail(t *testing.T) {
	head, tail := splitANSITail("ok \x1b[31mred\x1b[0m done")
	if head != "ok \x1b[31mred\x1b[0m done" || tail != "" {
		t.Fatalf("unexpected split complete sequence: head=%q tail=%q", head, tail)
	}

	head, tail = splitANSITail("ok \x1b[31")
	if head != "ok " || tail != "\x1b[31" {
		t.Fatalf("unexpected split partial sequence: head=%q tail=%q", head, tail)
	}
}

func TestSanitizeCSIPreservesColorCodes(t *testing.T) {
	in := "ok \x1b[32mgreen\x1b[0m \x1b[2Kgone"
	out := sanitizeCSI(in)
	if !strings.Contains(out, "\x1b[32mgreen\x1b[0m") {
		t.Fatalf("expected SGR color codes to be preserved, got: %q", out)
	}
	if strings.Contains(out, "\x1b[2K") {
		t.Fatalf("expected cursor control codes to be stripped, got: %q", out)
	}
}

func TestUpdateEnvKey(t *testing.T) {
	in := "APP_DEBUG=1\nDB_QUERY_LOGGING=false\n"
	out := updateEnvKey(in, "APP_DEBUG", "2")
	if !strings.Contains(out, "APP_DEBUG=2") {
		t.Fatalf("expected APP_DEBUG updated: %q", out)
	}
	out = updateEnvKey(out, "DB_QUERY_LOGGING", "true")
	if !strings.Contains(out, "DB_QUERY_LOGGING=true") {
		t.Fatalf("expected DB_QUERY_LOGGING updated: %q", out)
	}
}

func TestReadEnvKey(t *testing.T) {
	in := "# comment\nexport APP_DEBUG=3\nDB_QUERY_LOGGING=1\n"
	if got := readEnvKey(in, "APP_DEBUG"); got != "3" {
		t.Fatalf("expected APP_DEBUG=3, got %q", got)
	}
	if got := readEnvKey(in, "DB_QUERY_LOGGING"); got != "1" {
		t.Fatalf("expected DB_QUERY_LOGGING=1, got %q", got)
	}
}
