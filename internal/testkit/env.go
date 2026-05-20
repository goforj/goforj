package testkit

import (
	"os"
	"strings"
)

func cacheEnv() map[string]string {
	modCache, buildCache := GoCachePaths()
	return map[string]string{
		"GOMODCACHE": modCache,
		"GOCACHE":    buildCache,
		"GOFLAGS":    "",
		"GOWORK":     "off",
	}
}

func BuildEnv() []string {
	env := append([]string{}, os.Environ()...)
	return WithEnvOverrides(env, cacheEnv())
}

func ProcessEnv(toolsDir string, overrides map[string]string) []string {
	base := []string{}
	for _, key := range []string{
		"PATH", "HOME", "TMPDIR", "TMP", "TEMP",
		"GOPROXY", "GOSUMDB", "GOPRIVATE", "GONOPROXY", "GONOSUMDB", "GONOPRIVATE",
		"GOVCS", "GOFLAGS", "GOTOOLCHAIN", "GOPATH", "GOROOT",
		"DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH",
		"TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE", "TESTCONTAINERS_HOST_OVERRIDE",
		"REDIS_HOST", "REDIS_PORT",
	} {
		if value := os.Getenv(key); value != "" {
			base = append(base, key+"="+value)
		}
	}
	if toolsDir != "" {
		base = append(base, "PATH="+toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	base = append(base,
		"GOFLAGS=",
		"GOWORK=off",
	)
	return WithEnvOverrides(base, overrides)
}

func ProcessGoEnv(toolsDir string, overrides map[string]string) []string {
	merged := map[string]string{}
	for key, value := range cacheEnv() {
		merged[key] = value
	}
	for key, value := range overrides {
		merged[key] = value
	}
	return ProcessEnv(toolsDir, merged)
}

func WithEnvOverrides(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return append([]string{}, base...)
	}
	keys := make(map[string]struct{}, len(overrides))
	for key := range overrides {
		keys[key] = struct{}{}
	}
	env := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, skip := keys[key]; skip {
			continue
		}
		env = append(env, entry)
	}
	for key, value := range overrides {
		env = append(env, key+"="+value)
	}
	return env
}
