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
			name:    "grouped command uses package",
			args:    []string{"make:command", "hello:again"},
			runName: "hello:again",
			wantFiles: []string{
				filepath.Join(projectDir, "internal", "hello", "again_cmd.go"),
			},
			wantMarkers: []string{
				`HelloAgainCmd hello.AgainCmd`,
			},
		},
		{
			name:    "second package command imports both",
			args:    []string{"make:command", "something:again"},
			runName: "something:again",
			wantFiles: []string{
				filepath.Join(projectDir, "internal", "something", "again_cmd.go"),
			},
			wantMarkers: []string{
				`SomethingAgainCmd something.AgainCmd`,
			},
		},
		{
			name:    "override signature name",
			args:    []string{"make:command", "report:summary", "--name", "reports:summary"},
			runName: "reports:summary",
			wantFiles: []string{
				filepath.Join(projectDir, "internal", "report", "summary_cmd.go"),
			},
			wantMarkers: []string{
				`report.SummaryCmd`,
			},
		},
		{
			name:    "nested command group colocates with package path",
			args:    []string{"make:command", "billing:reports:sync"},
			runName: "billing:reports:sync",
			wantFiles: []string{
				filepath.Join(projectDir, "internal", "billing", "reports", "sync_cmd.go"),
			},
			wantMarkers: []string{
				`BillingReportsSyncCmd billingReports.SyncCmd`,
				`billingReportsSyncCmd *billingReports.SyncCmd`,
			},
		},
		{
			name:    "duplicate command name in nested billing package",
			args:    []string{"make:command", "Sync", "-d", "./internal/billing/sync", "--name", "billing:sync"},
			runName: "billing:sync",
			wantFiles: []string{
				filepath.Join(projectDir, "internal", "billing", "sync", "sync_cmd.go"),
			},
			wantMarkers: []string{
				`BillingSyncSyncCmd billingSync.SyncCmd`,
				`billingSyncSyncCmd *billingSync.SyncCmd`,
			},
		},
		{
			name:    "duplicate command name in nested accounts package",
			args:    []string{"make:command", "Sync", "-d", "./internal/accounts/sync", "--name", "accounts:sync"},
			runName: "accounts:sync",
			wantFiles: []string{
				filepath.Join(projectDir, "internal", "accounts", "sync", "sync_cmd.go"),
			},
			wantMarkers: []string{
				`AccountsSyncSyncCmd accountsSync.SyncCmd`,
				`accountsSyncSyncCmd *accountsSync.SyncCmd`,
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
			appCmds := filepath.Join(projectDir, "app", "commands.go")
			data, err := os.ReadFile(appCmds)
			if err != nil {
				t.Fatalf("read app/commands.go: %v", err)
			}
			content := string(data)
			for _, marker := range tc.wantMarkers {
				if !strings.Contains(normalizeWhitespace(content), normalizeWhitespace(marker)) {
					t.Fatalf("missing %q in app/commands.go\n\n%s", marker, content)
				}
			}
		})
	}

	buildApp(t)
	for _, tc := range cases {
		out := runApp(t, tc.runName)
		if !strings.Contains(out, "executed!") {
			t.Fatalf("expected generated command %s to run successfully, got:\n%s", tc.runName, out)
		}
	}

	assertImportBlock(t, filepath.Join(projectDir, "app", "commands.go"), []string{
		"internal/hello",
		"internal/something",
		"internal/report",
		"internal/billing/reports",
		"internal/billing/sync",
		"internal/accounts/sync",
	})
	assertFileContains(t, filepath.Join(projectDir, "app", "commands.go"), []string{
		`billingReports "example.com/testapp/internal/billing/reports"`,
		`billingSync "example.com/testapp/internal/billing/sync"`,
		`accountsSync "example.com/testapp/internal/accounts/sync"`,
	})
	assertFileContains(t, filepath.Join(projectDir, "app", "wire", "inject_cmd_app.go"), []string{
		`billingReports "example.com/testapp/internal/billing/reports"`,
		`billingSync "example.com/testapp/internal/billing/sync"`,
		`accountsSync "example.com/testapp/internal/accounts/sync"`,
		`billingReports.NewSyncCmd`,
		`billingSync.NewSyncCmd`,
		`accountsSync.NewSyncCmd`,
	})
}

func normalizeWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func assertFileContains(t *testing.T, path string, required []string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	content := string(data)
	for _, req := range required {
		if !strings.Contains(content, req) {
			t.Fatalf("missing %q in %s\n\n%s", req, path, content)
		}
	}
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
