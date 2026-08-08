//go:build !windows

package forj

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/goforj/goforj/project"
)

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
