package testkit

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goforj/execx"
)

func BuildForjBinary(modCache, buildCache string) (string, func(), error) {
	root, err := RepoRoot()
	if err != nil {
		return "", nil, err
	}
	tempDir, err := os.MkdirTemp("", "forj_exec_")
	if err != nil {
		return "", nil, fmt.Errorf("create temp forj build dir: %w", err)
	}
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
		return "", nil, fmt.Errorf("build current forj binary: %s", errMsg)
	}
	return binaryPath, func() { _ = os.RemoveAll(tempDir) }, nil
}
