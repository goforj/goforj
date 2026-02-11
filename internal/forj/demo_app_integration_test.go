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

	"github.com/goforj/goforj/internal/logger"
)

func TestDemoAppRenderIntegration(t *testing.T) {
	projectDir := t.TempDir()
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	config := `project_name: DemoApp
module_name: example.com/demoapp
updated_at: 2026-01-01 00:00:00 UTC
components:
  web_api: true
  web_ui: true
  scheduler: true
  jobs: true
  docker: false
  database_mysql: false
  database_postgres: false
  database_sqlite: true
  demo_app: true
`
	if err := os.WriteFile(".goforj.yml", []byte(config), 0o644); err != nil {
		t.Fatalf("write .goforj.yml: %v", err)
	}

	renderer := NewProjectRenderer(logger.NewSilentLogger())
	if err := renderer.Render(ComponentRenderInput{renderAll: true}); err != nil {
		t.Fatalf("render failed: %v", err)
	}

	required := []string{
		filepath.Join("internal", "monitoring", "controller.go"),
		filepath.Join("internal", "monitoring", "heartbeat_bucketing_test.go"),
		filepath.Join("internal", "monitoring", "check_service.go"),
		filepath.Join("internal", "monitoring", "monitor_check_job.go"),
		filepath.Join("internal", "monitoring", "incident_transition_service.go"),
		filepath.Join("frontend", "src", "views", "MonitoringView.vue"),
		filepath.Join("frontend", "src", "views", "StatusPublicView.vue"),
		filepath.Join("internal", "migrations", "2026_02_11_000012_demo_monitor_alert_policy_columns.sqlite.up.sql"),
		filepath.Join("internal", "migrations", "2026_02_11_000013_demo_incident_open_uniqueness.sqlite.up.sql"),
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

	monitorPollPath := filepath.Join("internal", "cmd", "monitor_poll_cmd.go")
	monitorPollSrc, err := os.ReadFile(monitorPollPath)
	if err != nil {
		t.Fatalf("read %s: %v", monitorPollPath, err)
	}
	for _, token := range []string{"MonitorID string", "Sync bool", "JSON bool", "RunNow(", "QueueNow(", "printJSON("} {
		if !strings.Contains(string(monitorPollSrc), token) {
			t.Fatalf("expected %q in %s", token, monitorPollPath)
		}
	}

	retentionCmdPath := filepath.Join("internal", "cmd", "monitor_retention_cmd.go")
	if _, err := os.Stat(retentionCmdPath); err != nil {
		t.Fatalf("expected %s: %v", retentionCmdPath, err)
	}

	pushTriggerCmdPath := filepath.Join("internal", "cmd", "push_monitor_trigger_cmd.go")
	if _, err := os.Stat(pushTriggerCmdPath); err != nil {
		t.Fatalf("expected %s: %v", pushTriggerCmdPath, err)
	}
	legacyPushTriggerCmdPath := filepath.Join("internal", "cmd", "demo_push_monitor_trigger_cmd.go")
	if _, err := os.Stat(legacyPushTriggerCmdPath); !os.IsNotExist(err) {
		t.Fatalf("expected legacy file to be removed: %s", legacyPushTriggerCmdPath)
	}

	schedulerRegistryPath := filepath.Join("internal", "scheduler", "scheduler_registry.go")
	schedulerRegistrySrc, err := os.ReadFile(schedulerRegistryPath)
	if err != nil {
		t.Fatalf("read %s: %v", schedulerRegistryPath, err)
	}
	for _, token := range []string{`Command("monitor:retention")`, `Command("monitor:poll")`, `Command("push-monitor-trigger")`} {
		if !strings.Contains(string(schedulerRegistrySrc), token) {
			t.Fatalf("expected %q in %s", token, schedulerRegistryPath)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "./internal/monitoring", "./internal/jobs")
	cmd.Dir = projectDir
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

	config := `project_name: DemoQueueDrivers
module_name: example.com/demoqueuedrivers
updated_at: 2026-01-01 00:00:00 UTC
components:
  web_api: true
  web_ui: false
  scheduler: true
  jobs: true
  docker: false
  database_mysql: false
  database_postgres: false
  database_sqlite: true
  demo_app: true
`
	if err := os.WriteFile(".goforj.yml", []byte(config), 0o644); err != nil {
		t.Fatalf("write .goforj.yml: %v", err)
	}

	renderer := NewProjectRenderer(logger.NewSilentLogger())
	if err := renderer.Render(ComponentRenderInput{renderAll: true}); err != nil {
		t.Fatalf("render failed: %v", err)
	}

	buildCtx, buildCancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer buildCancel()
	build := exec.CommandContext(buildCtx, "go", "build", "-o", "./bin/app", ".")
	build.Dir = projectDir
	var buildOut bytes.Buffer
	build.Stdout = &buildOut
	build.Stderr = &buildOut
	if err := build.Run(); err != nil {
		t.Fatalf("build app failed: %v\n%s", err, buildOut.String())
	}

	for _, driver := range []string{"sync", "memory"} {
		t.Run(driver, func(t *testing.T) {
			if err := setQueueDriverInEnvFiles(projectDir, driver); err != nil {
				t.Fatalf("set queue driver in env files: %v", err)
			}

			helloCtx, helloCancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer helloCancel()
			hello := exec.CommandContext(helloCtx, "./bin/app", "queue:hello-test")
			hello.Dir = projectDir
			hello.Env = append(os.Environ(), "QUEUE_DRIVER="+driver)
			var helloOut bytes.Buffer
			hello.Stdout = &helloOut
			hello.Stderr = &helloOut
			if err := hello.Run(); err != nil {
				t.Fatalf("queue:hello-test failed for %s: %v\n%s", driver, err, helloOut.String())
			}
			if !strings.Contains(helloOut.String(), "Hello ") {
				t.Fatalf("expected hello output for %s, got:\n%s", driver, helloOut.String())
			}

			workerCtx, workerCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer workerCancel()
			worker := exec.CommandContext(workerCtx, "./bin/app", "queue:work")
			worker.Dir = projectDir
			worker.Env = append(os.Environ(), "QUEUE_DRIVER="+driver)
			var workerOut bytes.Buffer
			worker.Stdout = &workerOut
			worker.Stderr = &workerOut
			if err := worker.Run(); err != nil {
				t.Fatalf("queue:work failed for %s: %v\n%s", driver, err, workerOut.String())
			}
			if !strings.Contains(workerOut.String(), "Queue worker not required for non-redis driver") {
				t.Fatalf("expected non-redis queue worker message for %s, got:\n%s", driver, workerOut.String())
			}
		})
	}
}

func setQueueDriverInEnvFiles(projectDir, driver string) error {
	for _, name := range []string{".env", ".env.host"} {
		path := filepath.Join(projectDir, name)
		if err := replaceOrAppendEnvValue(path, "QUEUE_DRIVER", driver); err != nil {
			return err
		}
	}
	return nil
}

func replaceOrAppendEnvValue(path, key, value string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(content), "\n")
	prefix := key + "="
	replaced := false
	for i := range lines {
		if strings.HasPrefix(lines[i], prefix) {
			lines[i] = prefix + value
			replaced = true
		}
	}
	if !replaced {
		lines = append(lines, prefix+value)
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}
