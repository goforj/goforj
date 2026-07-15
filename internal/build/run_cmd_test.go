package build

import (
	"bytes"
	"errors"
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
	if got := requireRunArgs(t, cmd); !reflect.DeepEqual(got, []string{"."}) {
		t.Fatalf("expected default run args to target current package, got %#v", got)
	}
}

// TestRunCmdCompileFailurePreventsStartAndAPIIndexPublication verifies an unbuildable source tree cannot start the app.
func TestRunCmdCompileFailurePreventsStartAndAPIIndexPublication(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod":          "module example.com/broken\n\ngo 1.25\n",
		"cmd/app/main.go": "package main\nfunc main() { missing() }\n",
	}
	for relativePath, content := range files {
		path := filepath.Join(root, relativePath)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create fixture directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", relativePath, err)
		}
	}
	artifactPaths := []string{
		filepath.Join(root, "build", "api_index.json"),
		filepath.Join(root, "build", "api_index.diagnostics.json"),
		filepath.Join(root, "build", "openapi.json"),
	}
	for _, path := range artifactPaths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create active artifact directory: %v", err)
		}
		if err := os.WriteFile(path, []byte("{\"generation\":\"active\"}\n"), 0o644); err != nil {
			t.Fatalf("write active artifact: %v", err)
		}
	}
	published := false
	pending := recordingAPIIndexCandidate{
		publish: func() error {
			published = true
			for _, path := range artifactPaths {
				if err := os.WriteFile(path, []byte("{\"generation\":\"candidate\"}\n"), 0o644); err != nil {
					return err
				}
			}
			return nil
		},
		discard: func() {},
	}

	command := &RunCmd{Root: root}
	_, err := runFinalAndPublishAPIIndex(root, Step{Name: "compile and start", Run: command.runBinary}, pending)
	if err == nil || !strings.Contains(err.Error(), "compile app target") {
		t.Fatalf("compile failure = %v, want preflight error", err)
	}
	if command.process != nil || command.waitCh != nil {
		t.Fatal("app process started despite failed preflight compilation")
	}
	if published {
		t.Fatal("API index candidate was published after compile failure")
	}
	for _, path := range artifactPaths {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read active artifact: %v", readErr)
		}
		if string(content) != "{\"generation\":\"active\"}\n" {
			t.Fatalf("active artifact was published after compile failure: %s", content)
		}
	}
}

// TestRunCmdPipelineErrorAfterStartTerminatesAndReapsProcess verifies publication failures cannot leak a started app.
func TestRunCmdPipelineErrorAfterStartTerminatesAndReapsProcess(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/running\n\ngo 1.25\n",
		"cmd/app/main.go": `package main

import "time"

func main() {
	for {
		time.Sleep(time.Second)
	}
}
`,
	}
	for relativePath, content := range files {
		path := filepath.Join(root, relativePath)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create fixture directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", relativePath, err)
		}
	}
	command := &RunCmd{Root: root}
	t.Cleanup(func() { _ = command.terminateStartedProcess() })
	status, err := command.runBinary(root)
	if err != nil {
		t.Fatalf("start run command: %v", err)
	}
	if status != "started" || command.process == nil || command.waitCh == nil {
		t.Fatalf("run state = %q, process %v, wait channel %v", status, command.process, command.waitCh)
	}
	process := command.process
	pipelineErr := errors.New("publish API index")
	if err := command.handlePipelineError(pipelineErr); !errors.Is(err, pipelineErr) {
		t.Fatalf("pipeline error = %v, want publication error", err)
	}
	if command.process != nil || command.waitCh != nil {
		t.Fatal("started process state remained after pipeline cleanup")
	}
	if err := process.Kill(); !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("started process was not reaped, kill returned %v", err)
	}
}

