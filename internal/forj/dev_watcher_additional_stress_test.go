//go:build watcherstress && !windows

package forj

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/devwatch"
)

const additionalStressBurstBytes = 4 << 20

type additionalStressSlowWriter struct {
	delay time.Duration
	bytes atomic.Int64
}

// Write slows transcript consumption enough to force child output through bounded PTY backpressure.
func (w *additionalStressSlowWriter) Write(data []byte) (int, error) {
	time.Sleep(w.delay)
	w.bytes.Add(int64(len(data)))
	return len(data), nil
}

// Bytes reports how much child output reached the deliberately slow sink.
func (w *additionalStressSlowWriter) Bytes() int64 {
	return w.bytes.Load()
}

// TestDevWatcherAdditionalStressHelper provides output-heavy child processes without external fixtures.
func TestDevWatcherAdditionalStressHelper(t *testing.T) {
	if os.Getenv("GOFORJ_ADDITIONAL_STRESS_HELPER") != "1" {
		return
	}
	action := devWatcherChurnHelperAction(os.Args)
	switch action {
	case "output-build", "output-runtime":
		runAdditionalStressOutputHelper(action)
	default:
		os.Exit(2)
	}
}

// TestDevWatcherMultiAppChurnIsolation proves one shared engine keeps simultaneous App graphs independent.
func TestDevWatcherMultiAppChurnIsolation(t *testing.T) {
	alpha := newDevWatcherChurnPaths(t)
	beta := newDevWatcherChurnPaths(t)
	writeDevWatcherChurnProject(t, alpha.root, "alpha-initial")
	writeDevWatcherChurnProject(t, beta.root, "beta-initial")
	alphaEnvironment := devWatcherChurnEnvironment(alpha)
	betaEnvironment := devWatcherChurnEnvironment(beta)
	runDevWatcherChurnInitialBuild(t, alpha, alphaEnvironment)
	runDevWatcherChurnInitialBuild(t, beta, betaEnvironment)

	goMatcher := mustDevWatcherChurnMatcher(t, ".go")
	binMatcher := mustDevWatcherChurnMatcher(t, "bin")
	alphaBuildID := "watcherstress/alpha/build"
	alphaRuntimeID := "watcherstress/alpha/runtime"
	betaBuildID := "watcherstress/beta/build"
	betaRuntimeID := "watcherstress/beta/runtime"
	compiled := []devCompiledWatcher{
		additionalStressAppBuild(alphaBuildID, alphaRuntimeID, "alpha", alpha, alphaEnvironment, goMatcher, binMatcher),
		additionalStressAppRuntime(alphaRuntimeID, "alpha", alpha, alphaEnvironment),
		additionalStressAppBuild(betaBuildID, betaRuntimeID, "beta", beta, betaEnvironment, goMatcher, binMatcher),
		additionalStressAppRuntime(betaRuntimeID, "beta", beta, betaEnvironment),
	}

	transcriptPath := filepath.Join(t.TempDir(), "multi-app.log")
	transcript, err := os.OpenFile(transcriptPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open multi-App transcript: %v", err)
	}
	controller, err := newDevWatcherController(compiled, nil, transcript, transcript, false)
	if err != nil {
		_ = transcript.Close()
		t.Fatalf("start multi-App watcher controller: %v", err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			controller.stop(5 * time.Second)
		}
		_ = transcript.Close()
		if t.Failed() {
			t.Logf("alpha build log:\n%s", readDevWatcherChurnFile(alpha.buildLog))
			t.Logf("beta build log:\n%s", readDevWatcherChurnFile(beta.buildLog))
			t.Logf("multi-App transcript:\n%s", readDevWatcherChurnFile(transcriptPath))
		}
	})
	if len(controller.engines) != 1 || controller.engines[0].Health().State != devwatch.HealthHealthy {
		t.Fatalf("multi-App watcher did not establish one healthy shared engine: %#v", controller.engines)
	}

	alphaInitial := waitForDevWatcherChurnState(t, alpha.state, func(state devWatcherChurnState) bool {
		return state.version == "alpha-initial"
	})
	betaInitial := waitForDevWatcherChurnState(t, beta.state, func(state devWatcherChurnState) bool {
		return state.version == "beta-initial"
	})
	runAdditionalStressConcurrentChurn(t, alpha.root, beta.root)
	waitForDevWatcherChurnBuildLine(t, alpha.buildLog, "build-success:2")
	waitForDevWatcherChurnBuildLine(t, beta.buildLog, "build-success:2")
	alphaTogether := waitForDevWatcherChurnState(t, alpha.state, func(state devWatcherChurnState) bool {
		return state.version == "alpha-together" && state.pid != alphaInitial.pid
	})
	betaTogether := waitForDevWatcherChurnState(t, beta.state, func(state devWatcherChurnState) bool {
		return state.version == "beta-together" && state.pid != betaInitial.pid
	})
	waitForDevWatcherChurnTaskIdle(t, controller.tasks[alphaBuildID])
	waitForDevWatcherChurnTaskIdle(t, controller.tasks[betaBuildID])
	assertDevWatcherChurnBuildCountStable(t, alpha.buildLog, 2)
	assertDevWatcherChurnBuildCountStable(t, beta.buildLog, 2)

	for index := range 24 {
		mustWriteAdditionalStressSource(t, alpha.root, fmt.Sprintf("alpha-only-%02d", index), index%2 == 0)
	}
	mustWriteAdditionalStressSource(t, alpha.root, "alpha-isolated", true)
	waitForDevWatcherChurnBuildLine(t, alpha.buildLog, "build-success:3")
	alphaIsolated := waitForDevWatcherChurnState(t, alpha.state, func(state devWatcherChurnState) bool {
		return state.version == "alpha-isolated" && state.pid != alphaTogether.pid
	})
	waitForDevWatcherChurnHeartbeat(t, beta.state, betaTogether)
	waitForDevWatcherChurnTaskIdle(t, controller.tasks[alphaBuildID])
	assertDevWatcherChurnBuildCountStable(t, alpha.buildLog, 3)
	assertDevWatcherChurnBuildCountStable(t, beta.buildLog, 2)
	if starts := countDevWatcherChurnLines(alpha.lifecycle, "runtime-start:"); starts != 3 {
		t.Fatalf("alpha runtime starts=%d, want 3\n%s", starts, readDevWatcherChurnFile(alpha.lifecycle))
	}
	if starts := countDevWatcherChurnLines(beta.lifecycle, "runtime-start:"); starts != 2 {
		t.Fatalf("beta runtime starts=%d, want 2\n%s", starts, readDevWatcherChurnFile(beta.lifecycle))
	}
	assertNoDevWatcherChurnExit(t, controller)

	controller.stop(5 * time.Second)
	stopped = true
	if err := transcript.Close(); err != nil {
		t.Fatalf("close multi-App transcript: %v", err)
	}
	assertDevWatcherChurnProcessesStopped(t, alpha.lifecycle, alphaIsolated)
	assertDevWatcherChurnProcessesStopped(t, beta.lifecycle, betaTogether)
	assertDevWatcherChurnTranscript(t, transcriptPath)
}

