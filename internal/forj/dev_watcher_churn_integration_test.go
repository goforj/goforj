//go:build watcherstress && !windows

package forj

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/build"
	"github.com/goforj/goforj/internal/devwatch"
	"github.com/goforj/goforj/internal/logger"
)

const (
	devWatcherChurnTimeout  = 45 * time.Second
	devWatcherChurnDebounce = 150 * time.Millisecond
)

type devWatcherChurnPaths struct {
	root       string
	buildLog   string
	buildCount string
	hold       string
	release    string
	lifecycle  string
	state      string
	transcript string
}

type devWatcherChurnState struct {
	version   string
	pid       int
	childPID  int
	heartbeat int
}

// TestDevWatcherChurnBuildHelper runs the production build pipeline in an observable subprocess.
func TestDevWatcherChurnBuildHelper(t *testing.T) {
	if os.Getenv("GOFORJ_WATCHER_CHURN_HELPER") != "1" {
		return
	}
	if devWatcherChurnHelperAction(os.Args) != "build" {
		os.Exit(2)
	}
	runDevWatcherChurnBuildHelper()
}

// TestDevWatcherChurnIntegration exercises filesystem delivery through atomic publication and process replacement.
func TestDevWatcherChurnIntegration(t *testing.T) {
	paths := newDevWatcherChurnPaths(t)
	writeDevWatcherChurnProject(t, paths.root, "initial")
	environment := devWatcherChurnEnvironment(paths)
	runDevWatcherChurnInitialBuild(t, paths, environment)

	goMatcher := mustDevWatcherChurnMatcher(t, ".go")
	binMatcher := mustDevWatcherChurnMatcher(t, "bin")
	buildID := "watcherstress/app/build"
	runtimeID := "watcherstress/app/runtime"
	compiled := []devCompiledWatcher{
		{
			ID: buildID, Name: "Build App", Kind: devWatcherAppBuild, App: "app",
			Watch: devwatch.Spec{
				Name: buildID, Roots: []string{paths.root}, Includes: []devwatch.Matcher{goMatcher},
				DirectoryExcludes: []devwatch.Matcher{binMatcher}, Debounce: devWatcherChurnDebounce, DebounceSet: true,
			},
			Command: devwatch.Command{
				Shell: devWatcherChurnHelperCommand(), Dir: paths.root, Env: environment,
			},
			Postpone: true, WatchChanges: true, OnSuccess: []string{runtimeID},
		},
		{
			ID: runtimeID, Name: "Run App", Kind: devWatcherAppRun, App: "app",
			Command: devwatch.Command{
				Shell: "./bin/app", Dir: paths.root, Env: environment, GracefulStopTimeout: 250 * time.Millisecond,
			},
			Restart: true,
		},
	}

	transcript, err := os.OpenFile(paths.transcript, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open watcher transcript: %v", err)
	}
	controller, err := newDevWatcherController(compiled, nil, transcript, transcript, false)
	if err != nil {
		_ = transcript.Close()
		t.Fatalf("start churn watcher controller: %v", err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			controller.stop(5 * time.Second)
		}
		_ = transcript.Close()
		if t.Failed() {
			t.Logf("build log:\n%s", readDevWatcherChurnFile(paths.buildLog))
			t.Logf("lifecycle log:\n%s", readDevWatcherChurnFile(paths.lifecycle))
			t.Logf("watcher transcript:\n%s", readDevWatcherChurnFile(paths.transcript))
		}
	})
	if len(controller.engines) != 1 || controller.engines[0].Health().State != devwatch.HealthHealthy {
		t.Fatalf("watcher engine did not establish healthy physical coverage: %#v", controller.engines)
	}
	buildTask := controller.tasks[buildID]

	initial := waitForDevWatcherChurnState(t, paths.state, func(state devWatcherChurnState) bool {
		return state.version == "initial"
	})

	for index := range 24 {
		writeDevWatcherChurnSource(t, paths.root, fmt.Sprintf("debounce-%02d", index), index%2 == 0)
	}
	writeDevWatcherChurnSource(t, paths.root, "debounced", true)
	waitForDevWatcherChurnBuildLine(t, paths.buildLog, "build-success:2")
	debounced := waitForDevWatcherChurnState(t, paths.state, func(state devWatcherChurnState) bool {
		return state.version == "debounced" && state.pid != initial.pid
	})
	waitForDevWatcherChurnTaskIdle(t, buildTask)
	assertDevWatcherChurnBuildCountStable(t, paths.buildLog, 2)

	writeDevWatcherChurnFile(t, paths.hold, "hold\n", 0o600)
	writeDevWatcherChurnSource(t, paths.root, "published-while-held", true)
	waitForDevWatcherChurnBuildLine(t, paths.buildLog, "build-blocked:3")
	waitForDevWatcherChurnHeartbeat(t, paths.state, debounced)

	for index := range 40 {
		writeDevWatcherChurnSource(t, paths.root, fmt.Sprintf("busy-%02d", index), index%2 == 1)
	}
	writeDevWatcherChurnSource(t, paths.root, "converged", true)
	waitForDevWatcherChurnQueuedFollowUp(t, buildTask)
	if err := os.Remove(paths.hold); err != nil {
		t.Fatalf("remove build hold: %v", err)
	}
	writeDevWatcherChurnFile(t, paths.release, "release\n", 0o600)
	waitForDevWatcherChurnBuildLine(t, paths.buildLog, "build-success:4")
	converged := waitForDevWatcherChurnState(t, paths.state, func(state devWatcherChurnState) bool {
		return state.version == "converged" && state.pid != debounced.pid
	})
	waitForDevWatcherChurnTaskIdle(t, buildTask)
	assertDevWatcherChurnBuildCountStable(t, paths.buildLog, 4)

	startsBeforeFailure := countDevWatcherChurnLines(paths.lifecycle, "runtime-start:")
	writeDevWatcherChurnFile(t, filepath.Join(paths.root, "cmd", "app", "main.go"), "package main\nfunc main() {\n", 0o644)
	waitForDevWatcherChurnBuildLine(t, paths.buildLog, "build-failed:5")
	waitForDevWatcherChurnHeartbeat(t, paths.state, converged)
	if starts := countDevWatcherChurnLines(paths.lifecycle, "runtime-start:"); starts != startsBeforeFailure {
		t.Fatalf("failed build restarted runtime: starts=%d, want %d", starts, startsBeforeFailure)
	}

	writeDevWatcherChurnSource(t, paths.root, "recovered", true)
	waitForDevWatcherChurnBuildLine(t, paths.buildLog, "build-success:6")
	recovered := waitForDevWatcherChurnState(t, paths.state, func(state devWatcherChurnState) bool {
		return state.version == "recovered" && state.pid != converged.pid
	})
	waitForDevWatcherChurnTaskIdle(t, buildTask)
	assertDevWatcherChurnBuildCountStable(t, paths.buildLog, 6)
	assertDevWatcherChurnLifecycleBounds(t, paths.lifecycle)
	assertNoDevWatcherChurnExit(t, controller)

	controller.stop(5 * time.Second)
	stopped = true
	if err := transcript.Close(); err != nil {
		t.Fatalf("close watcher transcript: %v", err)
	}
	assertDevWatcherChurnProcessesStopped(t, paths.lifecycle, recovered)
	assertDevWatcherChurnTranscript(t, paths.transcript)
}

