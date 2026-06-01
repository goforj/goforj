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
	"github.com/goforj/goforj/project"
)

func TestRenderedWorkerQueueSelectionIntegration(t *testing.T) {
	projectDir := t.TempDir()
	queueEnv := map[string]string{
		"QUEUE_DRIVER":            "workerpool",
		"QUEUE_SUPPORTED_DRIVERS": "workerpool",
		"QUEUE_EMAILS_WORKERS":    "2",
		"QUEUE_REPORTS_WORKERS":   "1",
	}
	testkit.RenderProjectWithForj(t, projectDir, testkit.RenderProjectRequest{
		Config: project.Config{
			ProjectName:  "QueueSelection",
			GoModuleName: "example.com/queueselection",
			UpdatedAt:    "2026-01-01 00:00:00 UTC",
			Render: project.RenderConfig{
				QueueDriver: "workerpool",
				Components: project.Components{
					Jobs: true,
				},
			},
		},
		EnvOverrides: queueEnv,
	})

	binPath := buildRenderedQueueSelectionApp(t, projectDir, queueEnv)

	out := runRenderedWorkerUntilStarted(t, projectDir, binPath, queueEnv, "worker")
	assertWorkerOutputContainsQueues(t, out, "default", "emails", "reports")
	assertWorkerOutputContainsWorkerConfig(t, out, "workers=33", "default=30", "emails=2", "reports=1")

	out = runRenderedWorkerUntilStarted(t, projectDir, binPath, queueEnv, "worker", "--queue", "reports")
	assertWorkerOutputContainsQueues(t, out, "reports")
	assertWorkerOutputContainsWorkerConfig(t, out, "workers=1", "reports=1")

	out = runRenderedWorkerUntilStarted(t, projectDir, binPath, queueEnv, "worker", "--queue", "emails", "--queue", "reports")
	assertWorkerOutputContainsQueues(t, out, "emails", "reports")
	assertWorkerOutputContainsWorkerConfig(t, out, "workers=3", "emails=2", "reports=1")

	out, err := runRenderedWorkerToExit(t, projectDir, binPath, queueEnv, "worker", "--queue", "missing")
	if err == nil {
		t.Fatalf("expected missing queue selection to fail, got success:\n%s", out)
	}
	if !strings.Contains(out, `unknown queue "missing"`) {
		t.Fatalf("expected missing queue error, got:\n%s", out)
	}
}

func buildRenderedQueueSelectionApp(t *testing.T, projectDir string, env map[string]string) string {
	t.Helper()
	binPath := filepath.Join(projectDir, "bin", "app")
	if err := os.MkdirAll(filepath.Dir(binPath), 0755); err != nil {
		t.Fatalf("mkdir bin dir: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, ".")
	cmd.Dir = projectDir
	cmd.Env = testkit.IntegrationGoProcessEnv(t, env)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("build rendered app failed: %v\n%s", err, out.String())
	}
	return binPath
}

func runRenderedWorkerUntilStarted(t *testing.T, projectDir, binPath string, env map[string]string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.Dir = projectDir
	cmd.Env = testkit.IntegrationProcessEnv(t, env)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		t.Fatalf("start rendered worker: %v", err)
	}

	started := false
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(out.String(), "Queue worker started") {
			started = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	_ = cmd.Wait()
	if !started {
		t.Fatalf("worker did not start before timeout:\n%s", out.String())
	}
	return out.String()
}

func runRenderedWorkerToExit(t *testing.T, projectDir, binPath string, env map[string]string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.Dir = projectDir
	cmd.Env = testkit.IntegrationProcessEnv(t, env)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

func assertWorkerOutputContainsQueues(t *testing.T, output string, names ...string) {
	t.Helper()
	for _, name := range names {
		if !strings.Contains(output, name) {
			t.Fatalf("expected worker output to mention queue %q, got:\n%s", name, output)
		}
	}
}

// assertWorkerOutputContainsWorkerConfig verifies worker startup metadata.
func assertWorkerOutputContainsWorkerConfig(t *testing.T, output string, tokens ...string) {
	t.Helper()
	output = ansiEscapeRe.ReplaceAllString(output, "")
	for _, token := range tokens {
		if !strings.Contains(output, token) {
			t.Fatalf("expected worker output to mention %q, got:\n%s", token, output)
		}
	}
}