// TestDevWatcherOutputBackpressureReapsProcesses proves output pressure cannot strand completed children.
func TestDevWatcherOutputBackpressureReapsProcesses(t *testing.T) {
	root := t.TempDir()
	lifecycle := filepath.Join(t.TempDir(), "output-lifecycle.log")
	environment := map[string]string{
		"GOFORJ_ADDITIONAL_STRESS_HELPER":    "1",
		"GOFORJ_ADDITIONAL_STRESS_LIFECYCLE": lifecycle,
	}
	buildID := "watcherstress/output/build"
	runtimeID := "watcherstress/output/runtime"
	compiled := []devCompiledWatcher{
		{
			ID: buildID, Name: "Output Build", Kind: devWatcherAppBuild, App: "output",
			Command:   devwatch.Command{Shell: additionalStressHelperCommand("output-build"), Dir: root, Env: environment},
			OnSuccess: []string{runtimeID},
		},
		{
			ID: runtimeID, Name: "Output Runtime", Kind: devWatcherAppRun, App: "output",
			Command:  devwatch.Command{Shell: additionalStressHelperCommand("output-runtime"), Dir: root, Env: environment},
			Postpone: true, Restart: true, FullProcessOverride: true,
		},
	}

	sink := &additionalStressSlowWriter{delay: time.Millisecond}
	controller, err := newDevWatcherController(compiled, nil, sink, sink, false)
	if err != nil {
		t.Fatalf("start output-pressure watcher controller: %v", err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			controller.stop(5 * time.Second)
		}
		if t.Failed() {
			t.Logf("output lifecycle:\n%s", readDevWatcherChurnFile(lifecycle))
		}
	})

	waitForDevWatcherChurnBuildLine(t, lifecycle, "output-build-exit:")
	waitForDevWatcherChurnTaskIdle(t, controller.tasks[buildID])
	waitForDevWatcherChurnBuildLine(t, lifecycle, "output-runtime-exit:")
	var runtimeExit watcherExit
	select {
	case runtimeExit = <-controller.exitCh:
	case <-time.After(devWatcherChurnTimeout):
		t.Fatal("timed out waiting for output-heavy runtime reaping")
	}
	if runtimeExit.id != runtimeID || runtimeExit.process == nil || runtimeExit.process.ExitCode != 0 {
		t.Fatalf("output-heavy runtime exit=%+v, want reaped successful runtime %q", runtimeExit, runtimeID)
	}
	buildPID := additionalStressLifecyclePID(t, lifecycle, "output-build-start:")
	runtimePID := additionalStressLifecyclePID(t, lifecycle, "output-runtime-start:")
	waitForDevWatcherChurnCondition(t, "output-heavy processes to be reaped", func() bool {
		return !devWatcherChurnProcessAlive(buildPID) && !devWatcherChurnProcessAlive(runtimePID)
	})
	wantBytes := int64(additionalStressBurstBytes * 4)
	if got := sink.Bytes(); got < wantBytes {
		t.Fatalf("slow sink bytes=%d, want at least %d from build/runtime stdout and stderr", got, wantBytes)
	}

	controller.stop(5 * time.Second)
	stopped = true
}

