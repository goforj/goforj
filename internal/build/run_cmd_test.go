package build

import (
	"bytes"
	"errors"
	"os"
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

func TestRunCmdRunArgsPassesAppArgsAfterCurrentPackage(t *testing.T) {
	cmd := &RunCmd{Args: []string{"run", "--port", "4000"}}
	if got := cmd.runArgs(); !reflect.DeepEqual(got, []string{".", "run", "--port", "4000"}) {
		t.Fatalf("expected app args after current package, got %#v", got)
	}
}

func TestRunCmdLaunchCommand(t *testing.T) {
	cmd := &RunCmd{}
	if got := cmd.launchCommand([]string{".", "route:list"}); got != "go run . route:list" {
		t.Fatalf("launch command = %q, want %q", got, "go run . route:list")
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
