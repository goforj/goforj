//go:build linux || darwin

package devwatch

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// TestDevProcessSupervisorPTYPreservesTerminalOutput verifies watcher children retain their historical TTY contract.
func TestDevProcessSupervisorPTYPreservesTerminalOutput(t *testing.T) {
	var output bytes.Buffer
	var observed bytes.Buffer
	supervisor := NewSupervisor(SupervisorOptions{})
	registerDevProcessSupervisorCleanup(t, supervisor)
	exit, err := supervisor.Run(context.Background(), "tty", Command{
		Shell:  `if [ -t 1 ]; then printf 'tty-output'; else printf 'plain-output'; fi`,
		Stdout: &output,
		PTY:    true,
		OnOutput: func(chunk Output) {
			observed.Write(chunk.Data)
		},
	})
	if err != nil || !exit.OK() {
		t.Fatalf("Run() exit = %+v, error = %v", exit, err)
	}
	if got := output.String(); !strings.Contains(got, "tty-output") || strings.Contains(got, "plain-output") {
		t.Fatalf("PTY output = %q", got)
	}
	if got := observed.String(); !strings.Contains(got, "tty-output") {
		t.Fatalf("PTY hook output = %q", got)
	}
	_ = <-supervisor.Exits()
}

// TestDevProcessSupervisorPTYBoundsBlockedSink verifies output consumers cannot prevent process completion publication.
func TestDevProcessSupervisorPTYBoundsBlockedSink(t *testing.T) {
	writer := &blockingDevProcessWriter{started: make(chan struct{}), release: make(chan struct{})}
	supervisor := NewSupervisor(SupervisorOptions{})
	registerDevProcessSupervisorCleanup(t, supervisor, writer.releaseOutput)
	if _, err := supervisor.StartRuntime(context.Background(), "blocked-pty", Command{
		Shell:  "printf blocked-pty",
		Stdout: writer,
		PTY:    true,
	}); err != nil {
		t.Fatalf("StartRuntime() error = %v", err)
	}
	waitForDevProcessWriter(t, writer)
	select {
	case exit := <-supervisor.Exits():
		if !exit.OK() {
			t.Fatalf("PTY exit = %+v, want successful process completion", exit)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("PTY completion remained blocked on its output sink")
	}
	writer.releaseOutput()
}

// TestDevProcessSupervisorInteractivePTYUsesTerminalOutput verifies implicit interactive writers survive PTY attachment.
func TestDevProcessSupervisorInteractivePTYUsesTerminalOutput(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	originalStdout := os.Stdout
	os.Stdout = writer
	t.Cleanup(func() {
		os.Stdout = originalStdout
		_ = writer.Close()
		_ = reader.Close()
	})

	supervisor := NewSupervisor(SupervisorOptions{})
	registerDevProcessSupervisorCleanup(t, supervisor)
	exit, runErr := supervisor.Run(context.Background(), "interactive-pty", Command{
		Shell:       "printf interactive-pty",
		Interactive: true,
		PTY:         true,
	})
	os.Stdout = originalStdout
	if err := writer.Close(); err != nil {
		t.Fatalf("close capture writer: %v", err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read interactive output: %v", err)
	}
	if runErr != nil || !exit.OK() {
		t.Fatalf("Run() exit = %+v, error = %v", exit, runErr)
	}
	if got := string(output); !strings.Contains(got, "interactive-pty") {
		t.Fatalf("interactive PTY output = %q", got)
	}
	_ = <-supervisor.Exits()
}
