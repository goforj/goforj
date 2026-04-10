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
	_ = testkit.EnsureIntegrationToolsDir(t, wireInstallTarget)

	runForj := func(args ...string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binPath, args...)
		cmd.Dir = projectDir
		cmd.Env = testkit.IntegrationProcessEnv(t, nil)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			t.Fatalf("forj %s failed: %v\n%s", strings.Join(args, " "), err, out.String())
		}
	}

	runWire := func() {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "go", "generate", "./wire")
		cmd.Dir = projectDir
		cmd.Env = testkit.IntegrationGoProcessEnv(t, nil)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			t.Fatalf("wire generation failed: %v\n%s", err, out.String())
		}
	}

	type testCase struct {
		name        string
		args        []string
		wantFiles   []string
		wantMarkers []string
	}

	cases := []testCase{
		{
			name: "grouped command uses package",
			args: []string{"make:command", "hello:again"},
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
			runForj(tc.args...)
			runWire()
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
