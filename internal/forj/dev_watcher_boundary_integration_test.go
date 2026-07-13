//go:build watcherstress && !windows

package forj

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/build"
	"github.com/goforj/goforj/internal/devwatch"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
)

const (
	devWatcherBoundaryTimeout   = 15 * time.Second
	devWatcherBoundaryDebounce  = 75 * time.Millisecond
	devWatcherBoundaryHeartbeat = 5 * time.Millisecond
)

type devWatcherBoundaryState struct {
	version   string
	pid       int
	heartbeat int
}

// TestDevWatcherBoundaryHelper provides observable build and runtime processes for the tagged integration tests.
func TestDevWatcherBoundaryHelper(t *testing.T) {
	if os.Getenv("GOFORJ_WATCHER_BOUNDARY_HELPER") != "1" {
		return
	}
	switch devWatcherChurnHelperAction(os.Args) {
	case "build":
		runDevWatcherBoundaryBuildHelper()
	case "runtime":
		runDevWatcherBoundaryRuntimeHelper()
	default:
		os.Exit(2)
	}
}

// TestDevWatcherBoundaryRejectedReplacementKeepsHealthyRuntime requires validation before a live process is stopped.
func TestDevWatcherBoundaryRejectedReplacementKeepsHealthyRuntime(t *testing.T) {
	testCases := []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{
			name: "missing binary",
			mutate: func(t *testing.T, target string) {
				t.Helper()
				if err := os.Remove(target); err != nil {
					t.Fatalf("remove replacement binary: %v", err)
				}
			},
		},
		{
			name: "invalid executable format",
			mutate: func(t *testing.T, target string) {
				t.Helper()
				if err := os.WriteFile(target, []byte("not-a-valid-executable\n"), 0o755); err != nil {
					t.Fatalf("replace runtime with invalid executable: %v", err)
				}
			},
		},
		{
			name: "truncated executable",
			mutate: func(t *testing.T, target string) {
				t.Helper()
				if err := os.WriteFile(target, devWatcherBoundaryTruncatedExecutable(), 0o755); err != nil {
					t.Fatalf("replace runtime with truncated executable: %v", err)
				}
			},
		},
		{
			name: "missing execute permission",
			mutate: func(t *testing.T, target string) {
				t.Helper()
				if err := os.Chmod(target, 0o600); err != nil {
					t.Fatalf("remove runtime execute permission: %v", err)
				}
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			runDevWatcherBoundaryRejectedReplacement(t, false, testCase.mutate)
		})
	}
	t.Run("legacy framework binary", func(t *testing.T) {
		runDevWatcherBoundaryRejectedReplacement(t, true, func(t *testing.T, target string) {
			t.Helper()
			if err := os.Remove(target); err != nil {
				t.Fatalf("remove legacy replacement binary: %v", err)
			}
		})
	})
}