// TestDevWatcherInitialInvalidProjectRecovers proves startup tolerates no runnable artifact until a valid build arrives.
func TestDevWatcherInitialInvalidProjectRecovers(t *testing.T) {
	paths := newDevWatcherChurnPaths(t)
	writeDevWatcherChurnProject(t, paths.root, "never-runs")
	writeDevWatcherChurnFile(t, filepath.Join(paths.root, "cmd", "app", "main.go"), "package main\nfunc main() {\n", 0o644)
	environment := devWatcherChurnEnvironment(paths)
	buildID := "watcherstress/invalid/build"
	runtimeID := "watcherstress/invalid/runtime"
	compiled := []devCompiledWatcher{
		additionalStressAppBuild(
			buildID,
			runtimeID,
			"invalid",
			paths,
			environment,
			mustDevWatcherChurnMatcher(t, ".go"),
			mustDevWatcherChurnMatcher(t, "bin"),
		),
		additionalStressAppRuntime(runtimeID, "invalid", paths, environment),
	}
	compiled[0].Postpone = false
	compiled[1].Postpone = true

	transcriptPath := paths.transcript
	transcript, err := os.OpenFile(transcriptPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open invalid-start transcript: %v", err)
	}
	controller, err := newDevWatcherController(compiled, nil, transcript, transcript, false)
	if err != nil {
		_ = transcript.Close()
		t.Fatalf("start watcher for initially invalid project: %v", err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			controller.stop(5 * time.Second)
		}
		_ = transcript.Close()
		if t.Failed() {
			t.Logf("invalid-start build log:\n%s", readDevWatcherChurnFile(paths.buildLog))
			t.Logf("invalid-start transcript:\n%s", readDevWatcherChurnFile(transcriptPath))
		}
	})

	waitForDevWatcherChurnBuildLine(t, paths.buildLog, "build-failed:1")
	initialBuilds := waitForAdditionalStressBuildQuiescence(t, controller.tasks[buildID], paths.buildLog)
	if successes := countDevWatcherChurnLines(paths.buildLog, "build-success:"); successes != 0 {
		t.Fatalf("invalid startup produced %d successful builds\n%s", successes, readDevWatcherChurnFile(paths.buildLog))
	}
	if starts := countDevWatcherChurnLines(paths.lifecycle, "runtime-start:"); starts != 0 {
		t.Fatalf("initial invalid build started %d runtimes\n%s", starts, readDevWatcherChurnFile(paths.lifecycle))
	}
	if _, err := os.Stat(paths.state); !os.IsNotExist(err) {
		t.Fatalf("invalid startup runtime state error=%v, want no runtime state", err)
	}
	runtimeTask := controller.tasks[runtimeID]
	runtimeTask.mu.Lock()
	runtimeLive := runtimeTask.runtimeLive
	runtimeTask.mu.Unlock()
	if runtimeLive {
		t.Fatal("runtime bookkeeping became live after the invalid initial build")
	}

	mustWriteAdditionalStressSource(t, paths.root, "repaired-first-runtime", true)
	repairedBuild := initialBuilds + 1
	waitForDevWatcherChurnBuildLine(t, paths.buildLog, fmt.Sprintf("build-success:%d", repairedBuild))
	repaired := waitForDevWatcherChurnState(t, paths.state, func(state devWatcherChurnState) bool {
		return state.version == "repaired-first-runtime"
	})
	if builds := waitForAdditionalStressBuildQuiescence(t, controller.tasks[buildID], paths.buildLog); builds != repairedBuild {
		t.Fatalf("repair builds=%d, want %d\n%s", builds, repairedBuild, readDevWatcherChurnFile(paths.buildLog))
	}
	if starts := countDevWatcherChurnLines(paths.lifecycle, "runtime-start:"); starts != 1 {
		t.Fatalf("repaired project runtime starts=%d, want 1\n%s", starts, readDevWatcherChurnFile(paths.lifecycle))
	}
	assertNoDevWatcherChurnExit(t, controller)

	controller.stop(5 * time.Second)
	stopped = true
	if err := transcript.Close(); err != nil {
		t.Fatalf("close invalid-start transcript: %v", err)
	}
	assertDevWatcherChurnProcessesStopped(t, paths.lifecycle, repaired)
	assertDevWatcherChurnTranscript(t, transcriptPath)
}

