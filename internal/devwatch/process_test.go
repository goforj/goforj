package devwatch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type blockingDevProcessWriter struct {
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
}

// Write holds os/exec's copy goroutine so shutdown timeout behavior can be exercised deterministically.
func (w *blockingDevProcessWriter) Write(data []byte) (int, error) {
	w.startedOnce.Do(func() { close(w.started) })
	<-w.release
	return len(data), nil
}

// releaseOutput unblocks copy goroutines before supervisor shutdown waits for process reaping.
func (w *blockingDevProcessWriter) releaseOutput() {
	w.releaseOnce.Do(func() { close(w.release) })
}

// TestDevProcessHelper provides portable child-process behavior to the supervisor tests.
func TestDevProcessHelper(t *testing.T) {
	if os.Getenv("GOFORJ_DEV_PROCESS_HELPER") != "1" {
		return
	}
	separator := -1
	for index, arg := range os.Args {
		if arg == "--" {
			separator = index
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		os.Exit(2)
	}
	action := os.Args[separator+1]
	switch action {
	case "report":
		cwd, err := os.Getwd()
		if err != nil {
			os.Exit(3)
		}
		fmt.Fprintf(os.Stdout, "cwd=%s env=%s\n", cwd, os.Getenv("GOFORJ_DEV_PROCESS_VALUE"))
		fmt.Fprintln(os.Stderr, "helper stderr")
	case "exit":
		code, err := strconv.Atoi(os.Args[separator+2])
		if err != nil {
			os.Exit(4)
		}
		os.Exit(code)
	case "wait":
		runProcessWaitHelper()
	case "delayed-write":
		delay, err := time.ParseDuration(os.Getenv("GOFORJ_DEV_PROCESS_DELAY"))
		if err != nil {
			os.Exit(5)
		}
		time.Sleep(delay)
		if err := os.WriteFile(os.Getenv("GOFORJ_DEV_PROCESS_MARKER"), []byte("written"), 0o600); err != nil {
			os.Exit(6)
		}
	default:
		os.Exit(2)
	}
	os.Exit(0)
}

// TestDevProcessSupervisorRun verifies argv execution, environment, workdir, streams, and hooks.
func TestDevProcessSupervisorRun(t *testing.T) {
	supervisor := NewSupervisor(SupervisorOptions{})
	registerDevProcessSupervisorCleanup(t, supervisor)
	workingDirectory := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var hookMu sync.Mutex
	var hookOutput []Output
	command := processHelperCommand("report")
	command.Dir = workingDirectory
	command.Env["GOFORJ_DEV_PROCESS_VALUE"] = "native"
	command.Stdout = &stdout
	command.Stderr = &stderr
	command.OnOutput = func(output Output) {
		hookMu.Lock()
		defer hookMu.Unlock()
		hookOutput = append(hookOutput, output)
	}

	exit, err := supervisor.Run(context.Background(), "build", command)
	if err != nil {
		t.Fatalf("run helper: %v", err)
	}
	if !exit.OK() {
		t.Fatalf("expected successful exit, got %+v", exit)
	}
	reportedDirectory, ok := strings.CutPrefix(stdout.String(), "cwd=")
	if !ok {
		t.Fatalf("unexpected stdout %q", stdout.String())
	}
	reportedDirectory, ok = strings.CutSuffix(reportedDirectory, " env=native\n")
	if !ok {
		t.Fatalf("unexpected stdout %q", stdout.String())
	}
	wantDirectory, err := os.Stat(workingDirectory)
	if err != nil {
		t.Fatalf("inspect requested working directory: %v", err)
	}
	gotDirectory, err := os.Stat(reportedDirectory)
	if err != nil {
		t.Fatalf("inspect reported working directory %q: %v", reportedDirectory, err)
	}
	if !os.SameFile(gotDirectory, wantDirectory) {
		t.Fatalf("reported working directory %q is not %q", reportedDirectory, workingDirectory)
	}
	if stderr.String() != "helper stderr\n" {
		t.Fatalf("unexpected stderr %q", stderr.String())
	}

	event := waitForProcessExit(t, supervisor.Exits())
	if event.Name != "build" || event.PID != exit.PID {
		t.Fatalf("unexpected exit event %+v", event)
	}
	hookMu.Lock()
	defer hookMu.Unlock()
	if len(hookOutput) < 2 {
		t.Fatalf("expected stdout and stderr hooks, got %+v", hookOutput)
	}
}

// TestDevProcessSupervisorRunShell verifies compatibility shell commands use the same execution path.
func TestDevProcessSupervisorRunShell(t *testing.T) {
	supervisor := NewSupervisor(SupervisorOptions{})
	registerDevProcessSupervisorCleanup(t, supervisor)
	var stdout bytes.Buffer
	shellCommand, expectedOutput := processTestShellCommand()
	exit, err := supervisor.Run(context.Background(), "shell", Command{
		Shell:  shellCommand,
		Stdout: &stdout,
	})
	if err != nil {
		t.Fatalf("run shell command: %v", err)
	}
	if !exit.OK() {
		t.Fatalf("expected successful shell exit, got %+v", exit)
	}
	if stdout.String() != expectedOutput {
		t.Fatalf("unexpected shell output %q", stdout.String())
	}
}

// TestDevProcessSupervisorRunFailurePreservesExitCode verifies failed commands remain structured results.
func TestDevProcessSupervisorRunFailurePreservesExitCode(t *testing.T) {
	supervisor := NewSupervisor(SupervisorOptions{})
	registerDevProcessSupervisorCleanup(t, supervisor)
	exit, err := supervisor.Run(context.Background(), "build", processHelperCommand("exit", "7"))
	if err == nil {
		t.Fatal("expected non-zero command error")
	}
	if exit.ExitCode != 7 || exit.Intentional() {
		t.Fatalf("unexpected failed exit %+v", exit)
	}
}

// TestDevProcessSupervisorRunCancellationGracefullyStopsCommand verifies canceled builds do not orphan children.
func TestDevProcessSupervisorRunCancellationGracefullyStopsCommand(t *testing.T) {
	supervisor := NewSupervisor(SupervisorOptions{StopTimeout: time.Second})
	registerDevProcessSupervisorCleanup(t, supervisor)
	ready := filepath.Join(t.TempDir(), "ready")
	command := processHelperCommand("wait")
	command.Env["GOFORJ_DEV_PROCESS_READY"] = ready
	ctx, cancel := context.WithCancel(context.Background())
	type runResult struct {
		exit Exit
		err  error
	}
	resultCh := make(chan runResult, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		exit, err := supervisor.Run(ctx, "build", command)
		resultCh <- runResult{exit: exit, err: err}
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("timed out cleaning up canceled command")
		}
	})
	waitForProcessFile(t, ready)
	cancel()

	select {
	case result := <-resultCh:
		if !errors.Is(result.err, context.Canceled) {
			t.Fatalf("expected canceled run, got %v", result.err)
		}
		if result.exit.StopReason != StopReasonCanceled {
			t.Fatalf("unexpected canceled exit %+v", result.exit)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for canceled command")
	}
}

