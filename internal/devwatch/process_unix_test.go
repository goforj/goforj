//go:build unix

package devwatch

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// processTestShellCommand preserves the exact POSIX shell output contract exercised by shared supervisor tests.
func processTestShellCommand() (string, string) {
	return "printf native-shell", "native-shell"
}

// processTestGracefulSignals returns the signals that Unix process groups receive during graceful shutdown.
func processTestGracefulSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}

// TestDevProcessUnixHelper provides a process tree that ignores graceful termination.
func TestDevProcessUnixHelper(t *testing.T) {
	if os.Getenv("GOFORJ_DEV_PROCESS_UNIX_HELPER") != "1" {
		return
	}
	signal.Ignore(syscall.SIGTERM)
	if len(os.Args) > 1 && os.Args[len(os.Args)-1] == "child" {
		time.Sleep(350 * time.Millisecond)
		group, _ := syscall.Getpgid(0)
		value := []byte(fmt.Sprintf("pid=%d pgid=%d", os.Getpid(), group))
		if err := os.WriteFile(os.Getenv("GOFORJ_DEV_PROCESS_MARKER"), value, 0o600); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}

	child := exec.Command(os.Args[0], "-test.run=^TestDevProcessUnixHelper$", "--", "child")
	child.Env = os.Environ()
	if err := child.Start(); err != nil {
		os.Exit(3)
	}
	if err := os.WriteFile(os.Getenv("GOFORJ_DEV_PROCESS_READY"), []byte("ready"), 0o600); err != nil {
		os.Exit(4)
	}
	if os.Getenv("GOFORJ_DEV_PROCESS_EXIT_LEADER") == "1" {
		os.Exit(0)
	}
	time.Sleep(time.Hour)
}

// TestDevProcessSupervisorEscalatesAcrossUnixProcessGroup verifies TERM-to-KILL reaches descendants.
func TestDevProcessSupervisorEscalatesAcrossUnixProcessGroup(t *testing.T) {
	t.Parallel()
	supervisor := NewSupervisor(SupervisorOptions{StopTimeout: 75 * time.Millisecond})
	registerDevProcessSupervisorCleanup(t, supervisor)
	directory := t.TempDir()
	ready := filepath.Join(directory, "ready")
	marker := filepath.Join(directory, "child-survived")
	command := Command{
		Args: []string{os.Args[0], "-test.run=^TestDevProcessUnixHelper$"},
		Env: map[string]string{
			"GOFORJ_DEV_PROCESS_UNIX_HELPER": "1",
			"GOFORJ_DEV_PROCESS_READY":       ready,
			"GOFORJ_DEV_PROCESS_MARKER":      marker,
		},
	}
	if _, err := supervisor.StartRuntime(context.Background(), "tree", command); err != nil {
		t.Fatalf("start process tree: %v", err)
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
	time.Sleep(400 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("descendant survived process-group escalation")
	} else if !os.IsNotExist(err) {
		t.Fatalf("inspect descendant marker: %v", err)
	}
}

// TestDevProcessSupervisorCleansDescendantsAfterLeaderExit verifies an unexpected leader cannot orphan its process group.
func TestDevProcessSupervisorCleansDescendantsAfterLeaderExit(t *testing.T) {
	t.Parallel()
	supervisor := NewSupervisor(SupervisorOptions{})
	registerDevProcessSupervisorCleanup(t, supervisor)
	directory := t.TempDir()
	ready := filepath.Join(directory, "ready")
	marker := filepath.Join(directory, "child-survived")
	command := Command{
		Args: []string{os.Args[0], "-test.run=^TestDevProcessUnixHelper$"},
		Env: map[string]string{
			"GOFORJ_DEV_PROCESS_UNIX_HELPER": "1",
			"GOFORJ_DEV_PROCESS_READY":       ready,
			"GOFORJ_DEV_PROCESS_MARKER":      marker,
			"GOFORJ_DEV_PROCESS_EXIT_LEADER": "1",
			"GORACE":                         "atexit_sleep_ms=0",
		},
	}
	pid, err := supervisor.StartRuntime(context.Background(), "short-leader", command)
	if err != nil {
		t.Fatalf("start process tree: %v", err)
	}
	waitForProcessFile(t, ready)
	exit := waitForProcessExit(t, supervisor.Exits())
	if exit.PID != pid || exit.ExitCode != 0 || exit.Intentional() {
		t.Fatalf("unexpected leader exit %+v", exit)
	}
	time.Sleep(400 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		data, _ := os.ReadFile(marker)
		t.Fatalf("descendant survived its unexpected leader exit: leader=%d %s", pid, data)
	} else if !os.IsNotExist(err) {
		t.Fatalf("inspect descendant marker: %v", err)
	}
}
