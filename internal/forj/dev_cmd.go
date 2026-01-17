package forj

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/goforj/execx"
	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/str"
)

type DevCmd struct {
	logger *logger.AppLogger
}

func NewDevCmd(logger *logger.AppLogger) *DevCmd {
	return &DevCmd{logger: logger}
}

// Run executes the dev workflow (pre tasks, watchers, and shutdown handling).
func (c *DevCmd) Run() error {
	// Prevent concurrent dev sessions from clobbering each other.
	unlock, err := c.acquireLock()
	if err != nil {
		return err
	}
	defer unlock()

	config, err := LoadProjectConfig()
	if err != nil {
		return err
	}

	// Ensure the lock is released on Ctrl+C or termination.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		<-sigCh
		if config != nil && config.Dev.DownOnExit {
			console.Actionf("forj down > auto (set dev.down_on_exit: false to disable)")
			if err := runDevDownTasks(config.Dev.Down); err != nil {
				console.Errorf("forj down failed: %v", err)
			} else {
				console.Successf("forj down complete")
			}
		}
		unlock()
		os.Exit(1)
	}()

	if len(config.Dev.Watches) == 0 {
		console.Warnf("No dev watches defined in .goforj.yml")
		return nil
	}

	if err := ensureDevTools(); err != nil {
		return err
	}
	if err := ensureBinDir(); err != nil {
		return err
	}

	restartCh := make(chan struct{}, 1)
	streamer := newDevwatchStreamerFromEnv()
	if streamer != nil {
		streamer.SetRestartChannel(restartCh)
		defer streamer.Close()
	}

	for {
		// Run pre-dev commands if any
		if len(config.Dev.Pre) > 0 {
			console.Actionf("Running pre-dev setup")
			for _, task := range config.Dev.Pre {
				fmt.Printf(" %s %s\n", console.ActionMark(), task.Name)
				res, err := execx.Command("bash", "-c", task.Cmd).
					EnvInherit().
					StdinReader(os.Stdin).
					StdoutWriter(os.Stdout).
					StderrWriter(os.Stderr).
					Run()
				if err != nil {
					return fmt.Errorf("pre-dev task '%s' failed: %v", task.Name, err)
				}
				if !res.OK() {
					return fmt.Errorf("pre-dev task '%s' failed with exit code %d", task.Name, res.ExitCode)
				}
			}
		}

		console.Actionf("Running dev watchers")

		var segments []string
		for _, watch := range config.Dev.Watches {
			fmt.Printf(" %s %s\n", console.ActionMark(), watch.Name)
			logPrefix := buildLogPrefix(config.ProjectName, watch.Name)
			execMsg := shellQuote(fmt.Sprintf(
				" · %s · %s",
				console.Colorize(console.ColorBoldWhite, "GoForj Watcher"),
				console.Colorize(console.ColorGray, watch.Name),
			))
			watchExec := fmt.Sprintf("APP_LOG_PREFIX=%s %s", shellQuote(logPrefix), watch.Exec)
			wgoCmd := fmt.Sprintf(
				"wgo %s -log-prefix='' -exec-log -exec-msg %s sh -c %q",
				watch.Watch,
				execMsg,
				watchExec,
			)

			segments = append(segments, wgoCmd)
		}

		// Build a full command with ::
		fullCmd := strings.Join(segments, " :: ")

		// strip any APP_ env vars from the command
		envMap := map[string]string{}
		for _, env := range os.Environ() {
			if !strings.HasPrefix(env, "APP_") {
				key, value, _ := strings.Cut(env, "=")
				envMap[key] = value
			}
		}
		prettyCmd := formatWatcherCommandList(config.Dev.Watches)
		seenEndOfWatcherOutput := uint32(0)
		outWriter := newWatcherSpacerWriter(os.Stdout, &seenEndOfWatcherOutput)
		errWriter := newWatcherSpacerWriter(os.Stderr, &seenEndOfWatcherOutput)
		cmd := execx.Command("bash", "-c", fullCmd).
			EnvOnly(envMap).
			StdinReader(os.Stdin).
			StdoutWriter(newDevwatchWriter(outWriter, streamer, "stdout")).
			StderrWriter(newDevwatchWriter(errWriter, streamer, "stderr")).
			ShadowPrint(
				execx.WithPrefix("\nforj dev"),
				execx.WithMask(func(cmd string) string {
					return "\n  " + prettyCmd
				}),
			)
		// PTY + stream hook handling is configured per-OS to preserve TTY behavior.
		cmd = configureWatcherPTY(cmd, config.Dev.SoundOnWatchError)
		proc := cmd.Start()

		done := make(chan struct{})
		var runRes execx.Result
		var runErr error
		go func() {
			runRes, runErr = proc.Wait()
			close(done)
		}()

		select {
		case <-restartCh:
			console.Actionf("Restarting dev watchers")
			_ = proc.GracefulShutdown(os.Interrupt, 5*time.Second)
			<-done
			for {
				select {
				case <-restartCh:
					continue
				default:
					goto drained
				}
			}
		drained:
			continue
		case <-done:
			if runErr != nil {
				return runErr
			}
			if !runRes.OK() {
				return fmt.Errorf("dev watchers exited with code %d", runRes.ExitCode)
			}
			return nil
		}
	}
}

