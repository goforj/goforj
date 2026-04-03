//go:build integration

package forj

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
)

func TestRenderedAppReadinessFailsWhenDatabaseUnavailable(t *testing.T) {
	if _, err := exec.LookPath("wire"); err != nil {
		t.Skip("wire is required for rendered app integration tests")
	}

	projectDir := t.TempDir()
	renderReadinessTestApp(t, projectDir)

	if err := os.MkdirAll(filepath.Join(projectDir, "storage", "app", "private"), 0o755); err != nil {
		t.Fatalf("create private storage dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "storage", "app", "public"), 0o755); err != nil {
		t.Fatalf("create public storage dir: %v", err)
	}

	dbAddr := findFreeAddr(t)
	_, dbPort, err := net.SplitHostPort(dbAddr)
	if err != nil {
		t.Fatalf("split db addr: %v", err)
	}
	writeReadinessTestEnv(t, projectDir, dbPort)

	binPath := filepath.Join(t.TempDir(), "app")
	modCache, buildCache := getCachePaths()
	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	buildCmd.Dir = projectDir
	buildCmd.Env = append(os.Environ(),
		"GOMODCACHE="+modCache,
		"GOCACHE="+buildCache,
	)
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build rendered app: %v\n%s", err, out)
	}

	httpAddr := findFreeAddr(t)
	_, httpPort, err := net.SplitHostPort(httpAddr)
	if err != nil {
		t.Fatalf("split http addr: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, "http:serve", "--port", httpPort)
	cmd.Dir = projectDir
	handle := &procHandle{
		name:   "api",
		cmd:    cmd,
		cancel: cancel,
	}
	cmd.Stdout = &handle.stdout
	cmd.Stderr = &handle.stderr
	if err := handle.Start(); err != nil {
		t.Fatalf("start rendered app: %v", err)
	}
	defer stopProcAsync(t, "server", handle, time.Second)

	baseURL := "http://127.0.0.1:" + httpPort
	waitForProbeEndpoint(t, baseURL+"/-/health", http.StatusOK)

	resp, err := http.Get(baseURL + "/-/ready")
	if err != nil {
		t.Fatalf("get readiness endpoint: %v\n%s", err, handle.Output())
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read readiness response: %v", err)
	}

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("GET /-/ready status = %d, want %d\nbody:\n%s\n%s", resp.StatusCode, http.StatusServiceUnavailable, body, handle.Output())
	}
	bodyText := string(body)
	if !strings.Contains(bodyText, `"status":"not_ready"`) {
		t.Fatalf("GET /-/ready body missing not_ready status:\n%s", bodyText)
	}
	if !strings.Contains(bodyText, `"db_default"`) {
		t.Fatalf("GET /-/ready body missing db_default failure:\n%s", bodyText)
	}
}

func renderReadinessTestApp(t *testing.T, dir string) {
	t.Helper()

	cfg := project.Config{
		ProjectName:  "Readiness Test App",
		GoModuleName: "example.com/readinesstestapp",
		UpdatedAt:    "2026-04-03 00:00:00 UTC",
		Render: project.RenderConfig{
			Components: project.Components{
				WebAPI:        true,
				CLI:           true,
				DatabaseMySQL: true,
			},
		},
	}

	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(orig) }()

	writeProjectConfigFile(t, dir, cfg)
	renderer := NewProjectRenderer(logger.NewSilentLogger())
	if err := renderer.Render(ComponentRenderInput{renderAll: true}); err != nil {
		t.Fatalf("render readiness test app: %v", err)
	}
}

func writeReadinessTestEnv(t *testing.T, projectDir, dbPort string) {
	t.Helper()

	content := strings.Join([]string{
		"APP_ENV=local",
		"APP_NAME=Readiness Test App",
		"APP_DEBUG=0",
		"DB_DRIVER=mysql",
		"DB_HOST=127.0.0.1",
		"DB_PORT=" + dbPort,
		"DB_DATABASE=db",
		"DB_USERNAME=user",
		"DB_PASSWORD=password",
		"CACHE_DRIVER=memory",
		"STORAGE_DRIVER=local",
		"STORAGE_ROOT=storage/app/private",
		"STORAGE_PUBLIC_DRIVER=local",
		"STORAGE_PUBLIC_ROOT=storage/app/public",
	}, "\n") + "\n"

	if err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte(content), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
}

func waitForProbeEndpoint(t *testing.T, url string, wantStatus int) {
	t.Helper()

	client := &http.Client{Timeout: 200 * time.Millisecond}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == wantStatus {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("probe %s did not return status %d before timeout", url, wantStatus)
}
