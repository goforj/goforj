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
	"github.com/goforj/goforj/project"
)

func TestLifecycleRegistryIntegration(t *testing.T) {
	projectDir := t.TempDir()
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	writeProjectConfigFile(t, ".", project.Config{
		ProjectName:  "LifecycleApp",
		GoModuleName: "example.com/lifecycleapp",
		UpdatedAt:    "2026-01-01 00:00:00 UTC",
		Render: project.RenderConfig{
			QueueDriver: "redis",
			Components: project.Components{},
		},
	})

	renderer := NewProjectRenderer(logger.NewSilentLogger())
	if err := renderer.Render(ComponentRenderInput{renderAll: true}); err != nil {
		t.Fatalf("render failed: %v", err)
	}

	registryPath := filepath.Join("internal", "lifecycle", "lifecycle_registry.go")
	registryCode := `package lifecycle

import (
	"context"
	"errors"
	"os"
)

type Registry struct{}

func NewRegistry() *Registry { return &Registry{} }

func (r *Registry) Register(manager *Manager) {
	appendTrace := func(entry string) {
		tracePath := os.Getenv("LIFECYCLE_TRACE_FILE")
		if tracePath == "" {
			return
		}
		f, err := os.OpenFile(tracePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		defer f.Close()
		_, _ = f.WriteString(entry + "\n")
	}

	manager.On(BeforeStartup, func(context.Context) error {
		appendTrace("before_startup")
		return nil
	})
	manager.On(Startup, func(context.Context) error {
		appendTrace("startup")
		if os.Getenv("LIFECYCLE_FAIL_STARTUP") == "1" {
			return errors.New("startup hook failed")
		}
		return nil
	})
	manager.On(AfterStartup, func(context.Context) error {
		appendTrace("after_startup")
		return nil
	})
	manager.On(BeforeShutdown, func(context.Context) error {
		appendTrace("before_shutdown")
		return nil
	})
	manager.On(Shutdown, func(context.Context) error {
		appendTrace("shutdown_one")
		if os.Getenv("LIFECYCLE_FAIL_SHUTDOWN") == "1" {
			return errors.New("shutdown err one")
		}
		return nil
	})
	manager.On(Shutdown, func(context.Context) error {
		appendTrace("shutdown_two")
		if os.Getenv("LIFECYCLE_FAIL_SHUTDOWN") == "1" {
			return errors.New("shutdown err two")
		}
		return nil
	})
	manager.On(AfterShutdown, func(context.Context) error {
		appendTrace("after_shutdown")
		return nil
	})
}
`
	if err := os.WriteFile(registryPath, []byte(registryCode), 0o644); err != nil {
		t.Fatalf("write lifecycle registry: %v", err)
	}

	buildCtx, buildCancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer buildCancel()
	build := exec.CommandContext(buildCtx, "go", "build", "-o", "./bin/app", ".")
	build.Dir = projectDir
	var buildOut bytes.Buffer
	build.Stdout = &buildOut
	build.Stderr = &buildOut
	if err := build.Run(); err != nil {
		t.Fatalf("build app failed: %v\n%s", err, buildOut.String())
	}

	t.Run("runs hooks in expected order on success", func(t *testing.T) {
		traceFile := filepath.Join(projectDir, "lifecycle.trace")
		_ = os.Remove(traceFile)
		runCtx, runCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer runCancel()

		cmd := exec.CommandContext(runCtx, "./bin/app", "hello:world")
		cmd.Dir = projectDir
		cmd.Env = append(os.Environ(), "LIFECYCLE_TRACE_FILE="+traceFile)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			t.Fatalf("hello:world run failed: %v\n%s", err, out.String())
		}

		trace, err := os.ReadFile(traceFile)
		if err != nil {
			t.Fatalf("read trace file: %v", err)
		}
		got := strings.Fields(strings.TrimSpace(string(trace)))
		want := []string{
			"before_startup",
			"startup",
			"after_startup",
			"before_shutdown",
			"shutdown_two",
			"shutdown_one",
			"after_shutdown",
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("unexpected lifecycle order:\n got: %v\nwant: %v", got, want)
		}
	})

	t.Run("startup failure stops run and skips shutdown", func(t *testing.T) {
		traceFile := filepath.Join(projectDir, "lifecycle-startup-fail.trace")
		_ = os.Remove(traceFile)
		runCtx, runCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer runCancel()

		cmd := exec.CommandContext(runCtx, "./bin/app", "hello:world")
		cmd.Dir = projectDir
		cmd.Env = append(os.Environ(),
			"LIFECYCLE_TRACE_FILE="+traceFile,
			"LIFECYCLE_FAIL_STARTUP=1",
		)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		err := cmd.Run()
		if err == nil {
			t.Fatalf("expected non-zero exit on startup hook failure\n%s", out.String())
		}
		if !strings.Contains(out.String(), "startup hook failed") {
			t.Fatalf("expected startup error output, got:\n%s", out.String())
		}

		trace, err := os.ReadFile(traceFile)
		if err != nil {
			t.Fatalf("read trace file: %v", err)
		}
		got := strings.Fields(strings.TrimSpace(string(trace)))
		want := []string{"before_startup", "startup"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("unexpected trace on startup failure:\n got: %v\nwant: %v", got, want)
		}
	})

	t.Run("shutdown errors are aggregated and logged", func(t *testing.T) {
		traceFile := filepath.Join(projectDir, "lifecycle-shutdown-fail.trace")
		_ = os.Remove(traceFile)
		runCtx, runCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer runCancel()

		cmd := exec.CommandContext(runCtx, "./bin/app", "hello:world")
		cmd.Dir = projectDir
		cmd.Env = append(os.Environ(),
			"LIFECYCLE_TRACE_FILE="+traceFile,
			"LIFECYCLE_FAIL_SHUTDOWN=1",
		)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			t.Fatalf("expected command success despite shutdown errors: %v\n%s", err, out.String())
		}
		if !strings.Contains(out.String(), "Application shutdown failed") {
			t.Fatalf("expected shutdown failure log, got:\n%s", out.String())
		}
		if !strings.Contains(out.String(), "shutdown err one") || !strings.Contains(out.String(), "shutdown err two") {
			t.Fatalf("expected aggregated shutdown errors in output, got:\n%s", out.String())
		}
	})
}
