package forj

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	envx "github.com/goforj/env/v2"
	"github.com/goforj/execx"
	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
	"github.com/goforj/str"
)

var errDevInterrupted = errors.New("dev interrupted")

type DevCmd struct {
	logger *logger.AppLogger
}

type devRuntimeState struct {
	restartCh      chan struct{}
	buildCh        chan struct{}
	renderCh       chan struct{}
	refreshWriters func()
	streamer       *devwatchStreamer
	firstLoad      bool
}

func (*DevCmd) Signature() string {
	return `name:"dev" help:"Run development watchers"`
}

func NewDevCmd(logger *logger.AppLogger) *DevCmd {
	return &DevCmd{logger: logger}
}

func newDevRuntimeState(restartCh chan struct{}, buildCh chan struct{}, renderCh chan struct{}) *devRuntimeState {
	return &devRuntimeState{
		restartCh:      restartCh,
		buildCh:        buildCh,
		renderCh:       renderCh,
		refreshWriters: func() {},
		firstLoad:      true,
	}
}

func (r *devRuntimeState) Close() {
	if r.streamer != nil {
		r.streamer.Close()
	}
}

func (r *devRuntimeState) Sync() (*devwatchStreamer, error) {
	var err error
	if r.firstLoad {
		err = envx.Load()
		r.firstLoad = false
	} else {
		err = envx.Reload()
	}
	if err != nil {
		return nil, err
	}
	if r.streamer != nil {
		r.streamer.Close()
	}
	r.streamer = newDevwatchStreamerFromEnv()
	if r.streamer != nil {
		r.streamer.SetRestartChannel(r.restartCh)
		r.streamer.SetRenderChannel(r.renderCh)
	}
	r.refreshWriters()
	return r.streamer, nil
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

	runCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

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
	buildCh := make(chan struct{}, 1)
	renderCh := make(chan struct{}, 1)
	requestRestart := func() {
		select {
		case restartCh <- struct{}{}:
		default:
		}
	}
	requestRender := func() {
		select {
		case renderCh <- struct{}{}:
		default:
		}
	}
	requestBuild := func() {
		select {
		case buildCh <- struct{}{}:
		default:
		}
	}
	stopEnvWatch := startDevEnvFileWatcher(runCtx, requestBuild, 250*time.Millisecond)
	defer stopEnvWatch()
	var outWriter io.Writer
	var errWriter io.Writer
	shutdownWriters := func() {}
	defer shutdownWriters()
	runtimeState := newDevRuntimeState(restartCh, buildCh, renderCh)
	defer runtimeState.Close()

	for {
		currentStreamer, err := runtimeState.Sync()
		if err != nil {
			return err
		}
		if err := runPreDevSetup(config); err != nil {
			return err
		}
		if outWriter == nil || errWriter == nil {
			outWriter, errWriter, shutdownWriters, runtimeState.refreshWriters = buildDevOutputWriters(requestRestart, requestRender)
			runtimeState.refreshWriters()
		}

		if err := c.runWatchersLoop(config, currentStreamer, restartCh, buildCh, renderCh, runCtx.Done(), outWriter, errWriter, runtimeState.Sync); err != nil {
			if errors.Is(err, errDevInterrupted) {
				if config != nil && config.Dev.DownOnExit {
					console.Actionf("forj down > auto (set dev.down_on_exit: false to disable)")
					if err := runDevDownTasks(config.Dev.Down); err != nil {
						console.Errorf("forj down failed: %v", err)
					} else {
						console.Successf("forj down complete")
					}
				}
				return nil
			}
			return err
		}
	}
}

