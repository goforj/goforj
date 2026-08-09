//go:build !windows

package forj

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/goforj/goforj/internal/devwatch"
	"github.com/goforj/goforj/project"
)

type devPTYTransitionCapture struct {
	mu     sync.Mutex
	output bytes.Buffer
	needle string
	seen   chan struct{}
	once   sync.Once
}

// Write records PTY output and releases the watcher only after Bubble Tea renders terminal progress.
func (c *devPTYTransitionCapture) Write(value []byte) (int, error) {
	c.mu.Lock()
	written, err := c.output.Write(value)
	found := strings.Contains(c.output.String(), c.needle)
	c.mu.Unlock()
	if found {
		c.once.Do(func() { close(c.seen) })
	}
	return written, err
}

// String returns a stable PTY transcript after the reader has completed.
func (c *devPTYTransitionCapture) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.output.String()
}

// TestDevTUIStartupTranscriptPTYHelper completes one startup transaction before restoring its pseudo-terminal.
func TestDevTUIStartupTranscriptPTYHelper(t *testing.T) {
	if os.Getenv("GOFORJ_DEV_TUI_STARTUP_PTY_HELPER") != "1" {
		return
	}

	config := &project.Config{}
	writer := newDevBubbleWriter(config, func() {}, func() {}, func(devShellCommandRequest) {})
	transaction := newDevStartupTransaction(config, []string{"Build App", "Run App"}, false)
	writer.BeginLifecycleTransaction(transaction)
	_, _ = io.WriteString(writer, "01:04:29.753 Jobs Starting queue worker\n")
	_, _ = io.WriteString(writer, "01:04:29.755 HTTP Routes registered\n")
	writer.CompleteLifecycleTransaction(transaction.Key, 66*time.Millisecond, devLifecycleTransactionSummary{})
	if err := writer.Close(); err != nil {
		t.Fatalf("close development TUI: %v", err)
	}
}

// TestDevTUIStartupTranscriptSurvivesAlternateScreen restores the initial App story to shell history on exit.
func TestDevTUIStartupTranscriptSurvivesAlternateScreen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDevTUIStartupTranscriptPTYHelper$")
	command.Env = append(os.Environ(), "GOFORJ_DEV_TUI_STARTUP_PTY_HELPER=1")
	terminal, err := pty.StartWithSize(command, &pty.Winsize{Rows: 30, Cols: 120})
	if err != nil {
		t.Fatalf("start startup transcript TUI in pseudo-terminal: %v", err)
	}
	defer terminal.Close()

	var transcript bytes.Buffer
	_, readErr := io.Copy(&transcript, terminal)
	waitErr := command.Wait()
	if ctx.Err() != nil {
		t.Fatalf("startup transcript TUI timed out: %v\n%s", ctx.Err(), transcript.String())
	}
	if waitErr != nil {
		t.Fatalf("startup transcript TUI failed: %v\n%s", waitErr, transcript.String())
	}
	if readErr != nil && !strings.Contains(strings.ToLower(readErr.Error()), "input/output error") {
		t.Fatalf("read startup transcript TUI: %v", readErr)
	}

	raw := transcript.String()
	restoredAt := strings.LastIndex(raw, "\x1b[?1049l")
	if restoredAt < 0 {
		t.Fatalf("startup transcript TUI did not restore its alternate screen: %q", raw)
	}
	for _, expected := range []string{"┏ App startup", "Starting queue worker", "Routes registered", "┗", "Ready"} {
		if index := strings.LastIndex(raw, expected); index < restoredAt {
			t.Fatalf("startup output %q was not replayed after terminal restoration: %q", expected, raw)
		}
	}
}

// TestDevTUIRecoveryPTYHelper renders the production TUI inside the parent test's real pseudo-terminal.
func TestDevTUIRecoveryPTYHelper(t *testing.T) {
	if os.Getenv("GOFORJ_DEV_TUI_PTY_HELPER") != "1" {
		return
	}

	writer := newDevBubbleWriter(&project.Config{}, func() {}, func() {}, func(devShellCommandRequest) {})
	time.Sleep(50 * time.Millisecond)
	writeDevRecoverableFailure(writer, writer, "Development build failed", errors.New("Vite could not resolve ./src/missing"))
	_, _ = io.WriteString(writer, "SPA repaired; application runtime started\n")
	time.Sleep(50 * time.Millisecond)
	if err := writer.Close(); err != nil {
		t.Fatalf("close development TUI: %v", err)
	}
}