// newDevWatcherChurnPaths allocates watched and control files beneath temporary storage.
func newDevWatcherChurnPaths(t *testing.T) devWatcherChurnPaths {
	t.Helper()
	control := t.TempDir()
	return devWatcherChurnPaths{
		root: t.TempDir(), buildLog: filepath.Join(control, "build.log"),
		buildCount: filepath.Join(control, "build.count"), hold: filepath.Join(control, "hold"),
		release: filepath.Join(control, "release"), lifecycle: filepath.Join(control, "lifecycle.log"),
		state: filepath.Join(control, "runtime.state"), transcript: filepath.Join(control, "watcher.log"),
	}
}

// devWatcherChurnEnvironment supplies build barriers, runtime probes, and isolated Go caches.
func devWatcherChurnEnvironment(paths devWatcherChurnPaths) map[string]string {
	return map[string]string{
		"GOFORJ_WATCHER_CHURN_HELPER": "1", "GOFORJ_WATCHER_CHURN_ROOT": paths.root,
		"GOFORJ_WATCHER_CHURN_BUILD_LOG": paths.buildLog, "GOFORJ_WATCHER_CHURN_BUILD_COUNT": paths.buildCount,
		"GOFORJ_WATCHER_CHURN_HOLD": paths.hold, "GOFORJ_WATCHER_CHURN_RELEASE": paths.release,
		"GOFORJ_WATCHER_CHURN_LIFECYCLE": paths.lifecycle, "GOFORJ_WATCHER_CHURN_STATE": paths.state,
		"GOCACHE":    devWatcherChurnEnvDefault("GOCACHE", "/tmp/gocache"),
		"GOMODCACHE": devWatcherChurnEnvDefault("GOMODCACHE", "/tmp/gomodcache"),
	}
}

