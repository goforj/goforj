//go:build integration

package forj

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func TestDemoAppRenderIntegration(t *testing.T) {
	projectDir := t.TempDir()
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	testkit.RenderProjectWithForj(t, projectDir, testkit.RenderProjectRequest{
		Config: project.Config{
			ProjectName:  "DemoApp",
			GoModuleName: "example.com/demoapp",
			UpdatedAt:    "2026-01-01 00:00:00 UTC",
			Render: project.RenderConfig{
				QueueDriver: "redis",
				Components: project.Components{
					WebAPI:         true,
					WebUI:          true,
					Scheduler:      true,
					Jobs:           true,
					DatabaseSQLite: true,
					DemoApp:        true,
				},
			},
		},
	})

	required := []string{
		filepath.Join("internal", "monitoring", "controller.go"),
		filepath.Join("internal", "monitoring", "heartbeat_bucketing_test.go"),
		filepath.Join("internal", "monitoring", "check_service.go"),
		filepath.Join("internal", "monitoring", "monitor_check_job.go"),
		filepath.Join("internal", "monitoring", "incident_transition_service.go"),
		filepath.Join("internal", "app", "lifecycle.go"),
		filepath.Join("internal", "app", "lifecycle_registry.go"),
		filepath.Join("internal", "app", "README.md"),
		filepath.Join("frontend", "src", "views", "MonitoringView.vue"),
		filepath.Join("frontend", "src", "views", "StatusPublicView.vue"),
		filepath.Join("migrations", "2026_02_11_000012_monitor_alert_policy_columns.sqlite.up.sql"),
		filepath.Join("migrations", "2026_02_11_000013_incident_open_uniqueness.sqlite.up.sql"),
	}
	for _, path := range required {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}

	controllerPath := filepath.Join("internal", "monitoring", "controller.go")
	controllerSrc, err := os.ReadFile(controllerPath)
	if err != nil {
		t.Fatalf("read %s: %v", controllerPath, err)
	}
	if !strings.Contains(string(controllerSrc), "/monitoring/diagnostics/cadence") {
		t.Fatalf("expected cadence diagnostics route in %s", controllerPath)
	}
	if !strings.Contains(string(controllerSrc), "down_confirm_attempts") {
		t.Fatalf("expected alert policy fields in %s", controllerPath)
	}

	checkServicePath := filepath.Join("internal", "monitoring", "check_service.go")
	checkServiceSrc, err := os.ReadFile(checkServicePath)
	if err != nil {
		t.Fatalf("read %s: %v", checkServicePath, err)
	}
	if !strings.Contains(string(checkServiceSrc), `Status:       "pending"`) {
		t.Fatalf("expected pending retry check writes in %s", checkServicePath)
	}

	monitorPollPath := filepath.Join("internal", "monitoring", "poll_cmd.go")
	monitorPollSrc, err := os.ReadFile(monitorPollPath)
	if err != nil {
		t.Fatalf("read %s: %v", monitorPollPath, err)
	}
	for _, token := range []string{"MonitorID", "Sync", "JSON", "RunNow(", "QueueNow(", "printJSON("} {
		if !strings.Contains(string(monitorPollSrc), token) {
			t.Fatalf("expected %q in %s", token, monitorPollPath)
		}
	}

	retentionCmdPath := filepath.Join("internal", "monitoring", "retention_cmd.go")
	if _, err := os.Stat(retentionCmdPath); err != nil {
		t.Fatalf("expected %s: %v", retentionCmdPath, err)
	}

	pushTriggerCmdPath := filepath.Join("internal", "monitoring", "push_trigger_cmd.go")
	if _, err := os.Stat(pushTriggerCmdPath); err != nil {
		t.Fatalf("expected %s: %v", pushTriggerCmdPath, err)
	}
	legacyPushTriggerCmdPath := filepath.Join("internal", "cmd", "demo_push_monitor_trigger_cmd.go")
	if _, err := os.Stat(legacyPushTriggerCmdPath); !os.IsNotExist(err) {
		t.Fatalf("expected legacy file to be removed: %s", legacyPushTriggerCmdPath)
	}
	legacyLifecycleHooksCmdPath := filepath.Join("internal", "cmd", "lifecycle_hooks.go")
	if _, err := os.Stat(legacyLifecycleHooksCmdPath); !os.IsNotExist(err) {
		t.Fatalf("expected legacy file to be removed: %s", legacyLifecycleHooksCmdPath)
	}

	schedulerRegistryPath := filepath.Join("internal", "schedules", "scheduler_registry.go")
	schedulerRegistrySrc, err := os.ReadFile(schedulerRegistryPath)
	if err != nil {
		t.Fatalf("read %s: %v", schedulerRegistryPath, err)
	}
	for _, token := range []string{
		`Do(s.inspectTask("monitor:retention", s.retentionService.RunScheduled))`,
		`Do(s.inspectTask("monitor:poll", s.monitorCheckJob.RunScheduledPoll))`,
		`Do(s.inspectTask("monitor:push-test-trigger", s.monitorCheckJob.RunScheduledPushTrigger))`,
	} {
		if !strings.Contains(string(schedulerRegistrySrc), token) {
			t.Fatalf("expected %q in %s", token, schedulerRegistryPath)
		}
	}
	schedulerPath := filepath.Join("internal", "schedules", "scheduler.go")
	schedulerSrc, err := os.ReadFile(schedulerPath)
	if err != nil {
		t.Fatalf("read %s: %v", schedulerPath, err)
	}
	for _, token := range []string{
		`WithTaskContextDecorator(func(ctx context.Context) context.Context {`,
		`return app.WithSource(ctx, app.SourceScheduler)`,
	} {
		if !strings.Contains(string(schedulerSrc), token) {
			t.Fatalf("expected %q in %s", token, schedulerPath)
		}
	}
	queueManagerPath := filepath.Join("internal", "queues", "manager_gen.go")
	queueManagerSrc, err := os.ReadFile(queueManagerPath)
	if err != nil {
		t.Fatalf("read %s: %v", queueManagerPath, err)
	}
	for _, token := range []string{
		`queue.WithHandlerContextDecorator(func(ctx context.Context) context.Context {`,
		`return app.WithSource(ctx, app.SourceJobs)`,
	} {
		if !strings.Contains(string(queueManagerSrc), token) {
			t.Fatalf("expected %q in %s", token, queueManagerPath)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "./internal/monitoring", "./internal/jobs")
	cmd.Dir = projectDir
	cmd.Env = testkit.ProcessGoEnv("", nil)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("go test compile check failed: %v\n%s", err, out.String())
	}
}

