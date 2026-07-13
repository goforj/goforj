package build

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/logger"
)

// TestRunCmdWithTimingsPrintsStepDurations verifies run-mode timing diagnostics survive the transactional build-and-launch path.
func TestRunCmdWithTimingsPrintsStepDurations(t *testing.T) {
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
	run := NewRunCmd(appLogger, stubAPIIndexer{root: root})
	run.Root = root
	run.Timings = true

	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	runErr := run.Run()
	_ = w.Close()
	if runErr != nil {
		t.Fatalf("run failed: %v", runErr)
	}

	var out bytes.Buffer
	if _, err := out.ReadFrom(r); err != nil {
		t.Fatalf("read stderr: %v", err)
	}

	output := out.String()
	for _, expected := range []string{
		"forj run wire:",
		"forj run generate:",
		"forj run build:api-index:",
		"forj run compile and start ./cmd/app:",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected timings output to contain %q, got %q", expected, output)
		}
	}
}
