package build

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		"forj build generate:",
		"forj build build:api-index:",
		"forj build go build:",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected timings output to contain %q, got %q", expected, output)
		}
	}
}

func TestWirePathStaleMissingGeneratedFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "inject_app.go"), []byte("package wire\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	stale, err := wirePathStale(root)
	if err != nil {
		t.Fatalf("wirePathStale returned error: %v", err)
	}
	if !stale {
		t.Fatalf("expected wire path to be stale when wire_gen.go is missing")
	}
}

func TestWirePathStaleFalseWhenGeneratedFileIsNewer(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "inject_app.go")
	generatedPath := filepath.Join(root, "wire_gen.go")
	if err := os.WriteFile(sourcePath, []byte("package wire\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(generatedPath, []byte("package wire\n"), 0o644); err != nil {
		t.Fatalf("write generated: %v", err)
	}

	stale, err := wirePathStale(root)
	if err != nil {
		t.Fatalf("wirePathStale returned error: %v", err)
	}
	if stale {
		t.Fatalf("expected wire path to be fresh when wire_gen.go is newer than sources")
	}
}

func TestWirePathStaleTrueWhenSourceIsNewer(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "inject_app.go")
	generatedPath := filepath.Join(root, "wire_gen.go")
	if err := os.WriteFile(generatedPath, []byte("package wire\n"), 0o644); err != nil {
		t.Fatalf("write generated: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(sourcePath, []byte("package wire\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	stale, err := wirePathStale(root)
	if err != nil {
		t.Fatalf("wirePathStale returned error: %v", err)
	}
	if !stale {
		t.Fatalf("expected wire path to be stale when a source is newer than wire_gen.go")
	}
}