// runDevWatcherBoundaryRejectedReplacement proves one rejected artifact cannot interrupt an established runtime.
func runDevWatcherBoundaryRejectedReplacement(t *testing.T, legacy bool, mutate func(*testing.T, string)) {
	t.Helper()
	root := t.TempDir()
	logPath := filepath.Join(root, "runtime.log")
	statePath := filepath.Join(root, "runtime.state")
	publishedPath := filepath.Join(root, "published-version")
	target := filepath.Join(root, "bin", "app")
	publishDevWatcherBoundaryFixture(t, target, publishedPath, "healthy")

	runtimeID := "watcherstress/boundary/rejected/runtime"
	environment := devWatcherBoundaryEnvironment(logPath, statePath, publishedPath)
	var watcher devCompiledWatcher
	var err error
	if legacy {
		environment["FORJ_APP"] = project.DefaultAppName
		watcher, err = compileLegacyDevWatcher(project.DevWatch{
			Name: "Run App", Exec: devWatcherBoundaryBinaryHelperCommand("runtime"), Env: environment,
		})
		if err != nil {
			t.Fatalf("compile legacy boundary runtime: %v", err)
		}
		if watcher.NativeRuntimeCommand == "" {
			t.Fatal("legacy framework binary was not selected for prepared runtime publication")
		}
		watcher.WatchChanges = false
		watcher.Command.Dir = root
	} else {
		watcher = devCompiledWatcher{
			ID: runtimeID, Name: "Run App", Kind: devWatcherAppRun, App: "app", Restart: true,
			Command: devwatch.Command{
				Shell: devWatcherBoundaryBinaryHelperCommand("runtime"), Dir: root, Env: environment,
			},
		}
	}
	watcher.ID = runtimeID
	watcher.Command.GracefulStopTimeout = 250 * time.Millisecond
	controller, err := newDevWatcherController([]devCompiledWatcher{watcher}, nil, io.Discard, io.Discard, false)
	if err != nil {
		t.Fatalf("start boundary controller: %v", err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			controller.stop(time.Second)
		}
	})

	initial := waitForDevWatcherBoundaryState(t, statePath, func(state devWatcherBoundaryState) bool {
		return state.version == "healthy" && state.heartbeat > 0
	})
	mutate(t, target)
	controller.tasks[runtimeID].request()

	preserved := waitForDevWatcherBoundaryState(t, statePath, func(state devWatcherBoundaryState) bool {
		return state.pid == initial.pid && state.heartbeat >= initial.heartbeat+50
	})
	if preserved.version != "healthy" {
		t.Fatalf("preserved runtime version = %q, want healthy", preserved.version)
	}
	lines := readDevWatcherChurnLines(logPath)
	if starts := countDevWatcherChurnLines(logPath, "runtime-start:"); starts != 1 {
		t.Fatalf("rejected replacement started another runtime: starts=%d\n%s", starts, strings.Join(lines, "\n"))
	}
	if stops := countDevWatcherChurnLines(logPath, "runtime-stop-begin:"); stops != 0 {
		t.Fatalf("rejected replacement sacrificed the healthy runtime: stops=%d\n%s", stops, strings.Join(lines, "\n"))
	}
	select {
	case exit := <-controller.exitCh:
		t.Fatalf("rejected replacement terminated the watcher: %+v", exit)
	default:
	}
	publishDevWatcherBoundaryFixture(t, target, publishedPath, "recovered")
	controller.tasks[runtimeID].request()
	recovered := waitForDevWatcherBoundaryState(t, statePath, func(state devWatcherBoundaryState) bool {
		return state.version == "recovered" && state.pid != initial.pid && state.heartbeat > 0
	})
	if starts := countDevWatcherChurnLines(logPath, "runtime-start:"); starts != 2 {
		t.Fatalf("valid retry runtime starts=%d, want 2\n%s", starts, readDevWatcherChurnFile(logPath))
	}

	controller.stop(time.Second)
	stopped = true
	waitForDevWatcherBoundaryProcessExit(t, initial.pid)
	waitForDevWatcherBoundaryProcessExit(t, recovered.pid)
	snapshots, err := filepath.Glob(filepath.Join(root, "bin", ".app.run-*"))
	if err != nil {
		t.Fatalf("inspect prepared executable cleanup: %v", err)
	}
	if len(snapshots) != 0 {
		t.Fatalf("prepared executable snapshots leaked after shutdown: %v", snapshots)
	}
}

// TestDevWatcherBoundaryPostPreparationStartFailureRetries keeps controller state aligned after the old runtime is gone.
func TestDevWatcherBoundaryPostPreparationStartFailureRetries(t *testing.T) {
	root := t.TempDir()
	logPath := filepath.Join(root, "runtime.log")
	statePath := filepath.Join(root, "runtime.state")
	publishedPath := filepath.Join(root, "published-version")
	target := filepath.Join(root, "bin", "app")
	publishDevWatcherBoundaryFixture(t, target, publishedPath, "initial")
	runtimeID := "watcherstress/boundary/start-failure/runtime"
	controller, err := newDevWatcherController([]devCompiledWatcher{{
		ID: runtimeID, Name: "Run App", Kind: devWatcherAppRun, App: "app", Restart: true,
		Command: devwatch.Command{
			Shell: devWatcherBoundaryBinaryHelperCommand("runtime"), Dir: root,
			Env:                 devWatcherBoundaryEnvironment(logPath, statePath, publishedPath),
			GracefulStopTimeout: 250 * time.Millisecond,
		},
	}}, nil, io.Discard, io.Discard, false)
	if err != nil {
		t.Fatalf("start post-preparation failure controller: %v", err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			controller.stop(time.Second)
		}
	})
	initial := waitForDevWatcherBoundaryState(t, statePath, func(state devWatcherBoundaryState) bool {
		return state.version == "initial" && state.heartbeat > 0
	})

	originalPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", originalPath) })
	if err := os.Setenv("PATH", t.TempDir()); err != nil {
		t.Fatalf("hide Bash for post-preparation start failure: %v", err)
	}
	controller.tasks[runtimeID].request()
	waitForDevWatcherChurnBuildLine(t, logPath, "runtime-stop:initial")
	waitForDevWatcherBoundaryProcessExit(t, initial.pid)
	waitForDevWatcherBoundaryCondition(t, "controller to record failed replacement as stopped", func() bool {
		task := controller.tasks[runtimeID]
		task.mu.Lock()
		defer task.mu.Unlock()
		return !task.runtimeLive && len(task.triggerCh) == 0 && !controller.supervisor.RuntimeRunning(runtimeID)
	})
	if err := os.Setenv("PATH", originalPath); err != nil {
		t.Fatalf("restore PATH for runtime retry: %v", err)
	}
	publishDevWatcherBoundaryFixture(t, target, publishedPath, "retried")
	controller.tasks[runtimeID].request()
	retried := waitForDevWatcherBoundaryState(t, statePath, func(state devWatcherBoundaryState) bool {
		return state.version == "retried" && state.pid != initial.pid && state.heartbeat > 0
	})
	select {
	case exit := <-controller.exitCh:
		t.Fatalf("recoverable start failure terminated the watcher: %+v", exit)
	default:
	}
	controller.stop(time.Second)
	stopped = true
	waitForDevWatcherBoundaryProcessExit(t, retried.pid)
}

