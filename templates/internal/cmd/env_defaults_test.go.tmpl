package cmd

import (
	"encoding/base64"
	"fmt"
	envpkg "github.com/goforj/env/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyCompiledEnvDefaultsOnlySeedsUnsetKeys(t *testing.T) {
	t.Setenv("FEATURE_B", "from-env")
	CompiledEnvDefaultsBase64 = base64.StdEncoding.EncodeToString([]byte("FEATURE_A=true,FEATURE_B=from-build"))
	t.Cleanup(func() { CompiledEnvDefaultsBase64 = "" })

	if err := ApplyCompiledEnvDefaults(); err != nil {
		t.Fatalf("ApplyCompiledEnvDefaults() error = %v", err)
	}

	if got := os.Getenv("FEATURE_A"); got != "true" {
		t.Fatalf("expected FEATURE_A to be seeded, got %q", got)
	}
	if got := os.Getenv("FEATURE_B"); got != "from-env" {
		t.Fatalf("expected FEATURE_B env value to win, got %q", got)
	}
}

func TestApplyCompiledEnvDefaultsRejectsMalformedPayload(t *testing.T) {
	CompiledEnvDefaultsBase64 = base64.StdEncoding.EncodeToString([]byte("BROKEN"))
	t.Cleanup(func() { CompiledEnvDefaultsBase64 = "" })

	if err := ApplyCompiledEnvDefaults(); err == nil {
		t.Fatal("expected malformed compiled env defaults to fail")
	}
}

func TestCompiledEnvDefaultsLayerWithEnvLoad(t *testing.T) {
	if os.Getenv("GO_WANT_ENV_DEFAULTS_LAYER_HELPER") == "1" {
		runEnvDefaultsLayerHelper()
		return
	}

	dir := t.TempDir()
	writeTestEnvFile(t, filepath.Join(dir, ".env"), strings.Join([]string{
		"FEATURE_FROM_ENVFILE=from-envfile",
		"FEATURE_COLLISION=from-envfile",
	}, "\n")+"\n")

	output := runEnvDefaultsHelperProcess(t, "TestCompiledEnvDefaultsLayerWithEnvLoad", dir, map[string]string{
		"FEATURE_FROM_OS":   "from-os",
		"FEATURE_COLLISION": "from-os",
	})

	got := parseEnvDefaultsOutput(t, output)
	if got["FEATURE_FROM_BUILD"] != "from-build" {
		t.Fatalf("expected FEATURE_FROM_BUILD to come from compiled defaults, got %q", got["FEATURE_FROM_BUILD"])
	}
	if got["FEATURE_FROM_ENVFILE"] != "from-envfile" {
		t.Fatalf("expected FEATURE_FROM_ENVFILE to come from .env, got %q", got["FEATURE_FROM_ENVFILE"])
	}
	if got["FEATURE_FROM_OS"] != "from-os" {
		t.Fatalf("expected FEATURE_FROM_OS to remain from OS env, got %q", got["FEATURE_FROM_OS"])
	}
	if got["FEATURE_COLLISION"] != "from-envfile" {
		t.Fatalf("expected FEATURE_COLLISION to reflect current env.Load precedence, got %q", got["FEATURE_COLLISION"])
	}
}

func TestCompiledEnvDefaultsCanSeedAppEnvBeforeEnvLoad(t *testing.T) {
	if os.Getenv("GO_WANT_ENV_DEFAULTS_APP_ENV_HELPER") == "1" {
		runAppEnvDefaultsHelper()
		return
	}

	dir := t.TempDir()
	writeTestEnvFile(t, filepath.Join(dir, ".env"), "\n")
	writeTestEnvFile(t, filepath.Join(dir, ".env.staging"), "APP_MARKER=from-staging\n")

	output := runEnvDefaultsHelperProcess(t, "TestCompiledEnvDefaultsCanSeedAppEnvBeforeEnvLoad", dir, nil)
	got := parseEnvDefaultsOutput(t, output)
	if got["APP_ENV"] != "staging" {
		t.Fatalf("expected APP_ENV to be seeded from compiled defaults, got %q", got["APP_ENV"])
	}
	if got["APP_MARKER"] != "from-staging" {
		t.Fatalf("expected .env.staging to be selected, got %q", got["APP_MARKER"])
	}
}

