package build

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/logger"
)

type stubAPIIndexer struct {
	root string
}

func (s stubAPIIndexer) RunQuiet() error {
	if err := os.MkdirAll(filepath.Join(s.root, "build"), 0o755); err != nil {
		return err
	}
	for _, name := range []string{"api_index.json", "api_index.diagnostics.json", "openapi.json"} {
		if err := os.WriteFile(filepath.Join(s.root, "build", name), []byte("{}"), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func TestCmdRunExecutesBuildPipeline(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod":  "module example.com/test\n\ngo 1.24\n",
		"main.go": "package main\nfunc main() {}\n",
	}
	for rel, contents := range files {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	appLogger := logger.NewSilentLogger()
	apiIndexRunner := &APIIndexRunner{
		runDefaultFunc: stubAPIIndexer{root: root}.RunQuiet,
	}
	build := NewCmd(appLogger, apiIndexRunner)
	build.Root = root
	if err := build.Run(); err != nil {
		t.Fatalf("build run failed: %v", err)
	}

	for _, p := range []string{
		filepath.Join(root, "build", "api_index.json"),
		filepath.Join(root, "build", "api_index.diagnostics.json"),
		filepath.Join(root, "build", "openapi.json"),
		filepath.Join(root, "bin", "app"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected artifact %s: %v", p, err)
		}
	}
}

func TestCmdRunWithTimingsPrintsStepDurations(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod":  "module example.com/test\n\ngo 1.24\n",
		"main.go": "package main\nfunc main() {}\n",
	}
	for rel, contents := range files {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	appLogger := logger.NewSilentLogger()
	apiIndexRunner := &APIIndexRunner{
		runDefaultFunc: stubAPIIndexer{root: root}.RunQuiet,
	}
	build := NewCmd(appLogger, apiIndexRunner)
	build.Root = root
	build.Timings = true

	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	runErr := build.Run()
	_ = w.Close()
	if runErr != nil {
		t.Fatalf("build run failed: %v", runErr)
	}

	var out bytes.Buffer
	if _, err := out.ReadFrom(r); err != nil {
		t.Fatalf("read stderr: %v", err)
	}

	output := out.String()
	for _, expected := range []string{
		"forj build wire:",
		"forj build generate:",
		"forj build build:api-index:",
		"forj build go build:",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected timings output to contain %q, got %q", expected, output)
		}
	}
}

func TestShouldRetryWire(t *testing.T) {
	retryable := `type-check failed for test/wire: wire/app.go:12:2: could not import test/internal/cmd`
	if !shouldRetryWire(retryable) {
		t.Fatalf("expected import-cascade wire error to be retryable")
	}

	nonRetryable := `wire: /private/tmp/test/wire/app.go:181:14: queue.DriverSync undefined`
	if shouldRetryWire(nonRetryable) {
		t.Fatalf("expected direct symbol error not to be retryable")
	}
}

func TestRunWireCommandPrintsDetailSeparately(t *testing.T) {
	root := t.TempDir()
	wireDir := filepath.Join(root, "wire")
	if err := os.MkdirAll(wireDir, 0o755); err != nil {
		t.Fatalf("mkdir wire dir: %v", err)
	}

	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin dir: %v", err)
	}

	wireScript := filepath.Join(binDir, "wire")
	script := "#!/bin/sh\n" +
		"echo 'wire: /tmp/test/wire.go:13:2: multiple bindings for Example' 1>&2\n" +
		"echo 'current:' 1>&2\n" +
		"echo '  <- provider \"NewExample\"' 1>&2\n" +
		"exit 1\n"
	if err := os.WriteFile(wireScript, []byte(script), 0o755); err != nil {
		t.Fatalf("write wire script: %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	pipeline := NewPipeline(logger.NewSilentLogger(), nil)
	_, runErr := pipeline.runWireCommand(wireDir, false)
	_ = w.Close()
	if runErr == nil {
		t.Fatal("expected wire command to fail")
	}
	if strings.Contains(runErr.Error(), "multiple bindings") || strings.Contains(runErr.Error(), "current:") {
		t.Fatalf("expected short wire error, got %q", runErr)
	}

	var out bytes.Buffer
	if _, err := out.ReadFrom(r); err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"multiple bindings for Example",
		"current:",
		"<- provider \"NewExample\"",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected stderr to contain %q, got %q", want, output)
		}
	}
}

func TestMissingModulePackages(t *testing.T) {
	detail := `internal/storages/manager_gen.go:14:2: no required module provides package github.com/goforj/storage/driver/redisstorage; to add it:

go get github.com/goforj/storage/driver/redisstorage
other.go:3:1: no required module provides package example.com/foo/bar; to add it:
	go get example.com/foo/bar`

	got := missingModulePackages(detail)
	want := []string{
		"github.com/goforj/storage/driver/redisstorage",
		"example.com/foo/bar",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("missingModulePackages() = %v, want %v", got, want)
	}
}

func TestAttemptMissingModuleRecovery(t *testing.T) {
	build := &Cmd{}
	var got []string
	build.goGetFunc = func(packages []string) error {
		got = append(got, packages...)
		return nil
	}

	recovered, err := build.attemptMissingModuleRecovery(`internal/storages/manager_gen.go:14:2: no required module provides package github.com/goforj/storage/driver/redisstorage; to add it:

go get github.com/goforj/storage/driver/redisstorage`)
	if err != nil {
		t.Fatalf("attemptMissingModuleRecovery() error = %v", err)
	}
	if !recovered {
		t.Fatalf("attemptMissingModuleRecovery() recovered = false, want true")
	}
	if strings.Join(got, ",") != "github.com/goforj/storage/driver/redisstorage" {
		t.Fatalf("go get packages = %v", got)
	}
	if build.lastBuildStatus != "synced deps, retried" {
		t.Fatalf("lastBuildStatus = %q", build.lastBuildStatus)
	}
}

func TestBuildProgressMarkers(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod":  "module example.com/test\n\ngo 1.24\n",
		"main.go": "package main\nfunc main() {}\n",
	}
	for rel, contents := range files {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	t.Setenv("FORJ_BUILD_PROGRESS", "1")

	appLogger := logger.NewSilentLogger()
	apiIndexRunner := &APIIndexRunner{
		runDefaultFunc: stubAPIIndexer{root: root}.RunQuiet,
	}
	build := NewCmd(appLogger, apiIndexRunner)
	build.Root = root

	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	runErr := build.Run()
	_ = w.Close()
	if runErr != nil {
		t.Fatalf("build run failed: %v", runErr)
	}

	var out bytes.Buffer
	if _, err := out.ReadFrom(r); err != nil {
		t.Fatalf("read stderr: %v", err)
	}

	output := out.String()
	for _, expected := range []string{
		"__FORJ_BUILD_PROGRESS__ step 1/4 generate",
		"__FORJ_BUILD_PROGRESS__ step 2/4 wire",
		"__FORJ_BUILD_PROGRESS__ step 3/4 build:api-index",
		"__FORJ_BUILD_PROGRESS__ step 4/4 go build",
		"__FORJ_BUILD_PROGRESS__ done",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected progress output to contain %q, got %q", expected, output)
		}
	}
}