// TestDevWatcherBoundaryRelativeCommandDirectoryLaunchesPreparedSnapshot prevents command directories from prefixing twice.
func TestDevWatcherBoundaryRelativeCommandDirectoryLaunchesPreparedSnapshot(t *testing.T) {
	root := t.TempDir()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve boundary working directory: %v", err)
	}
	relativeRoot, err := filepath.Rel(workingDirectory, root)
	if err != nil {
		t.Fatalf("resolve relative boundary project: %v", err)
	}
	logPath := filepath.Join(root, "runtime.log")
	statePath := filepath.Join(root, "runtime.state")
	publishedPath := filepath.Join(root, "published-version")
	publishDevWatcherBoundaryFixture(t, filepath.Join(root, "bin", "app"), publishedPath, "relative")
	runtimeID := "watcherstress/boundary/relative/runtime"
	controller, err := newDevWatcherController([]devCompiledWatcher{{
		ID: runtimeID, Name: "Run App", Kind: devWatcherAppRun, App: "app", Restart: true,
		Command: devwatch.Command{
			Shell: devWatcherBoundaryBinaryHelperCommand("runtime"), Dir: relativeRoot,
			Env:                 devWatcherBoundaryEnvironment(logPath, statePath, publishedPath),
			GracefulStopTimeout: 250 * time.Millisecond,
		},
	}}, nil, io.Discard, io.Discard, false)
	if err != nil {
		t.Fatalf("start relative-directory controller: %v", err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			controller.stop(time.Second)
		}
	})
	runtimeState := waitForDevWatcherBoundaryState(t, statePath, func(state devWatcherBoundaryState) bool {
		return state.version == "relative" && state.heartbeat > 0
	})
	controller.stop(time.Second)
	stopped = true
	waitForDevWatcherBoundaryProcessExit(t, runtimeState.pid)
}

