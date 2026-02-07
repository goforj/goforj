//go:build integration

package forj

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
  mysql: false
  postgres: false
  sqlite: true
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
		filepath.Join("internal", "jobs", "monitor_check_job.go"),
		filepath.Join("frontend", "src", "views", "MonitoringView.vue"),
		filepath.Join("frontend", "src", "views", "StatusPublicView.vue"),
	}
	for _, path := range required {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
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

