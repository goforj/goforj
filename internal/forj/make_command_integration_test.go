//go:build integration

package forj

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/testkit"
)

func TestMakeCommandIntegration(t *testing.T) {
	projectDir := t.TempDir()
	renderAppAtDir(t, projectDir)
	binPath := testkit.EnsureIntegrationForjBinary(t)
	_ = testkit.EnsureIntegrationToolsDir(t)

	runForj := func(tb testing.TB, args ...string) {
		tb.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binPath, args...)
		cmd.Dir = projectDir
		cmd.Env = testkit.IntegrationProcessEnv(t, nil)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			tb.Fatalf("forj %s failed: %v\n%s", strings.Join(args, " "), err, out.String())
		}
	}

	runApp := func(tb testing.TB, args ...string) string {
		tb.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, filepath.Join(projectDir, "bin", "app"), args...)
		cmd.Dir = projectDir
		cmd.Env = testkit.IntegrationProcessEnv(t, nil)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			tb.Fatalf("app %s failed: %v\n%s", strings.Join(args, " "), err, out.String())
		}
		return out.String()
	}

	buildApp := func(tb testing.TB) {
		tb.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binPath, "build", "-o", "./bin/app")
		cmd.Dir = projectDir
		cmd.Env = testkit.IntegrationGoProcessEnv(t, nil)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			tb.Fatalf("forj build failed: %v\n%s", err, out.String())
		}
		if _, err := os.Stat(filepath.Join(projectDir, "bin", "app")); err != nil {
			tb.Fatalf("expected built app binary: %v", err)
		}
	}

	type testCase struct {
		name        string
		args        []string
		runName     string
		wantFiles   []string
		wantMarkers []string
	}

	cases := []testCase{
		{
			name: "grouped command uses package",
			args: []string{"make:command", "hello:again"},
			runName: "hello:again",
			wantFiles: []string{
				filepath.Join(projectDir, "internal", "hello", "again_cmd.go"),
			},
			wantMarkers: []string{
				`HelloAgainCmd hello.AgainCmd`,
			},
		},
		{
			name: "second package command imports both",
			args: []string{"make:command", "something:again"},
			runName: "something:again",
			wantFiles: []string{
				filepath.Join(projectDir, "internal", "something", "again_cmd.go"),
			},
			wantMarkers: []string{
				`SomethingAgainCmd something.AgainCmd`,
			},
		},
		{
			name: "override signature name",
			args: []string{"make:command", "report:summary", "--name", "reports:summary"},
			runName: "reports:summary",
			wantFiles: []string{
				filepath.Join(projectDir, "internal", "report", "summary_cmd.go"),
			},
			wantMarkers: []string{
				`report.SummaryCmd`,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runForj(t, tc.args...)
			for _, path := range tc.wantFiles {
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("expected file %s: %v", path, err)
				}
			}
			appCmds := filepath.Join(projectDir, "internal", "cmd", "app_commands.go")
			data, err := os.ReadFile(appCmds)
			if err != nil {
				t.Fatalf("read app_commands.go: %v", err)
			}
			content := string(data)
			for _, marker := range tc.wantMarkers {
				if !strings.Contains(normalizeWhitespace(content), normalizeWhitespace(marker)) {
					t.Fatalf("missing %q in app_commands.go\n\n%s", marker, content)
				}
			}
			buildApp(t)
			out := runApp(t, tc.runName)
			if !strings.Contains(out, "executed!") {
				t.Fatalf("expected generated command to run successfully, got:\n%s", out)
			}
		})
	}

	assertImportBlock(t, filepath.Join(projectDir, "internal", "cmd", "app_commands.go"), []string{
		"internal/hello",
		"internal/something",
		"internal/report",
	})
}

func normalizeWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func assertImportBlock(t *testing.T, path string, required []string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := strings.Split(string(data), "\n")
	var block []string
	inBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "import (" {
			inBlock = true
			continue
		}
		if inBlock && trimmed == ")" {
			break
		}
		if inBlock {
			block = append(block, strings.Trim(trimmed, "\""))
		}
		if strings.HasPrefix(trimmed, "import ") {
			t.Fatalf("found single-line import after normalization: %s", trimmed)
		}
	}
	if !inBlock {
		t.Fatalf("missing import block in %s", path)
	}
	for _, req := range required {
		found := false
		for _, imp := range block {
			if strings.Contains(imp, req) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing import %q in %s", req, path)
		}
	}
}