// TestDevWatcherBoundaryShutdownCancelsBlockedBuild proves shutdown closes both active and queued work without publication.
func TestDevWatcherBoundaryShutdownCancelsBlockedBuild(t *testing.T) {
	root := t.TempDir()
	originalDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("enter boundary project: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDirectory) })

	logPath := filepath.Join(root, "lifecycle.log")
	statePath := filepath.Join(root, "runtime.state")
	publishedPath := filepath.Join(root, "published-version")
	buildCountPath := filepath.Join(root, "build.count")
	releasePath := filepath.Join(root, "never-release")
	binaryPath := filepath.Join(root, "bin", "app")
	readyPath := filepath.Join(root, "bin", ".app.ready")
	writeDevWatcherChurnFile(t, publishedPath, "initial\n", 0o600)

	buildID := "watcherstress/boundary/shutdown/build"
	runtimeID := "watcherstress/boundary/shutdown/runtime"
	buildCommand := devWatcherRunnerHelperCommand("build", map[string]string{
		"GOFORJ_DEV_WATCHER_LOG":     logPath,
		"GOFORJ_DEV_WATCHER_COUNTER": buildCountPath,
		"GOFORJ_DEV_WATCHER_RELEASE": releasePath,
		"GOFORJ_DEV_WATCHER_BINARY":  binaryPath,
	})
	runtimeCommand := devwatch.Command{
		Shell: devWatcherBoundaryHelperCommand("runtime"), Dir: root,
		Env:                 devWatcherBoundaryEnvironment(logPath, statePath, publishedPath),
		GracefulStopTimeout: 250 * time.Millisecond,
	}
	controller, err := newDevWatcherController([]devCompiledWatcher{
		{
			ID: buildID, Name: "Build App", Kind: devWatcherAppBuild, App: "app",
			Command: buildCommand, Postpone: true, OnSuccess: []string{runtimeID},
		},
		{
			ID: runtimeID, Name: "Run App", Kind: devWatcherAppRun, App: "app",
			Command: runtimeCommand, Restart: true,
		},
	}, nil, io.Discard, io.Discard, false)
	if err != nil {
		t.Fatalf("start shutdown boundary controller: %v", err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			controller.stop(2 * time.Second)
		}
	})

	runtime := waitForDevWatcherBoundaryState(t, statePath, func(state devWatcherBoundaryState) bool {
		return state.version == "initial" && state.heartbeat > 0
	})
	controller.tasks[buildID].request()
	buildPID := waitForDevWatcherBoundaryLogPID(t, logPath, "build-start:1:")
	controller.tasks[buildID].request()
	waitForDevWatcherBoundaryCondition(t, "queued build while active build is blocked", func() bool {
		task := controller.tasks[buildID]
		task.mu.Lock()
		defer task.mu.Unlock()
		return task.busy && task.activeCancel != nil && len(task.triggerCh) == 1
	})

	controller.stop(2 * time.Second)
	stopped = true
	waitForDevWatcherBoundaryProcessExit(t, buildPID)
	waitForDevWatcherBoundaryProcessExit(t, runtime.pid)
	lines := readDevWatcherChurnLines(logPath)
	if builds := countDevWatcherChurnLines(logPath, "build-start:"); builds != 1 {
		t.Fatalf("shutdown started queued build: starts=%d\n%s", builds, strings.Join(lines, "\n"))
	}
	if successes := countDevWatcherChurnLines(logPath, "build-success:"); successes != 0 {
		t.Fatalf("canceled build reported late success: successes=%d\n%s", successes, strings.Join(lines, "\n"))
	}
	if starts := countDevWatcherChurnLines(logPath, "runtime-start:"); starts != 1 {
		t.Fatalf("shutdown published a replacement runtime: starts=%d\n%s", starts, strings.Join(lines, "\n"))
	}
	for _, path := range []string{binaryPath, readyPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("shutdown left late publication %s: %v", path, err)
		}
	}
}

