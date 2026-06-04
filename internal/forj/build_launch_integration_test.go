//go:build integration

package forj

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

func TestBuildAutoRunAndCompiledEnvModes(t *testing.T) {
	projectDir := t.TempDir()
	testkit.RenderProjectWithForj(t, projectDir, testkit.RenderProjectRequest{
		Config: project.Config{
			ProjectName:  "BuildLaunch",
			GoModuleName: "example.com/buildlaunch",
			UpdatedAt:    "2026-05-24 00:00:00 UTC",
			Render: project.RenderConfig{
				Components: project.Components{
					CLI: true,
				},
			},
		},
	})

	writeIntegrationMain(t, projectDir)

	buildForj := testkit.EnsureIntegrationForjBinary(t)
	buildEnv := testkit.IntegrationGoProcessEnv(t, nil)
	runBuild := func(args ...string) {
		t.Helper()
		cmd := exec.Command(buildForj, args...)
		cmd.Dir = projectDir
		cmd.Env = buildEnv
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			t.Fatalf("forj %s failed: %v\n%s", strings.Join(args, " "), err, out.String())
		}
	}

	runBuild("build", "--skip-wire", "--auto-run", `--env-defaults=INTEGRATION_MODE=default,APP_ENV=staging`, `--env-overrides=FORCED_MODE=forced,APP_ENV=production`)

	appPath := filepath.Join(projectDir, "bin", "app")

	t.Run("no args uses run and compiled env semantics", func(t *testing.T) {
		cmd := exec.Command(appPath)
		cmd.Dir = projectDir
		cmd.Env = append(os.Environ(),
			"FORCED_MODE=from-os",
			"INTEGRATION_MODE=from-os",
		)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			t.Fatalf("run compiled app: %v\n%s", err, out.String())
		}
		got := strings.TrimSpace(out.String())
		wantParts := []string{
			"mode=from-env",
			"forced=forced",
			"app_env=production",
		}
		for _, part := range wantParts {
			if !strings.Contains(got, part) {
				t.Fatalf("expected %q in output, got %q", part, got)
			}
		}
	})

	t.Run("explicit args bypass auto-run", func(t *testing.T) {
		cmd := exec.Command(appPath, "--help")
		cmd.Dir = projectDir
		cmd.Env = os.Environ()
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			t.Fatalf("run explicit command: %v\n%s", err, out.String())
		}
		got := strings.TrimSpace(out.String())
		if got != "args=[--help]" {
			t.Fatalf("expected explicit args to bypass auto-run, got %q", got)
		}
	})
}

func writeIntegrationMain(t *testing.T, projectDir string) {
	t.Helper()
	mainPath := filepath.Join(projectDir, "cmd", "app", "main.go")
	if err := os.MkdirAll(filepath.Dir(mainPath), 0o755); err != nil {
		t.Fatalf("mkdir cmd/app: %v", err)
	}
	source := `package main

import (
	"fmt"
	"example.com/buildlaunch/app"
	"github.com/goforj/env/v2"
	"os"
	"example.com/buildlaunch/internal/cmd"
)

func main() {
	args := cmd.EffectiveLaunchArgs(os.Args[1:])

	if err := cmd.ApplyCompiledEnvOverrides(); err != nil {
		fmt.Println("override_err=" + err.Error())
		return
	}
	if err := cmd.ApplyCompiledEnvDefaults(); err != nil {
		fmt.Println("default_err=" + err.Error())
		return
	}
	if err := env.Load(); err != nil {
		fmt.Println("load_err=" + err.Error())
		return
	}
	if err := cmd.ApplyCompiledEnvOverrides(); err != nil {
		fmt.Println("reoverride_err=" + err.Error())
		return
	}

	if len(args) == 1 && args[0] == "run" {
		fmt.Printf("mode=%s forced=%s app_env=%s marker=%s\n",
			os.Getenv("INTEGRATION_MODE"),
			os.Getenv("FORCED_MODE"),
			os.Getenv("APP_ENV"),
			os.Getenv("APP_MARKER"),
		)
		return
	}

	if handled, err := cmd.DispatchPrebootCommand(args, &app.RootCmd{}); handled {
		if err != nil {
			fmt.Println("preboot_err=" + err.Error())
		}
		return
	}

	fmt.Printf("args=%v\n", args)
}
`
	if err := os.WriteFile(mainPath, []byte(source), 0o644); err != nil {
		t.Fatalf("write %s: %v", mainPath, err)
	}
	writeIntegrationEnv(t, filepath.Join(projectDir, ".env"), "INTEGRATION_MODE=from-env\nAPP_ENV=local\n")
	writeIntegrationEnv(t, filepath.Join(projectDir, ".env.production"), "APP_MARKER=from-production\n")
	writeIntegrationEnv(t, filepath.Join(projectDir, ".env.staging"), "APP_MARKER=from-staging\n")
}

func writeIntegrationEnv(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