// shellQuote safely quotes a string for bash shell usage.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\'\''`) + "'"
}

// buildLogPrefix formats the log prefix for watcher output.
func buildLogPrefix(appName, watchName string) string {
	appName = strings.TrimSpace(appName)
	if appName == "" {
		appName = "App"
	}
	watchName = strings.TrimSpace(watchName)
	if watchName == "" {
		watchName = "Service"
	}
	component := formatLogComponent(watchName)
	return fmt.Sprintf("%s › %s", appName, component)
}

func formatLogComponent(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return value
	}
	return string(runes)
}

// formatWatcherCommandList renders the human-friendly watcher list.
func formatWatcherCommandList(watches []DevWatch) string {
	var b strings.Builder
	for i, watch := range watches {
		if i > 0 {
			b.WriteString("\n  ")
		}
		b.WriteString("forj wgo ")
		b.WriteString("--label ")
		b.WriteString(shellQuote(watch.Name))
		b.WriteString(" ")
		b.WriteString(watch.Watch)
		b.WriteString(" -- ")
		b.WriteString(watch.Exec)
	}
	return b.String()
}

// separatorWriter inserts a single blank line after the watcher EXECUTING block.
type separatorWriter struct {
	out io.Writer    // underlying writer
	sep *uint32      // atomic flag to track whether to insert a separator
	buf bytes.Buffer // buffer for incomplete lines
}

// newWatcherSpacerWriter wraps output so we can emit one spacer between watcher exec logs and app output.
func newWatcherSpacerWriter(out io.Writer, sep *uint32) io.Writer {
	if out == nil || sep == nil {
		return out
	}
	return &separatorWriter{out: out, sep: sep}
}