// TestDevWatcherBoundaryChangeDuringRuntimeReplacementConverges proves an event crossing restart queues one latest follow-up.
func TestDevWatcherBoundaryChangeDuringRuntimeReplacementConverges(t *testing.T) {
	root := t.TempDir()
	originalDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("enter boundary project: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDirectory) })

	logPath := filepath.Join(root, "lifecycle.log")
	statePath := filepath.Join(root, "runtime.state")
	publishedPath := filepath.Join(root, "published-version")
	sourcePath := filepath.Join(root, "cmd", "app", "main.go")
	buildCountPath := filepath.Join(root, "build.count")
	holdPath := filepath.Join(root, "hold-runtime-stop")
	releasePath := filepath.Join(root, "release-runtime-stop")
	writeDevWatcherBoundaryArtifactProject(t, root, sourcePath, "initial")
	runDevWatcherBoundaryInitialArtifactBuild(t, root)
	writeDevWatcherChurnFile(t, holdPath, "hold\n", 0o600)

	goMatcher := mustDevWatcherChurnMatcher(t, ".go")
	binMatcher := mustDevWatcherChurnMatcher(t, "bin")
	buildID := "watcherstress/boundary/restart/build"
	runtimeID := "watcherstress/boundary/restart/runtime"
	environment := devWatcherBoundaryEnvironment(logPath, statePath, publishedPath)
	environment["GOFORJ_WATCHER_BOUNDARY_SOURCE"] = sourcePath
	environment["GOFORJ_WATCHER_BOUNDARY_BUILD_COUNT"] = buildCountPath
	environment["GOFORJ_WATCHER_BOUNDARY_STOP_HOLD"] = holdPath
	environment["GOFORJ_WATCHER_BOUNDARY_STOP_RELEASE"] = releasePath
	compiled := []devCompiledWatcher{
		{
			ID: buildID, Name: "Build App", Kind: devWatcherAppBuild, App: "app",
			Watch: devwatch.Spec{
				Name: buildID, Roots: []string{root}, Includes: []devwatch.Matcher{goMatcher},
				DirectoryExcludes: []devwatch.Matcher{binMatcher}, Debounce: devWatcherBoundaryDebounce,
				DebounceSet: true,
			},
			Command: devwatch.Command{
				Shell: devWatcherBoundaryHelperCommand("build"), Dir: root, Env: environment,
			},
			Postpone: true, WatchChanges: true, OnSuccess: []string{runtimeID},
		},
		{
			ID: runtimeID, Name: "Run App", Kind: devWatcherAppRun, App: "app",
			Command: devwatch.Command{
				Shell: "./bin/app", Dir: root, Env: environment,
				GracefulStopTimeout: 2 * time.Second,
			},
			Restart: true,
		},
	}
	transcript, err := os.OpenFile(filepath.Join(root, "watcher.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open boundary transcript: %v", err)
	}
	controller, err := newDevWatcherController(compiled, nil, transcript, transcript, false)
	if err != nil {
		_ = transcript.Close()
		t.Fatalf("start replacement boundary controller: %v", err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			controller.stop(3 * time.Second)
		}
		_ = transcript.Close()
		if t.Failed() {
			t.Logf("boundary lifecycle:\n%s", readDevWatcherChurnFile(logPath))
			t.Logf("boundary transcript:\n%s", readDevWatcherChurnFile(filepath.Join(root, "watcher.log")))
		}
	})

	initial := waitForDevWatcherBoundaryState(t, statePath, func(state devWatcherBoundaryState) bool {
		return state.version == "initial" && state.heartbeat > 0
	})
	writeDevWatcherBoundarySource(t, sourcePath, "first", false)
	waitForDevWatcherChurnBuildLine(t, logPath, "build-success:1:first")
	waitForDevWatcherChurnBuildLine(t, logPath, "runtime-stop-blocked:initial")

	for index := range 16 {
		writeDevWatcherBoundarySource(t, sourcePath, fmt.Sprintf("pending-%02d", index), index%2 == 0)
	}
	writeDevWatcherBoundarySource(t, sourcePath, "latest", true)
	waitForDevWatcherChurnBuildLine(t, logPath, "build-success:2:latest")
	waitForDevWatcherChurnTaskIdle(t, controller.tasks[buildID])
	waitForDevWatcherBoundaryCondition(t, "one runtime follow-up queued during replacement", func() bool {
		return len(controller.tasks[runtimeID].triggerCh) == 1
	})
	if err := os.WriteFile(releasePath, []byte("release\n"), 0o600); err != nil {
		t.Fatalf("release runtime replacement: %v", err)
	}

	stablePID := 0
	stableHeartbeat := 0
	latest := waitForDevWatcherBoundaryState(t, statePath, func(state devWatcherBoundaryState) bool {
		select {
		case exit := <-controller.exitCh:
			t.Fatalf("runtime follow-up exited at the replacement boundary: %+v", exit)
		default:
		}
		if state.version != "latest" || state.pid == initial.pid ||
			strings.Count(readDevWatcherChurnFile(filepath.Join(root, "watcher.log")), "Starting Run App") != 3 ||
			len(controller.tasks[runtimeID].triggerCh) != 0 || !controller.supervisor.RuntimeRunning(runtimeID) {
			return false
		}
		if state.pid != stablePID {
			stablePID = state.pid
			stableHeartbeat = state.heartbeat
			return false
		}
		return state.heartbeat >= stableHeartbeat+20
	})
	assertDevWatcherChurnBuildCountStable(t, logPath, 2)
	lines := readDevWatcherChurnLines(logPath)
	starts := countDevWatcherChurnLines(logPath, "runtime-start:")
	if starts < 2 || starts > 3 {
		t.Fatalf("restart boundary runtime starts=%d, want final convergence within two replacement attempts\n%s", starts, strings.Join(lines, "\n"))
	}
	if stops := countDevWatcherChurnLines(logPath, "runtime-stop-begin:"); stops != starts-1 {
		t.Fatalf("restart boundary runtime stops=%d, want %d completed predecessors\n%s", stops, starts-1, strings.Join(lines, "\n"))
	}
	lastRuntimeStart := ""
	for _, line := range lines {
		if strings.HasPrefix(line, "runtime-start:") {
			lastRuntimeStart = line
		}
	}
	if !strings.HasPrefix(lastRuntimeStart, "runtime-start:latest:") {
		t.Fatalf("final runtime artifact is not latest: %q\n%s", lastRuntimeStart, strings.Join(lines, "\n"))
	}
	select {
	case exit := <-controller.exitCh:
		t.Fatalf("final latest runtime failed after convergence: %+v", exit)
	default:
	}
	controller.stop(3 * time.Second)
	stopped = true
	if err := transcript.Close(); err != nil {
		t.Fatalf("close boundary transcript: %v", err)
	}
	waitForDevWatcherBoundaryProcessExit(t, latest.pid)
}

// devWatcherBoundaryHelperCommand returns the tagged helper command accepted by the production shell wrapper.
func devWatcherBoundaryHelperCommand(action string) string {
	return devWatcherRunnerShellQuote(os.Args[0]) + " -test.run='^TestDevWatcherBoundaryHelper$' -- " + action
}