func ensureDevDatabaseExists(config *project.Config) error {
	if config == nil {
		return nil
	}
	components := config.Render.Components
	switch {
	case components.DatabaseMySQL:
		res, err := execx.Command("bash", "-c", "docker-compose exec -T mysql sh -c 'mysql -h \"mysql\" -uroot -p\"$MARIADB_ROOT_PASSWORD\" -e \"CREATE DATABASE IF NOT EXISTS \\`$MARIADB_DATABASE\\`;\"'").
			EnvInherit().
			StdinReader(os.Stdin).
			StdoutWriter(os.Stdout).
			StderrWriter(os.Stderr).
			Run()
		if err != nil {
			return fmt.Errorf("ensure mysql database failed: %v", err)
		}
		if !res.OK() {
			return fmt.Errorf("ensure mysql database failed with exit code %d", res.ExitCode)
		}
	case components.DatabasePostgres:
		res, err := execx.Command("bash", "-c", "docker-compose exec -T postgres sh -c 'psql -U \"$POSTGRES_USER\" -h \"postgres\" -d postgres -v ON_ERROR_STOP=1 -tc \"SELECT 1 FROM pg_database WHERE datname = '\\''$POSTGRES_DB'\\''\" | grep -q 1 || psql -U \"$POSTGRES_USER\" -h \"postgres\" -d postgres -v ON_ERROR_STOP=1 -c \"CREATE DATABASE \\\"$POSTGRES_DB\\\";\"'").
			EnvInherit().
			StdinReader(os.Stdin).
			StdoutWriter(os.Stdout).
			StderrWriter(os.Stderr).
			Run()
		if err != nil {
			return fmt.Errorf("ensure postgres database failed: %v", err)
		}
		if !res.OK() {
			return fmt.Errorf("ensure postgres database failed with exit code %d", res.ExitCode)
		}
	}
	return nil
}

func runDevTasks(heading string, tasks []project.DevTask) error {
	if len(tasks) == 0 {
		return nil
	}
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		return fmt.Errorf("open %s: %w", os.DevNull, err)
	}
	defer devNull.Close()
	console.Actionf("%s", heading)
	for _, task := range tasks {
		fmt.Printf(" %s %s\n", console.ActionMark(), task.Name)
		res, err := execx.Command("bash", "-c", task.Cmd).
			EnvInherit().
			StdinReader(devNull).
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
	return nil
}

func runPreDevSetup(config *project.Config) error {
	if config == nil {
		return nil
	}
	components := config.Render.Components
	preTasks := config.Dev.Pre
	postMigrateTasks := make([]project.DevTask, 0, len(config.Dev.Pre))
	if config.Dev.AutoMigrate && components.HasDatabase() {
		preTasks = make([]project.DevTask, 0, len(config.Dev.Pre))
		for _, task := range config.Dev.Pre {
			if shouldRunAfterMigrate(task) {
				postMigrateTasks = append(postMigrateTasks, task)
				continue
			}
			preTasks = append(preTasks, task)
		}
	}
	if err := runDevTasks("Running pre-dev setup", preTasks); err != nil {
		return err
	}
	if config.Dev.AutoMigrate && components.HasDatabase() && components.Docker {
		if err := ensureDevDatabaseExists(config); err != nil {
			return err
		}
	}
	if config.Dev.AutoMigrate && components.HasDatabase() {
		console.Actionf("Running auto-migrate")
		res, err := execx.Command("bash", "-c", "./bin/app migrate").
			EnvInherit().
			StdinReader(os.Stdin).
			StdoutWriter(os.Stdout).
			StderrWriter(os.Stderr).
			Run()
		if err != nil {
			return fmt.Errorf("auto-migrate failed: %v", err)
		}
		if !res.OK() {
			return fmt.Errorf("auto-migrate failed with exit code %d", res.ExitCode)
		}
	}
	if err := runDevTasks("Running post-migrate setup", postMigrateTasks); err != nil {
		return err
	}
	return nil
}

