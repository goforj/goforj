package scenarios

import (
	"context"
	"errors"
	"os"
	"os/exec"
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

// TestScenarioOutputBufferFindsRequiredTextBeyondRetainedDiagnostics keeps successful checks independent from diagnostic truncation.
func TestScenarioOutputBufferFindsRequiredTextBeyondRetainedDiagnostics(t *testing.T) {
	buffer := newScenarioOutputBuffer([]string{"complete marker", "across chunks"})
	if _, err := buffer.Write([]byte(strings.Repeat("x", scenarioCommandOutputLimit+16))); err != nil {
		t.Fatal(err)
	}
	if _, err := buffer.Write([]byte("complete marker across ")); err != nil {
		t.Fatal(err)
	}
	if _, err := buffer.Write([]byte("chunks")); err != nil {
		t.Fatal(err)
	}
	if missing := buffer.missingContains(); len(missing) != 0 {
		t.Fatalf("missing streamed output = %q", missing)
	}
}

// TestSnapshotToolsUsesPrivateCopies prevents later source replacement from changing the executable bytes selected for a scenario.
func TestSnapshotToolsUsesPrivateCopies(t *testing.T) {
	source := filepath.Join(t.TempDir(), "forj")
	if err := os.WriteFile(source, []byte("selected bytes"), 0o700); err != nil {
		t.Fatal(err)
	}
	execution, err := (scenarioExecution{
		workspace: scenarioWorkspace{root: t.TempDir(), removeAfter: true},
		tools:     map[string]string{"forj": source},
	}).snapshotTools()
	if err != nil {
		t.Fatalf("snapshotTools(): %v", err)
	}
	t.Cleanup(func() { _ = execution.workspace.cleanupAfter(nil) })
	if err := os.WriteFile(source, []byte("replacement bytes"), 0o700); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(execution.tools["forj"])
	if err != nil || string(body) != "selected bytes" {
		t.Fatalf("private tool bytes = %q, %v", body, err)
	}
	if execution.forjExec != execution.tools["forj"] {
		t.Fatalf("forj executable = %q, want private snapshot %q", execution.forjExec, execution.tools["forj"])
	}
}

// TestSnapshotToolsRunsInstalledGo proves a runtime-dependent Go binary keeps its GOROOT after scenario tool preparation.
func TestSnapshotToolsRunsInstalledGo(t *testing.T) {
	goExecutable, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("resolve go: %v", err)
	}
	execution, err := (scenarioExecution{
		context:     context.Background(),
		logger:      logger.NewSilentLogger(),
		workspace:   scenarioWorkspace{root: t.TempDir(), removeAfter: true},
		tools:       map[string]string{"go": goExecutable},
		environment: scenarioProcessEnv(),
	}).snapshotTools()
	if err != nil {
		t.Fatalf("snapshotTools(): %v", err)
	}
	t.Cleanup(func() { _ = execution.workspace.cleanupAfter(nil) })
	if execution.tools["go"] == goExecutable || execution.toolEnvironment["go"]["GOROOT"] == "" {
		t.Fatalf("Go snapshot = %q runtime binding = %#v", execution.tools["go"], execution.toolEnvironment["go"])
	}
	if err := execution.runCommand(ScenarioCommand{Run: []string{"go", "env", "GOROOT"}}, "verify Go runtime"); err != nil {
		t.Fatalf("runCommand() = %v", err)
	}
}

