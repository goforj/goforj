package build

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRunCmdRunArgsDefaultsToCurrentPackage(t *testing.T) {
	cmd := &RunCmd{}
	if got := cmd.runArgs(); !reflect.DeepEqual(got, []string{"."}) {
		t.Fatalf("expected default run args to target current package, got %#v", got)
	}
}

func TestRunCmdRunArgsDefaultsToCmdAppWhenPresent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cmd", "app"), 0o755); err != nil {
		t.Fatalf("mkdir cmd/app: %v", err)
	}
	cmd := &RunCmd{Root: root}
	if got := cmd.runArgs(); !reflect.DeepEqual(got, []string{"./cmd/app"}) {
		t.Fatalf("expected default run args to target cmd/app, got %#v", got)
	}
}

func TestRunCmdRunArgsPassesAppArgsAfterCurrentPackage(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cmd", "app"), 0o755); err != nil {
		t.Fatalf("mkdir cmd/app: %v", err)
	}
	cmd := &RunCmd{Root: root, Args: []string{"run", "--port", "4000"}}
	if got := cmd.runArgs(); !reflect.DeepEqual(got, []string{"./cmd/app", "run", "--port", "4000"}) {
		t.Fatalf("expected app args after cmd/app package, got %#v", got)
	}
}

func TestRunCmdLaunchCommand(t *testing.T) {
	cmd := &RunCmd{}
	if got := cmd.launchCommand([]string{".", "route:list"}); got != "go run . route:list" {
		t.Fatalf("launch command = %q, want %q", got, "go run . route:list")
	}
}

func TestRunCmdPreservesTTYForAppCommands(t *testing.T) {
	if (&RunCmd{}).shouldPreserveTTY() {
		t.Fatal("expected no-arg app runtime to keep transient progress behavior")
	}
	if !(&RunCmd{Args: []string{"make:queue"}}).shouldPreserveTTY() {
		t.Fatal("expected app command args to preserve TTY")
	}
	if !(&RunCmd{PreserveTTY: true}).shouldPreserveTTY() {
		t.Fatal("expected explicit preserve TTY override")
	}
}

func TestShouldClearRunProgressBeforeFinal(t *testing.T) {
	if !shouldClearRunProgressBeforeFinal(true, true) {
		t.Fatal("expected transient progress to clear before a TTY-preserved app command")
	}
	if shouldClearRunProgressBeforeFinal(false, true) {
		t.Fatal("expected non-transient progress not to clear before final command")
	}
	if shouldClearRunProgressBeforeFinal(true, false) {
		t.Fatal("expected non-TTY-preserved app command to use the output gate instead")
	}
}

func TestRunCmdTransientProgressRequiresTTY(t *testing.T) {
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

	if shouldUseTransientRunProgress(false) {
		t.Fatal("expected transient run progress to be disabled when stderr is not a TTY")
	}
}

func TestFirstOutputGateBlocksUntilReleased(t *testing.T) {
	gate := newFirstOutputGate()
	var buf bytes.Buffer
	writer := gate.Writer(&buf)
	done := make(chan error, 1)
	go func() {
		_, err := writer.Write([]byte("runtime log\n"))
		done <- err
	}()

	select {
	case <-gate.First():
	case <-time.After(time.Second):
		t.Fatal("expected first output signal")
	}

	select {
	case err := <-done:
		t.Fatalf("write completed before release: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	if got := buf.String(); got != "" {
		t.Fatalf("expected no output before release, got %q", got)
	}

	gate.Release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("write after release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("expected write to complete after release")
	}
	if got := buf.String(); got != "runtime log\n" {
		t.Fatalf("output after release = %q", got)
	}
}

func TestWaitForRunProcessReturnsChildErrorWithoutInterrupt(t *testing.T) {
	cmd := &RunCmd{waitCh: make(chan error, 1)}
	cmd.waitCh <- errors.New("exit status 1")

	err := cmd.waitForRunProcess()
	if err == nil || !strings.Contains(err.Error(), "exit status 1") {
		t.Fatalf("waitForRunProcess error = %v, want child error", err)
	}
}

func TestRunCmdReturnsChildExitErrorForProcessExit(t *testing.T) {
	child := exec.Command("sh", "-c", "exit 7")
	err := child.Run()
	if err == nil {
		t.Fatal("expected child command to fail")
	}

	cmd := &RunCmd{waitCh: make(chan error, 1)}
	cmd.waitCh <- err

	runErr := cmd.waitForRunProcess()
	if runErr == nil {
		t.Fatal("expected waitForRunProcess to return child error")
	}
	code, ok := exitCodeFromError(runErr)
	if !ok || code != 7 {
		t.Fatalf("exitCodeFromError() = %d, %v; want 7, true", code, ok)
	}

	childErr := ChildExitError{Code: code, Err: runErr}
	gotCode, ok := ChildExitCode(childErr)
	if !ok || gotCode != 7 {
		t.Fatalf("ChildExitCode() = %d, %v; want 7, true", gotCode, ok)
	}
}

func TestGoRunExitStatusFilterDropsSyntheticExitLine(t *testing.T) {
	var out bytes.Buffer
	filter := newGoRunExitStatusFilter(&out)

	if _, err := io.WriteString(filter, "✖ expected \"<name>\"\nexit status 1\n"); err != nil {
		t.Fatalf("write filter: %v", err)
	}
	if err := filter.Close(); err != nil {
		t.Fatalf("close filter: %v", err)
	}

	if got := out.String(); got != "✖ expected \"<name>\"\n" {
		t.Fatalf("filtered output = %q", got)
	}
}

func TestGoRunExitStatusFilterKeepsNormalExitText(t *testing.T) {
	var out bytes.Buffer
	filter := newGoRunExitStatusFilter(&out)

	if _, err := io.WriteString(filter, "worker exit status changed\npartial"); err != nil {
		t.Fatalf("write filter: %v", err)
	}
	if err := filter.Close(); err != nil {
		t.Fatalf("close filter: %v", err)
	}

	if got := out.String(); got != "worker exit status changed\npartial" {
		t.Fatalf("filtered output = %q", got)
	}
}