func TestCompiledEnvOverridesBeatEnvAndOS(t *testing.T) {
	if os.Getenv("GO_WANT_ENV_OVERRIDES_LAYER_HELPER") == "1" {
		runEnvOverridesLayerHelper()
		return
	}

	dir := t.TempDir()
	writeTestEnvFile(t, filepath.Join(dir, ".env"), strings.Join([]string{
		"FEATURE_FROM_ENVFILE=from-envfile",
		"FEATURE_COLLISION=from-envfile",
	}, "\n")+"\n")

	output := runEnvDefaultsHelperProcess(t, "TestCompiledEnvOverridesBeatEnvAndOS", dir, map[string]string{
		"FEATURE_FROM_OS":   "from-os",
		"FEATURE_COLLISION": "from-os",
	})

	got := parseEnvDefaultsOutput(t, output)
	if got["FEATURE_FROM_BUILD"] != "from-build" {
		t.Fatalf("expected FEATURE_FROM_BUILD to come from compiled overrides, got %q", got["FEATURE_FROM_BUILD"])
	}
	if got["FEATURE_FROM_ENVFILE"] != "from-build" {
		t.Fatalf("expected FEATURE_FROM_ENVFILE to be overridden by compiled value, got %q", got["FEATURE_FROM_ENVFILE"])
	}
	if got["FEATURE_FROM_OS"] != "from-build" {
		t.Fatalf("expected FEATURE_FROM_OS to be overridden by compiled value, got %q", got["FEATURE_FROM_OS"])
	}
	if got["FEATURE_COLLISION"] != "from-build" {
		t.Fatalf("expected FEATURE_COLLISION to be overridden by compiled value, got %q", got["FEATURE_COLLISION"])
	}
}

func TestCompiledEnvOverridesCanForceAppEnvAcrossEnvLoad(t *testing.T) {
	if os.Getenv("GO_WANT_ENV_OVERRIDES_APP_ENV_HELPER") == "1" {
		runAppEnvOverrideHelper()
		return
	}

	dir := t.TempDir()
	writeTestEnvFile(t, filepath.Join(dir, ".env"), "APP_ENV=production\nAPP_MARKER=from-base\n")
	writeTestEnvFile(t, filepath.Join(dir, ".env.staging"), "APP_MARKER=from-staging\n")
	writeTestEnvFile(t, filepath.Join(dir, ".env.production"), "APP_MARKER=from-production\n")

	output := runEnvDefaultsHelperProcess(t, "TestCompiledEnvOverridesCanForceAppEnvAcrossEnvLoad", dir, nil)
	got := parseEnvDefaultsOutput(t, output)
	if got["APP_ENV"] != "staging" {
		t.Fatalf("expected APP_ENV override to persist after env.Load, got %q", got["APP_ENV"])
	}
	if got["APP_MARKER"] != "from-staging" {
		t.Fatalf("expected .env.staging to be selected by APP_ENV override, got %q", got["APP_MARKER"])
	}
}

func TestApplyAppEnvOverlayPromotesAnyAppPrefixedKey(t *testing.T) {
	t.Setenv("FORJ_APP", "customer-portal")
	t.Setenv("CUSTOMER_PORTAL_APP_URL", "http://localhost:3007")
	t.Setenv("CUSTOMER_PORTAL_DB_AUTH_DATABASE", "customer_auth")
	t.Setenv("CUSTOMER_PORTAL_FORJ_APP", "wrong")

	if err := ApplyAppEnvOverlay(); err != nil {
		t.Fatalf("ApplyAppEnvOverlay() error = %v", err)
	}
	if got := os.Getenv("APP_URL"); got != "http://localhost:3007" {
		t.Fatalf("APP_URL = %q, want app overlay", got)
	}
	if got := os.Getenv("DB_AUTH_DATABASE"); got != "customer_auth" {
		t.Fatalf("DB_AUTH_DATABASE = %q, want app overlay", got)
	}
	if got := os.Getenv("FORJ_APP"); got != "customer-portal" {
		t.Fatalf("FORJ_APP = %q, want original app identity", got)
	}
}

