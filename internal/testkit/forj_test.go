package testkit

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBuiltForjCleanupIsIdempotent verifies shared harness teardown can safely release a build more than once.
func TestBuiltForjCleanupIsIdempotent(t *testing.T) {
	buildDir := t.TempDir()
	binaryPath := filepath.Join(buildDir, "forj")
	builtForj := BuiltForj{
		Path:     binaryPath,
		buildDir: buildDir,
	}

	builtForj.Cleanup()
	builtForj.Cleanup()

	if _, err := os.Stat(buildDir); !os.IsNotExist(err) {
		t.Fatalf("build dir stat error = %v, want not exist", err)
	}
}

// TestAbsoluteForjBuildDir verifies a relative TMPDIR cannot make the child build resolve its output beneath the repository.
func TestAbsoluteForjBuildDir(t *testing.T) {
	buildDir, err := absoluteForjBuildDir(filepath.Join("relative-tmp", "forj_exec_123"))
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(buildDir) {
		t.Fatalf("build dir = %q, want absolute path", buildDir)
	}
}
