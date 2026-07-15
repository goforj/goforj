package testkit

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

	"github.com/goforj/goforj/project"
)

var (
	sharedForjPath  string
	sharedForjOnce  sync.Once
	sharedForjErr   error
	sharedForjStop  func()
	sharedToolsDir  string
	sharedToolsOnce sync.Once
	sharedToolsErr  error
	sharedRedisHost string
	sharedRedisPort string
	sharedRedisOnce sync.Once
	sharedRedisErr  error
	sharedRedisStop func()
)

const integrationWireInstallTarget = "github.com/goforj/wire/cmd/wire@v1.2.0"

type RenderProjectRequest struct {
	Config         project.Config
	EnvOverrides   map[string]string
	ModuleReplaces map[string]string
}

func CleanupIntegrationHarness() {
	if sharedRedisStop != nil {
		sharedRedisStop()
		sharedRedisStop = nil
	}
	if sharedForjStop != nil {
		sharedForjStop()
		sharedForjStop = nil
	}
	if sharedToolsDir != "" {
		_ = os.RemoveAll(sharedToolsDir)
		sharedToolsDir = ""
	}
}

func IntegrationEnvOverrides(t *testing.T) map[string]string {
	t.Helper()

	overrides := map[string]string{}
	if sharedRedisHost != "" {
		overrides["REDIS_HOST"] = sharedRedisHost
	}
	if sharedRedisPort != "" {
		overrides["REDIS_PORT"] = sharedRedisPort
	}
	return overrides
}

func IntegrationProcessEnv(t *testing.T, overrides map[string]string) []string {
	t.Helper()
	return ProcessEnv(EnsureIntegrationToolsDir(t), mergeStringMaps(IntegrationEnvOverrides(t), overrides))
}

func IntegrationGoProcessEnv(t *testing.T, overrides map[string]string) []string {
	t.Helper()
	return ProcessGoEnv(EnsureIntegrationToolsDir(t), mergeStringMaps(IntegrationEnvOverrides(t), overrides))
}

func EnsureIntegrationRedis(t *testing.T) (string, string) {
	t.Helper()

	sharedRedisOnce.Do(func() {
		envHost := strings.TrimSpace(os.Getenv("REDIS_HOST"))
		envPort := strings.TrimSpace(os.Getenv("REDIS_PORT"))
		if envHost != "" && envPort != "" {
			if err := WaitForTCPReadyAddress(envHost, envPort, 2*time.Second); err == nil {
				sharedRedisHost = envHost
				sharedRedisPort = envPort
				return
			}
		}
		env := map[string]string{}
		stop, err := StartRedisTestcontainer(nil, env)
		if err != nil {
			sharedRedisErr = err
			return
		}
		sharedRedisHost = strings.TrimSpace(env["REDIS_HOST"])
		sharedRedisPort = strings.TrimSpace(env["REDIS_PORT"])
		sharedRedisStop = stop
		if sharedRedisHost == "" || sharedRedisPort == "" {
			sharedRedisErr = fmt.Errorf("redis testcontainer did not expose REDIS_HOST/REDIS_PORT")
		}
	})
	if sharedRedisErr != nil {
		t.Fatal(sharedRedisErr)
	}
	return sharedRedisHost, sharedRedisPort
}

func EnsureIntegrationToolsDir(t *testing.T) string {
	t.Helper()

	sharedToolsOnce.Do(func() {
		toolsDir, err := os.MkdirTemp("", "forj-integration-tools-*")
		if err != nil {
			sharedToolsErr = fmt.Errorf("create integration tools dir: %w", err)
			return
		}
		sharedToolsDir = toolsDir
		installTarget := integrationWireInstallTarget
		toolName := filepath.Base(strings.SplitN(installTarget, "@", 2)[0])
		toolPath := filepath.Join(toolsDir, toolName)
		buildEnv := append(BuildEnv(), "GOBIN="+toolsDir)
		installCmd := exec.Command("go", "install", installTarget)
		installCmd.Dir = integrationRepoRoot(t)
		installCmd.Env = buildEnv
		var installOut bytes.Buffer
		installCmd.Stdout = &installOut
		installCmd.Stderr = &installOut
		if err := installCmd.Run(); err != nil {
			sharedToolsErr = fmt.Errorf("install integration tool %q: %w\n%s", toolName, err, installOut.String())
			return
		}
		if _, err := os.Stat(toolPath); err != nil {
			sharedToolsErr = fmt.Errorf("integration tool %q missing after install: %w", toolName, err)
			return
		}
	})
	if sharedToolsErr != nil {
		t.Fatal(sharedToolsErr)
	}
	return sharedToolsDir
}