// devWatcherChurnEnvDefault keeps the caller's cache choice while enforcing repository defaults when absent.
func devWatcherChurnEnvDefault(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

// devWatcherChurnHelperCommand returns the test subprocess command accepted by the production shell wrapper.
func devWatcherChurnHelperCommand() string {
	return devWatcherRunnerShellQuote(os.Args[0]) + " -test.run='^TestDevWatcherChurnBuildHelper$' -- build"
}

// devWatcherChurnHelperAction reads the helper action after the test flag separator.
func devWatcherChurnHelperAction(args []string) string {
	for index := len(args) - 1; index >= 0; index-- {
		if args[index] == "--" && index+1 < len(args) {
			return args[index+1]
		}
	}
	return ""
}

// runDevWatcherChurnInitialBuild publishes the baseline binary before filesystem subscriptions begin.
func runDevWatcherChurnInitialBuild(t *testing.T, paths devWatcherChurnPaths, environment map[string]string) {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestDevWatcherChurnBuildHelper$", "--", "build")
	command.Dir = paths.root
	command.Env = mergeDevWatcherChurnEnvironment(os.Environ(), environment)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("initial churn build: %v\n%s", err, output)
	}
	waitForDevWatcherChurnBuildLine(t, paths.buildLog, "build-success:1")
}

// runDevWatcherChurnBuildHelper records barriers around the exact production atomic build pipeline.
func runDevWatcherChurnBuildHelper() {
	count, err := incrementDevWatcherChurnBuildCount(os.Getenv("GOFORJ_WATCHER_CHURN_BUILD_COUNT"))
	if err != nil {
		os.Exit(3)
	}
	appendDevWatcherChurnLine(os.Getenv("GOFORJ_WATCHER_CHURN_BUILD_LOG"), fmt.Sprintf("build-start:%d", count))
	appLogger := logger.NewSilentLogger()
	command := build.NewCmd(appLogger, ProvideAPIIndexRunner(appLogger))
	command.Root = os.Getenv("GOFORJ_WATCHER_CHURN_ROOT")
	command.SkipWire = true
	command.Args = []string{"-o", "./bin/app", "./cmd/app"}
	if err := command.Run(); err != nil {
		appendDevWatcherChurnLine(os.Getenv("GOFORJ_WATCHER_CHURN_BUILD_LOG"), fmt.Sprintf("build-failed:%d", count))
		os.Exit(1)
	}
	appendDevWatcherChurnLine(os.Getenv("GOFORJ_WATCHER_CHURN_BUILD_LOG"), fmt.Sprintf("build-published:%d", count))
	if _, err := os.Stat(os.Getenv("GOFORJ_WATCHER_CHURN_HOLD")); err == nil {
		appendDevWatcherChurnLine(os.Getenv("GOFORJ_WATCHER_CHURN_BUILD_LOG"), fmt.Sprintf("build-blocked:%d", count))
		deadline := time.Now().Add(devWatcherChurnTimeout)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(os.Getenv("GOFORJ_WATCHER_CHURN_RELEASE")); err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if _, err := os.Stat(os.Getenv("GOFORJ_WATCHER_CHURN_RELEASE")); err != nil {
			os.Exit(4)
		}
	}
	appendDevWatcherChurnLine(os.Getenv("GOFORJ_WATCHER_CHURN_BUILD_LOG"), fmt.Sprintf("build-success:%d", count))
}