// devWatcherBoundaryBinaryHelperCommand executes the atomically published fixture through native snapshot preparation.
func devWatcherBoundaryBinaryHelperCommand(action string) string {
	return "./bin/app -test.run='^TestDevWatcherBoundaryHelper$' -- " + action
}

// devWatcherBoundaryTruncatedExecutable preserves host magic while omitting the structures required to launch it.
func devWatcherBoundaryTruncatedExecutable() []byte {
	if runtime.GOOS == "darwin" {
		return []byte{0xcf, 0xfa, 0xed, 0xfe}
	}
	return []byte{0x7f, 'E', 'L', 'F'}
}

// devWatcherBoundaryEnvironment supplies the probes shared by boundary build and runtime helpers.
func devWatcherBoundaryEnvironment(logPath string, statePath string, publishedPath string) map[string]string {
	return map[string]string{
		"GOFORJ_WATCHER_BOUNDARY_HELPER":    "1",
		"GOFORJ_WATCHER_BOUNDARY_LOG":       logPath,
		"GOFORJ_WATCHER_BOUNDARY_STATE":     statePath,
		"GOFORJ_WATCHER_BOUNDARY_PUBLISHED": publishedPath,
		"GOCACHE":                           devWatcherChurnEnvDefault("GOCACHE", "/tmp/gocache"),
		"GOMODCACHE":                        devWatcherChurnEnvDefault("GOMODCACHE", "/tmp/gomodcache"),
	}
}

// runDevWatcherBoundaryBuildHelper publishes a version-embedded executable through the production build pipeline.
func runDevWatcherBoundaryBuildHelper() {
	count, err := incrementDevWatcherRunnerCounter(os.Getenv("GOFORJ_WATCHER_BOUNDARY_BUILD_COUNT"))
	if err != nil {
		os.Exit(3)
	}
	source, err := os.ReadFile(os.Getenv("GOFORJ_WATCHER_BOUNDARY_SOURCE"))
	if err != nil {
		os.Exit(4)
	}
	version, err := devWatcherBoundaryArtifactVersion(string(source))
	if err != nil {
		os.Exit(5)
	}
	appendDevWatcherChurnLine(os.Getenv("GOFORJ_WATCHER_BOUNDARY_LOG"), fmt.Sprintf("build-start:%d:%s", count, version))
	root := filepath.Dir(filepath.Dir(filepath.Dir(os.Getenv("GOFORJ_WATCHER_BOUNDARY_SOURCE"))))
	appLogger := logger.NewSilentLogger()
	command := build.NewCmd(appLogger, build.NewAPIIndexRunner(appLogger))
	command.Root = root
	command.SkipWire = true
	command.Args = []string{"-o", "./bin/app", "./cmd/app"}
	if err := command.Run(); err != nil {
		os.Exit(6)
	}
	appendDevWatcherChurnLine(os.Getenv("GOFORJ_WATCHER_BOUNDARY_LOG"), fmt.Sprintf("build-success:%d:%s", count, version))
}

// runDevWatcherBoundaryInitialArtifactBuild creates the baseline executable before physical subscriptions begin.
func runDevWatcherBoundaryInitialArtifactBuild(t *testing.T, root string) {
	t.Helper()
	appLogger := logger.NewSilentLogger()
	command := build.NewCmd(appLogger, build.NewAPIIndexRunner(appLogger))
	command.Root = root
	command.SkipWire = true
	command.Args = []string{"-o", "./bin/app", "./cmd/app"}
	if err := command.Run(); err != nil {
		t.Fatalf("build initial boundary artifact: %v", err)
	}
}

// devWatcherBoundaryArtifactVersion extracts the immutable version constant recorded in one build log.
func devWatcherBoundaryArtifactVersion(source string) (string, error) {
	const prefix = "const version = "
	for _, line := range strings.Split(source, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		version, err := strconv.Unquote(strings.TrimSpace(strings.TrimPrefix(line, prefix)))
		if err != nil {
			return "", err
		}
		return version, nil
	}
	return "", fmt.Errorf("boundary artifact source has no version constant")
}