func TestBuildArgsAddsAutoRunDefaultLaunchLdflags(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/demo\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	cmd := &Cmd{
		Root:    root,
		AutoRun: true,
	}

	args := cmd.buildArgs()
	got := strings.Join(args, " ")
	if !strings.Contains(got, "-ldflags") {
		t.Fatalf("expected build args to include -ldflags, got %v", args)
	}
	if !strings.Contains(got, "-X example.com/demo/internal/cmd.DefaultLaunchCommand=run") {
		t.Fatalf("expected default launch ldflags in build args, got %v", args)
	}
}

func TestBuildArgsMergesDefaultLaunchWithExistingLdflags(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/demo\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	cmd := &Cmd{
		Root:          root,
		DefaultLaunch: "run",
		Args:          []string{"-trimpath", "-ldflags", "-s -w", "-o", "./bin/app", "."},
	}

	args := cmd.buildArgs()
	got := strings.Join(args, " ")
	if !strings.Contains(got, "-s -w -X example.com/demo/internal/cmd.DefaultLaunchCommand=run") {
		t.Fatalf("expected merged ldflags, got %v", args)
	}
}

func TestValidateLaunchDefaultsRejectsConflictingFlags(t *testing.T) {
	cmd := &Cmd{
		AutoRun:       true,
		DefaultLaunch: "http:serve",
	}
	if err := cmd.validateLaunchDefaults(); err == nil {
		t.Fatal("expected conflicting auto-run/default-launch to fail")
	}
}

func TestBuildArgsAddsCompiledEnvDefaultsLdflags(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/demo\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	cmd := &Cmd{
		Root:        root,
		EnvDefaults: "FEATURE_A=true,FEATURE_B=false",
	}

	args := cmd.buildArgs()
	got := strings.Join(args, " ")
	wantPayload := base64.StdEncoding.EncodeToString([]byte("FEATURE_A=true,FEATURE_B=false"))
	if !strings.Contains(got, "-X example.com/demo/internal/cmd.CompiledEnvDefaultsBase64="+wantPayload) {
		t.Fatalf("expected compiled env default ldflags in build args, got %v", args)
	}
}

func TestBuildArgsAddsCompiledEnvOverridesLdflags(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/demo\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	cmd := &Cmd{
		Root:         root,
		EnvOverrides: "FEATURE_A=true,FEATURE_B=false",
	}

	args := cmd.buildArgs()
	got := strings.Join(args, " ")
	wantPayload := base64.StdEncoding.EncodeToString([]byte("FEATURE_A=true,FEATURE_B=false"))
	if !strings.Contains(got, "-X example.com/demo/internal/cmd.CompiledEnvOverridesBase64="+wantPayload) {
		t.Fatalf("expected compiled env override ldflags in build args, got %v", args)
	}
}

func TestValidateLaunchDefaultsRejectsMalformedEnvDefaults(t *testing.T) {
	cmd := &Cmd{
		EnvDefaults: "BROKEN",
	}
	if err := cmd.validateLaunchDefaults(); err == nil {
		t.Fatal("expected malformed env defaults to fail")
	}
}

func TestValidateLaunchDefaultsRejectsMalformedEnvOverrides(t *testing.T) {
	cmd := &Cmd{
		EnvOverrides: "BROKEN",
	}
	if err := cmd.validateLaunchDefaults(); err == nil {
		t.Fatal("expected malformed env overrides to fail")
	}
}
