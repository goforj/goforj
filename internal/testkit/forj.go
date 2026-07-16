package testkit

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goforj/execx"
)

// BuiltForj owns a CLI binary built from the current repository source.
type BuiltForj struct {
	// Path is the absolute path to the built CLI executable.
	Path string

	buildDir string
}

// Cleanup releases the temporary build directory when the caller no longer needs the executable.
// Repeated calls are safe so lifecycle cleanup can remain idempotent.
func (built BuiltForj) Cleanup() {
	_ = os.RemoveAll(built.buildDir)
}

// BuildForjBinary builds the current CLI source and returns ownership of its temporary executable.
func BuildForjBinary(modCache, buildCache string) (BuiltForj, error) {
	root, err := RepoRoot()
	if err != nil {
		return BuiltForj{}, err
	}
	tempDir, err := os.MkdirTemp("", "forj_exec_")
	if err != nil {
		return BuiltForj{}, fmt.Errorf("create temp forj build dir: %w", err)
	}
	absoluteTempDir, err := absoluteForjBuildDir(tempDir)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return BuiltForj{}, err
	}
	tempDir = absoluteTempDir
	binaryPath := filepath.Join(tempDir, "forj")
	cmd := execx.Command("go", "build", "-o", binaryPath, "./cmd/forj").
		Dir(root).
		EnvAppend(map[string]string{
			"GOMODCACHE": modCache,
			"GOCACHE":    buildCache,
			"GOFLAGS":    "",
			"GOWORK":     "off",
		})
	res, err := cmd.Run()
	if err != nil || !res.OK() {
		_ = os.RemoveAll(tempDir)
		errMsg := ""
		if res.Stderr != "" {
			errMsg = res.Stderr
		} else if res.Stdout != "" {
			errMsg = res.Stdout
		}
		if errMsg == "" && err != nil {
			errMsg = err.Error()
		}
		if errMsg == "" {
			errMsg = "go build failed"
		}
		return BuiltForj{}, fmt.Errorf("build current forj binary: %s", errMsg)
	}
	return BuiltForj{
		Path:     binaryPath,
		buildDir: tempDir,
	}, nil
}

// absoluteForjBuildDir keeps the output path stable when the build command runs from the repository root.
func absoluteForjBuildDir(buildDir string) (string, error) {
	absoluteDir, err := filepath.Abs(buildDir)
	if err != nil {
		return "", fmt.Errorf("resolve absolute temp forj build dir: %w", err)
	}
	return absoluteDir, nil
}