// TestDevProcessSupervisorRestartRuntime verifies restart ordering and intentional exit events.
func TestDevProcessSupervisorRestartRuntime(t *testing.T) {
	supervisor := NewSupervisor(SupervisorOptions{StopTimeout: time.Second})
	registerDevProcessSupervisorCleanup(t, supervisor)
	firstReady := filepath.Join(t.TempDir(), "first-ready")
	firstCommand := processHelperCommand("wait")
	firstCommand.Env["GOFORJ_DEV_PROCESS_READY"] = firstReady
	firstPID, err := supervisor.StartRuntime(context.Background(), "app", firstCommand)
	if err != nil {
		t.Fatalf("start first runtime: %v", err)
	}
	waitForProcessFile(t, firstReady)

	secondReady := filepath.Join(t.TempDir(), "second-ready")
	secondCommand := processHelperCommand("wait")
	secondCommand.Env["GOFORJ_DEV_PROCESS_READY"] = secondReady
	secondPID, err := supervisor.RestartRuntime(context.Background(), "app", secondCommand)
	if err != nil {
		t.Fatalf("restart runtime: %v", err)
	}
	if secondPID == firstPID {
		t.Fatalf("expected replacement PID, both were %d", firstPID)
	}
	waitForProcessFile(t, secondReady)
	firstExit := waitForProcessExit(t, supervisor.Exits())
	if firstExit.PID != firstPID || firstExit.StopReason != StopReasonRestart {
		t.Fatalf("unexpected restart exit %+v", firstExit)
	}

	if err := supervisor.StopRuntime(context.Background(), "app"); err != nil {
		t.Fatalf("stop replacement runtime: %v", err)
	}
	secondExit := waitForProcessExit(t, supervisor.Exits())
	if secondExit.PID != secondPID || secondExit.StopReason != StopReasonManual {
		t.Fatalf("unexpected manual stop exit %+v", secondExit)
	}
}