func TestAppEnvOverlayCanSelectAppEnvFile(t *testing.T) {
	if os.Getenv("GO_WANT_APP_ENV_HELPER") == "1" {
		runAppEnvOverlayHelper()
		return
	}

	dir := t.TempDir()
	writeTestEnvFile(t, filepath.Join(dir, ".env"), strings.Join([]string{
		"APP_ENV=local",
		"MARKER=base",
		"BILLING_APP_ENV=staging",
		"BILLING_MARKER=app-base",
	}, "\n")+"\n")
	writeTestEnvFile(t, filepath.Join(dir, ".env.staging"), strings.Join([]string{
		"MARKER=staging",
		"BILLING_MARKER=app-staging",
	}, "\n")+"\n")

	output := runEnvDefaultsHelperProcess(t, "TestAppEnvOverlayCanSelectAppEnvFile", dir, map[string]string{
		"FORJ_APP": "billing",
	})
	got := parseEnvDefaultsOutput(t, output)
	if got["APP_ENV"] != "staging" {
		t.Fatalf("expected APP_ENV to come from billing overlay, got %q", got["APP_ENV"])
	}
	if got["MARKER"] != "app-staging" {
		t.Fatalf("expected app overlay to reapply after .env.staging, got %q", got["MARKER"])
	}
}

func runEnvDefaultsLayerHelper() {
	dir := os.Getenv("GO_WANT_ENV_DEFAULTS_DIR")
	if err := os.Chdir(dir); err != nil {
		fmt.Printf("CHDIR_ERROR=%v\n", err)
		os.Exit(1)
	}
	CompiledEnvDefaultsBase64 = base64.StdEncoding.EncodeToString([]byte(strings.Join([]string{
		"FEATURE_FROM_BUILD=from-build",
		"FEATURE_FROM_ENVFILE=from-build",
		"FEATURE_FROM_OS=from-build",
		"FEATURE_COLLISION=from-build",
	}, ",")))
	if err := ApplyCompiledEnvDefaults(); err != nil {
		fmt.Printf("APPLY_ERROR=%v\n", err)
		os.Exit(1)
	}
	if err := envpkg.Load(); err != nil {
		fmt.Printf("LOAD_ERROR=%v\n", err)
		os.Exit(1)
	}
	printEnvDefaultsOutput("FEATURE_FROM_BUILD", "FEATURE_FROM_ENVFILE", "FEATURE_FROM_OS", "FEATURE_COLLISION")
	os.Exit(0)
}

func runAppEnvDefaultsHelper() {
	dir := os.Getenv("GO_WANT_ENV_DEFAULTS_DIR")
	if err := os.Chdir(dir); err != nil {
		fmt.Printf("CHDIR_ERROR=%v\n", err)
		os.Exit(1)
	}
	CompiledEnvDefaultsBase64 = base64.StdEncoding.EncodeToString([]byte("APP_ENV=staging"))
	if err := ApplyCompiledEnvDefaults(); err != nil {
		fmt.Printf("APPLY_ERROR=%v\n", err)
		os.Exit(1)
	}
	if err := LoadEnv(); err != nil {
		fmt.Printf("LOAD_ERROR=%v\n", err)
		os.Exit(1)
	}
	printEnvDefaultsOutput("APP_ENV", "APP_MARKER")
	os.Exit(0)
}

func runEnvOverridesLayerHelper() {
	dir := os.Getenv("GO_WANT_ENV_DEFAULTS_DIR")
	if err := os.Chdir(dir); err != nil {
		fmt.Printf("CHDIR_ERROR=%v\n", err)
		os.Exit(1)
	}
	CompiledEnvOverridesBase64 = base64.StdEncoding.EncodeToString([]byte(strings.Join([]string{
		"FEATURE_FROM_BUILD=from-build",
		"FEATURE_FROM_ENVFILE=from-build",
		"FEATURE_FROM_OS=from-build",
		"FEATURE_COLLISION=from-build",
	}, ",")))
	if err := ApplyCompiledEnvOverrides(); err != nil {
		fmt.Printf("APPLY_OVERRIDE_ERROR=%v\n", err)
		os.Exit(1)
	}
	if err := envpkg.Load(); err != nil {
		fmt.Printf("LOAD_ERROR=%v\n", err)
		os.Exit(1)
	}
	if err := ApplyCompiledEnvOverrides(); err != nil {
		fmt.Printf("REAPPLY_OVERRIDE_ERROR=%v\n", err)
		os.Exit(1)
	}
	printEnvDefaultsOutput("FEATURE_FROM_BUILD", "FEATURE_FROM_ENVFILE", "FEATURE_FROM_OS", "FEATURE_COLLISION")
	os.Exit(0)
}

