package scenarios

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/logger"
)

// scenarioWriteCloser exposes independent write and close failures for file-lifecycle tests.
type scenarioWriteCloser struct {
	writeErr error
	closeErr error
	closes   int
}

// Write returns the selected failure without involving host filesystem buffering.
func (file *scenarioWriteCloser) Write(content []byte) (int, error) {
	if file.writeErr != nil {
		return 0, file.writeErr
	}
	return len(content), nil
}

// Close records lifecycle ownership before returning the selected delayed failure.
func (file *scenarioWriteCloser) Close() error {
	file.closes++
	return file.closeErr
}

// TestScenarioWorkspaceCleanupAfterReportsCleanupWithoutMaskingRunFailure verifies teardown remains observable on both success and failure paths.
func TestScenarioWorkspaceCleanupAfterReportsCleanupWithoutMaskingRunFailure(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write cleanup blocker: %v", err)
	}
	workspace := scenarioWorkspace{
		root:        filepath.Join(blocker, "workspace"),
		removeAfter: true,
	}
	primaryErr := errors.New("scenario failed")
	for _, test := range []struct {
		name    string
		primary error
	}{
		{name: "cleanup only"},
		{name: "primary and cleanup", primary: primaryErr},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := workspace.cleanupAfter(test.primary)
			if test.primary != nil && !errors.Is(err, test.primary) {
				t.Fatalf("cleanupAfter() error = %v, want primary scenario failure", err)
			}
			if !strings.Contains(err.Error(), "remove temporary scenario workspace") {
				t.Fatalf("cleanupAfter() error = %v, want cleanup failure", err)
			}
		})
	}
}

// TestAppendAndCloseScenarioFileReportsEveryFailure verifies delayed close failures remain visible alone and beside write failures.
func TestAppendAndCloseScenarioFileReportsEveryFailure(t *testing.T) {
	writeErr := errors.New("write failed")
	closeErr := errors.New("close failed")
	tests := []struct {
		name     string
		writeErr error
		want     []error
	}{
		{name: "close failure", want: []error{closeErr}},
		{name: "write and close failures", writeErr: writeErr, want: []error{writeErr, closeErr}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file := &scenarioWriteCloser{writeErr: test.writeErr, closeErr: closeErr}
			err := appendAndCloseScenarioFile(file, []byte("content"))

			for _, want := range test.want {
				if !errors.Is(err, want) {
					t.Fatalf("appendAndCloseScenarioFile() error = %v, want %v", err, want)
				}
			}
			if file.closes != 1 {
				t.Fatalf("Close() calls = %d, want 1", file.closes)
			}
		})
	}
}

// TestCreateScenarioWorkspacePreservesExistingPaths proves a caller-selected work root never grants ownership of a preexisting scenario directory.
func TestCreateScenarioWorkspacePreservesExistingPaths(t *testing.T) {
	workRoot := t.TempDir()
	preexisting := filepath.Join(workRoot, "example")
	if err := os.MkdirAll(preexisting, 0o755); err != nil {
		t.Fatalf("create preexisting directory: %v", err)
	}
	sentinel := filepath.Join(preexisting, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("preserve"), 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	workspace, err := createScenarioWorkspace(
		ValidateOptions{WorkDir: workRoot},
		ScenarioSpec{ID: "example"},
	)
	if err != nil {
		t.Fatalf("create scenario workspace: %v", err)
	}
	if workspace.root == preexisting {
		t.Fatalf("workspace reused preexisting path %q", preexisting)
	}
	if !workspace.removeAfter {
		t.Fatal("temporary workspace under a selected root must be cleaned by default")
	}
	if err := workspace.cleanupAfter(nil); err != nil {
		t.Fatalf("clean scenario workspace: %v", err)
	}
	if body, err := os.ReadFile(sentinel); err != nil || string(body) != "preserve" {
		t.Fatalf("preexisting sentinel after cleanup = %q, %v", body, err)
	}
}

// TestReplaceTextRequiresOneTarget keeps template drift from applying a scenario edit at an arbitrary location.
func TestReplaceTextRequiresOneTarget(t *testing.T) {
	root := t.TempDir()
	execution := scenarioExecution{workspace: scenarioWorkspace{root: root}}
	path := filepath.Join(root, "example.txt")
	tests := []struct {
		name        string
		content     string
		old         string
		replacement string
		wantErr     string
	}{
		{name: "empty target", content: "value\n", replacement: "replacement", wantErr: "replace target is required"},
		{name: "unchanged target", content: "value\n", old: "value", replacement: "value", wantErr: "replacement must differ from target"},
		{name: "missing target", content: "value\n", old: "other", replacement: "replacement", wantErr: "replace target not found"},
		{name: "ambiguous target", content: "value\nvalue\n", old: "value", replacement: "replacement", wantErr: "replace target occurs 2 times"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(test.content), 0o644); err != nil {
				t.Fatalf("write replacement fixture: %v", err)
			}
			err := execution.replaceText(ScenarioSpec{}, ScenarioReplace{
				Path: "example.txt",
				Old:  test.old,
				New:  test.replacement,
			})
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("replaceText() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

// TestWriteFileRejectsInvalidGo reports the originating scenario file before a later build obscures the syntax error.
func TestWriteFileRejectsInvalidGo(t *testing.T) {
	execution := scenarioExecution{workspace: scenarioWorkspace{root: t.TempDir()}}
	err := execution.writeFile(ScenarioSpec{}, ScenarioFileChange{
		Path:    "broken.go",
		Content: "package broken\nfunc Broken(\n",
	})
	if err == nil || !strings.Contains(err.Error(), "format broken.go") {
		t.Fatalf("writeFile() error = %v, want formatting failure", err)
	}
}

// TestScenarioOutputBufferBoundsDiagnostics keeps noisy commands from retaining unbounded output.
func TestScenarioOutputBufferBoundsDiagnostics(t *testing.T) {
	buffer := scenarioOutputBuffer{}
	content := strings.Repeat("x", scenarioCommandOutputLimit+1)
	if _, err := buffer.Write([]byte(content)); err != nil {
		t.Fatalf("write output: %v", err)
	}
	if got := len(buffer.buffer.Bytes()); got != scenarioCommandOutputLimit {
		t.Fatalf("retained output = %d, want %d", got, scenarioCommandOutputLimit)
	}
	if !strings.Contains(buffer.String(), "output truncated") {
		t.Fatalf("bounded output did not identify truncation: %q", buffer.String())
	}
}

// TestRunCommandRejectsUnboundExecutable prevents later PATH lookup from changing a resolved scenario.
func TestRunCommandRejectsUnboundExecutable(t *testing.T) {
	execution := scenarioExecution{workspace: scenarioWorkspace{root: t.TempDir()}, logger: logger.NewSilentLogger()}
	err := execution.runCommand(ScenarioCommand{Run: []string{"unbound-tool"}}, "unbound")
	if err == nil || !strings.Contains(err.Error(), "not bound to a resolved tool") {
		t.Fatalf("runCommand() error = %v, want unbound tool rejection", err)
	}
}
