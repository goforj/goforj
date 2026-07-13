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
		"go.mod":          "module example.com/test\n\ngo 1.24\n",
		"cmd/app/main.go": "package main\nfunc main() {}\n",
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

func TestRunPlainGoBuildPublishesExecutableAtomically(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod":          "module example.com/test\n\ngo 1.24\n",
		"cmd/app/main.go": "package main\nfunc main() {}\n",
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
	binPath := filepath.Join(root, "bin", "app")
	if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.WriteFile(binPath, []byte("not executable"), 0o644); err != nil {
		t.Fatalf("write stale binary: %v", err)
	}
	legacyCachePath := filepath.Join(root, "bin", ".forj-build-cache", "app")
	if err := os.MkdirAll(filepath.Dir(legacyCachePath), 0o755); err != nil {
		t.Fatalf("mkdir legacy cache: %v", err)
	}
	if err := os.WriteFile(legacyCachePath, []byte("old shared cache binary"), 0o755); err != nil {
		t.Fatalf("write legacy cache binary: %v", err)
	}

	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousWD) })

	cmd := &Cmd{Root: "."}
	if _, err := cmd.runPlainGoBuild([]string{"-o", "./bin/app", "./cmd/app"}); err != nil {
		t.Fatalf("runPlainGoBuild returned error: %v", err)
	}
	info, err := os.Stat(binPath)
	if err != nil {
		t.Fatalf("stat built binary: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("expected built binary to be executable, mode %s", info.Mode())
	}
	if _, err := os.Stat(filepath.Join(root, "bin", ".app.publish")); !os.IsNotExist(err) {
		t.Fatalf("expected publish temporary build output to be cleaned up, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "bin", ".app.ready")); err != nil {
		t.Fatalf("expected build ready stamp: %v", err)
	}
	cacheEntries, err := os.ReadDir(filepath.Join(root, "bin", ".forj-build-cache"))
	if err != nil {
		t.Fatalf("read build cache dir: %v", err)
	}
	if len(cacheEntries) != 0 {
		t.Fatalf("expected hidden build temp output to be cleaned up, got %d entries", len(cacheEntries))
	}
}

func TestAtomicGoBuildArgsUsesUniqueHiddenBuildOutputs(t *testing.T) {
	firstArgs, firstOutput, firstCleanup, firstOK, err := atomicGoBuildArgs([]string{"-o", "./bin/app", "./cmd/app"})
	if err != nil {
		t.Fatalf("first atomicGoBuildArgs returned error: %v", err)
	}
	if !firstOK {
		t.Fatal("expected first build output to use atomic publishing")
	}
	defer firstCleanup()

	secondArgs, secondOutput, secondCleanup, secondOK, err := atomicGoBuildArgs([]string{"-o", "./bin/app", "./cmd/app"})
	if err != nil {
		t.Fatalf("second atomicGoBuildArgs returned error: %v", err)
	}
	if !secondOK {
		t.Fatal("expected second build output to use atomic publishing")
	}
	defer secondCleanup()

	if firstOutput.build == secondOutput.build {
		t.Fatalf("expected unique build outputs, got %q", firstOutput.build)
	}
	if firstOutput.ready != filepath.Join("bin", ".app.ready") || secondOutput.ready != filepath.Join("bin", ".app.ready") {
		t.Fatalf("expected shared ready stamp path, got %q and %q", firstOutput.ready, secondOutput.ready)
	}
	for _, path := range []string{firstOutput.build, secondOutput.build} {
		if filepath.Dir(path) != filepath.Join("bin", ".forj-build-cache") {
			t.Fatalf("expected hidden build output dir, got %q", path)
		}
	}
	if strings.Contains(strings.Join(firstArgs, " "), "./bin/app") || strings.Contains(strings.Join(secondArgs, " "), "./bin/app") {
		t.Fatalf("expected go build args to avoid final output path, got %#v and %#v", firstArgs, secondArgs)
	}
}

func TestCmdRunWithTimingsPrintsStepDurations(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod":          "module example.com/test\n\ngo 1.24\n",
		"cmd/app/main.go": "package main\nfunc main() {}\n",
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

func TestBuildArgsAppendDefaultPackageWhenOnlyFlagsProvided(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cmd", "app"), 0o755); err != nil {
		t.Fatalf("mkdir cmd/app: %v", err)
	}
	cmd := &Cmd{Root: root, Args: []string{"-o", "./bin/app"}}
	got := cmd.buildArgs()
	want := []string{"-o", "./bin/app", "./cmd/app"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("build args = %#v, want %#v", got, want)
	}
}

func TestBuildArgsUseActiveConventionalTarget(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cmd", "reporting"), 0o755); err != nil {
		t.Fatalf("mkdir cmd/reporting: %v", err)
	}
	t.Setenv("FORJ_APP", "reporting")

	cmd := &Cmd{Root: root}
	got := cmd.buildArgs()
	want := []string{"-o", "bin/reporting", "./cmd/reporting"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("build args = %#v, want %#v", got, want)
	}
}

func TestLoadWirePathsUsesActiveConventionalTarget(t *testing.T) {
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()

	wireDir := filepath.Join("app", "reporting", "wire")
	if err := os.MkdirAll(wireDir, 0o755); err != nil {
		t.Fatalf("mkdir target wire dir: %v", err)
	}
	t.Setenv("FORJ_APP", "reporting")

	got := loadWirePaths()
	want := []string{wireDir}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("wire paths = %#v, want %#v", got, want)
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
		"go.mod":          "module example.com/test\n\ngo 1.24\n",
		"cmd/app/main.go": "package main\nfunc main() {}\n",
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

func TestBuildProgressReporterNoopsWithoutTTY(t *testing.T) {
	t.Setenv("FORJ_BUILD_PROGRESS", "")
	t.Setenv("FORJ_DEBUG", "")
	t.Setenv("DEBUG", "")

	origStderr := os.Stderr
	_, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	defer func() {
		_ = w.Close()
		os.Stderr = origStderr
	}()

	if _, ok := newBuildProgressReporter(false, RunOptions{}).(buildProgressNoop); !ok {
		t.Fatal("expected build progress reporter to be noop when stderr is not a TTY")
	}
}

func TestBuildArgsMergesCompiledEnvWithExistingLdflags(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/demo\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	cmd := &Cmd{
		Root:        root,
		EnvDefaults: "FEATURE_A=true",
		Args:        []string{"-trimpath", "-ldflags", "-s -w", "-o", "./bin/app", "."},
	}

	args := cmd.buildArgs()
	got := strings.Join(args, " ")
	wantPayload := base64.StdEncoding.EncodeToString([]byte("FEATURE_A=true"))
	if !strings.Contains(got, "-s -w -X example.com/demo/internal/cmd.CompiledEnvDefaultsBase64="+wantPayload) {
		t.Fatalf("expected merged ldflags, got %v", args)
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

func TestValidateCompiledEnvRejectsMalformedEnvDefaults(t *testing.T) {
	cmd := &Cmd{
		EnvDefaults: "BROKEN",
	}
	if err := cmd.validateCompiledEnv(); err == nil {
		t.Fatal("expected malformed env defaults to fail")
	}
}

func TestValidateCompiledEnvRejectsMalformedEnvOverrides(t *testing.T) {
	cmd := &Cmd{
		EnvOverrides: "BROKEN",
	}
	if err := cmd.validateCompiledEnv(); err == nil {
		t.Fatal("expected malformed env overrides to fail")
	}
}