// TestSnapshotToolsRejectsReplacementAfterResolution ensures the immutable copy is checked against the tool identity embedded in the plan.
func TestSnapshotToolsRejectsReplacementAfterResolution(t *testing.T) {
	source := filepath.Join(t.TempDir(), "forj")
	if err := os.WriteFile(source, []byte("selected bytes"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, digest, err := resolveScenarioExecutable(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("replacement bytes"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err = (scenarioExecution{
		workspace:  scenarioWorkspace{root: t.TempDir(), removeAfter: true},
		tools:      map[string]string{"forj": source},
		toolDigest: digestScenarioTools(map[string]string{"forj": digest}),
	}).snapshotTools()
	if err == nil || !strings.Contains(err.Error(), "tool bytes changed") {
		t.Fatalf("snapshotTools() error = %v, want changed-byte rejection", err)
	}
}

// TestSnapshotToolsCleansFailedPrivateToolRoot keeps a failed snapshot from leaking supervisor-owned executable copies.
func TestSnapshotToolsCleansFailedPrivateToolRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "tools")
	previous := createScenarioToolRoot
	createScenarioToolRoot = func(parent string) (string, error) {
		if parent == "" {
			t.Fatal("tool snapshot parent is empty")
		}
		if err := os.Mkdir(root, 0o700); err != nil {
			return "", err
		}
		return root, nil
	}
	t.Cleanup(func() { createScenarioToolRoot = previous })
	_, err := (scenarioExecution{
		workspace: scenarioWorkspace{root: t.TempDir(), removeAfter: true},
		tools:     map[string]string{"forj": filepath.Join(t.TempDir(), "missing-forj")},
	}).snapshotTools()
	if err == nil {
		t.Fatal("snapshotTools() unexpectedly succeeded")
	}
	if _, statErr := os.Stat(root); !os.IsNotExist(statErr) {
		t.Fatalf("failed snapshot tool root remains: %v", statErr)
	}
}

// TestSnapshotToolsUsesWorkspaceParent keeps disposable executable copies inside the caller-owned command tree.
func TestSnapshotToolsUsesWorkspaceParent(t *testing.T) {
	workspaceRoot := filepath.Join(t.TempDir(), "workspace")
	if err := os.Mkdir(workspaceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "forj")
	if err := os.WriteFile(source, []byte("selected bytes"), 0o700); err != nil {
		t.Fatal(err)
	}
	execution, err := (scenarioExecution{
		workspace: scenarioWorkspace{root: workspaceRoot, removeAfter: true},
		tools:     map[string]string{"forj": source},
	}).snapshotTools()
	if err != nil {
		t.Fatalf("snapshotTools(): %v", err)
	}
	if got, want := filepath.Dir(execution.workspace.toolRoot), filepath.Dir(workspaceRoot); got != want {
		t.Fatalf("tool snapshot parent = %q, want %q", got, want)
	}
	prepared := &PreparedScenario{workspace: execution.workspace}
	if err := prepared.Close(); err != nil {
		t.Fatalf("PreparedScenario.Close(): %v", err)
	}
	if _, statErr := os.Stat(execution.workspace.toolRoot); !os.IsNotExist(statErr) {
		t.Fatalf("prepared scenario tool snapshot remains: %v", statErr)
	}
}

// TestScenarioWorkspaceCleanupOwnsSuccessfulToolSnapshot ensures a successful run releases the private executable copies as well as its project tree.
func TestScenarioWorkspaceCleanupOwnsSuccessfulToolSnapshot(t *testing.T) {
	toolRoot := filepath.Join(t.TempDir(), "tools")
	if err := os.Mkdir(toolRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	workspace := scenarioWorkspace{root: t.TempDir(), toolRoot: toolRoot, removeAfter: true}
	if err := workspace.cleanupAfter(nil); err != nil {
		t.Fatalf("cleanupAfter(): %v", err)
	}
	if _, err := os.Stat(toolRoot); !os.IsNotExist(err) {
		t.Fatalf("successful tool snapshot remains: %v", err)
	}
}

// TestScenarioWorkspaceCleanupOwnsToolSnapshotAfterRunError ensures primary failures do not bypass tool-snapshot cleanup.
func TestScenarioWorkspaceCleanupOwnsToolSnapshotAfterRunError(t *testing.T) {
	toolRoot := filepath.Join(t.TempDir(), "tools")
	if err := os.Mkdir(toolRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	workspace := scenarioWorkspace{root: t.TempDir(), toolRoot: toolRoot, removeAfter: true}
	runErr := errors.New("scenario failed")
	err := workspace.cleanupAfter(runErr)
	if !errors.Is(err, runErr) {
		t.Fatalf("cleanupAfter() error = %v, want primary failure", err)
	}
	if _, statErr := os.Stat(toolRoot); !os.IsNotExist(statErr) {
		t.Fatalf("failed run tool snapshot remains: %v", statErr)
	}
}

// TestApplyStepStopsBeforeMutationAfterCancellation keeps cancellation from leaking partial candidate state between phases.
func TestApplyStepStopsBeforeMutationAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root := t.TempDir()
	execution := scenarioExecution{context: ctx, workspace: scenarioWorkspace{root: root}}
	err := execution.applyStep(ScenarioSpec{}, ScenarioStep{Write: &ScenarioFileChange{Path: "late.txt", Content: "late"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("applyStep() error = %v, want cancellation", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "late.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("cancelled step wrote a file: %v", statErr)
	}
}

// TestCanonicalizeGoSourcesStabilizesComposedFixtures keeps harmless formatter cleanup outside candidate diffs.
func TestCanonicalizeGoSourcesStabilizesComposedFixtures(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app", "wire", "inject_services_app.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package wire\nvar providers=[]string{\"one\",\"two\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	execution := scenarioExecution{context: context.Background(), workspace: scenarioWorkspace{root: root}}
	if err := execution.canonicalizeGoSources(); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "package wire\n\nvar providers = []string{\"one\", \"two\"}\n"
	if string(body) != want {
		t.Fatalf("canonical source = %q, want %q", body, want)
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
