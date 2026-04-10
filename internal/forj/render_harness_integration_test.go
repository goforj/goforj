//go:build integration

package forj

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

var (
	integrationForjPath  string
	integrationForjOnce  sync.Once
	integrationForjErr   error
	integrationForjStop  func()
	integrationToolsDir  string
	integrationToolsOnce sync.Once
	integrationToolsErr  error
	integrationRedisHost string
	integrationRedisPort string
	integrationRedisOnce sync.Once
	integrationRedisErr  error
	integrationRedisStop func()
)

func integrationEnvOverrides() map[string]string {
	overrides := map[string]string{}
	if integrationRedisHost != "" {
		overrides["REDIS_HOST"] = integrationRedisHost
	}
	if integrationRedisPort != "" {
		overrides["REDIS_PORT"] = integrationRedisPort
	}
	return overrides
}

func integrationProcessEnv() []string {
	return testkit.ProcessEnv(integrationToolsDir, integrationEnvOverrides())
}

func integrationGoProcessEnv(overrides map[string]string) []string {
	merged := integrationEnvOverrides()
	for key, value := range overrides {
		merged[key] = value
	}
	return testkit.ProcessGoEnv(integrationToolsDir, merged)
}

func ensureIntegrationRedis(t *testing.T) (string, string) {
	t.Helper()

	integrationRedisOnce.Do(func() {
		env := map[string]string{}
		stop, err := testkit.StartRedisTestcontainer(nil, env)
		if err != nil {
			integrationRedisErr = err
			return
		}
		integrationRedisHost = strings.TrimSpace(env["REDIS_HOST"])
		integrationRedisPort = strings.TrimSpace(env["REDIS_PORT"])
		integrationRedisStop = stop
		if integrationRedisHost == "" || integrationRedisPort == "" {
			integrationRedisErr = fmt.Errorf("redis testcontainer did not expose REDIS_HOST/REDIS_PORT")
		}
	})
	if integrationRedisErr != nil {
		t.Fatal(integrationRedisErr)
	}
	return integrationRedisHost, integrationRedisPort
}

func ensureIntegrationWireTool(t *testing.T) string {
	t.Helper()

	integrationToolsOnce.Do(func() {
		toolsDir, err := os.MkdirTemp("", "forj-integration-tools-*")
		if err != nil {
			integrationToolsErr = fmt.Errorf("create integration tools dir: %w", err)
			return
		}
		wirePath := filepath.Join(toolsDir, "wire")
		buildEnv := append(testkit.BuildEnv(),
			"GOBIN="+toolsDir,
		)
		installCmd := exec.Command("go", "install", wireInstallTarget)
		installCmd.Dir = integrationRepoRoot(t)
		installCmd.Env = buildEnv
		var installOut bytes.Buffer
		installCmd.Stdout = &installOut
		installCmd.Stderr = &installOut
		if err := installCmd.Run(); err != nil {
			integrationToolsErr = fmt.Errorf("install integration wire tool: %w\n%s", err, installOut.String())
			return
		}
		if _, err := os.Stat(wirePath); err != nil {
			integrationToolsErr = fmt.Errorf("integration wire tool missing after install: %w", err)
			return
		}
		integrationToolsDir = toolsDir
	})
	if integrationToolsErr != nil {
		t.Fatal(integrationToolsErr)
	}
	return integrationToolsDir
}

func ensureIntegrationForjBinary(t *testing.T) string {
	t.Helper()

	if provided := strings.TrimSpace(os.Getenv("FORJ_INTEGRATION_FORJ_PATH")); provided != "" {
		if _, statErr := os.Stat(provided); statErr == nil {
			return provided
		} else {
			t.Fatalf("FORJ_INTEGRATION_FORJ_PATH is set but invalid: %v", statErr)
		}
	}

	integrationForjOnce.Do(func() {
		repoRoot := integrationRepoRoot(t)
		downloadCmd := exec.Command("go", "mod", "download")
		downloadCmd.Dir = repoRoot
		downloadCmd.Env = testkit.BuildEnv()
		var downloadOut bytes.Buffer
		downloadCmd.Stdout = &downloadOut
		downloadCmd.Stderr = &downloadOut
		if err := downloadCmd.Run(); err != nil {
			integrationForjErr = fmt.Errorf("prepare forj module deps: %w\nrepo: %s\n%s", err, repoRoot, downloadOut.String())
			return
		}

		binPath, cleanup, err := testkit.BuildForjBinary("/tmp/gomodcache", "/tmp/gocache")
		if err != nil {
			gomodOut, _ := exec.Command("go", "env", "GOMOD", "GOWORK").CombinedOutput()
			integrationForjErr = fmt.Errorf("build forj binary: %w\nrepo: %s\ngo env:\n%s", err, repoRoot, strings.TrimSpace(string(gomodOut)))
			return
		}
		integrationForjPath = binPath
		integrationForjStop = cleanup
	})
	if integrationForjErr != nil {
		t.Fatal(integrationForjErr)
	}
	return integrationForjPath
}

func integrationRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := testkit.RepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func renderProjectWithForj(t *testing.T, dir string, cfg project.Config, env map[string]string) {
	t.Helper()

	writeProjectConfigFile(t, dir, cfg)
	_ = ensureIntegrationWireTool(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, ensureIntegrationForjBinary(t), "render")
	cmd.Dir = dir
	cmd.Env = testkit.WithEnvOverrides(integrationProcessEnv(), env)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("render failed: %v\n%s", err, out.String())
	}
}
