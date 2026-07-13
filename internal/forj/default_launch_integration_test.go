//go:build integration

package forj

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

// TestRenderedDefaultLaunchUsesEachAppsRuntimeCapability proves mixed App binaries keep independent no-argument behavior.
func TestRenderedDefaultLaunchUsesEachAppsRuntimeCapability(t *testing.T) {
	projectDir := t.TempDir()
	if err := writeConventionalAppMarker(projectDir, "ship"); err != nil {
		t.Fatalf("write CLI App marker: %v", err)
	}
	testkit.RenderProjectWithForj(t, projectDir, testkit.RenderProjectRequest{
		Config: project.Config{
			ProjectName:  "DefaultLaunch",
			GoModuleName: "example.com/defaultlaunch",
			UpdatedAt:    "2026-07-13 00:00:00 UTC",
			Render: project.RenderConfig{
				Components: project.Components{CLI: true, Scheduler: true},
			},
			Apps: map[string]project.AppConfig{
				"ship": {Components: project.Components{CLI: true}},
			},
		},
	})
	buildMultiAppRuntimeBinaries(t, projectDir, []multiAppRuntimeSpec{
		{name: "app", packagePath: "./cmd/app"},
		{name: "ship", packagePath: "./cmd/ship"},
	})

	runtimePath := filepath.Join(projectDir, "bin", "app")
	assertGeneratedRuntimeLaunchStaysRunning(t, projectDir, runtimePath)
	assertGeneratedRuntimeLaunchStaysRunning(t, projectDir, runtimePath, "run")
	assertGeneratedLaunchPrintsHelp(t, projectDir, runtimePath, "--help")

	cliPath := filepath.Join(projectDir, "bin", "ship")
	assertGeneratedLaunchPrintsHelp(t, projectDir, cliPath)
	assertGeneratedLaunchPrintsHelp(t, projectDir, cliPath, "--help")
}

// assertGeneratedRuntimeLaunchStaysRunning verifies a runtime command does not return through the help path.
func assertGeneratedRuntimeLaunchStaysRunning(t *testing.T, projectDir string, binary string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = projectDir
	cmd.Env = testkit.IntegrationProcessEnv(t, nil)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start %s %s: %v", filepath.Base(binary), strings.Join(args, " "), err)
	}
	waited := make(chan error, 1)
	go func() {
		waited <- cmd.Wait()
	}()

	select {
	case err := <-waited:
		cancel()
		t.Fatalf("%s %s exited before acting as a runtime: %v\n%s", filepath.Base(binary), strings.Join(args, " "), err, output.String())
	case <-time.After(750 * time.Millisecond):
	}

	cancel()
	select {
	case <-waited:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s %s did not stop after test cancellation", filepath.Base(binary), strings.Join(args, " "))
	}
}

// assertGeneratedLaunchPrintsHelp verifies discovery paths exit instead of starting a runtime.
func assertGeneratedLaunchPrintsHelp(t *testing.T, projectDir string, binary string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = projectDir
	cmd.Env = testkit.IntegrationProcessEnv(t, nil)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s help launch failed: %v\n%s", filepath.Base(binary), strings.Join(args, " "), err, output)
	}
	if !strings.Contains(string(output), "make:command") {
		t.Fatalf("%s %s did not print help:\n%s", filepath.Base(binary), strings.Join(args, " "), output)
	}
}