// incrementDevWatcherChurnBuildCount advances the serialized build-helper sequence.
func incrementDevWatcherChurnBuildCount(path string) (int, error) {
	count := 0
	if data, err := os.ReadFile(path); err == nil {
		count, err = strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			return 0, err
		}
	} else if !os.IsNotExist(err) {
		return 0, err
	}
	count++
	return count, os.WriteFile(path, []byte(strconv.Itoa(count)), 0o600)
}

// appendDevWatcherChurnLine persists a complete synchronization record before returning.
func appendDevWatcherChurnLine(path string, line string) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		os.Exit(5)
	}
	_, writeErr := fmt.Fprintln(file, line)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		os.Exit(6)
	}
}

// mergeDevWatcherChurnEnvironment replaces duplicate keys before starting helper processes.
func mergeDevWatcherChurnEnvironment(base []string, overrides map[string]string) []string {
	values := make(map[string]string, len(base)+len(overrides))
	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = entry
		}
	}
	for key, value := range overrides {
		values[key] = key + "=" + value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		environment = append(environment, values[key])
	}
	return environment
}

// writeDevWatcherChurnProject creates the smallest project accepted by the production build pipeline.
func writeDevWatcherChurnProject(t *testing.T, root string, version string) {
	t.Helper()
	writeDevWatcherChurnFile(t, filepath.Join(root, "go.mod"), "module example.com/watcherchurn\n\ngo 1.24\n", 0o644)
	writeDevWatcherChurnFile(t, filepath.Join(root, ".goforj.yml"), "project_name: WatcherChurn\ngo_module_name: example.com/watcherchurn\nupdated_at: \"2026-07-13 00:00:00 UTC\"\nrender:\n  components: []\n  component_contract: 1\n", 0o644)
	writeDevWatcherChurnSource(t, root, version, false)
}

// writeDevWatcherChurnSource applies direct writes and atomic editor replacements to the watched source.
func writeDevWatcherChurnSource(t *testing.T, root string, version string, atomicReplace bool) {
	t.Helper()
	path := filepath.Join(root, "cmd", "app", "main.go")
	source := strings.ReplaceAll(devWatcherChurnProgram, "__WATCHER_CHURN_VERSION__", strconv.Quote(version))
	if !atomicReplace {
		writeDevWatcherChurnFile(t, path, source, 0o644)
		return
	}
	temporary := path + ".editor-swap"
	writeDevWatcherChurnFile(t, temporary, source, 0o644)
	if err := os.Rename(temporary, path); err != nil {
		t.Fatalf("atomically replace churn source: %v", err)
	}
}

// writeDevWatcherChurnFile creates parent directories before replacing a test fixture.
func writeDevWatcherChurnFile(t *testing.T, path string, contents string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create churn fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatalf("write churn fixture %s: %v", path, err)
	}
}

// mustDevWatcherChurnMatcher compiles a watcher matcher required by the test graph.
func mustDevWatcherChurnMatcher(t *testing.T, value string) devwatch.Matcher {
	t.Helper()
	matcher, err := devwatch.NewMatcher(value)
	if err != nil {
		t.Fatalf("compile churn matcher %q: %v", value, err)
	}
	return matcher
}

// waitForDevWatcherChurnBuildLine waits for a durable helper synchronization record.
func waitForDevWatcherChurnBuildLine(t *testing.T, path string, expected string) {
	t.Helper()
	waitForDevWatcherChurnCondition(t, "build line "+expected, func() bool {
		return countDevWatcherChurnLines(path, expected) > 0
	})
}

