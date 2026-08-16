//go:build integration

package forj

import (
	"bytes"
	"context"
	"fmt"
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
				Components: project.Components{
					Jobs:      true,
					Scheduler: true,
				},
			},
		},
		EnvOverrides: queueEnv,
	})
	if err := testkit.ReplaceOrAppendEnvValues([]string{filepath.Join(projectDir, ".env")}, queueEnv); err != nil {
		t.Fatalf("persist workerpool queue selection: %v", err)
	}
	generateRenderedProject(t, projectDir, queueEnv)

	binPath := buildRenderedQueueSelectionApp(t, projectDir, queueEnv)
	runRenderedMaintenanceUnitTests(t, projectDir, queueEnv)

	out := runRenderedWorkerUntilStarted(t, projectDir, binPath, queueEnv, "worker")
	assertWorkerOutputContainsQueues(t, out, "default", "emails", "reports")
	assertWorkerOutputContainsWorkerCount(t, out, 33)

	out = runRenderedWorkerUntilStarted(t, projectDir, binPath, queueEnv, "worker", "--queue", "reports")
	assertWorkerOutputContainsQueues(t, out, "reports")
	assertWorkerOutputContainsWorkerCount(t, out, 1)

	out = runRenderedWorkerUntilStarted(t, projectDir, binPath, queueEnv, "worker", "--queue", "emails", "--queue", "reports")
	assertWorkerOutputContainsQueues(t, out, "emails", "reports")
	assertWorkerOutputContainsWorkerCount(t, out, 3)

	out, err := runRenderedWorkerToExit(t, projectDir, binPath, queueEnv, "worker", "--queue", "missing")
	if err == nil {
		t.Fatalf("expected missing queue selection to fail, got success:\n%s", out)
	}
	if !strings.Contains(out, `unknown queue "missing"`) {
		t.Fatalf("expected missing queue error, got:\n%s", out)
	}

	assertRenderedWorkerFollowsMaintenance(t, projectDir, binPath, queueEnv)
}

// runRenderedMaintenanceUnitTests executes generated maintenance, runtime, and scheduler contracts that are not part of a binary build.
func runRenderedMaintenanceUnitTests(t *testing.T, projectDir string, env map[string]string) {
	t.Helper()
	cmd := exec.Command("go", "test", "./internal/maintenance", "./internal/runtime", "./internal/schedules", "-count=1")
	cmd.Dir = projectDir
	cmd.Env = testkit.IntegrationGoProcessEnv(t, env)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("test rendered maintenance packages: %v\n%s", err, output)
	}
}

// assertRenderedWorkerFollowsMaintenance verifies a running worker drains, waits, and starts a fresh generation across maintenance transitions.
func assertRenderedWorkerFollowsMaintenance(t *testing.T, projectDir, binPath string, env map[string]string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, binPath, "worker")
	cmd.Dir = projectDir
	cmd.Env = testkit.IntegrationProcessEnv(t, env)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		t.Fatalf("start rendered maintenance worker: %v", err)
	}
	waitForWorkerOutput(t, &out, "Queue worker started", 1)
	runRenderedMaintenanceCommand(t, projectDir, binPath, env, "maintenance:enable")
	waitForWorkerOutput(t, &out, "Queue worker paused for maintenance mode", 1)
	runRenderedMaintenanceCommand(t, projectDir, binPath, env, "maintenance:disable")
	waitForWorkerOutput(t, &out, "Queue worker started", 2)
	cancel()
	if err := cmd.Wait(); err != nil && ctx.Err() == nil {
		t.Fatalf("maintenance worker exit: %v\n%s", err, out.String())
	}
}

// runRenderedMaintenanceCommand invokes the same App binary that owns the running worker state.
func runRenderedMaintenanceCommand(t *testing.T, projectDir, binPath string, env map[string]string, command string) {
	t.Helper()
	cmd := exec.Command(binPath, command)
	cmd.Dir = projectDir
	cmd.Env = testkit.IntegrationProcessEnv(t, env)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run %s: %v\n%s", command, err, output)
	}
}

// waitForWorkerOutput waits for a stable lifecycle message count from the rendered worker.
func waitForWorkerOutput(t *testing.T, out *bytes.Buffer, message string, count int) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Count(out.String(), message) >= count {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("worker output did not contain %q %d times:\n%s", message, count, out.String())
}

// buildRenderedQueueSelectionApp keeps the build rendered queue selection app representation consistent.
func buildRenderedQueueSelectionApp(t *testing.T, projectDir string, env map[string]string) string {
	t.Helper()
	binPath := filepath.Join(projectDir, "bin", "app")
	buildRenderedDefaultAppTo(t, projectDir, binPath, env, "build rendered app")
	return binPath
}

// runRenderedWorkerUntilStarted centralizes run rendered worker until started behavior so callers follow the same contract.
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

// runRenderedWorkerToExit centralizes run rendered worker to exit behavior so callers follow the same contract.
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

// assertWorkerOutputContainsQueues centralizes assert worker output contains queues behavior so callers follow the same contract.
func assertWorkerOutputContainsQueues(t *testing.T, output string, names ...string) {
	t.Helper()
	for _, name := range names {
		if !strings.Contains(output, name) {
			t.Fatalf("expected worker output to mention queue %q, got:\n%s", name, output)
		}
	}
}

// assertWorkerOutputContainsWorkerCount verifies the concise worker startup summary.
func assertWorkerOutputContainsWorkerCount(t *testing.T, output string, count int) {
	t.Helper()
	output = ansiEscapeRe.ReplaceAllString(output, "")
	unit := "workers"
	if count == 1 {
		unit = "worker"
	}
	token := fmt.Sprintf("· %d %s", count, unit)
	if !strings.Contains(output, token) {
		t.Fatalf("expected worker output to mention %q, got:\n%s", token, output)
	}
}
