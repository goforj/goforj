package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
)

func TestMakeAppCmdCreatesNamedTarget(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := writeProjectConfig(".goforj.yml", &project.Config{
		ProjectName:  "TestApp",
		GoModuleName: "example.com/testapp",
		Render: project.RenderConfig{
			Components: project.Components{
				WebAPI: true,
			},
		},
	}); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := NewMakeAppCmd(logger.NewSilentLogger(), NewProjectRenderer(logger.NewSilentLogger()))
	cmd.Name = "billing"
	cmd.SkipWire = true
	if err := cmd.Run(); err != nil {
		t.Fatalf("make app: %v", err)
	}

	for _, path := range []string{
		filepath.Join("cmd", "billing", "main.go"),
		filepath.Join("app", "billing", "root_cmd.go"),
		filepath.Join("app", "billing", "routes.go"),
		filepath.Join("app", "billing", "wire", "wire.go"),
		filepath.Join("app", "billing", "wire", "inject_cmd.go"),
		filepath.Join("app", "billing", "wire", "inject_http_controllers_app.go"),
		filepath.Join("internal", "runtime", "targets.go"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}

	mainSrc := readMakeAppTestFile(t, filepath.Join("cmd", "billing", "main.go"))
	if !strings.Contains(mainSrc, `cmd.ApplyLaunchTarget("billing")`) {
		t.Fatalf("expected billing target identity in cmd/billing/main.go")
	}
	runtimeSrc := readMakeAppTestFile(t, filepath.Join("internal", "runtime", "targets.go"))
	if !strings.Contains(runtimeSrc, `Name: "billing"`) || !strings.Contains(runtimeSrc, `HTTPPort: 3001`) {
		t.Fatalf("expected billing runtime target metadata, got:\n%s", runtimeSrc)
	}
}

func TestMakeAppCmdRejectsExistingTargetPaths(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := os.MkdirAll(filepath.Join("cmd", "billing"), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	cmd := NewMakeAppCmd(logger.NewSilentLogger(), NewProjectRenderer(logger.NewSilentLogger()))
	cmd.Name = "billing"
	cmd.SkipWire = true
	err = cmd.Run()
	if err == nil {
		t.Fatal("expected existing target error")
	}
	if !strings.Contains(err.Error(), "already has files") {
		t.Fatalf("expected existing target path error, got %v", err)
	}
}

func TestMakeAppCmdRejectsNativeCommandName(t *testing.T) {
	cmd := NewMakeAppCmd(logger.NewSilentLogger(), NewProjectRenderer(logger.NewSilentLogger()))
	cmd.Name = "render"
	if _, err := cmd.target(); err == nil {
		t.Fatal("expected native command target name error")
	}
}

func readMakeAppTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
