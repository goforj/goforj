//go:build integration

package atlas

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

// TestBaselineGuidanceSurvivesProjectLifecycle proves ordinary framework workflows preserve the selected native instructions.
func TestBaselineGuidanceSurvivesProjectLifecycle(t *testing.T) {
	projectRoot := t.TempDir()
	testkit.RenderProjectWithForj(t, projectRoot, testkit.RenderProjectRequest{Config: project.Config{
		ProjectName:  "Guidance Lifecycle",
		GoModuleName: "example.com/guidance-lifecycle",
		UpdatedAt:    "2026-01-01 00:00:00 UTC",
		Render: project.RenderConfig{
			AgentGuidance: project.AgentGuidanceBaseline,
			Components:    project.Components{CLI: true, WebAPI: true},
		},
	}})
	guidancePath := filepath.Join(projectRoot, "AGENTS.md")
	want, err := os.ReadFile(guidancePath)
	if err != nil {
		t.Fatalf("read rendered guidance: %v", err)
	}
	if !bytes.Contains(want, []byte("`forj make:*`")) {
		t.Fatalf("rendered guidance omitted generator workflow:\n%s", want)
	}
	if !bytes.Contains(want, []byte("flat, self-contained, and portable")) || !bytes.Contains(want, []byte("can stand on its own")) {
		t.Fatalf("rendered guidance omitted package-boundary workflow:\n%s", want)
	}

	forjExecutable := testkit.EnsureIntegrationForjBinary(t)
	runGuidanceLifecycleCommand(t, projectRoot, forjExecutable, "build", "-o", filepath.Join(t.TempDir(), "app"))
	assertGuidanceLifecycleContent(t, guidancePath, want, "build")
	runGuidanceLifecycleCommand(t, projectRoot, forjExecutable, "make:controller", "invoices")
	assertGuidanceLifecycleContent(t, guidancePath, want, "make:controller")
	runGuidanceLifecycleCommand(t, projectRoot, forjExecutable, "render")
	assertGuidanceLifecycleContent(t, guidancePath, want, "render")
}

// runGuidanceLifecycleCommand executes one ordinary Project workflow with bounded integration-test ownership.
func runGuidanceLifecycleCommand(t *testing.T, projectRoot, forjExecutable string, arguments ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, forjExecutable, arguments...)
	command.Dir = projectRoot
	command.Env = testkit.IntegrationGoProcessEnv(t, nil)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err != nil {
		t.Fatalf("forj %s failed: %v\n%s", strings.Join(arguments, " "), err, output.String())
	}
}

// assertGuidanceLifecycleContent rejects silent treatment drift after an unrelated framework workflow.
func assertGuidanceLifecycleContent(t *testing.T, path string, want []byte, operation string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read guidance after %s: %v", operation, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("guidance changed after %s\ngot:\n%s\nwant:\n%s", operation, got, want)
	}
}
