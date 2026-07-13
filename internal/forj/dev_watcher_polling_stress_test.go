//go:build watcherstress && !windows

package forj

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/build"
	"github.com/goforj/goforj/internal/devwatch"
	"github.com/goforj/goforj/internal/logger"
)

const (
	devWatcherPollingInterval = 20 * time.Millisecond
	devWatcherPollingDebounce = 250 * time.Millisecond
)

type devWatcherPollingPaths struct {
	devWatcherChurnPaths
	preBuildHold    string
	preBuildRelease string
}

type devWatcherPollingWriteResult struct {
	version string
	err     error
}

// TestDevWatcherPollingBuildHelper exposes a pre-compilation barrier around the production build pipeline.
func TestDevWatcherPollingBuildHelper(t *testing.T) {
	if os.Getenv("GOFORJ_WATCHER_POLLING_HELPER") != "1" {
		return
	}
	if devWatcherChurnHelperAction(os.Args) != "build" {
		os.Exit(2)
	}
	runDevWatcherPollingBuildHelper()
}

// TestDevWatcherPollingStress verifies continuous debounce, build overlap, and watched-root recovery.
func TestDevWatcherPollingStress(t *testing.T) {
	paths := newDevWatcherPollingPaths(t)
	writeDevWatcherChurnProject(t, paths.root, "poll-initial")
	environment := devWatcherPollingEnvironment(paths)
	runDevWatcherPollingInitialBuild(t, paths, environment)

	watchRoot := filepath.Join(paths.root, "cmd", "app")
	goMatcher := mustDevWatcherChurnMatcher(t, ".go")
	buildID := "watcherstress/polling/build"
	runtimeID := "watcherstress/polling/runtime"
	compiled := []devCompiledWatcher{
		{
			ID: buildID, Name: "Build App", Kind: devWatcherAppBuild, App: "app",
			Watch: devwatch.Spec{
				Name: buildID, Roots: []string{watchRoot}, Includes: []devwatch.Matcher{goMatcher},
				Debounce: devWatcherPollingDebounce, DebounceSet: true,
			},
			Command: devwatch.Command{
				Shell: devWatcherPollingHelperCommand(), Dir: paths.root, Env: environment,
			},
			Postpone: true, WatchChanges: true, PollInterval: devWatcherPollingInterval,
			OnSuccess: []string{runtimeID},
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
		t.Fatalf("open polling watcher transcript: %v", err)
	}
	controller, err := newDevWatcherController(compiled, nil, transcript, transcript, false)
	if err != nil {
		_ = transcript.Close()
		t.Fatalf("start polling watcher controller: %v", err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			controller.stop(5 * time.Second)
		}
		_ = transcript.Close()
		if t.Failed() {
			t.Logf("polling build log:\n%s", readDevWatcherChurnFile(paths.buildLog))
			t.Logf("polling lifecycle log:\n%s", readDevWatcherChurnFile(paths.lifecycle))
			t.Logf("polling watcher transcript:\n%s", readDevWatcherChurnFile(paths.transcript))
		}
	})
	if len(controller.engines) != 1 {
		t.Fatalf("polling controller engines=%d, want 1", len(controller.engines))
	}
	engine := controller.engines[0]
	if health := engine.Health(); health.State != devwatch.HealthHealthy || health.Backend != devwatch.BackendPoll {
		t.Fatalf("polling engine health=%+v, want healthy polling coverage", health)
	}
	buildTask := controller.tasks[buildID]
	waitForDevWatcherChurnState(t, paths.state, func(state devWatcherChurnState) bool {
		return state.version == "poll-initial"
	})

	continuousVersion := ""
	for index := range 48 {
		continuousVersion = fmt.Sprintf("continuous-%02d", index)
		mustWriteDevWatcherPollingSource(t, paths.root, continuousVersion, index%2 == 0)
		time.Sleep(15 * time.Millisecond)
	}
	if starts := countDevWatcherChurnLines(paths.buildLog, "build-start:"); starts != 1 {
		t.Fatalf("continuous sub-debounce writes started %d builds before quiescence, want baseline only", starts)
	}
	waitForDevWatcherChurnBuildLine(t, paths.buildLog, "build-success:2")
	continuous := waitForDevWatcherChurnState(t, paths.state, func(state devWatcherChurnState) bool {
		return state.version == continuousVersion
	})
	waitForDevWatcherChurnTaskIdle(t, buildTask)
	assertDevWatcherPollingBuildCountStable(t, paths.buildLog, 2)

	writeDevWatcherChurnFile(t, paths.preBuildHold, "hold\n", 0o600)
	mustWriteDevWatcherPollingSource(t, paths.root, "compile-anchor", true)
	waitForDevWatcherChurnBuildLine(t, paths.buildLog, "build-preblocked:3")
	writes := make(chan devWatcherPollingWriteResult, 1)
	releasePoint := make(chan struct{}, 1)
	go runDevWatcherPollingWriteStream(paths.root, "compile-overlap", 60, releasePoint, writes)
	select {
	case <-releasePoint:
	case result := <-writes:
		t.Fatalf("overlap writer stopped before release point: %v", result.err)
	case <-time.After(devWatcherChurnTimeout):
		t.Fatal("timed out waiting for overlap writer release point")
	}
	if err := os.Remove(paths.preBuildHold); err != nil {
		t.Fatalf("remove polling pre-build hold: %v", err)
	}
	writeDevWatcherChurnFile(t, paths.preBuildRelease, "release\n", 0o600)
	waitForDevWatcherChurnBuildLine(t, paths.buildLog, "build-compiling:3")
	select {
	case result := <-writes:
		t.Fatalf("overlap writer completed before real compilation began: version=%s err=%v", result.version, result.err)
	default:
	}
	var overlapResult devWatcherPollingWriteResult
	select {
	case overlapResult = <-writes:
	case <-time.After(devWatcherChurnTimeout):
		t.Fatalf(
			"timed out waiting for continuous compile-overlap writes\n%s",
			readDevWatcherChurnFile(paths.buildLog),
		)
	}
	if overlapResult.err != nil {
		t.Fatalf("continuous compile-overlap writes: %v", overlapResult.err)
	}
	waitForDevWatcherChurnBuildLine(t, paths.buildLog, "build-success:4")
	overlapped := waitForDevWatcherChurnState(t, paths.state, func(state devWatcherChurnState) bool {
		return state.version == overlapResult.version && state.pid != continuous.pid
	})
	waitForDevWatcherChurnTaskIdle(t, buildTask)
	assertDevWatcherPollingBuildCountStable(t, paths.buildLog, 4)

	if err := os.RemoveAll(watchRoot); err != nil {
		t.Fatalf("remove polling watch root: %v", err)
	}
	waitForDevWatcherPollingHealth(t, engine, devwatch.HealthDegraded)
	mustWriteDevWatcherPollingSource(t, paths.root, "root-recreated", true)
	waitForDevWatcherPollingHealth(t, engine, devwatch.HealthHealthy)
	waitForDevWatcherChurnBuildLine(t, paths.buildLog, "build-success:5")
	recreated := waitForDevWatcherChurnState(t, paths.state, func(state devWatcherChurnState) bool {
		return state.version == "root-recreated" && state.pid != overlapped.pid
	})
	waitForDevWatcherChurnTaskIdle(t, buildTask)
	assertDevWatcherPollingBuildCountStable(t, paths.buildLog, 5)

	replacement := filepath.Join(paths.root, "cmd", "app.next")
	backup := filepath.Join(paths.root, "cmd", "app.previous")
	if err := writeDevWatcherPollingProgram(filepath.Join(replacement, "main.go"), "root-replaced"); err != nil {
		t.Fatalf("prepare atomic polling root replacement: %v", err)
	}
	if err := os.Rename(watchRoot, backup); err != nil {
		t.Fatalf("move current polling root: %v", err)
	}
	if err := os.Rename(replacement, watchRoot); err != nil {
		t.Fatalf("publish atomic polling root replacement: %v", err)
	}
	waitForDevWatcherChurnBuildLine(t, paths.buildLog, "build-success:6")
	final := waitForDevWatcherChurnState(t, paths.state, func(state devWatcherChurnState) bool {
		return state.version == "root-replaced" && state.pid != recreated.pid
	})
	waitForDevWatcherChurnTaskIdle(t, buildTask)
	assertDevWatcherPollingBuildCountStable(t, paths.buildLog, 6)

	identityVersion := strings.Repeat("i", len("root-replaced"))
	replaceDevWatcherPollingSourcePreservingMetadata(t, paths.root, identityVersion)
	waitForDevWatcherChurnBuildLine(t, paths.buildLog, "build-success:7")
	identityFinal := waitForDevWatcherChurnState(t, paths.state, func(state devWatcherChurnState) bool {
		return state.version == identityVersion && state.pid != final.pid
	})
	waitForDevWatcherChurnTaskIdle(t, buildTask)
	assertDevWatcherPollingBuildCountStable(t, paths.buildLog, 7)
	final = identityFinal
	assertNoDevWatcherChurnExit(t, controller)

	controller.stop(5 * time.Second)
	stopped = true
	if err := transcript.Close(); err != nil {
		t.Fatalf("close polling watcher transcript: %v", err)
	}
	assertDevWatcherChurnProcessesStopped(t, paths.lifecycle, final)
	assertDevWatcherPollingTranscript(t, paths.transcript)
}

