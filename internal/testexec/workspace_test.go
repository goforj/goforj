package testexec_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/testexec"
)

const (
	helperModeEnv       = "FORJ_TESTEXEC_HELPER_MODE"
	helperDirEnv        = "FORJ_TESTEXEC_EXPECT_DIR"
	helperModCacheEnv   = "FORJ_TESTEXEC_EXPECT_MOD_CACHE"
	helperBuildCacheEnv = "FORJ_TESTEXEC_EXPECT_BUILD_CACHE"
	helperStepEnv       = "FORJ_TESTEXEC_STEP_VALUE"
	helperExpectedStep  = "FORJ_TESTEXEC_EXPECT_STEP_VALUE"
)

// TestWorkspaceExecutionAppliesWorkspacePolicy verifies ordinary and streaming subprocesses share directory and cache isolation.
func TestWorkspaceExecutionAppliesWorkspacePolicy(t *testing.T) {
	dir := t.TempDir()
	modCache := filepath.Join(t.TempDir(), "module")
	buildCache := filepath.Join(t.TempDir(), "build")
	t.Setenv(helperModeEnv, "verify")
	t.Setenv(helperDirEnv, dir)
	t.Setenv(helperModCacheEnv, modCache)
	t.Setenv(helperBuildCacheEnv, buildCache)
	workspace := testexec.NewWorkspace(
		logger.NewSilentLogger(),
		true,
		dir,
		testexec.GoCaches{ModulePath: modCache, BuildPath: buildCache},
	)
	if err := workspace.Run("verify ordinary workspace", os.Args[0], "-test.run=^TestWorkspaceHelperProcess$"); err != nil {
		t.Fatalf("run ordinary workspace helper: %v", err)
	}

	t.Setenv(helperExpectedStep, "step-specific")
	err := workspace.RunStreaming(testexec.StreamingStep{
		Name:        "verify workspace",
		Command:     []string{os.Args[0], "-test.run=^TestWorkspaceHelperProcess$"},
		Environment: map[string]string{helperStepEnv: "step-specific"},
	})
	if err != nil {
		t.Fatalf("run workspace helper: %v", err)
	}
}

// TestWorkspaceRunStreamingReturnsCommandFailure verifies streamed failures retain their step and both output streams.
func TestWorkspaceRunStreamingReturnsCommandFailure(t *testing.T) {
	t.Setenv(helperModeEnv, "fail")
	workspace := testexec.NewWorkspace(logger.NewSilentLogger(), true, t.TempDir(), testexec.GoCaches{})

	err := workspace.RunStreaming(testexec.StreamingStep{
		Name:    "failing workspace",
		Command: []string{os.Args[0], "-test.run=^TestWorkspaceHelperProcess$"},
	})
	if err == nil {
		t.Fatal("expected workspace command failure")
	}
	for _, expected := range []string{"failing workspace", "helper stdout details", "helper stderr details"} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("failure %q does not contain %q", err, expected)
		}
	}
}

// TestWorkspaceExecutionRejectsEmptyCommands keeps invalid command state on the error path instead of panicking at the process boundary.
func TestWorkspaceExecutionRejectsEmptyCommands(t *testing.T) {
	workspace := testexec.NewWorkspace(logger.NewSilentLogger(), true, t.TempDir(), testexec.GoCaches{})
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "ordinary", run: func() error { return workspace.Run("ordinary") }},
		{name: "streaming", run: func() error { return workspace.RunStreaming(testexec.StreamingStep{Name: "streaming"}) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); err == nil || !strings.Contains(err.Error(), "command is required") {
				t.Fatalf("empty command error = %v, want required-command diagnostic", err)
			}
		})
	}
}

// TestWorkspaceHelperProcess provides a subprocess that can inspect the execution policy applied by Workspace.
func TestWorkspaceHelperProcess(t *testing.T) {
	switch os.Getenv(helperModeEnv) {
	case "":
		return
	case "fail":
		_, _ = os.Stdout.WriteString("helper stdout details\n")
		_, _ = os.Stderr.WriteString("helper stderr details\n")
		os.Exit(7)
	case "verify":
		assertHelperValue(t, "working directory", currentDir(t), os.Getenv(helperDirEnv))
		assertHelperValue(t, "GOMODCACHE", os.Getenv("GOMODCACHE"), os.Getenv(helperModCacheEnv))
		assertHelperValue(t, "GOCACHE", os.Getenv("GOCACHE"), os.Getenv(helperBuildCacheEnv))
		if expected := os.Getenv(helperExpectedStep); expected != "" {
			assertHelperValue(t, helperStepEnv, os.Getenv(helperStepEnv), expected)
		}
	default:
		t.Fatalf("unknown helper mode %q", os.Getenv(helperModeEnv))
	}
}

// currentDir resolves the helper's process directory so policy failures identify the operating-system error.
func currentDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve working directory: %v", err)
	}
	return dir
}

// assertHelperValue keeps subprocess policy failures focused on the mismatched setting.
func assertHelperValue(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %q, want %q", name, got, want)
	}
}
