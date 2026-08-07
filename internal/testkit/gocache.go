package testkit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GoCachePaths centralizes go cache paths behavior so callers follow the same contract.
func GoCachePaths() (string, string) {
	modCache := os.Getenv("GOMODCACHE")
	buildCache := os.Getenv("GOCACHE")

	if modCache == "" {
		modCache = goEnv("GOMODCACHE")
	}
	if buildCache == "" {
		buildCache = goEnv("GOCACHE")
	}

	if modCache == "" || buildCache == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			base = os.TempDir()
		}
		if modCache == "" {
			modCache = filepath.Join(base, "go", "pkg", "mod")
		}
		if buildCache == "" {
			buildCache = filepath.Join(base, "go-build")
		}
	}

	_ = os.MkdirAll(modCache, 0o755)
	_ = os.MkdirAll(buildCache, 0o755)

	return modCache, buildCache
}

// goEnv centralizes go env behavior so callers follow the same contract.
func goEnv(key string) string {
	cmd := exec.Command("go", "env", key)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
