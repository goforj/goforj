package forj

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/goforj/execx"
	"github.com/goforj/goforj/internal/logger"
)

type DevCmd struct {
	logger *logger.AppLogger
}

func NewDevCmd(logger *logger.AppLogger) *DevCmd {
	return &DevCmd{logger: logger}
}

func (c *DevCmd) Run() error {
	// Prevent concurrent dev sessions from clobbering each other.
	unlock, err := c.acquireLock()
	if err != nil {
		return err
	}
	defer unlock()

	// Ensure the lock is released on Ctrl+C or termination.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		<-sigCh
		unlock()
		os.Exit(1)
	}()

	config, err := LoadProjectConfig()
	if err != nil {
		return err
	}

	if len(config.DevWatches) == 0 {
		fmt.Println("No dev watches defined in .goforj.yml")
		return nil
	}

	// 🐾 Run pre-dev commands if any
	if len(config.PreDev) > 0 {
		fmt.Println("🔧 Running pre-dev setup:")
		for _, task := range config.PreDev {
			fmt.Printf(" > %s...\n", task.Name)
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

	fmt.Println("🚀 Running dev watchers:")

	var segments []string
	for _, watch := range config.DevWatches {
		execMsg := shellQuote(fmt.Sprintf(
			"· %sGoForj Watcher%s > %s%s%s",
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
	if envMap["NO_COLOR"] == "" && envMap["CLICOLOR_FORCE"] == "" {
		envMap["CLICOLOR_FORCE"] = "1"
	}

	prettyCmd := formatWatcherCommandList(config.DevWatches)
	res, err := execx.Command("bash", "-c", fullCmd).
		EnvOnly(envMap).
		StdinReader(os.Stdin).
		StdoutWriter(os.Stdout).
		StderrWriter(os.Stderr).
		ShadowPrint(
			execx.WithPrefix("\nforj dev"),
			execx.WithMask(func(cmd string) string {
				return "\n\n  " + prettyCmd + "\n"
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