// newDevWatcherPollingPaths allocates the polling-specific build barrier alongside shared probes.
func newDevWatcherPollingPaths(t *testing.T) devWatcherPollingPaths {
	t.Helper()
	paths := newDevWatcherChurnPaths(t)
	control := filepath.Dir(paths.buildLog)
	return devWatcherPollingPaths{
		devWatcherChurnPaths: paths,
		preBuildHold:         filepath.Join(control, "pre-build-hold"),
		preBuildRelease:      filepath.Join(control, "pre-build-release"),
	}
}

// devWatcherPollingEnvironment adds the polling helper's pre-compilation barriers.
func devWatcherPollingEnvironment(paths devWatcherPollingPaths) map[string]string {
	environment := devWatcherChurnEnvironment(paths.devWatcherChurnPaths)
	delete(environment, "GOFORJ_WATCHER_CHURN_HELPER")
	environment["GOFORJ_WATCHER_POLLING_HELPER"] = "1"
	environment["GOFORJ_WATCHER_POLLING_PRE_HOLD"] = paths.preBuildHold
	environment["GOFORJ_WATCHER_POLLING_PRE_RELEASE"] = paths.preBuildRelease
	return environment
}

// devWatcherPollingHelperCommand returns the polling helper command accepted by the production shell wrapper.
func devWatcherPollingHelperCommand() string {
	return devWatcherRunnerShellQuote(os.Args[0]) + " -test.run='^TestDevWatcherPollingBuildHelper$' -- build"
}