func runAppEnvOverrideHelper() {
	dir := os.Getenv("GO_WANT_ENV_DEFAULTS_DIR")
	if err := os.Chdir(dir); err != nil {
		fmt.Printf("CHDIR_ERROR=%v\n", err)
		os.Exit(1)
	}
	CompiledEnvOverridesBase64 = base64.StdEncoding.EncodeToString([]byte("APP_ENV=staging"))
	if err := ApplyCompiledEnvOverrides(); err != nil {
		fmt.Printf("APPLY_OVERRIDE_ERROR=%v\n", err)
		os.Exit(1)
	}
	if err := LoadEnv(); err != nil {
		fmt.Printf("LOAD_ERROR=%v\n", err)
		os.Exit(1)
	}
	printEnvDefaultsOutput("APP_ENV", "APP_MARKER")
	os.Exit(0)
}

func runAppEnvOverlayHelper() {
	dir := os.Getenv("GO_WANT_ENV_DEFAULTS_DIR")
	if err := os.Chdir(dir); err != nil {
		fmt.Printf("CHDIR_ERROR=%v\n", err)
		os.Exit(1)
	}
	if err := LoadEnv(); err != nil {
		fmt.Printf("LOAD_ERROR=%v\n", err)
		os.Exit(1)
	}
	printEnvDefaultsOutput("APP_ENV", "MARKER")
	os.Exit(0)
}

// runEnvDefaultsHelperProcess starts in the fixture directory so package initialization cannot load the rendered project's .env first.
func runEnvDefaultsHelperProcess(t *testing.T, testName string, dir string, extraEnv map[string]string) string {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^"+testName+"$")
	cmd.Dir = dir
	baseEnv := os.Environ()
	filtered := make([]string, 0, len(baseEnv)+1)
	for _, entry := range baseEnv {
		if strings.HasPrefix(entry, "APP_ENV=") {
			continue
		}
		filtered = append(filtered, entry)
	}
	cmd.Env = append(filtered, "GO_WANT_ENV_DEFAULTS_DIR="+dir)
	switch testName {
	case "TestCompiledEnvDefaultsLayerWithEnvLoad":
		cmd.Env = append(cmd.Env, "GO_WANT_ENV_DEFAULTS_LAYER_HELPER=1")
	case "TestCompiledEnvDefaultsCanSeedAppEnvBeforeEnvLoad":
		cmd.Env = append(cmd.Env, "GO_WANT_ENV_DEFAULTS_APP_ENV_HELPER=1")
	case "TestCompiledEnvOverridesBeatEnvAndOS":
		cmd.Env = append(cmd.Env, "GO_WANT_ENV_OVERRIDES_LAYER_HELPER=1")
	case "TestCompiledEnvOverridesCanForceAppEnvAcrossEnvLoad":
		cmd.Env = append(cmd.Env, "GO_WANT_ENV_OVERRIDES_APP_ENV_HELPER=1")
	case "TestAppEnvOverlayCanSelectAppEnvFile":
		cmd.Env = append(cmd.Env, "GO_WANT_APP_ENV_HELPER=1")
	}
	for key, value := range extraEnv {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helper process failed: %v\n%s", err, output)
	}
	return string(output)
}

func writeTestEnvFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func parseEnvDefaultsOutput(t *testing.T, raw string) map[string]string {
	t.Helper()
	got := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("unexpected helper output line %q", line)
		}
		got[key] = value
	}
	return got
}

func printEnvDefaultsOutput(keys ...string) {
	for _, key := range keys {
		fmt.Printf("%s=%s\n", key, os.Getenv(key))
	}
}