// runDevWatcherBoundaryRuntimeHelper exposes replacement ordering and liveness through durable probes.
func runDevWatcherBoundaryRuntimeHelper() {
	published, err := os.ReadFile(os.Getenv("GOFORJ_WATCHER_BOUNDARY_PUBLISHED"))
	if err != nil {
		os.Exit(8)
	}
	version := strings.TrimSpace(string(published))
	pid := os.Getpid()
	logPath := os.Getenv("GOFORJ_WATCHER_BOUNDARY_LOG")
	statePath := os.Getenv("GOFORJ_WATCHER_BOUNDARY_STATE")
	appendDevWatcherChurnLine(logPath, fmt.Sprintf("runtime-start:%s:%d", version, pid))
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	ticker := time.NewTicker(devWatcherBoundaryHeartbeat)
	defer ticker.Stop()
	heartbeat := 0
	writeDevWatcherBoundaryRuntimeState(statePath, version, pid, heartbeat)
	for {
		select {
		case <-ticker.C:
			heartbeat++
			writeDevWatcherBoundaryRuntimeState(statePath, version, pid, heartbeat)
		case <-signals:
			appendDevWatcherChurnLine(logPath, fmt.Sprintf("runtime-stop-begin:%s:%d", version, pid))
			holdPath := os.Getenv("GOFORJ_WATCHER_BOUNDARY_STOP_HOLD")
			releasePath := os.Getenv("GOFORJ_WATCHER_BOUNDARY_STOP_RELEASE")
			if _, err := os.Stat(holdPath); holdPath != "" && err == nil {
				appendDevWatcherChurnLine(logPath, fmt.Sprintf("runtime-stop-blocked:%s:%d", version, pid))
				for {
					if _, err := os.Stat(releasePath); err == nil {
						break
					}
					time.Sleep(5 * time.Millisecond)
				}
			}
			appendDevWatcherChurnLine(logPath, fmt.Sprintf("runtime-stop:%s:%d", version, pid))
			return
		}
	}
}

// publishDevWatcherBoundaryFixture installs one known-good test binary and its matching version probe.
func publishDevWatcherBoundaryFixture(t *testing.T, target string, publishedPath string, version string) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve boundary test executable: %v", err)
	}
	if err := copyDevWatcherBoundaryExecutable(executable, target); err != nil {
		t.Fatalf("publish boundary test executable: %v", err)
	}
	if err := writeDevWatcherBoundaryAtomicFile(publishedPath, version+"\n", 0o600); err != nil {
		t.Fatalf("publish boundary version: %v", err)
	}
}

// copyDevWatcherBoundaryExecutable uses rename publication so the watcher never observes a partial test binary.
func copyDevWatcherBoundaryExecutable(source string, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	output, err := os.CreateTemp(filepath.Dir(target), ".boundary-binary-*")
	if err != nil {
		return err
	}
	temporary := output.Name()
	removeTemporary := true
	defer func() {
		_ = output.Close()
		if removeTemporary {
			_ = os.Remove(temporary)
		}
	}()
	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	if err := output.Chmod(0o755); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary, target); err != nil {
		return err
	}
	removeTemporary = false
	return nil
}