func TestDemoAppQueueDriversIntegration(t *testing.T) {
	projectDir := t.TempDir()
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	testkit.RenderProjectWithForj(t, projectDir, testkit.RenderProjectRequest{
		Config: project.Config{
			ProjectName:  "DemoQueueDrivers",
			GoModuleName: "example.com/demoqueuedrivers",
			UpdatedAt:    "2026-01-01 00:00:00 UTC",
			Render: project.RenderConfig{
				QueueDriver: "redis",
				Components: project.Components{
					WebAPI:         true,
					WebUI:          false,
					Scheduler:      true,
					Jobs:           true,
					DatabaseSQLite: true,
					DemoApp:        true,
				},
			},
		},
		EnvOverrides: map[string]string{
			"QUEUE_SUPPORTED_DRIVERS": "redis,sync,workerpool",
		},
	})
	if err := testkit.ReplaceOrAppendEnvValues(
		[]string{filepath.Join(projectDir, ".env"), filepath.Join(projectDir, ".env.host")},
		map[string]string{"QUEUE_SUPPORTED_DRIVERS": "redis,sync,workerpool"},
	); err != nil {
		t.Fatalf("set queue supported drivers: %v", err)
	}
	generateCtx, generateCancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer generateCancel()
	generateCmd := exec.CommandContext(generateCtx, testkit.EnsureIntegrationForjBinary(t), "generate")
	generateCmd.Dir = projectDir
	generateCmd.Env = testkit.IntegrationGoProcessEnv(t, nil)
	var generateOut bytes.Buffer
	generateCmd.Stdout = &generateOut
	generateCmd.Stderr = &generateOut
	if err := generateCmd.Run(); err != nil {
		t.Fatalf("generate app failed: %v\n%s", err, generateOut.String())
	}

	buildCtx, buildCancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer buildCancel()
	build := exec.CommandContext(buildCtx, "go", "build", "-o", "./bin/app", ".")
	build.Dir = projectDir
	build.Env = testkit.ProcessGoEnv("", nil)
	var buildOut bytes.Buffer
	build.Stdout = &buildOut
	build.Stderr = &buildOut
	if err := build.Run(); err != nil {
		t.Fatalf("build app failed: %v\n%s", err, buildOut.String())
	}

	for _, driver := range []string{"sync", "workerpool"} {
		t.Run(driver, func(t *testing.T) {
			if err := setQueueDriverInEnvFiles(projectDir, driver); err != nil {
				t.Fatalf("set queue driver in env files: %v", err)
			}

			workerCtx, workerCancel := context.WithCancel(context.Background())
			worker := exec.CommandContext(workerCtx, "./bin/app", "queue:work")
			worker.Dir = projectDir
			worker.Env = append(os.Environ(),
				"QUEUE_DRIVER="+driver,
				"QUEUE_SUPPORTED_DRIVERS=redis,sync,workerpool",
			)
			proc := &procHandle{
				name:   "queue-worker-" + driver,
				cmd:    worker,
				cancel: workerCancel,
			}
			worker.Stdout = &proc.stdout
			worker.Stderr = &proc.stderr
			if err := proc.Start(); err != nil {
				workerCancel()
				t.Fatalf("start queue:work for %s: %v", driver, err)
			}
			defer stopProcAsync(t, "queue-worker-"+driver, proc, time.Second)

			tokens := []string{"Queue worker started", "driver=" + strings.ToLower(driver)}
			if !waitForOutputContains(proc, tokens, 5*time.Second) {
				t.Fatalf("expected queue worker start log for %s, got:\n%s", driver, ansiEscapeRe.ReplaceAllString(proc.Output(), ""))
			}
			stopProcAsync(t, "queue-worker-"+driver, proc, time.Second)
		})
	}
}

func setQueueDriverInEnvFiles(projectDir, driver string) error {
	for _, name := range []string{".env", ".env.host"} {
		path := filepath.Join(projectDir, name)
		if err := testkit.ReplaceOrAppendEnvValue(path, "QUEUE_DRIVER", driver); err != nil {
			return err
		}
	}
	return nil
}
