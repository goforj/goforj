//go:build windows

package devwatch

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"testing"
	"time"
)

// processTestShellCommand uses cmd.exe's native line ending so shared shell tests retain an exact output assertion.
func processTestShellCommand() (string, string) {
	return "echo native-shell", "native-shell\r\n"
}

// processTestGracefulSignals returns os.Interrupt because Go maps both CTRL_C and CTRL_BREAK to that signal.
func processTestGracefulSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}

// TestDevProcessWindowsHelper provides a leader and descendant that survive CTRL_BREAK until their Job Object is closed.
func TestDevProcessWindowsHelper(t *testing.T) {
	if os.Getenv("GOFORJ_DEV_PROCESS_WINDOWS_HELPER") != "1" {
		return
	}
	// Go exposes the supervisor's CTRL_BREAK event as os.Interrupt rather than a distinct Windows signal.
	signal.Ignore(os.Interrupt)
	if len(os.Args) == 0 {
		os.Exit(2)
	}
	action := os.Args[len(os.Args)-1]
	switch action {
	case "child":
		if err := os.WriteFile(os.Getenv("GOFORJ_DEV_PROCESS_CHILD_READY"), []byte("ready"), 0o600); err != nil {
			os.Exit(3)
		}
		time.Sleep(500 * time.Millisecond)
		if err := os.WriteFile(os.Getenv("GOFORJ_DEV_PROCESS_MARKER"), []byte("survived"), 0o600); err != nil {
			os.Exit(4)
		}
	case "leader":
		if !waitForWindowsProcessFile(os.Getenv("GOFORJ_DEV_PROCESS_START"), 3*time.Second) {
			os.Exit(5)
		}
		child := exec.Command(os.Args[0], "-test.run=^TestDevProcessWindowsHelper$", "--", "child")
		child.Env = os.Environ()
		if err := child.Start(); err != nil {
			os.Exit(6)
		}
		if err := child.Process.Release(); err != nil {
			os.Exit(7)
		}
		if !waitForWindowsProcessFile(os.Getenv("GOFORJ_DEV_PROCESS_CHILD_READY"), 3*time.Second) {
			os.Exit(8)
		}
		if os.Getenv("GOFORJ_DEV_PROCESS_EXIT_LEADER") == "1" {
			os.Exit(0)
		}
		time.Sleep(time.Hour)
	default:
		os.Exit(2)
	}
	os.Exit(0)
}

// TestDevProcessSupervisorEscalatesAcrossWindowsJobObject verifies forced shutdown reaches descendants after CTRL_BREAK is ignored.
func TestDevProcessSupervisorEscalatesAcrossWindowsJobObject(t *testing.T) {
	t.Parallel()
	supervisor := NewSupervisor(SupervisorOptions{StopTimeout: 75 * time.Millisecond})
	registerDevProcessSupervisorCleanup(t, supervisor)
	directory := t.TempDir()
	command, start, ready, marker := windowsProcessTreeCommand(directory, false)
	if _, err := supervisor.StartRuntime(context.Background(), "tree", command); err != nil {
		t.Fatalf("start process tree: %v", err)
	}
	if err := os.WriteFile(start, []byte("start"), 0o600); err != nil {
		t.Fatalf("release process tree fixture: %v", err)
	}
	waitForProcessFile(t, ready)
	startedAt := time.Now()
	if err := supervisor.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown process tree: %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed < 50*time.Millisecond {
		t.Fatalf("expected graceful timeout before escalation, took %s", elapsed)
	}
	exit := waitForProcessExit(t, supervisor.Exits())
	if exit.StopReason != StopReasonShutdown {
		t.Fatalf("unexpected escalated exit %+v", exit)
	}
	assertWindowsProcessMarkerAbsent(t, marker)
}

// TestDevProcessSupervisorCleansWindowsJobAfterLeaderExit verifies an unexpected leader cannot orphan Job Object descendants.
func TestDevProcessSupervisorCleansWindowsJobAfterLeaderExit(t *testing.T) {
	t.Parallel()
	supervisor := NewSupervisor(SupervisorOptions{})
	registerDevProcessSupervisorCleanup(t, supervisor)
	directory := t.TempDir()
	command, start, ready, marker := windowsProcessTreeCommand(directory, true)
	pid, err := supervisor.StartRuntime(context.Background(), "short-leader", command)
	if err != nil {
		t.Fatalf("start process tree: %v", err)
	}
	if err := os.WriteFile(start, []byte("start"), 0o600); err != nil {
		t.Fatalf("release process tree fixture: %v", err)
	}
	waitForProcessFile(t, ready)
	exit := waitForProcessExit(t, supervisor.Exits())
	if exit.PID != pid || exit.ExitCode != 0 || exit.Intentional() {
		t.Fatalf("unexpected leader exit %+v", exit)
	}
	assertWindowsProcessMarkerAbsent(t, marker)
}

// windowsProcessTreeCommand creates a gated leader so Job Object attachment finishes before it spawns its descendant.
func windowsProcessTreeCommand(directory string, exitLeader bool) (Command, string, string, string) {
	start := filepath.Join(directory, "start")
	ready := filepath.Join(directory, "child-ready")
	marker := filepath.Join(directory, "child-survived")
	command := Command{
		Args: []string{os.Args[0], "-test.run=^TestDevProcessWindowsHelper$", "--", "leader"},
		Env: map[string]string{
			"GOFORJ_DEV_PROCESS_WINDOWS_HELPER": "1",
			"GOFORJ_DEV_PROCESS_START":          start,
			"GOFORJ_DEV_PROCESS_CHILD_READY":    ready,
			"GOFORJ_DEV_PROCESS_MARKER":         marker,
		},
	}
	if exitLeader {
		command.Env["GOFORJ_DEV_PROCESS_EXIT_LEADER"] = "1"
	}
	return command, start, ready, marker
}

// waitForWindowsProcessFile waits inside a helper process without depending on testing state owned by its parent.
func waitForWindowsProcessFile(path string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// assertWindowsProcessMarkerAbsent waits beyond the descendant delay and confirms Job Object cleanup stopped it.
func assertWindowsProcessMarkerAbsent(t *testing.T, marker string) {
	t.Helper()
	time.Sleep(600 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("descendant survived Windows Job Object cleanup")
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect descendant marker: %v", err)
	}
}
