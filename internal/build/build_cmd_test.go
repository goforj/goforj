package build

import (
	"bytes"
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