func EnsureIntegrationForjBinary(t *testing.T) string {
	t.Helper()

	if provided := strings.TrimSpace(os.Getenv("FORJ_INTEGRATION_FORJ_PATH")); provided != "" {
		if _, statErr := os.Stat(provided); statErr == nil {
			return provided
		} else {
			t.Fatalf("FORJ_INTEGRATION_FORJ_PATH is set but invalid: %v", statErr)
		}
	}

	sharedForjOnce.Do(func() {
		repoRoot := integrationRepoRoot(t)
		downloadCmd := exec.Command("go", "mod", "download")
		downloadCmd.Dir = repoRoot
		downloadCmd.Env = BuildEnv()
		var downloadOut bytes.Buffer
		downloadCmd.Stdout = &downloadOut
		downloadCmd.Stderr = &downloadOut
		if err := downloadCmd.Run(); err != nil {
			sharedForjErr = fmt.Errorf("prepare forj module deps: %w\nrepo: %s\n%s", err, repoRoot, downloadOut.String())
			return
		}

		builtForj, err := BuildForjBinary("/tmp/gomodcache", "/tmp/gocache")
		if err != nil {
			gomodOut, _ := exec.Command("go", "env", "GOMOD", "GOWORK").CombinedOutput()
			sharedForjErr = fmt.Errorf("build forj binary: %w\nrepo: %s\ngo env:\n%s", err, repoRoot, strings.TrimSpace(string(gomodOut)))
			return
		}
		sharedForjPath = builtForj.Path
		sharedForjStop = builtForj.Cleanup
	})
	if sharedForjErr != nil {
		t.Fatal(sharedForjErr)
	}
	return sharedForjPath
}

func RenderProjectWithForj(t *testing.T, dir string, request RenderProjectRequest) {
	t.Helper()

	cfg := request.Config
	if len(request.ModuleReplaces) > 0 {
		if cfg.Render.ModuleReplaces == nil {
			cfg.Render.ModuleReplaces = map[string]string{}
		}
		for module, target := range request.ModuleReplaces {
			module = strings.TrimSpace(module)
			target = strings.TrimSpace(target)
			if module == "" || target == "" {
				continue
			}
			cfg.Render.ModuleReplaces[module] = target
		}
	}

	WriteProjectConfigFile(t, dir, cfg)
	_ = EnsureIntegrationToolsDir(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, EnsureIntegrationForjBinary(t), "render")
	cmd.Dir = dir
	cmd.Env = WithEnvOverrides(IntegrationProcessEnv(t, nil), request.EnvOverrides)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("render failed: %v\n%s", err, out.String())
	}
}

func LocalSiblingRepoPath(t *testing.T, name string) string {
	t.Helper()

	name = strings.TrimSpace(name)
	if name == "" {
		t.Fatal("local sibling repo name is required")
	}

	root := integrationRepoRoot(t)
	parent := filepath.Dir(root)
	path := filepath.Join(parent, name)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("resolve sibling repo %q: %v", name, err)
	}
	if !info.IsDir() {
		t.Fatalf("sibling repo %q is not a directory: %s", name, path)
	}
	return path
}

// WriteProjectConfigFile writes a current-contract configuration for an integration render.
func WriteProjectConfigFile(t *testing.T, dir string, cfg project.Config) {
	t.Helper()
	if err := WriteProjectConfig(filepath.Join(dir, ".goforj.yml"), cfg); err != nil {
		t.Fatalf("write .goforj.yml: %v", err)
	}
}

func integrationRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := RepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func mergeStringMaps(base map[string]string, overrides map[string]string) map[string]string {
	if len(base) == 0 && len(overrides) == 0 {
		return map[string]string{}
	}
	merged := make(map[string]string, len(base)+len(overrides))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range overrides {
		merged[key] = value
	}
	return merged
}