// TestDevProcessSupervisorNormalizesRuntimeNamesAcrossLifecycle prevents whitespace variants from leaking duplicate runtimes.
func TestDevProcessSupervisorNormalizesRuntimeNamesAcrossLifecycle(t *testing.T) {
	supervisor := NewSupervisor(SupervisorOptions{StopTimeout: time.Second})
	registerDevProcessSupervisorCleanup(t, supervisor)
	firstReady := filepath.Join(t.TempDir(), "first-ready")
	firstCommand := processHelperCommand("wait")
	firstCommand.Env["GOFORJ_DEV_PROCESS_READY"] = firstReady
	firstPID, err := supervisor.StartRuntime(context.Background(), " app ", firstCommand)
	if err != nil {
		t.Fatalf("StartRuntime() error = %v", err)
	}
	waitForProcessFile(t, firstReady)
	if !supervisor.RuntimeRunning("\tapp\n") {
		t.Fatal("RuntimeRunning() = false for the canonical runtime name")
	}

	secondReady := filepath.Join(t.TempDir(), "second-ready")
	secondCommand := processHelperCommand("wait")
	secondCommand.Env["GOFORJ_DEV_PROCESS_READY"] = secondReady
	secondPID, err := supervisor.RestartRuntime(context.Background(), "  app\t", secondCommand)
	if err != nil {
		t.Fatalf("RestartRuntime() error = %v", err)
	}
	if secondPID == firstPID {
		t.Fatalf("RestartRuntime() PID = %d, want a replacement for %d", secondPID, firstPID)
	}
	waitForProcessFile(t, secondReady)
	firstExit := waitForProcessExit(t, supervisor.Exits())
	if firstExit.Name != "app" || firstExit.PID != firstPID || firstExit.StopReason != StopReasonRestart {
		t.Fatalf("restart exit = %+v, want canonical name and PID %d", firstExit, firstPID)
	}

	if err := supervisor.StopRuntime(context.Background(), " app "); err != nil {
		t.Fatalf("StopRuntime() error = %v", err)
	}
	secondExit := waitForProcessExit(t, supervisor.Exits())
	if secondExit.Name != "app" || secondExit.PID != secondPID || secondExit.StopReason != StopReasonManual {
		t.Fatalf("stop exit = %+v, want canonical name and PID %d", secondExit, secondPID)
	}
	if supervisor.RuntimeRunning(" app ") {
		t.Fatal("RuntimeRunning() = true after stopping the whitespace-normalized runtime")
	}
}

// TestDevProcessSupervisorReportsUnexpectedRuntimeExit verifies natural runtime failures are not hidden.
func TestDevProcessSupervisorReportsUnexpectedRuntimeExit(t *testing.T) {
	supervisor := NewSupervisor(SupervisorOptions{})
	registerDevProcessSupervisorCleanup(t, supervisor)
	pid, err := supervisor.StartRuntime(context.Background(), "worker", processHelperCommand("exit", "9"))
	if err != nil {
		t.Fatalf("start failing runtime: %v", err)
	}
	exit := waitForProcessExit(t, supervisor.Exits())
	if exit.PID != pid || exit.ExitCode != 9 || exit.Intentional() {
		t.Fatalf("unexpected runtime exit %+v", exit)
	}
}