// TestRunCmdCompiledBinaryReceivesAppArgsAndIsRemovedAfterWait verifies the temp executable preserves app arguments and cleanup semantics.
func TestRunCmdCompiledBinaryReceivesAppArgsAndIsRemovedAfterWait(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/args\n\ngo 1.25\n",
		"cmd/app/main.go": `package main

import (
	"os"
	"strings"
)

func main() {
	executable, err := os.Executable()
	if err != nil {
		panic(err)
	}
	content := executable + "\n" + strings.Join(os.Args[2:], " ")
	if err := os.WriteFile(os.Args[1], []byte(content), 0o644); err != nil {
		panic(err)
	}
}
`,
	}
	for relativePath, content := range files {
		path := filepath.Join(root, relativePath)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create fixture directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", relativePath, err)
		}
	}
	resultPath := filepath.Join(root, "run-result.txt")
	command := &RunCmd{Root: root, Args: []string{resultPath, "hello", "portal"}}
	if _, err := command.runBinary(root); err != nil {
		t.Fatalf("start compiled run binary: %v", err)
	}
	if err := command.waitForRunProcess(); err != nil {
		t.Fatalf("wait for compiled run binary: %v", err)
	}
	content, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatalf("read app argument result: %v", err)
	}
	parts := strings.SplitN(string(content), "\n", 2)
	if len(parts) != 2 || parts[1] != "hello portal" {
		t.Fatalf("app result = %q, want executable path and forwarded args", content)
	}
	if _, err := os.Stat(parts[0]); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("compiled run binary remained after Wait: %v", err)
	}
}

func TestRunCmdRunArgsDefaultsToCmdAppWhenPresent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cmd", "app"), 0o755); err != nil {
		t.Fatalf("mkdir cmd/app: %v", err)
	}
	cmd := &RunCmd{Root: root}
	if got := requireRunArgs(t, cmd); !reflect.DeepEqual(got, []string{"./cmd/app"}) {
		t.Fatalf("expected default run args to target cmd/app, got %#v", got)
	}
}

func TestRunCmdRunArgsUseActiveConventionalTarget(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cmd", "reporting"), 0o755); err != nil {
		t.Fatalf("mkdir cmd/reporting: %v", err)
	}
	t.Setenv("FORJ_APP", "reporting")

	cmd := &RunCmd{Root: root}
	if got := requireRunArgs(t, cmd); !reflect.DeepEqual(got, []string{"./cmd/reporting"}) {
		t.Fatalf("expected run args to target cmd/reporting, got %#v", got)
	}
}

func TestRunCmdRunArgsPassesAppArgsAfterCurrentPackage(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cmd", "app"), 0o755); err != nil {
		t.Fatalf("mkdir cmd/app: %v", err)
	}
	cmd := &RunCmd{Root: root, Args: []string{"run", "--port", "4000"}}
	if got := requireRunArgs(t, cmd); !reflect.DeepEqual(got, []string{"./cmd/app", "run", "--port", "4000"}) {
		t.Fatalf("expected app args after cmd/app package, got %#v", got)
	}
}

// TestRunCmdLaunchCommand verifies watcher status identifies both compilation and the exact App command being launched.
func TestRunCmdLaunchCommand(t *testing.T) {
	cmd := &RunCmd{}
	if got := cmd.launchCommand([]string{".", "route:list"}); got != "compile and start . route:list" {
		t.Fatalf("launch command = %q, want %q", got, "compile and start . route:list")
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

// TestWaitForRunProcessRequiresStartedProcess verifies invalid run state fails instead of reporting success or blocking.
func TestWaitForRunProcessRequiresStartedProcess(t *testing.T) {
	err := (&RunCmd{}).waitForRunProcess()
	if err == nil || !strings.Contains(err.Error(), "process was not started") {
		t.Fatalf("waitForRunProcess error = %v, want missing process state", err)
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

// requireRunArgs keeps argument-shape assertions focused while still failing on package-probe errors.
func requireRunArgs(t *testing.T, command *RunCmd) []string {
	t.Helper()
	args, err := command.runArgs()
	if err != nil {
		t.Fatalf("run args: %v", err)
	}
	return args
}
