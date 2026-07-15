package testexec_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/testexec"
)

const (
	helperModeEnv       = "FORJ_TESTEXEC_HELPER_MODE"
	helperDirEnv        = "FORJ_TESTEXEC_EXPECT_DIR"
	helperModCacheEnv   = "FORJ_TESTEXEC_EXPECT_MOD_CACHE"
	helperBuildCacheEnv = "FORJ_TESTEXEC_EXPECT_BUILD_CACHE"
)

// TestWorkspaceRunAppliesWorkspacePolicy verifies subprocesses receive the bound directory and Go caches.
func TestWorkspaceRunAppliesWorkspacePolicy(t *testing.T) {
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
		testexec.NewGoCaches(modCache, buildCache),
	)
	if err := workspace.Run("verify workspace", os.Args[0], "-test.run=^TestWorkspaceHelperProcess$"); err != nil {
		t.Fatalf("run workspace helper: %v", err)
	}
}

// TestWorkspaceRunReturnsCommandFailure verifies a failed subprocess remains visible to its caller.
func TestWorkspaceRunReturnsCommandFailure(t *testing.T) {
	t.Setenv(helperModeEnv, "fail")
	workspace := testexec.NewWorkspace(logger.NewSilentLogger(), true, t.TempDir(), testexec.GoCaches{})

	if err := workspace.Run("failing workspace", os.Args[0], "-test.run=^TestWorkspaceHelperProcess$"); err == nil {
		t.Fatal("expected workspace command failure")
	}
}

// TestWorkspaceHelperProcess provides a subprocess that can inspect the execution policy applied by Workspace.
func TestWorkspaceHelperProcess(t *testing.T) {
	switch os.Getenv(helperModeEnv) {
	case "":
		return
	case "fail":
		os.Exit(7)
	case "verify":
		assertHelperValue(t, "working directory", currentDir(t), os.Getenv(helperDirEnv))
		assertHelperValue(t, "GOMODCACHE", os.Getenv("GOMODCACHE"), os.Getenv(helperModCacheEnv))
		assertHelperValue(t, "GOCACHE", os.Getenv("GOCACHE"), os.Getenv(helperBuildCacheEnv))
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