// TestDevProcessSupervisorExitDeliveryQueuesWithoutLoss protects runtime failures behind slow observers.
func TestDevProcessSupervisorExitDeliveryQueuesWithoutLoss(t *testing.T) {
	t.Parallel()
	supervisor := NewSupervisor(SupervisorOptions{ExitBuffer: 1})
	registerDevProcessSupervisorCleanup(t, supervisor)
	done := make(chan struct{})
	go func() {
		supervisor.emitExit(Exit{Name: "first"})
		supervisor.emitExit(Exit{Name: "second"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("exit publication blocked after its bounded buffer filled")
	}
	if exit := <-supervisor.Exits(); exit.Name != "first" {
		t.Fatalf("buffered exit = %+v, want first completion", exit)
	}
	if exit := <-supervisor.Exits(); exit.Name != "second" {
		t.Fatalf("queued exit = %+v, want second completion", exit)
	}
}

// TestDevProcessSupervisorForcedStopBoundsWaitReaping verifies stuck output cannot defeat the shutdown deadline.
func TestDevProcessSupervisorForcedStopBoundsWaitReaping(t *testing.T) {
	t.Parallel()
	writer := &blockingDevProcessWriter{started: make(chan struct{}), release: make(chan struct{})}
	supervisor := NewSupervisor(SupervisorOptions{StopTimeout: 40 * time.Millisecond})
	registerDevProcessSupervisorCleanup(t, supervisor, writer.releaseOutput)
	command := processHelperCommand("report")
	command.Stdout = writer
	if _, err := supervisor.StartRuntime(context.Background(), "blocked-output", command); err != nil {
		t.Fatalf("StartRuntime() error = %v", err)
	}
	waitForDevProcessWriter(t, writer)
	startedAt := time.Now()
	err := supervisor.StopRuntime(context.Background(), "blocked-output")
	if err == nil || !strings.Contains(err.Error(), "did not exit after forced shutdown") {
		t.Fatalf("StopRuntime() error = %v, want bounded forced-shutdown error", err)
	}
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("StopRuntime() took %s, want a bounded return", elapsed)
	}
	supervisor.mu.Lock()
	retained := supervisor.runtimes["blocked-output"] != nil
	supervisor.mu.Unlock()
	if !retained {
		t.Fatal("failed stop discarded runtime ownership before process completion")
	}
	writer.releaseOutput()
	if err := supervisor.StopRuntime(context.Background(), "blocked-output"); err != nil {
		t.Fatalf("retry StopRuntime() error = %v", err)
	}
	_ = waitForProcessExit(t, supervisor.Exits())
}

// TestDevProcessSupervisorShutdownRetainsFailedRuntimes verifies a later shutdown can retry incomplete reaping.
func TestDevProcessSupervisorShutdownRetainsFailedRuntimes(t *testing.T) {
	t.Parallel()
	writer := &blockingDevProcessWriter{started: make(chan struct{}), release: make(chan struct{})}
	supervisor := NewSupervisor(SupervisorOptions{StopTimeout: 40 * time.Millisecond})
	registerDevProcessSupervisorCleanup(t, supervisor, writer.releaseOutput)
	command := processHelperCommand("report")
	command.Stdout = writer
	if _, err := supervisor.StartRuntime(context.Background(), "blocked-shutdown", command); err != nil {
		t.Fatalf("StartRuntime() error = %v", err)
	}
	waitForDevProcessWriter(t, writer)
	if err := supervisor.Shutdown(context.Background()); err == nil {
		t.Fatal("Shutdown() error = nil, want bounded forced-shutdown error")
	}
	supervisor.mu.Lock()
	retained := supervisor.runtimes["blocked-shutdown"] != nil
	supervisor.mu.Unlock()
	if !retained {
		t.Fatal("failed shutdown discarded runtime ownership before process completion")
	}
	writer.releaseOutput()
	if err := supervisor.Shutdown(context.Background()); err != nil {
		t.Fatalf("retry Shutdown() error = %v", err)
	}
	_ = waitForProcessExit(t, supervisor.Exits())
}

// TestMergeCommandEnvironmentUsesWindowsKeySemantics verifies explicit casing cannot leave inherited duplicates.
func TestMergeCommandEnvironmentUsesWindowsKeySemantics(t *testing.T) {
	t.Parallel()
	environment := mergeCommandEnvironment(
		[]string{"HOME=C:\\users\\dev", "Path=C:\\windows"},
		map[string]string{"PATH": "C:\\tools"},
		false,
		true,
	)
	if got, want := environment, []string{"HOME=C:\\users\\dev", "PATH=C:\\tools"}; !slices.Equal(got, want) {
		t.Fatalf("merged environment = %q, want %q", got, want)
	}
}

// TestDevProcessOutputWritersSerializeSharedSinks verifies stdout and stderr cannot race a shared writer or hook.
func TestDevProcessOutputWritersSerializeSharedSinks(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	var hookOutput bytes.Buffer
	sharedMu := &sync.Mutex{}
	stdout := &outputWriter{
		stream: StreamStdout,
		writer: &output,
		hook: func(output Output) {
			hookOutput.Write(output.Data)
		},
		mu: sharedMu,
	}
	stderr := &outputWriter{
		stream: StreamStderr,
		writer: &output,
		hook: func(output Output) {
			hookOutput.Write(output.Data)
		},
		mu: sharedMu,
	}
	var wait sync.WaitGroup
	for _, writer := range []*outputWriter{stdout, stderr} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range 100 {
				if _, err := writer.Write([]byte("x")); err != nil {
					t.Errorf("Write() error = %v", err)
					return
				}
			}
		}()
	}
	wait.Wait()
	if output.Len() != 200 || hookOutput.Len() != 200 {
		t.Fatalf("serialized output lengths = %d/%d, want 200/200", output.Len(), hookOutput.Len())
	}
}