// runDevWatcherPollingInitialBuild publishes the baseline using the same helper used by watched rebuilds.
func runDevWatcherPollingInitialBuild(t *testing.T, paths devWatcherPollingPaths, environment map[string]string) {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestDevWatcherPollingBuildHelper$", "--", "build")
	command.Dir = paths.root
	command.Env = mergeDevWatcherChurnEnvironment(os.Environ(), environment)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("initial polling build: %v\n%s", err, output)
	}
	waitForDevWatcherChurnBuildLine(t, paths.buildLog, "build-success:1")
}

// runDevWatcherPollingBuildHelper brackets real compilation so source writes can overlap it deterministically.
func runDevWatcherPollingBuildHelper() {
	count, err := incrementDevWatcherChurnBuildCount(os.Getenv("GOFORJ_WATCHER_CHURN_BUILD_COUNT"))
	if err != nil {
		os.Exit(3)
	}
	buildLog := os.Getenv("GOFORJ_WATCHER_CHURN_BUILD_LOG")
	appendDevWatcherChurnLine(buildLog, fmt.Sprintf("build-start:%d", count))
	if _, err := os.Stat(os.Getenv("GOFORJ_WATCHER_POLLING_PRE_HOLD")); err == nil {
		appendDevWatcherChurnLine(buildLog, fmt.Sprintf("build-preblocked:%d", count))
		if !waitForDevWatcherPollingHelperFile(os.Getenv("GOFORJ_WATCHER_POLLING_PRE_RELEASE")) {
			os.Exit(4)
		}
	}
	appendDevWatcherChurnLine(buildLog, fmt.Sprintf("build-compiling:%d", count))
	appLogger := logger.NewSilentLogger()
	command := build.NewCmd(appLogger, ProvideAPIIndexRunner(appLogger))
	command.Root = os.Getenv("GOFORJ_WATCHER_CHURN_ROOT")
	command.SkipWire = true
	command.Args = []string{"-o", "./bin/app", "./cmd/app"}
	if err := command.Run(); err != nil {
		appendDevWatcherChurnLine(buildLog, fmt.Sprintf("build-failed:%d", count))
		os.Exit(1)
	}
	appendDevWatcherChurnLine(buildLog, fmt.Sprintf("build-published:%d", count))
	appendDevWatcherChurnLine(buildLog, fmt.Sprintf("build-success:%d", count))
}