// additionalStressAppBuild returns one conventional App build driven by real filesystem changes.
func additionalStressAppBuild(
	buildID string,
	runtimeID string,
	app string,
	paths devWatcherChurnPaths,
	environment map[string]string,
	goMatcher devwatch.Matcher,
	binMatcher devwatch.Matcher,
) devCompiledWatcher {
	return devCompiledWatcher{
		ID: buildID, Name: "Build " + app, Kind: devWatcherAppBuild, App: app,
		Watch: devwatch.Spec{
			Name: buildID, Roots: []string{paths.root}, Includes: []devwatch.Matcher{goMatcher},
			DirectoryExcludes: []devwatch.Matcher{binMatcher}, Debounce: devWatcherChurnDebounce, DebounceSet: true,
		},
		Command: devwatch.Command{
			Shell: devWatcherChurnHelperCommand(), Dir: paths.root, Env: environment,
		},
		Postpone: true, WatchChanges: true, OnSuccess: []string{runtimeID},
	}
}

// additionalStressAppRuntime returns one bare runtime launched only after its App build succeeds.
func additionalStressAppRuntime(
	runtimeID string,
	app string,
	paths devWatcherChurnPaths,
	environment map[string]string,
) devCompiledWatcher {
	return devCompiledWatcher{
		ID: runtimeID, Name: "Run " + app, Kind: devWatcherAppRun, App: app,
		Command: devwatch.Command{
			Shell: "./bin/app", Dir: paths.root, Env: environment, GracefulStopTimeout: 250 * time.Millisecond,
		},
		Restart: true,
	}
}

// runAdditionalStressConcurrentChurn starts two editor storms from the same synchronization edge.
func runAdditionalStressConcurrentChurn(t *testing.T, alphaRoot string, betaRoot string) {
	t.Helper()
	type churn struct {
		root   string
		prefix string
		final  string
	}
	churns := []churn{
		{root: alphaRoot, prefix: "alpha", final: "alpha-together"},
		{root: betaRoot, prefix: "beta", final: "beta-together"},
	}
	start := make(chan struct{})
	errorsCh := make(chan error, len(churns))
	var wait sync.WaitGroup
	for _, item := range churns {
		item := item
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			for index := range 32 {
				if err := writeAdditionalStressSource(
					item.root,
					fmt.Sprintf("%s-together-%02d", item.prefix, index),
					index%2 == 0,
				); err != nil {
					errorsCh <- err
					return
				}
			}
			if err := writeAdditionalStressSource(item.root, item.final, true); err != nil {
				errorsCh <- err
			}
		}()
	}
	close(start)
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
}