// TestDevProcessSupervisorShutdownSignalsRuntimesInParallel verifies app shutdown is coordinated concurrently.
func TestDevProcessSupervisorShutdownSignalsRuntimesInParallel(t *testing.T) {
	supervisor := NewSupervisor(SupervisorOptions{StopTimeout: 2 * time.Second})
	registerDevProcessSupervisorCleanup(t, supervisor)
	directory := t.TempDir()
	stopFiles := []string{filepath.Join(directory, "app-stop"), filepath.Join(directory, "worker-stop")}
	for index, name := range []string{"app", "worker"} {
		ready := filepath.Join(directory, name+"-ready")
		command := processHelperCommand("wait")
		command.Env["GOFORJ_DEV_PROCESS_READY"] = ready
		command.Env["GOFORJ_DEV_PROCESS_STOP"] = stopFiles[index]
		command.Env["GOFORJ_DEV_PROCESS_STOP_DELAY"] = "250ms"
		if _, err := supervisor.StartRuntime(context.Background(), name, command); err != nil {
			t.Fatalf("start %s runtime: %v", name, err)
		}
		waitForProcessFile(t, ready)
	}

	if err := supervisor.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown runtimes: %v", err)
	}

	firstSignal := readProcessTimestamp(t, stopFiles[0])
	secondSignal := readProcessTimestamp(t, stopFiles[1])
	difference := firstSignal.Sub(secondSignal)
	if difference < 0 {
		difference = -difference
	}
	if difference >= 150*time.Millisecond {
		t.Fatalf("expected concurrent stop signals, timestamps differed by %s", difference)
	}
	for range stopFiles {
		exit := waitForProcessExit(t, supervisor.Exits())
		if exit.StopReason != StopReasonShutdown {
			t.Fatalf("unexpected shutdown exit %+v", exit)
		}
	}
}

// registerDevProcessSupervisorCleanup prevents failed assertions from stranding children or the exit dispatcher.
func registerDevProcessSupervisorCleanup(t *testing.T, supervisor *Supervisor, beforeShutdown ...func()) {
	t.Helper()
	t.Cleanup(func() {
		for _, cleanup := range beforeShutdown {
			cleanup()
		}
		_ = supervisor.Shutdown(context.Background())
		supervisor.Close()
	})
}

// waitForDevProcessWriter bounds fixture setup so its registered cleanup can still release blocked output.
func waitForDevProcessWriter(t *testing.T, writer *blockingDevProcessWriter) {
	t.Helper()
	select {
	case <-writer.started:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for process output writer")
	}
}

// processHelperCommand constructs a direct argv invocation of the current test binary.
func processHelperCommand(action string, args ...string) Command {
	commandArgs := []string{"-test.run=^TestDevProcessHelper$", "--", action}
	commandArgs = append(commandArgs, args...)
	return Command{
		Args: append([]string{os.Args[0]}, commandArgs...),
		Env: map[string]string{
			"GOFORJ_DEV_PROCESS_HELPER": "1",
		},
	}
}

// runProcessWaitHelper waits for termination and records when the signal arrived.
func runProcessWaitHelper() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, processTestGracefulSignals()...)
	defer signal.Stop(signals)
	if err := os.WriteFile(os.Getenv("GOFORJ_DEV_PROCESS_READY"), []byte("ready"), 0o600); err != nil {
		os.Exit(7)
	}
	<-signals
	if path := os.Getenv("GOFORJ_DEV_PROCESS_STOP"); path != "" {
		value := strconv.FormatInt(time.Now().UnixNano(), 10)
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			os.Exit(8)
		}
	}
	if rawDelay := os.Getenv("GOFORJ_DEV_PROCESS_STOP_DELAY"); rawDelay != "" {
		delay, err := time.ParseDuration(rawDelay)
		if err != nil {
			os.Exit(9)
		}
		time.Sleep(delay)
	}
}

// waitForProcessExit receives one exit record with a test-bounded timeout.
func waitForProcessExit(t *testing.T, exits <-chan Exit) Exit {
	t.Helper()
	select {
	case exit := <-exits:
		return exit
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for process exit")
		return Exit{}
	}
}

// waitForProcessFile waits until a helper reports that it is ready.
func waitForProcessFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

// readProcessTimestamp reads a nanosecond timestamp recorded by a helper process.
func readProcessTimestamp(t *testing.T, path string) time.Time {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read timestamp %s: %v", path, err)
	}
	nanoseconds, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		t.Fatalf("parse timestamp %s: %v", path, err)
	}
	return time.Unix(0, nanoseconds)
}