// waitForDevWatcherPollingHelperFile bounds the pre-compilation barrier inside its subprocess.
func waitForDevWatcherPollingHelperFile(path string) bool {
	deadline := time.Now().Add(devWatcherChurnTimeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// runDevWatcherPollingWriteStream keeps valid atomic source replacements flowing across compilation.
func runDevWatcherPollingWriteStream(
	root string,
	prefix string,
	count int,
	releasePoint chan<- struct{},
	result chan<- devWatcherPollingWriteResult,
) {
	version := ""
	for index := range count {
		version = fmt.Sprintf("%s-%02d", prefix, index)
		if err := writeDevWatcherPollingSource(root, version, index%2 == 0); err != nil {
			result <- devWatcherPollingWriteResult{version: version, err: err}
			return
		}
		if index == 9 {
			releasePoint <- struct{}{}
		}
		time.Sleep(15 * time.Millisecond)
	}
	result <- devWatcherPollingWriteResult{version: version}
}

// mustWriteDevWatcherPollingSource fails the owning test when an editor-style replacement cannot be applied.
func mustWriteDevWatcherPollingSource(t *testing.T, root string, version string, atomicReplace bool) {
	t.Helper()
	if err := writeDevWatcherPollingSource(root, version, atomicReplace); err != nil {
		t.Fatalf("write polling source %q: %v", version, err)
	}
}

// writeDevWatcherPollingSource writes one complete valid program using direct or atomic editor semantics.
func writeDevWatcherPollingSource(root string, version string, atomicReplace bool) error {
	path := filepath.Join(root, "cmd", "app", "main.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if !atomicReplace {
		source := strings.ReplaceAll(devWatcherChurnProgram, "__WATCHER_CHURN_VERSION__", strconv.Quote(version))
		return os.WriteFile(path, []byte(source), 0o644)
	}
	temporary := path + ".polling-editor-swap"
	if err := writeDevWatcherPollingProgram(temporary, version); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

// writeDevWatcherPollingProgram writes the shared runtime probe with one embedded version.
func writeDevWatcherPollingProgram(path string, version string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	source := strings.ReplaceAll(devWatcherChurnProgram, "__WATCHER_CHURN_VERSION__", strconv.Quote(version))
	return os.WriteFile(path, []byte(source), 0o644)
}

// replaceDevWatcherPollingSourcePreservingMetadata reproduces editors that retain every legacy fingerprint field.
func replaceDevWatcherPollingSourcePreservingMetadata(t *testing.T, root string, version string) {
	t.Helper()
	path := filepath.Join(root, "cmd", "app", "main.go")
	originalInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat polling source before same-metadata replacement: %v", err)
	}
	temporary := path + ".same-metadata-swap"
	if err := writeDevWatcherPollingProgram(temporary, version); err != nil {
		t.Fatalf("write same-metadata polling replacement: %v", err)
	}
	if err := os.Chmod(temporary, originalInfo.Mode().Perm()); err != nil {
		t.Fatalf("restore polling replacement mode: %v", err)
	}
	if err := os.Chtimes(temporary, originalInfo.ModTime(), originalInfo.ModTime()); err != nil {
		t.Fatalf("restore polling replacement timestamps: %v", err)
	}
	replacementInfo, err := os.Stat(temporary)
	if err != nil {
		t.Fatalf("stat same-metadata polling replacement: %v", err)
	}
	if replacementInfo.Size() != originalInfo.Size() || replacementInfo.Mode() != originalInfo.Mode() ||
		!replacementInfo.ModTime().Equal(originalInfo.ModTime()) {
		t.Fatalf(
			"polling replacement metadata = size %d mode %v mtime %v, want size %d mode %v mtime %v",
			replacementInfo.Size(),
			replacementInfo.Mode(),
			replacementInfo.ModTime(),
			originalInfo.Size(),
			originalInfo.Mode(),
			originalInfo.ModTime(),
		)
	}
	if err := os.Rename(temporary, path); err != nil {
		t.Fatalf("publish same-metadata polling replacement: %v", err)
	}
}

// waitForDevWatcherPollingHealth waits for polling root coverage to reach the requested state.
func waitForDevWatcherPollingHealth(t *testing.T, engine *devwatch.Engine, expected devwatch.HealthState) {
	t.Helper()
	waitForDevWatcherChurnCondition(t, "polling watcher health "+string(expected), func() bool {
		health := engine.Health()
		return health.State == expected && health.Backend == devwatch.BackendPoll
	})
}

// assertDevWatcherPollingBuildCountStable verifies the polling stream has fully quiesced.
func assertDevWatcherPollingBuildCountStable(t *testing.T, path string, expected int) {
	t.Helper()
	time.Sleep(2 * devWatcherPollingDebounce)
	if starts := countDevWatcherChurnLines(path, "build-start:"); starts != expected {
		t.Fatalf("polling build starts=%d, want %d\n%s", starts, expected, readDevWatcherChurnFile(path))
	}
}

// assertDevWatcherPollingTranscript allows expected coverage degradation while rejecting binary launch races.
func assertDevWatcherPollingTranscript(t *testing.T, path string) {
	t.Helper()
	transcript := strings.ToLower(readDevWatcherChurnFile(path))
	for _, forbidden := range []string{
		"exec format error", "text file busy", "permission denied", "cannot execute",
		"/bin/app: no such file or directory", "cannot stat './bin/app'",
		"snapshot is not executable", "is not ready; waiting for a successful build",
	} {
		if strings.Contains(transcript, forbidden) {
			t.Fatalf("polling transcript contains binary launch failure %q:\n%s", forbidden, transcript)
		}
	}
}
