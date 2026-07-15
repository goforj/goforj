//go:build integration

package forj

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

type procHandle struct {
	name   string
	cmd    *exec.Cmd
	cancel context.CancelFunc
	done   chan error
	stdout bytes.Buffer
	stderr bytes.Buffer
}

var (
	sharedAppOnce    sync.Once
	sharedAppErr     error
	sharedProjectDir string
	sharedBinPath    string
	sharedCleanup    func()
)

func TestMain(m *testing.M) {
	code := m.Run()
	cleanupAuthDatabaseFixtures()
	testkit.CleanupIntegrationHarness()
	if sharedCleanup != nil {
		sharedCleanup()
	}
	os.Exit(code)
}

func (p *procHandle) Output() string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("stdout:\n%s\nstderr:\n%s", p.stdout.String(), p.stderr.String())
}

func (p *procHandle) Start() error {
	if p == nil || p.cmd == nil {
		return fmt.Errorf("invalid process")
	}
	if err := p.cmd.Start(); err != nil {
		return err
	}
	p.done = make(chan error, 1)
	go func() {
		p.done <- p.cmd.Wait()
	}()
	return nil
}

func (p *procHandle) Stop() {
	if p == nil {
		return
	}
	if p.cancel != nil {
		p.cancel()
	}
	if p.done == nil {
		return
	}
	select {
	case <-p.done:
		return
	case <-time.After(300 * time.Millisecond):
	}
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	select {
	case <-p.done:
	case <-time.After(300 * time.Millisecond):
	}
}

func (p *procHandle) ExitError() error {
	if p == nil || p.done == nil {
		return nil
	}
	select {
	case err := <-p.done:
		if err != nil {
			return fmt.Errorf("%s exited: %v\nstdout:\n%s\nstderr:\n%s", p.name, err, p.stdout.String(), p.stderr.String())
		}
		return fmt.Errorf("%s exited unexpectedly\nstdout:\n%s\nstderr:\n%s", p.name, p.stdout.String(), p.stderr.String())
	default:
		return nil
	}
}

func stopProcAsync(t *testing.T, label string, proc *procHandle, timeout time.Duration) {
	t.Helper()
	if proc == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		proc.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Logf("stop timeout for %s", label)
	}
}

func waitForTCP(t *testing.T, addr string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func findFreeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func renderAppAtDir(t *testing.T, dir string) {
	t.Helper()
	testkit.RenderProjectWithForj(t, dir, testkit.RenderProjectRequest{
		Config: project.Config{
			ProjectName:  "TestApp",
			GoModuleName: "example.com/testapp",
			UpdatedAt:    "2026-01-01 00:00:00 UTC",
			Render: project.RenderConfig{
				Components: project.Components{
					Cache:     true,
					Events:    true,
					WebAPI:    true,
					WebUI:     true,
					Scheduler: true,
					Storage:   true,
					Jobs:      true,
				},
			},
		},
	})
}

func buildRenderedDefaultApp(t *testing.T, projectDir string, env map[string]string, label string) string {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "app")
	buildRenderedDefaultAppTo(t, projectDir, binPath, env, label)
	return binPath
}

func buildRenderedDefaultAppTo(t *testing.T, projectDir string, binPath string, env map[string]string, label string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
		t.Fatalf("mkdir bin dir: %v", err)
	}
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/app")
	cmd.Dir = projectDir
	cmd.Env = testkit.IntegrationGoProcessEnv(t, env)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		if label == "" {
			label = "build rendered app"
		}
		t.Fatalf("%s: %v\n%s", label, err, out.String())
	}
}

// selectRenderedDemoSQLite exercises Demo's built-in SQLite fallback without changing its normal MySQL starting contract.
func selectRenderedDemoSQLite(t *testing.T, projectDir string) {
	t.Helper()
	if err := testkit.ReplaceOrAppendEnvValues(
		[]string{filepath.Join(projectDir, ".env"), filepath.Join(projectDir, ".env.host")},
		map[string]string{"DB_DRIVER": "sqlite"},
	); err != nil {
		t.Fatalf("select rendered Demo SQLite fallback: %v", err)
	}
}

// generateRenderedProject rebuilds generated source after a fixture changes its persisted build contract.
func generateRenderedProject(t *testing.T, projectDir string, env map[string]string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, testkit.EnsureIntegrationForjBinary(t), "generate")
	cmd.Dir = projectDir
	cmd.Env = testkit.IntegrationGoProcessEnv(t, env)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("generate rendered project: %v\n%s", err, out.String())
	}
}