// mustWriteAdditionalStressSource applies one editor operation or stops the current test.
func mustWriteAdditionalStressSource(t *testing.T, root string, version string, atomicReplace bool) {
	t.Helper()
	if err := writeAdditionalStressSource(root, version, atomicReplace); err != nil {
		t.Fatal(err)
	}
}

// writeAdditionalStressSource updates the shared churn fixture without calling testing APIs from worker goroutines.
func writeAdditionalStressSource(root string, version string, atomicReplace bool) error {
	path := filepath.Join(root, "cmd", "app", "main.go")
	source := strings.ReplaceAll(devWatcherChurnProgram, "__WATCHER_CHURN_VERSION__", strconv.Quote(version))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create additional-stress source directory: %w", err)
	}
	if !atomicReplace {
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			return fmt.Errorf("write additional-stress source: %w", err)
		}
		return nil
	}
	temporary := path + ".additional-editor-swap"
	if err := os.WriteFile(temporary, []byte(source), 0o644); err != nil {
		return fmt.Errorf("write additional-stress editor swap: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("publish additional-stress editor swap: %w", err)
	}
	return nil
}

// additionalStressHelperCommand returns one output-helper command accepted by the production shell wrapper.
func additionalStressHelperCommand(action string) string {
	return devWatcherRunnerShellQuote(os.Args[0]) +
		" -test.run='^TestDevWatcherAdditionalStressHelper$' -- " + devWatcherRunnerShellQuote(action)
}

// runAdditionalStressOutputHelper emits enough data to fill child pipes before exiting normally.
func runAdditionalStressOutputHelper(action string) {
	lifecycle := os.Getenv("GOFORJ_ADDITIONAL_STRESS_LIFECYCLE")
	appendDevWatcherChurnLine(lifecycle, fmt.Sprintf("%s-start:%d", action, os.Getpid()))
	if err := emitAdditionalStressBurst(os.Stdout); err != nil {
		os.Exit(3)
	}
	if err := emitAdditionalStressBurst(os.Stderr); err != nil {
		os.Exit(4)
	}
	appendDevWatcherChurnLine(lifecycle, fmt.Sprintf("%s-exit:%d", action, os.Getpid()))
}

// emitAdditionalStressBurst writes a fixed byte count so the parent can detect output loss.
func emitAdditionalStressBurst(writer io.Writer) error {
	payload := bytes.Repeat([]byte("watcher-output-pressure-"), 2048)
	remaining := additionalStressBurstBytes
	for remaining > 0 {
		chunk := payload
		if len(chunk) > remaining {
			chunk = chunk[:remaining]
		}
		written, err := writer.Write(chunk)
		if err != nil {
			return err
		}
		if written != len(chunk) {
			return io.ErrShortWrite
		}
		remaining -= written
	}
	return nil
}

// additionalStressLifecyclePID extracts the managed PID from a durable helper record.
func additionalStressLifecyclePID(t *testing.T, path string, prefix string) int {
	t.Helper()
	for _, line := range readDevWatcherChurnLines(path) {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimPrefix(line, prefix))
		if err != nil {
			t.Fatalf("parse PID from %q: %v", line, err)
		}
		return pid
	}
	t.Fatalf("missing lifecycle record %q in:\n%s", prefix, readDevWatcherChurnFile(path))
	return 0
}

// waitForAdditionalStressBuildQuiescence requires idle bookkeeping and a stable start count across debounce windows.
func waitForAdditionalStressBuildQuiescence(t *testing.T, task *devWatcherTask, buildLog string) int {
	t.Helper()
	deadline := time.Now().Add(devWatcherChurnTimeout)
	lastCount := -1
	stableSince := time.Now()
	for time.Now().Before(deadline) {
		task.mu.Lock()
		idle := task.activeCancel == nil && !task.busy && len(task.triggerCh) == 0
		task.mu.Unlock()
		count := countDevWatcherChurnLines(buildLog, "build-start:")
		if !idle || count != lastCount {
			lastCount = count
			stableSince = time.Now()
		} else if time.Since(stableSince) >= 2*devWatcherChurnDebounce {
			return count
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for stable build quiescence\n%s", readDevWatcherChurnFile(buildLog))
	return 0
}