// TestDevTUIRecoveryPreservesRealTerminalChrome verifies failure recovery through Bubble Tea's terminal lifecycle.
func TestDevTUIRecoveryPreservesRealTerminalChrome(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDevTUIRecoveryPTYHelper$")
	command.Env = append(os.Environ(), "GOFORJ_DEV_TUI_PTY_HELPER=1")
	terminal, err := pty.StartWithSize(command, &pty.Winsize{Rows: 30, Cols: 120})
	if err != nil {
		t.Fatalf("start development TUI in pseudo-terminal: %v", err)
	}
	defer terminal.Close()

	var transcript bytes.Buffer
	_, readErr := io.Copy(&transcript, terminal)
	waitErr := command.Wait()
	if ctx.Err() != nil {
		t.Fatalf("development TUI helper timed out: %v\n%s", ctx.Err(), transcript.String())
	}
	if waitErr != nil {
		t.Fatalf("development TUI helper failed: %v\n%s", waitErr, transcript.String())
	}
	if readErr != nil && !strings.Contains(strings.ToLower(readErr.Error()), "input/output error") {
		t.Fatalf("read development TUI transcript: %v", readErr)
	}

	raw := transcript.String()
	if !strings.Contains(raw, "\x1b[?1049h") || !strings.Contains(raw, "\x1b[?1049l") {
		t.Fatalf("TUI did not enter and restore alternate-screen mode: %q", raw)
	}
	plain := stripANSI(raw)
	for _, want := range []string{
		"Resources",
		"[?] Controls",
		"Development build failed",
		"Vite could not resolve ./src/missing",
		"Watching for changes; fix the error to retry",
		"SPA repaired; application runtime started",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("development TUI transcript omitted %q:\n%s", want, plain)
		}
	}
}

// TestDevTUIWatcherTransitionPTYHelper runs a real structured watcher through the production TUI writer.
func TestDevTUIWatcherTransitionPTYHelper(t *testing.T) {
	if os.Getenv("GOFORJ_DEV_TUI_WATCHER_PTY_HELPER") != "1" {
		return
	}

	writer := newDevBubbleWriter(&project.Config{}, func() {}, func() {}, func(devShellCommandRequest) {})
	releaseFile := os.Getenv("GOFORJ_DEV_TUI_RELEASE_FILE")
	if releaseFile == "" {
		t.Fatal("release file is required")
	}
	controller, err := newDevWatcherController([]devCompiledWatcher{{
		ID:       "structured:app:spa:frontend",
		Name:     "Build app SPA frontend",
		App:      "app",
		Kind:     devWatcherSPABuild,
		Postpone: true,
		Command:  devwatch.Command{Shell: "while [ ! -f " + shellSingleQuote(releaseFile) + " ]; do sleep 0.01; done"},
	}}, nil, writer, writer, false)
	if err != nil {
		t.Fatalf("start watcher controller: %v", err)
	}
	task := controller.tasks["structured:app:spa:frontend"]
	task.request()
	waitForDevWatcherTaskState(t, task, true)
	waitForDevWatcherTaskState(t, task, false)
	controller.stop(time.Second)
	if err := writer.Close(); err != nil {
		t.Fatalf("close development TUI: %v", err)
	}
}

// TestDevTUIWatcherTransitionIsVisibleInRealTerminal verifies watcher work reaches the alternate-screen lifecycle row.
func TestDevTUIWatcherTransitionIsVisibleInRealTerminal(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	releaseFile := filepath.Join(t.TempDir(), "release")

	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDevTUIWatcherTransitionPTYHelper$")
	command.Env = append(
		os.Environ(),
		"GOFORJ_DEV_TUI_WATCHER_PTY_HELPER=1",
		"GOFORJ_DEV_TUI_RELEASE_FILE="+releaseFile,
	)
	terminal, err := pty.StartWithSize(command, &pty.Winsize{Rows: 30, Cols: 120})
	if err != nil {
		t.Fatalf("start watcher transition TUI in pseudo-terminal: %v", err)
	}
	defer terminal.Close()

	transcript := &devPTYTransitionCapture{
		needle: devTerminalProgressBusy,
		seen:   make(chan struct{}),
	}
	readDone := make(chan error, 1)
	go func() {
		_, readErr := io.Copy(transcript, terminal)
		readDone <- readErr
	}()
	select {
	case <-transcript.seen:
	case <-ctx.Done():
		t.Fatalf("watcher transition was not rendered before timeout: %v\n%s", ctx.Err(), transcript.String())
	}
	if err := os.WriteFile(releaseFile, []byte("continue\n"), 0o600); err != nil {
		t.Fatalf("release watcher command: %v", err)
	}
	waitErr := command.Wait()
	readErr := <-readDone
	if ctx.Err() != nil {
		t.Fatalf("watcher transition TUI timed out: %v\n%s", ctx.Err(), transcript.String())
	}
	if waitErr != nil {
		t.Fatalf("watcher transition TUI failed: %v\n%s", waitErr, transcript.String())
	}
	if readErr != nil && !strings.Contains(strings.ToLower(readErr.Error()), "input/output error") {
		t.Fatalf("read watcher transition TUI: %v", readErr)
	}

	raw := transcript.String()
	if !strings.Contains(raw, devTerminalProgressBusy) || !strings.Contains(raw, devTerminalProgressClear) {
		t.Fatalf("watcher transition did not publish terminal progress lifecycle: %q", raw)
	}
	if strings.LastIndex(raw, devTerminalProgressClear) < strings.LastIndex(raw, devTerminalProgressBusy) {
		t.Fatalf("watcher transition left terminal progress active: %q", raw)
	}
	plain := stripANSI(raw)
	if !strings.Contains(plain, "Building app frontend") {
		t.Fatalf("watcher transition was not visible in TUI:\n%s", plain)
	}
}

// waitForDevWatcherTaskState waits for a structured watcher to enter or leave its command phase.
func waitForDevWatcherTaskState(t *testing.T, task *devWatcherTask, want bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		task.mu.Lock()
		busy := task.busy
		task.mu.Unlock()
		if busy == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("watcher busy state did not become %t", want)
}