// Write buffers until newlines to detect watcher exec lines and insert a spacer once.
// This is to improve readability between watcher logs and application output.
func (w *separatorWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if _, err := w.buf.Write(p); err != nil {
		return 0, err
	}
	for {
		data := w.buf.Bytes()
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			break
		}
		line := data[:idx]
		w.buf.Next(idx + 1)
		trimmed := bytes.TrimSuffix(line, []byte{'\r'})
		isExec := bytes.Contains(trimmed, []byte("GoForj Watcher")) && bytes.Contains(trimmed, []byte("EXECUTING"))
		if isExec {
			atomic.StoreUint32(w.sep, 1)
		} else if atomic.SwapUint32(w.sep, 0) == 1 {
			if _, err := w.out.Write([]byte("\n")); err != nil {
				return 0, err
			}
		}
		if _, err := w.out.Write(line); err != nil {
			return 0, err
		}
		if _, err := w.out.Write([]byte{'\n'}); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// mapToEnv converts a map into KEY=VALUE environment entries.
func mapToEnv(vars map[string]string) []string {
	if len(vars) == 0 {
		return nil
	}
	out := make([]string, 0, len(vars))
	for key, value := range vars {
		out = append(out, key+"="+value)
	}
	return out
}

// runDevDownTasks executes the dev down commands sequentially.
func runDevDownTasks(tasks []DevTask) error {
	if len(tasks) == 0 {
		return nil
	}
	console.Infof("Bringing down resources")
	for _, task := range tasks {
		res, err := execx.Command("bash", "-c", task.Cmd).
			EnvInherit().
			StdinReader(os.Stdin).
			StdoutWriter(os.Stdout).
			StderrWriter(os.Stderr).
			Run()
		if err != nil {
			return fmt.Errorf("dev_down task '%s' failed: %v", task.Name, err)
		}
		if !res.OK() {
			return fmt.Errorf("dev_down task '%s' failed with exit code %d", task.Name, res.ExitCode)
		}
		console.Successf("%s", task.Name)
	}
	return nil
}

// configureWatcherPTY wires PTY and output hooks based on platform constraints.
// PTY preserves native TTY behavior (colors, cursor control) but merges stdout/stderr.
// On PTY platforms, we avoid attaching stderr writers to prevent duplicate output.
func configureWatcherPTY(cmd *execx.Cmd, soundEnabled bool) *execx.Cmd {
	switch runtime.GOOS {
	case "linux", "darwin":
		// PTY merges stdout/stderr into a single stream.
		cmd = cmd.WithPTY()
		cmd = cmd.StderrWriter(nil)
		if soundEnabled {
			cmd = cmd.OnStdout(errorSoundHook(true))
		}
	default:
		if soundEnabled {
			cmd = cmd.OnStdout(errorSoundHook(true)).OnStderr(errorSoundHook(true))
		}
	}
	return cmd
}

// errorSoundHook emits a sound when matching error output appears.
func errorSoundHook(enabled bool) func(string) {
	if !enabled {
		return nil
	}
	limiter := newSoundLimiter(2 * time.Second)
	return func(line string) {
		if !containsErrorWord(line) {
			return
		}
		if !limiter.Allow() {
			return
		}
		playErrorSound()
	}
}

// containsErrorWord reports whether a line looks like an error signal.
func containsErrorWord(line string) bool {
	matches := []string{
		"syntax error:",
		"undefined:",
		"cannot use",
		"invalid operation:",
		"assignment mismatch:",
		"redeclared",
		"does not implement",
		"cannot find package",
		"no required module provides",
		"go: error",
	}
	m := str.Of(line)
	for _, match := range matches {
		if m.ContainsFold(match) {
			return true
		}
	}
	// Wire noise guard: only beep on actual wire failures, not successful "wire:" logs.
	if m.ContainsFold("wire:") &&
		(m.ContainsFold("error") ||
			m.ContainsFold("failed") ||
			m.ContainsFold("generate failed") ||
			(m.ContainsFold("inject") && m.ContainsFold("failed"))) {
		return true
	}
	return false
}

// playErrorSound plays a macOS alert sound when available.
func playErrorSound() {
	if runtime.GOOS != "darwin" {
		return
	}
	_ = execx.Command("afplay", "/System/Library/Sounds/Submarine.aiff").Start()
}

type soundLimiter struct {
	cooldown time.Duration
	last     time.Time
}

// newSoundLimiter throttles repeated sound triggers.
func newSoundLimiter(cooldown time.Duration) *soundLimiter {
	return &soundLimiter{cooldown: cooldown}
}

// Allow reports whether enough time has elapsed since the last trigger.
func (l *soundLimiter) Allow() bool {
	now := time.Now()
	if now.Sub(l.last) < l.cooldown {
		return false
	}
	l.last = now
	return true
}

// ensureDevTools installs required dev binaries if they're missing.
func ensureDevTools() error {
	if err := ensureTool("wgo", "github.com/goforj/wgo@latest"); err != nil {
		return err
	}
	if err := ensureTool("wire", "github.com/goforj/wire/cmd/wire@latest"); err != nil {
		return err
	}
	return nil
}

// ensureBinDir creates ./bin for local builds if it's missing.
func ensureBinDir() error {
	if err := os.MkdirAll("bin", 0755); err != nil {
		return fmt.Errorf("ensure bin directory: %w", err)
	}
	return nil
}

// ensureTool installs a CLI if it is missing from PATH.
func ensureTool(name, module string) error {
	if _, err := exec.LookPath(name); err == nil {
		return nil
	}

	res, err := execx.Command("go", "install", module).
		EnvInherit().
		StdinReader(os.Stdin).
		StdoutWriter(os.Stdout).
		StderrWriter(os.Stderr).
		Run()
	if err != nil {
		return fmt.Errorf("install %s: %w", name, err)
	}
	if !res.OK() {
		return fmt.Errorf("install %s failed with exit code %d", name, res.ExitCode)
	}
	return nil
}

// acquireLock prevents concurrent forj dev sessions from running in the same project.
func (c *DevCmd) acquireLock() (func(), error) {
	lockPath := ".forj-dev.lock"
	pid := os.Getpid()
	lockFile, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err == nil {
		_, _ = lockFile.WriteString(strconv.Itoa(pid))
		_ = lockFile.Close()
		return func() { _ = os.Remove(lockPath) }, nil
	}

	// If the lock already exists, verify whether the PID is still alive.
	data, err := os.ReadFile(lockPath)
	if err == nil {
		if existing, parseErr := strconv.Atoi(strings.TrimSpace(string(data))); parseErr == nil {
			if isProcessRunning(existing) {
				return nil, fmt.Errorf("forj dev already running (pid %d). remove %s if you are sure it's stale", existing, lockPath)
			}
		}
	}

	// Stale lock file: remove it and retry.
	_ = os.Remove(lockPath)
	lockFile, err = os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create dev lock: %w", err)
	}
	_, _ = lockFile.WriteString(strconv.Itoa(pid))
	_ = lockFile.Close()
	return func() { _ = os.Remove(lockPath) }, nil
}

// isProcessRunning checks whether a PID exists on this host.
func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	return err == syscall.EPERM
}