func shouldRunAfterMigrate(task project.DevTask) bool {
	name := str.Of(task.Name)
	cmd := str.Of(task.Cmd)
	if name.ContainsFold("generate db accessors") {
		return true
	}
	return cmd.ContainsFold("generate")
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

type watcherLifecycleState string

const (
	watcherStateStarted  watcherLifecycleState = "started"
	watcherStateStopping watcherLifecycleState = "stopping"
	watcherStateStopped  watcherLifecycleState = "stopped"
)

// runWatchersLoop starts all configured watchers, handles restart requests, and surfaces exit errors.
func (c *DevCmd) runWatchersLoop(
	config *project.Config,
	streamer *devwatchStreamer,
	restartCh chan struct{},
	buildCh chan struct{},
	renderCh chan struct{},
	stopCh <-chan struct{},
	outWriter io.Writer,
	errWriter io.Writer,
	reloadRuntime func() (*devwatchStreamer, error),
) error {
	for {
		watchers, exitCh := startWatchers(
			config.ProjectName,
			config.Dev.Watches,
			streamer,
			outWriter,
			errWriter,
			config.Dev.SoundOnWatchError,
		)
		printDevReadySummary(outWriter, config, snapshotProcessEnv())
		select {
		case <-stopCh:
			disableDevFooter(outWriter)
			disableDevFooter(errWriter)
			fmt.Println(buildDevFooterSeparatorLine())
			stopWatchers(watchers, 5*time.Second, outWriter, streamer, true)
			drainWatcherExits(exitCh, len(watchers), outWriter, streamer, true)
			return errDevInterrupted
		case <-restartCh:
			console.Actionf("Restarting dev watchers")
			stopWatchers(watchers, 5*time.Second, outWriter, streamer, true)
			drainWatcherExits(exitCh, len(watchers), outWriter, streamer, true)
			drainRestartSignals(restartCh)
			refreshedStreamer, err := reloadRuntime()
			if err != nil {
				return err
			}
			streamer = refreshedStreamer
			continue
		case <-buildCh:
			console.Actionf("Rebuilding app and restarting watchers")
			stopWatchers(watchers, 5*time.Second, outWriter, streamer, true)
			drainWatcherExits(exitCh, len(watchers), outWriter, streamer, true)
			refreshedStreamer, err := reloadRuntime()
			if err != nil {
				return err
			}
			streamer = refreshedStreamer
			if err := runDevBuild(outWriter, errWriter); err != nil {
				disableDevFooter(outWriter)
				disableDevFooter(errWriter)
				fmt.Println(buildDevFooterSeparatorLine())
				console.Errorf("forj build failed: %v", err)
				return fmt.Errorf("forj build failed: %w", err)
			}
			console.Successf("forj build complete")
			drainBuildSignals(buildCh)
			continue
		case <-renderCh:
			console.Actionf("Rendering app and restarting watchers")
			stopWatchers(watchers, 5*time.Second, outWriter, streamer, true)
			drainWatcherExits(exitCh, len(watchers), outWriter, streamer, true)
			refreshedStreamer, err := reloadRuntime()
			if err != nil {
				return err
			}
			streamer = refreshedStreamer
			if err := runDevRender(outWriter, errWriter); err != nil {
				disableDevFooter(outWriter)
				disableDevFooter(errWriter)
				fmt.Println(buildDevFooterSeparatorLine())
				console.Errorf("forj render failed: %v", err)
				return fmt.Errorf("forj render failed: %w", err)
			}
			console.Successf("forj render/build complete")
			drainRenderSignals(renderCh)
			continue
		case exit := <-exitCh:
			emitWatcherLifecycleLine(outWriter, streamer, exit.name, watcherStateStopped)
			stopWatchers(removeWatcherByName(watchers, exit.name), 5*time.Second, outWriter, streamer, false)
			drainWatcherExits(exitCh, len(watchers)-1, outWriter, streamer, false)
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

func snapshotProcessEnv() map[string]string {
	envMap := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		envMap[key] = value
	}
	return envMap
}

func runDevRender(outWriter io.Writer, errWriter io.Writer) error {
	if err := runDevTerminalCommand(outWriter, errWriter, "Running forj render", "forj render"); err != nil {
		return fmt.Errorf("forj render failed: %w", err)
	}
	if err := runDevBuild(outWriter, errWriter); err != nil {
		return err
	}
	return nil
}

func runDevBuild(outWriter io.Writer, errWriter io.Writer) error {
	if err := runDevTerminalCommand(outWriter, errWriter, "Running forj build", "forj build"); err != nil {
		return fmt.Errorf("forj build failed: %w", err)
	}
	return nil
}

func runDevTerminalCommand(outWriter io.Writer, errWriter io.Writer, heading string, command string) error {
	console.Actionf("%s", heading)
	// Render output should go straight to the terminal so the renderer keeps
	// its native colors/box drawing and the sticky footer does not get replayed
	// into the transcript while ad hoc commands are running.
	disableDevFooter(outWriter)
	disableDevFooter(errWriter)
	defer enableDevFooter(outWriter)
	defer enableDevFooter(errWriter)

	cmd := execx.Command("bash", "-c", command).
		EnvInherit().
		EnvAppend(map[string]string{"CLICOLOR_FORCE": "1"}).
		StdinReader(os.Stdin).
		StdoutWriter(os.Stdout).
		StderrWriter(os.Stderr)
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		cmd = cmd.WithPTY().StderrWriter(nil)
	}
	res, err := cmd.Run()
	if err != nil {
		return err
	}
	if !res.OK() {
		return fmt.Errorf("%s exited with code %d", command, res.ExitCode)
	}
	return nil
}

func printDevReadySummary(out io.Writer, config *project.Config, env map[string]string) {
	for _, line := range buildDevReadySummaryLines(config, env) {
		_, _ = io.WriteString(out, line+"\n")
	}
}

type devToolLink struct {
	Label  string
	URL    string
	Detail string
}

func buildDevReadySummaryLines(config *project.Config, env map[string]string) []string {
	tools := collectDevToolLinks(config, env)
	if len(tools) == 0 {
		return nil
	}

	lines := []string{
		fmt.Sprintf("%s %s", console.SuccessMark(), console.Colorize(console.ColorBoldWhite, "Dev ready")),
		fmt.Sprintf("%s %s", console.ActionMark(), console.Colorize(console.ColorGray, "Local tools")),
	}
	for _, tool := range tools {
		line := fmt.Sprintf("  %s %s", console.Colorize(console.ColorBoldGreen, "→"), console.Colorize(console.ColorBoldWhite, tool.Label))
		if tool.Detail != "" {
			line += " " + console.Colorize(console.ColorGray, tool.Detail)
		}
		line += ": " + console.Colorize(console.ColorBoldWhite, tool.URL)
		lines = append(lines, line)
	}
	return lines
}

func collectDevToolLinks(config *project.Config, env map[string]string) []devToolLink {
	tools := []devToolLink{}

	if apiURL := resolveAPIURL(env); apiURL != "" {
		tools = append(tools, devToolLink{Label: "App", URL: apiURL})
	}
	if lighthouseURL := resolveLighthouseUIURL(env); lighthouseURL != "" {
		tools = append(tools, devToolLink{Label: "Lighthouse", URL: lighthouseURL})
	}

	if config == nil {
		return tools
	}

	components := config.Render.Components
	if components.Mail && components.Docker {
		tools = append(tools, devToolLink{
			Label:  "Mailpit",
			Detail: "(inbox)",
			URL:    resolveURLWithPort(env, "http", "localhost", "MAILPIT_HTTP_PORT", "8025"),
		})
	}
	if components.Observability {
		tools = append(tools, devToolLink{
			Label: "VictoriaMetrics",
			URL:   resolveURLWithPort(env, "http", "localhost", "OBSERVABILITY_VM_PORT", "8428"),
		})
	}
	if components.Grafana {
		adminUser := strings.TrimSpace(envValue(env, "GRAFANA_ADMIN_USER"))
		if adminUser == "" {
			adminUser = "admin"
		}
		tools = append(tools, devToolLink{
			Label:  "Grafana",
			Detail: fmt.Sprintf("(%s / admin)", adminUser),
			URL:    resolveURLWithPort(env, "http", "localhost", "GRAFANA_PORT", "3001"),
		})
	}

	return tools
}

func resolveAPIURL(env map[string]string) string {
	if raw := strings.TrimSpace(envValue(env, "APP_URL")); raw != "" {
		return raw
	}
	return "http://localhost:3000"
}

func resolveLighthouseUIURL(env map[string]string) string {
	enabled := strings.ToLower(strings.TrimSpace(envValue(env, "LIGHTHOUSE_ENABLED")))
	if enabled == "false" || enabled == "0" || enabled == "off" || enabled == "no" {
		return ""
	}

	raw := strings.TrimSpace(envValue(env, "LIGHTHOUSE_URL"))
	if raw == "" {
		return "http://localhost:3000/lighthouse"
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "http://localhost:3000/lighthouse"
	}
	switch strings.ToLower(u.Scheme) {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	case "":
		u.Scheme = "http"
	}
	u.Path = "/lighthouse"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func resolveURLWithPort(env map[string]string, scheme, host, portKey, fallbackPort string) string {
	port := strings.TrimSpace(envValue(env, portKey))
	if port == "" {
		port = fallbackPort
	}
	return fmt.Sprintf("%s://%s:%s", scheme, host, port)
}

func envValue(env map[string]string, key string) string {
	if env != nil {
		return env[key]
	}
	return os.Getenv(key)
}

// startWatchers launches each watcher command with its own process and returns a channel for exits.
func startWatchers(
	projectName string,
	watches []project.DevWatch,
	streamer *devwatchStreamer,
	outWriter io.Writer,
	errWriter io.Writer,
	soundOnError bool,
) ([]runningWatcher, <-chan watcherExit) {
	exitCh := make(chan watcherExit, len(watches))
	watchers := make([]runningWatcher, 0, len(watches))
	// Only non-postponed watchers emit an initial trigger during boot, so the
	// startup block should close after those watchers have reported "starting".
	// The single runtime supervisor watcher restarts after a fresh binary lands,
	// so we bracket that restart with explicit shutdown/start section separators.
	lifecycleState := newDevwatchLifecycleState(countImmediateStartupWatchers(watches), []string{"Run App"})
	if len(watches) > 0 {
		_, _ = io.WriteString(outWriter, buildDevFooterSeparatorLine()+"\n")
	}
	startedNames := make([]string, 0, len(watches))
	for _, watch := range watches {
		watchEnv, watchExecCmd := splitWatcherEnvAssignments(watch.Exec)
		watchExec := buildWatcherExec(watchExecCmd)
		triggerCmd := strings.Join(strings.Fields(watch.Exec), " ")
		wgoArgs := buildWatcherCommandArgs(watch.Watch, watchExec)
		cmdEnv := snapshotProcessEnv()
		for key, value := range watch.Env {
			cmdEnv[key] = value
		}
		for key, value := range watchEnv {
			cmdEnv[key] = value
		}
		if watch.Name == "Build App" {
			cmdEnv["FORJ_BUILD_PROGRESS"] = "1"
		}
		cmd := execx.Command("wgo").
			Arg(wgoArgs).
			EnvOnly(cmdEnv).
			StdoutWriter(newDevwatchWriter(outWriter, streamer, "stdout", watch.Name, triggerCmd, lifecycleState)).
			StderrWriter(newDevwatchWriter(errWriter, streamer, "stderr", watch.Name, triggerCmd, lifecycleState))
		cmd = configureWatcherPTY(cmd, soundOnError)
		proc := cmd.Start()
		watchers = append(watchers, runningWatcher{name: watch.Name, proc: proc})
		startedNames = append(startedNames, watch.Name)
		go func(name string, proc *execx.Process) {
			res, err := proc.Wait()
			exitCh <- watcherExit{name: name, result: res, err: err}
		}(watch.Name, proc)
	}
	emitWatcherLifecycleSummary(outWriter, streamer, startedNames, watcherStateStarted)
	return watchers, exitCh
}

func buildWatcherExec(execCmd string) string {
	return fmt.Sprintf("echo __FORJ_WATCHER_TRIGGER__; exec %s", execCmd)
}

func buildWatcherCommandArgs(watchExpr string, execCmd string) []string {
	args, err := shellSplitArgs(watchExpr)
	if err != nil {
		args = strings.Fields(watchExpr)
	}
	args = append(args, "sh", "-c", execCmd)
	return args
}

// stopWatchers gracefully terminates every running watcher process.
// Coordinated shutdowns (restart/render/Ctrl+C) collapse the in-progress signal
// into one summary line; single watcher failures still emit per-watcher stops.
func stopWatchers(watchers []runningWatcher, timeout time.Duration, out io.Writer, streamer *devwatchStreamer, collapse bool) {
	if collapse {
		names := make([]string, 0, len(watchers))
		for _, watcher := range watchers {
			if watcher.proc == nil {
				continue
			}
			names = append(names, watcher.name)
		}
		emitWatcherLifecycleSummary(out, streamer, names, watcherStateStopping)
	}
	for _, watcher := range watchers {
		if watcher.proc == nil {
			continue
		}
		if !collapse {
			emitWatcherLifecycleLine(out, streamer, watcher.name, watcherStateStopping)
		}
		_ = watcher.proc.GracefulShutdown(os.Interrupt, timeout)
	}
}

// drainWatcherExits drains a fixed number of exit events to ensure goroutines finish cleanly.
func drainWatcherExits(exitCh <-chan watcherExit, count int, out io.Writer, streamer *devwatchStreamer, collapse bool) {
	names := make([]string, 0, count)
	for i := 0; i < count; i++ {
		exit := <-exitCh
		if collapse {
			names = append(names, exit.name)
			continue
		}
		emitWatcherLifecycleLine(out, streamer, exit.name, watcherStateStopped)
	}
	if collapse {
		emitWatcherLifecycleSummary(out, streamer, names, watcherStateStopped)
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

func drainRenderSignals(ch chan struct{}) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func drainBuildSignals(ch chan struct{}) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func removeWatcherByName(watchers []runningWatcher, name string) []runningWatcher {
	if len(watchers) == 0 {
		return nil
	}
	filtered := make([]runningWatcher, 0, len(watchers))
	for _, watcher := range watchers {
		if watcher.name == name {
			continue
		}
		filtered = append(filtered, watcher)
	}
	return filtered
}

// formatWatcherNameList renders a compact watcher summary.
func formatWatcherNameList(watches []project.DevWatch) string {
	var b strings.Builder
	for i, watch := range watches {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strings.TrimSpace(watch.Name))
	}
	return b.String()
}

func countImmediateStartupWatchers(watches []project.DevWatch) int {
	count := 0
	for _, watch := range watches {
		// wgo -postpone waits for the first file change, so those watchers do not
		// participate in the initial startup burst.
		if strings.Contains(watch.Watch, "-postpone") {
			continue
		}
		count++
	}
	return count
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

func copyEnvMap(vars map[string]string) map[string]string {
	if len(vars) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(vars))
	for key, value := range vars {
		out[key] = value
	}
	return out
}

func splitWatcherEnvAssignments(execCmd string) (map[string]string, string) {
	fields := strings.Fields(execCmd)
	if len(fields) == 0 {
		return nil, execCmd
	}
	env := map[string]string{}
	consumed := 0
	for _, field := range fields {
		key, value, ok := strings.Cut(field, "=")
		if !ok || key == "" || !isShellEnvName(key) {
			break
		}
		env[key] = value
		consumed++
	}
	if consumed == 0 {
		return nil, execCmd
	}
	rest := strings.Join(fields[consumed:], " ")
	if strings.TrimSpace(rest) == "" {
		return nil, execCmd
	}
	return env, rest
}

func shellSplitArgs(value string) ([]string, error) {
	var (
		args        []string
		current     strings.Builder
		inSingle    bool
		inDouble    bool
		escaped     bool
		sawFragment bool
	)
	flush := func() {
		if !sawFragment {
			return
		}
		args = append(args, current.String())
		current.Reset()
		sawFragment = false
	}
	for _, r := range value {
		switch {
		case escaped:
			current.WriteRune(r)
			sawFragment = true
			escaped = false
		case r == '\\' && !inSingle && !inDouble:
			escaped = true
		case r == '\\' && inDouble:
			current.WriteRune(r)
			sawFragment = true
		case r == '\'' && !inDouble:
			inSingle = !inSingle
			sawFragment = true
		case r == '"' && !inSingle:
			inDouble = !inDouble
			sawFragment = true
		case (r == ' ' || r == '\t' || r == '\n') && !inSingle && !inDouble:
			flush()
		default:
			current.WriteRune(r)
			sawFragment = true
		}
	}
	if escaped || inSingle || inDouble {
		return nil, fmt.Errorf("unterminated shell argument")
	}
	flush()
	return args, nil
}

func isShellEnvName(name string) bool {
	for i, r := range name {
		switch {
		case r == '_':
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
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
	errorLimiter := newSoundLimiter(2 * time.Second)
	hadError := false
	var recoveryTimer *time.Timer
	return func(line string) {
		if isWatcherTriggerLine(line) {
			if hadError {
				if recoveryTimer != nil {
					recoveryTimer.Stop()
				}
				recoveryTimer = time.AfterFunc(750*time.Millisecond, func() {
					hadError = false
					playRecoverySound()
				})
			}
			return
		}
		if !containsErrorWord(line) {
			return
		}
		if recoveryTimer != nil {
			recoveryTimer.Stop()
			recoveryTimer = nil
		}
		hadError = true
		if errorLimiter.Allow() {
			playErrorSound()
		}
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
		"too many arguments in call",
		"not in selector",
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
	if m.ContainsFold("build app") && m.ContainsFold("error executing command") {
		return true
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

// playRecoverySound plays a macOS recovery sound when available.
func playRecoverySound() {
	if runtime.GOOS != "darwin" {
		return
	}
	_ = execx.Command("afplay", "/System/Library/Sounds/Glass.aiff").Start()
}

func emitWatcherLifecycleLine(out io.Writer, streamer *devwatchStreamer, watcher string, state watcherLifecycleState) {
	line := formatWatcherLifecycleLine(watcher, state)
	if line == "" {
		return
	}
	timestamp := time.Now()
	if streamer != nil {
		streamer.Send(devwatchLine{
			Line:      line,
			Stream:    "stdout",
			Timestamp: timestamp,
			ID:        timestamp.UnixMilli(),
			Watcher:   watcher,
		})
	}
	if out == nil {
		return
	}
	devwatchOutputMu.Lock()
	defer devwatchOutputMu.Unlock()
	_, _ = io.WriteString(out, line)
	_, _ = io.WriteString(out, "\n")
}

func emitWatcherLifecycleSummary(out io.Writer, streamer *devwatchStreamer, watchers []string, state watcherLifecycleState) {
	line := formatWatcherLifecycleSummary(watchers, state)
	if line == "" {
		return
	}
	timestamp := time.Now()
	if streamer != nil {
		streamer.Send(devwatchLine{
			Line:      line,
			Stream:    "stdout",
			Timestamp: timestamp,
			ID:        timestamp.UnixMilli(),
		})
	}
	if out == nil {
		return
	}
	devwatchOutputMu.Lock()
	defer devwatchOutputMu.Unlock()
	_, _ = io.WriteString(out, line)
	_, _ = io.WriteString(out, "\n")
}

func formatWatcherLifecycleLine(watcher string, state watcherLifecycleState) string {
	watcher = str.Of(watcher).TrimSpace().String()
	if watcher == "" {
		return ""
	}

	mark := console.InfoMark()
	label := console.Colorize(console.ColorGray, string(state))
	switch state {
	case watcherStateStarted:
		mark = console.SuccessMark()
		label = console.Colorize(console.ColorGreen, string(state))
	case watcherStateStopping:
		mark = console.InfoMark()
		label = console.Colorize(console.ColorGray, string(state))
	case watcherStateStopped:
		mark = console.SuccessMark()
		label = console.Colorize(console.ColorGreen, string(state))
	}

	return fmt.Sprintf(
		"%s %s · %s · %s",
		mark,
		console.Colorize(console.ColorBoldWhite, "GoForj Watcher"),
		console.Colorize(console.ColorGray, watcher),
		label,
	)
}

func formatWatcherLifecycleSummary(watchers []string, state watcherLifecycleState) string {
	names := make([]string, 0, len(watchers))
	for _, watcher := range watchers {
		watcher = str.Of(watcher).TrimSpace().String()
		if watcher == "" {
			continue
		}
		names = append(names, watcher)
	}
	if len(names) == 0 {
		return ""
	}

	mark := console.InfoMark()
	label := console.Colorize(console.ColorGray, string(state))
	switch state {
	case watcherStateStarted:
		mark = console.SuccessMark()
		label = console.Colorize(console.ColorGreen, string(state))
	}

	return fmt.Sprintf(
		"%s %s · %s · %s",
		mark,
		console.Colorize(console.ColorBoldWhite, "GoForj Watchers"),
		label,
		console.Colorize(console.ColorGray, strings.Join(names, ", ")),
	)
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
	if err := ensureTool("wire", wireInstallTarget); err != nil {
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
