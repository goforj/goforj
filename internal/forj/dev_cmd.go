package forj

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/goforj/execx"
	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
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

	config, err := project.LoadProjectConfig()
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
	envMap := map[string]string{}
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "APP_") {
			key, value, _ := strings.Cut(env, "=")
			envMap[key] = value
		}
	}
	prettyCmd := formatWatcherCommandList(config.Dev.Watches)
	outWriter := io.Writer(os.Stdout)
	errWriter := io.Writer(os.Stderr)

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
		for _, watch := range config.Dev.Watches {
			fmt.Printf(" %s %s\n", console.ActionMark(), watch.Name)
		}
		if err := c.runWatchersLoop(config, envMap, streamer, restartCh, outWriter, errWriter, prettyCmd); err != nil {
			return err
		}
	}
}

// runningWatcher tracks a watcher process and its configured name.
type runningWatcher struct {
	name string
	proc *execx.Process
}

// watcherExit reports the result of a watcher process after it finishes.
type watcherExit struct {
	name   string
	result execx.Result
	err    error
}

// runWatchersLoop starts all configured watchers, handles restart requests, and surfaces exit errors.
func (c *DevCmd) runWatchersLoop(
	config *project.Config,
	envMap map[string]string,
	streamer *devwatchStreamer,
	restartCh chan struct{},
	outWriter io.Writer,
	errWriter io.Writer,
	prettyCmd string,
) error {
	for {
		watchers, exitCh := startWatchers(
			config.ProjectName,
			config.Dev.Watches,
			envMap,
			streamer,
			outWriter,
			errWriter,
			prettyCmd,
			config.Dev.SoundOnWatchError,
		)
		select {
		case <-restartCh:
			console.Actionf("Restarting dev watchers")
			stopWatchers(watchers, 5*time.Second)
			drainWatcherExits(exitCh, len(watchers))
			drainRestartSignals(restartCh)
			continue
		case exit := <-exitCh:
			stopWatchers(watchers, 5*time.Second)
			drainWatcherExits(exitCh, len(watchers)-1)
			if exit.err != nil {
				return exit.err
			}
			if !exit.result.OK() {
				return fmt.Errorf("dev watchers exited with code %d", exit.result.ExitCode)
			}
			return nil
		}
	}
}

// startWatchers launches each watcher command with its own process and returns a channel for exits.
func startWatchers(
	projectName string,
	watches []project.DevWatch,
	envMap map[string]string,
	streamer *devwatchStreamer,
	outWriter io.Writer,
	errWriter io.Writer,
	prettyCmd string,
	soundOnError bool,
) ([]runningWatcher, <-chan watcherExit) {
	exitCh := make(chan watcherExit, len(watches))
	watchers := make([]runningWatcher, 0, len(watches))
	for i, watch := range watches {
		logPrefix := buildLogPrefix(projectName, watch.Name)
		watchExec := fmt.Sprintf("echo __FORJ_WATCHER_TRIGGER__; APP_LOG_PREFIX=%s %s", strconv.Quote(logPrefix), watch.Exec)
		triggerCmd := strings.Join(strings.Fields(watch.Exec), " ")
		wgoCmd := fmt.Sprintf(
			"wgo %s sh -c %s",
			watch.Watch,
			shellQuote(watchExec),
		)
		cmd := execx.Command("bash", "-c", wgoCmd).
			EnvOnly(envMap).
			StdinReader(os.Stdin).
			StdoutWriter(newDevwatchWriter(outWriter, streamer, "stdout", watch.Name, triggerCmd)).
			StderrWriter(newDevwatchWriter(errWriter, streamer, "stderr", watch.Name, triggerCmd))
		if i == 0 && prettyCmd != "" {
			cmd = cmd.ShadowPrint(
				execx.WithPrefix("\nforj dev"),
				execx.WithMask(func(cmd string) string {
					return "\n  " + prettyCmd + "\n"
				}),
			)
		}
		cmd = configureWatcherPTY(cmd, soundOnError)
		proc := cmd.Start()
		watchers = append(watchers, runningWatcher{name: watch.Name, proc: proc})
		go func(name string, proc *execx.Process) {
			res, err := proc.Wait()
			exitCh <- watcherExit{name: name, result: res, err: err}
		}(watch.Name, proc)
	}
	fmt.Println("")
	return watchers, exitCh
}

// stopWatchers gracefully terminates every running watcher process.
func stopWatchers(watchers []runningWatcher, timeout time.Duration) {
	for _, watcher := range watchers {
		if watcher.proc == nil {
			continue
		}
		_ = watcher.proc.GracefulShutdown(os.Interrupt, timeout)
	}
}

// drainWatcherExits drains a fixed number of exit events to ensure goroutines finish cleanly.
func drainWatcherExits(exitCh <-chan watcherExit, count int) {
	for i := 0; i < count; i++ {
		<-exitCh
	}
}

// drainRestartSignals empties pending restart notifications.
func drainRestartSignals(ch chan struct{}) {
	for {
		select {
		case <-ch:
		default:
			return
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
func formatWatcherCommandList(watches []project.DevWatch) string {
	var b strings.Builder
	for i, watch := range watches {
		if i > 0 {
			b.WriteString("\n  ")
		}
		b.WriteString("wgo ")
		b.WriteString(watch.Watch)
		b.WriteString(" -- ")
		b.WriteString(watch.Exec)
	}
	return b.String()
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
func runDevDownTasks(tasks []project.DevTask) error {
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
	if err := ensureTool("wgo", "github.com/bokwoon95/wgo@v0.6.3"); err != nil {
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