// writeDevWatcherBoundaryAtomicFile prevents probes from exposing mixed versions to assertions or runtime helpers.
func writeDevWatcherBoundaryAtomicFile(path string, contents string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".boundary-state-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if _, err := io.WriteString(temporary, contents); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

// writeDevWatcherBoundaryRuntimeState publishes one complete heartbeat record.
func writeDevWatcherBoundaryRuntimeState(path string, version string, pid int, heartbeat int) {
	contents := fmt.Sprintf("version=%s\npid=%d\nheartbeat=%d\n", version, pid, heartbeat)
	if err := writeDevWatcherBoundaryAtomicFile(path, contents, 0o600); err != nil {
		os.Exit(9)
	}
}

// writeDevWatcherBoundaryArtifactProject creates the isolated module consumed by production build publication.
func writeDevWatcherBoundaryArtifactProject(t *testing.T, root string, sourcePath string, version string) {
	t.Helper()
	writeDevWatcherChurnFile(t, filepath.Join(root, "go.mod"), "module example.com/watcherboundary\n\ngo 1.24\n", 0o644)
	writeDevWatcherChurnFile(t, filepath.Join(root, ".goforj.yml"), "project_name: WatcherBoundary\ngo_module_name: example.com/watcherboundary\nupdated_at: \"2026-07-13 00:00:00 UTC\"\n", 0o644)
	writeDevWatcherBoundarySource(t, sourcePath, version, false)
}

// writeDevWatcherBoundarySource applies editor updates to a source whose version is compiled into the artifact.
func writeDevWatcherBoundarySource(t *testing.T, path string, version string, atomicReplace bool) {
	t.Helper()
	source := strings.ReplaceAll(devWatcherBoundaryArtifactProgram, "__BOUNDARY_ARTIFACT_VERSION__", strconv.Quote(version))
	if !atomicReplace {
		writeDevWatcherChurnFile(t, path, source, 0o644)
		return
	}
	temporary := path + ".editor-swap"
	writeDevWatcherChurnFile(t, temporary, source, 0o644)
	if err := os.Rename(temporary, path); err != nil {
		t.Fatalf("replace boundary source: %v", err)
	}
}

const devWatcherBoundaryArtifactProgram = `package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

const version = __BOUNDARY_ARTIFACT_VERSION__

// main exposes the immutable artifact version and a graceful-stop synchronization barrier.
func main() {
	pid := os.Getpid()
	appendLifecycle(fmt.Sprintf("runtime-start:%s:%d", version, pid))
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	heartbeat := 0
	writeState(pid, heartbeat)
	for {
		select {
		case <-ticker.C:
			heartbeat++
			writeState(pid, heartbeat)
		case <-signals:
			appendLifecycle(fmt.Sprintf("runtime-stop-begin:%s:%d", version, pid))
			waitForRelease(pid)
			appendLifecycle(fmt.Sprintf("runtime-stop:%s:%d", version, pid))
			return
		}
	}
}

// waitForRelease holds a replacement at the exact graceful-stop boundary selected by the test.
func waitForRelease(pid int) {
	holdPath := os.Getenv("GOFORJ_WATCHER_BOUNDARY_STOP_HOLD")
	if holdPath == "" {
		return
	}
	if _, err := os.Stat(holdPath); err != nil {
		return
	}
	appendLifecycle(fmt.Sprintf("runtime-stop-blocked:%s:%d", version, pid))
	releasePath := os.Getenv("GOFORJ_WATCHER_BOUNDARY_STOP_RELEASE")
	for {
		if _, err := os.Stat(releasePath); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// writeState atomically publishes a complete artifact identity and heartbeat.
func writeState(pid int, heartbeat int) {
	path := os.Getenv("GOFORJ_WATCHER_BOUNDARY_STATE")
	temporary := fmt.Sprintf("%s.%d.tmp", path, pid)
	contents := fmt.Sprintf("version=%s\npid=%d\nheartbeat=%d\n", version, pid, heartbeat)
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if os.WriteFile(temporary, []byte(contents), 0o600) == nil {
		_ = os.Rename(temporary, path)
	}
}

// appendLifecycle records artifact identity at every process boundary.
func appendLifecycle(line string) {
	file, err := os.OpenFile(os.Getenv("GOFORJ_WATCHER_BOUNDARY_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintln(file, line)
	_ = file.Close()
}
`

// waitForDevWatcherBoundaryState waits for a complete runtime probe satisfying the requested barrier.
func waitForDevWatcherBoundaryState(t *testing.T, path string, condition func(devWatcherBoundaryState) bool) devWatcherBoundaryState {
	t.Helper()
	var latest devWatcherBoundaryState
	waitForDevWatcherBoundaryCondition(t, "runtime state at "+path, func() bool {
		state, err := readDevWatcherBoundaryState(path)
		if err != nil {
			return false
		}
		latest = state
		return condition(state)
	})
	return latest
}

// readDevWatcherBoundaryState parses an atomically published runtime heartbeat.
func readDevWatcherBoundaryState(path string) (devWatcherBoundaryState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return devWatcherBoundaryState{}, err
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
		return devWatcherBoundaryState{}, err
	}
	heartbeat, err := strconv.Atoi(values["heartbeat"])
	if err != nil {
		return devWatcherBoundaryState{}, err
	}
	return devWatcherBoundaryState{version: values["version"], pid: pid, heartbeat: heartbeat}, nil
}

// waitForDevWatcherBoundaryCondition polls a durable condition with a short diagnostic deadline.
func waitForDevWatcherBoundaryCondition(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(devWatcherBoundaryTimeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

// waitForDevWatcherBoundaryLogPID returns the process recorded by one durable lifecycle barrier.
func waitForDevWatcherBoundaryLogPID(t *testing.T, path string, prefix string) int {
	t.Helper()
	pid := 0
	waitForDevWatcherBoundaryCondition(t, "log line "+prefix, func() bool {
		for _, line := range readDevWatcherChurnLines(path) {
			if !strings.HasPrefix(line, prefix) {
				continue
			}
			raw := strings.TrimPrefix(line, prefix)
			parsed, err := strconv.Atoi(raw)
			if err == nil {
				pid = parsed
				return true
			}
		}
		return false
	})
	return pid
}

// waitForDevWatcherBoundaryProcessExit proves an observed child no longer exists after shutdown.
func waitForDevWatcherBoundaryProcessExit(t *testing.T, pid int) {
	t.Helper()
	waitForDevWatcherBoundaryCondition(t, fmt.Sprintf("process %d to exit", pid), func() bool {
		return !devWatcherChurnProcessAlive(pid)
	})
}