// waitForDevWatcherChurnTaskIdle waits for command bookkeeping and its queued follow-up to drain.
func waitForDevWatcherChurnTaskIdle(t *testing.T, task *devWatcherTask) {
	t.Helper()
	waitForDevWatcherChurnCondition(t, "watcher task to become idle", func() bool {
		task.mu.Lock()
		defer task.mu.Unlock()
		return task.activeCancel == nil && !task.busy && len(task.triggerCh) == 0
	})
}

// waitForDevWatcherChurnQueuedFollowUp proves filesystem churn crossed debounce while a build was active.
func waitForDevWatcherChurnQueuedFollowUp(t *testing.T, task *devWatcherTask) {
	t.Helper()
	waitForDevWatcherChurnCondition(t, "one queued follow-up build", func() bool {
		task.mu.Lock()
		defer task.mu.Unlock()
		return task.busy && task.activeCancel != nil && len(task.triggerCh) == 1
	})
}

// waitForDevWatcherChurnState waits for an atomically published runtime probe.
func waitForDevWatcherChurnState(t *testing.T, path string, condition func(devWatcherChurnState) bool) devWatcherChurnState {
	t.Helper()
	var latest devWatcherChurnState
	waitForDevWatcherChurnCondition(t, "runtime state", func() bool {
		state, err := readDevWatcherChurnState(path)
		if err != nil {
			return false
		}
		latest = state
		return condition(state)
	})
	return latest
}

// waitForDevWatcherChurnHeartbeat proves the same last-good process remains live.
func waitForDevWatcherChurnHeartbeat(t *testing.T, path string, previous devWatcherChurnState) {
	t.Helper()
	waitForDevWatcherChurnState(t, path, func(state devWatcherChurnState) bool {
		return state.version == previous.version && state.pid == previous.pid && state.heartbeat > previous.heartbeat
	})
}

// waitForDevWatcherChurnCondition polls an observable barrier with a bounded diagnostic timeout.
func waitForDevWatcherChurnCondition(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(devWatcherChurnTimeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

// assertDevWatcherChurnBuildCountStable verifies no late debounce batch creates an extra build.
func assertDevWatcherChurnBuildCountStable(t *testing.T, path string, expected int) {
	t.Helper()
	time.Sleep(2 * devWatcherChurnDebounce)
	if starts := countDevWatcherChurnLines(path, "build-start:"); starts != expected {
		t.Fatalf("build starts=%d, want %d\n%s", starts, expected, readDevWatcherChurnFile(path))
	}
}

// assertDevWatcherChurnLifecycleBounds checks that each successful publication causes at most one replacement.
func assertDevWatcherChurnLifecycleBounds(t *testing.T, path string) {
	t.Helper()
	starts := countDevWatcherChurnLines(path, "runtime-start:")
	if starts < 4 || starts > 5 {
		t.Fatalf("runtime starts=%d, want 4..5\n%s", starts, readDevWatcherChurnFile(path))
	}
}

// assertNoDevWatcherChurnExit rejects unexpected runtime failures before coordinated shutdown.
func assertNoDevWatcherChurnExit(t *testing.T, controller *devWatcherController) {
	t.Helper()
	select {
	case exit := <-controller.exitCh:
		t.Fatalf("unexpected watcher exit before shutdown: %+v", exit)
	default:
	}
}

// assertDevWatcherChurnProcessesStopped verifies shutdown reaches every runtime descendant.
func assertDevWatcherChurnProcessesStopped(t *testing.T, path string, latest devWatcherChurnState) {
	t.Helper()
	pids := []int{latest.pid, latest.childPID}
	for _, line := range readDevWatcherChurnLines(path) {
		if !strings.HasPrefix(line, "runtime-start:") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) != 4 {
			continue
		}
		for _, raw := range parts[2:] {
			if pid, err := strconv.Atoi(raw); err == nil {
				pids = append(pids, pid)
			}
		}
	}
	waitForDevWatcherChurnCondition(t, "runtime process trees to stop", func() bool {
		for _, pid := range pids {
			if devWatcherChurnProcessAlive(pid) {
				return false
			}
		}
		return true
	})
}

// devWatcherChurnProcessAlive reports whether a Unix PID still accepts signal zero.
func devWatcherChurnProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// assertDevWatcherChurnTranscript rejects partial or unavailable binary launch failures.
func assertDevWatcherChurnTranscript(t *testing.T, path string) {
	t.Helper()
	transcript := strings.ToLower(readDevWatcherChurnFile(path))
	for _, forbidden := range []string{
		"exec format error", "text file busy", "permission denied", "cannot execute",
		"no such file or directory", "cannot stat", "snapshot is not executable",
		"is not ready; waiting for a successful build",
	} {
		if strings.Contains(transcript, forbidden) {
			t.Fatalf("watcher transcript contains binary launch failure %q:\n%s", forbidden, transcript)
		}
	}
}

// readDevWatcherChurnState parses the runtime's atomic key-value probe.
func readDevWatcherChurnState(path string) (devWatcherChurnState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return devWatcherChurnState{}, err
	}
	values := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[key] = value
		}
	}
	pid, err := strconv.Atoi(values["pid"])
	if err != nil {
		return devWatcherChurnState{}, err
	}
	childPID, err := strconv.Atoi(values["child"])
	if err != nil {
		return devWatcherChurnState{}, err
	}
	heartbeat, err := strconv.Atoi(values["heartbeat"])
	if err != nil {
		return devWatcherChurnState{}, err
	}
	return devWatcherChurnState{version: values["version"], pid: pid, childPID: childPID, heartbeat: heartbeat}, nil
}

