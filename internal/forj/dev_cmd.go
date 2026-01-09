package forj

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/goforj/execx"
	"github.com/goforj/goforj/internal/logger"
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
			fmt.Printf("\n %s forj down > auto (set dev.down_on_exit: false to disable)\n", actionMark())
			if err := runDevDownTasks(config.Dev.Down); err != nil {
				fmt.Printf(" %s forj down failed: %v\n", errorMark(), err)
			} else {
				fmt.Printf(" %s forj down complete\n", successMark())
			}
		}
		unlock()
		os.Exit(1)
	}()

	if len(config.Dev.Watches) == 0 {
		fmt.Printf("%s No dev watches defined in .goforj.yml\n", warnMark())
		return nil
	}

	if err := ensureDevTools(); err != nil {
		return err
	}
	if err := ensureBinDir(); err != nil {
		return err
	}

	// Run pre-dev commands if any
	if len(config.Dev.Pre) > 0 {
		fmt.Printf(" %s Running pre-dev setup\n", actionMark())
		for _, task := range config.Dev.Pre {
			fmt.Printf("  %s %s \n", actionMark(), task.Name)
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

	fmt.Printf(" %s Running dev watchers\n", actionMark())

	var segments []string
	for _, watch := range config.Dev.Watches {
		fmt.Printf("  %s %s\n", actionMark(), watch.Name)
		execMsg := shellQuote(fmt.Sprintf(
			" · %sGoForj Watcher%s · %s%s%s",
			colorBoldWhite,
			colorReset,
			colorGray,
			watch.Name,
			colorReset,
		))

		wgoCmd := fmt.Sprintf(
			"wgo %s -log-prefix='' -exec-log -exec-msg %s sh -c %q",
			watch.Watch,
			execMsg,
			watch.Exec,
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
	res, err := execx.Command("bash", "-c", fullCmd).
		EnvOnly(envMap).
		StdinReader(os.Stdin).
		StdoutWriter(os.Stdout).
		StderrWriter(os.Stderr).
		OnStdout(errorSoundHook(config.Dev.SoundOnWatchError)).
		OnStderr(errorSoundHook(config.Dev.SoundOnWatchError)).
		ShadowPrint(
			execx.WithPrefix("\nforj dev"),
			execx.WithMask(func(cmd string) string {
				return "\n  " + prettyCmd
			}),
		).
		Run()
	if err != nil {
		return err
	}
	if !res.OK() {
		return fmt.Errorf("dev watchers exited with code %d", res.ExitCode)
	}

	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\'\''`) + "'"
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
	fmt.Printf(" %s Bringing down resources\n", infoMark())
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
		fmt.Printf(" %s %s\n", successMark(), task.Name)
	}
	return nil
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
	lower := strings.ToLower(line)
	return strings.Contains(lower, "error") ||
		strings.Contains(lower, "failed") ||
		strings.Contains(lower, "fatal") ||
		strings.Contains(lower, "panic") ||
		strings.Contains(lower, "undefined") ||
		strings.Contains(lower, "imported and not used") ||
		strings.Contains(lower, "cannot") ||
		strings.Contains(lower, "no such file")
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