// countDevWatcherChurnLines counts durable records with the requested prefix or exact value.
func countDevWatcherChurnLines(path string, expected string) int {
	count := 0
	for _, line := range readDevWatcherChurnLines(path) {
		if line == expected || strings.HasPrefix(line, expected) {
			count++
		}
	}
	return count
}

// readDevWatcherChurnLines returns complete records currently visible in an append-only probe.
func readDevWatcherChurnLines(path string) []string {
	contents := strings.TrimSpace(readDevWatcherChurnFile(path))
	if contents == "" {
		return nil
	}
	return strings.Split(contents, "\n")
}

// readDevWatcherChurnFile reads a diagnostic file while tolerating probes that have not been created yet.
func readDevWatcherChurnFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

const devWatcherChurnProgram = `package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

const version = __WATCHER_CHURN_VERSION__

func main() {
	if len(os.Args) > 1 && os.Args[1] == "child" {
		signal.Ignore(syscall.SIGINT, syscall.SIGTERM)
		for {
			time.Sleep(time.Second)
		}
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	child := exec.Command(os.Args[0], "child")
	child.Env = os.Environ()
	if err := child.Start(); err != nil {
		appendLifecycle(fmt.Sprintf("runtime-child-error:%s:%d:%v", version, os.Getpid(), err))
		os.Exit(3)
	}
	pid := os.Getpid()
	childPID := child.Process.Pid
	appendLifecycle(fmt.Sprintf("runtime-start:%s:%d:%d", version, pid, childPID))
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	heartbeat := 0
	writeState(pid, childPID, heartbeat)
	for {
		select {
		case <-ticker.C:
			heartbeat++
			writeState(pid, childPID, heartbeat)
		case <-signals:
			appendLifecycle(fmt.Sprintf("runtime-stop:%s:%d:%d", version, pid, childPID))
			return
		}
	}
}

func writeState(pid int, childPID int, heartbeat int) {
	path := os.Getenv("GOFORJ_WATCHER_CHURN_STATE")
	temporary := fmt.Sprintf("%s.%d.tmp", path, pid)
	contents := fmt.Sprintf("version=%s\npid=%d\nchild=%d\nheartbeat=%d\n", version, pid, childPID, heartbeat)
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if os.WriteFile(temporary, []byte(contents), 0o600) == nil {
		_ = os.Rename(temporary, path)
	}
}

func appendLifecycle(line string) {
	file, err := os.OpenFile(os.Getenv("GOFORJ_WATCHER_CHURN_LIFECYCLE"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintln(file, line)
	_ = file.Close()
}
`
