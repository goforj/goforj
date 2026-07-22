package forj

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/devwatch"
	"github.com/goforj/goforj/project"
)

// TestDevWatcherRunnerHelper provides observable build and runtime behavior to controller tests.
func TestDevWatcherRunnerHelper(t *testing.T) {
	if os.Getenv("GOFORJ_DEV_WATCHER_RUNNER_HELPER") != "1" {
		return
	}
	action := devWatcherRunnerHelperAction(os.Args)
	switch action {
	case "build":
		runDevWatcherRunnerBuildHelper()
	case "runtime":
		runDevWatcherRunnerRuntimeHelper()
	case "runtime-tree":
		runDevWatcherRunnerTreeHelper()
	case "delayed-marker":
		runDevWatcherRunnerDelayedMarkerHelper()
	default:
		os.Exit(2)
	}
	os.Exit(0)
}

// TestConfigureCompiledDevCommandPreservesFullProcessOverride verifies mapped runtimes bypass binary snapshots.
func TestConfigureCompiledDevCommandPreservesFullProcessOverride(t *testing.T) {
	command := `MODE=dev "./bin/custom app" --flag='quoted value'`
	spec := devCompiledWatcher{
		Name: "Run App", Kind: devWatcherAppRun, App: "app", FullProcessOverride: true,
		Command: devwatch.Command{Shell: command},
	}
	configureCompiledDevCommand(
		&spec,
		nil,
		io.Discard,
		io.Discard,
		nil,
		false,
		newDevwatchLifecycleState(1, []string{"Run App"}),
		0,
		false,
		false,
	)
	if len(spec.Command.Args) != 3 {
		t.Fatalf("configured command args = %q, want Bash wrapper", spec.Command.Args)
	}
	script := spec.Command.Args[2]
	if !strings.HasSuffix(script, command) {
		t.Fatalf("full process override changed command text: %q", script)
	}
	for _, rewritten := range []string{"mktemp", "forj_dev_snapshot", "cp \"$forj_dev_target\""} {
		if strings.Contains(script, rewritten) {
			t.Fatalf("full process override unexpectedly contained %q: %q", rewritten, script)
		}
	}
}

// TestConfigureCompiledDevCommandSelectsRuntimeWrapper limits prevalidated snapshots to recognized App binaries.
func TestConfigureCompiledDevCommandSelectsRuntimeWrapper(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		spec            devCompiledWatcher
		wantNative      string
		wantScript      []string
		forbiddenScript []string
	}{
		{
			name: "structured native runtime",
			spec: devCompiledWatcher{
				Name: "Run App", Kind: devWatcherAppRun, App: "app",
				Command: devwatch.Command{Shell: "./bin/app http:serve"},
			},
			wantNative:      "./bin/app http:serve",
			wantScript:      []string{"refusing to start an unprepared native executable", "exit 1"},
			forbiddenScript: []string{"mktemp", "cp "},
		},
		{
			name: "legacy native runtime",
			spec: devCompiledWatcher{
				Name: "Run App", Kind: devWatcherAppRun, App: "app", Legacy: true,
				NativeRuntimeCommand: "./bin/app http:serve",
				Command:              devwatch.Command{Shell: "./bin/app http:serve"},
			},
			wantNative:      "./bin/app http:serve",
			wantScript:      []string{"refusing to start an unprepared native executable", "exit 1"},
			forbiddenScript: []string{"mktemp", "cp "},
		},
		{
			name: "legacy framework shell command",
			spec: devCompiledWatcher{
				Name: "Run App", Kind: devWatcherAppRun, App: "app", Legacy: true,
				Command: devwatch.Command{Shell: "air -c .air.toml"},
			},
			wantScript:      []string{"exec air -c .air.toml"},
			forbiddenScript: []string{"forj_dev_target", "refusing to start"},
		},
		{
			name: "legacy custom binary command",
			spec: devCompiledWatcher{
				Name: "Run Tests", Kind: devWatcherCustom, Legacy: true,
				Command: devwatch.Command{Shell: "./bin/app test"},
			},
			wantScript:      []string{"forj_dev_target='./bin/app'", "forj_dev_exec_target", "mktemp"},
			forbiddenScript: []string{"refusing to start"},
		},
		{
			name: "legacy custom shell command",
			spec: devCompiledWatcher{
				Name: "Watch Assets", Kind: devWatcherCustom, Legacy: true,
				Command: devwatch.Command{Shell: "npm run dev"},
			},
			wantScript:      []string{"exec npm run dev"},
			forbiddenScript: []string{"forj_dev_target", "refusing to start"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			spec := test.spec
			configureCompiledDevCommand(
				&spec,
				nil,
				io.Discard,
				io.Discard,
				nil,
				false,
				newDevwatchLifecycleState(1, []string{spec.Name}),
				0,
				false,
				false,
			)
			if spec.NativeRuntimeCommand != test.wantNative {
				t.Fatalf("NativeRuntimeCommand = %q, want %q", spec.NativeRuntimeCommand, test.wantNative)
			}
			if len(spec.Command.Args) != 3 {
				t.Fatalf("configured command args = %q, want Bash wrapper", spec.Command.Args)
			}
			script := spec.Command.Args[2]
			for _, expected := range test.wantScript {
				if !strings.Contains(script, expected) {
					t.Fatalf("wrapper script missing %q: %q", expected, script)
				}
			}
			for _, forbidden := range test.forbiddenScript {
				if strings.Contains(script, forbidden) {
					t.Fatalf("wrapper script unexpectedly contained %q: %q", forbidden, script)
				}
			}
		})
	}
}

// TestDevWatcherControllerTracksRuntimeExitGeneration verifies delayed old exits and immediate current exits preserve PID ownership.
func TestDevWatcherControllerTracksRuntimeExitGeneration(t *testing.T) {
	const runtimeID = "structured:app:app:app_run"
	output := newDevTaskOutputTail(40)
	controller := &devWatcherController{
		ctx: context.Background(), tasks: make(map[string]*devWatcherTask),
		exitCh: make(chan watcherExit, 1), exited: make(map[string]bool), errWriter: output,
	}
	task := &devWatcherTask{
		controller: controller,
		spec: devCompiledWatcher{
			ID: runtimeID, Name: "Run App", Kind: devWatcherAppRun,
		},
		runtimeLive: true,
		runtimePID:  202,
	}
	controller.tasks[runtimeID] = task

	controller.handleProcessExit(devwatch.Exit{Name: runtimeID, PID: 101, ExitCode: 1})
	task.mu.Lock()
	runtimeLive := task.runtimeLive
	runtimePID := task.runtimePID
	task.mu.Unlock()
	if !runtimeLive || runtimePID != 202 {
		t.Fatalf("stale runtime exit changed replacement ownership: live=%t PID=%d", runtimeLive, runtimePID)
	}
	select {
	case exit := <-controller.exitCh:
		t.Fatalf("stale runtime exit terminated the replacement watcher: %+v", exit)
	default:
	}

	controller.handleProcessExit(devwatch.Exit{Name: runtimeID, PID: 202, ExitCode: 7})
	task.mu.Lock()
	runtimeLive = task.runtimeLive
	runtimePID = task.runtimePID
	task.mu.Unlock()
	if runtimeLive || runtimePID != 0 {
		t.Fatalf("current runtime exit retained ownership: live=%t PID=%d", runtimeLive, runtimePID)
	}
	select {
	case exit := <-controller.exitCh:
		t.Fatalf("current structured runtime exit terminated the controller: %+v", exit)
	default:
	}
	if !strings.Contains(output.String(), "Run App exited; waiting for the next successful build") {
		t.Fatalf("runtime exit output = %q, want recoverable failure guidance", output.String())
	}

	const immediateRuntimeID = "structured:app:immediate:app_run"
	immediateOutput := newDevTaskOutputTail(40)
	immediateController := &devWatcherController{
		ctx: context.Background(), tasks: make(map[string]*devWatcherTask),
		exitCh: make(chan watcherExit, 1), exited: make(map[string]bool), errWriter: immediateOutput,
	}
	immediateTask := &devWatcherTask{
		controller: immediateController,
		spec: devCompiledWatcher{
			ID: immediateRuntimeID, Name: "Run Immediate", Kind: devWatcherAppRun,
		},
	}
	immediateController.tasks[immediateRuntimeID] = immediateTask
	immediateTask.runtimeMu.Lock()
	handled := make(chan struct{})
	go func() {
		immediateController.handleProcessExit(devwatch.Exit{
			Name: immediateRuntimeID, PID: 303, ExitCode: 9,
		})
		close(handled)
	}()
	select {
	case <-handled:
	case <-time.After(50 * time.Millisecond):
	}
	immediateTask.mu.Lock()
	immediateTask.runtimeLive = true
	immediateTask.runtimePID = 303
	immediateTask.mu.Unlock()
	immediateTask.runtimeMu.Unlock()
	select {
	case <-handled:
	case <-time.After(time.Second):
		t.Fatal("immediate runtime exit did not resume after PID publication")
	}
	immediateTask.mu.Lock()
	runtimeLive = immediateTask.runtimeLive
	runtimePID = immediateTask.runtimePID
	immediateTask.mu.Unlock()
	if runtimeLive || runtimePID != 0 {
		t.Fatalf("immediate runtime exit raced PID publication: live=%t PID=%d", runtimeLive, runtimePID)
	}
	select {
	case exit := <-immediateController.exitCh:
		t.Fatalf("immediate structured runtime exit terminated the controller: %+v", exit)
	default:
	}
	if !strings.Contains(immediateOutput.String(), "Run Immediate exited; waiting for the next successful build") {
		t.Fatalf("immediate runtime output = %q, want recoverable failure guidance", immediateOutput.String())
	}
}

// TestDevWatcherControllerImmediateRuntimeExitUsesPublishedPID verifies an early crash clears ownership without ending dev.
func TestDevWatcherControllerImmediateRuntimeExitUsesPublishedPID(t *testing.T) {
	requireDevWatcherRunnerTestPlatform(t)
	output := newDevTaskOutputTail(40)
	controller, err := newDevWatcherController([]devCompiledWatcher{{
		Name: "Run Immediate", Kind: devWatcherAppRun, App: "app",
		Command: devwatch.Command{Shell: "exit 23"}, FullProcessOverride: true,
	}}, nil, io.Discard, output, false)
	if err != nil {
		t.Fatalf("start dev watcher controller: %v", err)
	}
	defer controller.stop(time.Second)

	waitForDevWatcherRunnerCondition(t, "immediate runtime failure to remain visible", func() bool {
		return strings.Contains(output.String(), "Run Immediate exited; waiting for the next successful build")
	})
	task := controller.tasks["Run Immediate"]
	task.mu.Lock()
	runtimeLive := task.runtimeLive
	runtimePID := task.runtimePID
	task.mu.Unlock()
	if runtimeLive || runtimePID != 0 {
		t.Fatalf("immediate runtime ownership after exit: live=%t PID=%d", runtimeLive, runtimePID)
	}
	select {
	case exit := <-controller.exitCh:
		t.Fatalf("immediate structured runtime exit terminated the controller: %+v", exit)
	default:
	}
}

// TestDevWatcherControllerRestartsCrashedRuntimeAfterSuccessfulBuild verifies build publication recovers a stopped App.
func TestDevWatcherControllerRestartsCrashedRuntimeAfterSuccessfulBuild(t *testing.T) {
	requireDevWatcherRunnerTestPlatform(t)
	const buildID = "structured:app:app:app_build"
	const runtimeID = "structured:app:app:app_run"
	crashMarker := filepath.Join(t.TempDir(), "first-runtime-crashed")
	runtimeCommand := "if [ ! -f " + devWatcherRunnerShellQuote(crashMarker) + " ]; then : > " +
		devWatcherRunnerShellQuote(crashMarker) + "; exit 23; fi; exec sleep 30"
	output := newDevTaskOutputTail(40)
	controller, err := newDevWatcherController([]devCompiledWatcher{
		{
			ID: buildID, Name: "Build App", Kind: devWatcherAppBuild, App: "app", Postpone: true,
			Command: devwatch.Command{Shell: "true"}, OnSuccess: []string{runtimeID},
		},
		{
			ID: runtimeID, Name: "Run App", Kind: devWatcherAppRun, App: "app",
			Command: devwatch.Command{Shell: runtimeCommand}, FullProcessOverride: true,
		},
	}, nil, io.Discard, output, false)
	if err != nil {
		t.Fatalf("start dev watcher controller: %v", err)
	}
	defer controller.stop(time.Second)

	waitForDevWatcherRunnerCondition(t, "runtime crash to become recoverable", func() bool {
		return strings.Contains(output.String(), "Run App exited; waiting for the next successful build")
	})
	select {
	case exit := <-controller.exitCh:
		t.Fatalf("runtime crash terminated the controller: %+v", exit)
	default:
	}

	controller.tasks[buildID].request()
	waitForDevWatcherRunnerCondition(t, "successful build to restart runtime", func() bool {
		task := controller.tasks[runtimeID]
		task.mu.Lock()
		live := task.runtimeLive
		task.mu.Unlock()
		return live && controller.supervisor.RuntimeRunning(runtimeID)
	})
	select {
	case exit := <-controller.exitCh:
		t.Fatalf("restarted runtime terminated the controller: %+v", exit)
	default:
	}
}

// TestDevWatcherControllerCustomExitRemainsTerminal verifies explicit custom watcher exit policy is unchanged.
func TestDevWatcherControllerCustomExitRemainsTerminal(t *testing.T) {
	requireDevWatcherRunnerTestPlatform(t)
	controller := newDevWatcherRunnerTestController(t, []devCompiledWatcher{{
		ID: "custom:desktop", Name: "Harbor Desktop", Kind: devWatcherCustom, Exit: true,
		Command: devwatch.Command{Shell: "false"},
	}})
	defer controller.stop(time.Second)

	select {
	case exit := <-controller.exitCh:
		if exit.name != "Harbor Desktop" || exit.process == nil || exit.process.ExitCode != 1 {
			t.Fatalf("custom watcher exit = %+v, want terminal code 1", exit)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("timed out waiting for custom watcher terminal exit")
	}
}

// TestUnexpectedWatcherExitErrorIncludesWatcherName verifies child errors retain their watcher identity.
func TestUnexpectedWatcherExitErrorIncludesWatcherName(t *testing.T) {
	processErr := errors.New("exit status 1")
	err := unexpectedWatcherExitError(watcherExit{
		name:   "Harbor Desktop",
		err:    processErr,
		output: "Wails development server port is already in use",
	})
	if err == nil {
		t.Fatal("expected watcher exit error")
	}
	if !strings.Contains(err.Error(), `dev watcher "Harbor Desktop" exited`) {
		t.Fatalf("error = %q, want watcher name", err)
	}
	if !errors.Is(err, processErr) {
		t.Fatalf("error = %v, want wrapped process error", err)
	}
	if !strings.Contains(err.Error(), "Last watcher output:\nWails development server port is already in use") {
		t.Fatalf("error = %q, want retained watcher output", err)
	}
}

// TestUnexpectedWatcherExitErrorIncludesWatcherNameForExitCode verifies status-only failures retain their watcher identity.
func TestUnexpectedWatcherExitErrorIncludesWatcherNameForExitCode(t *testing.T) {
	err := unexpectedWatcherExitError(watcherExit{
		name:    "Harbor Desktop",
		process: &devwatch.Exit{ExitCode: 23},
	})
	if err == nil {
		t.Fatal("expected watcher exit error")
	}
	if got, want := err.Error(), `dev watcher "Harbor Desktop" exited with code 23`; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

// TestDevWatcherControllerRetainsOnlyFailingTUIWatcherOutput verifies fatal replay stays scoped to one watcher.
func TestDevWatcherControllerRetainsOnlyFailingTUIWatcherOutput(t *testing.T) {
	compiled := []devCompiledWatcher{
		{
			ID: "desktop", Name: "Harbor Desktop", Kind: devWatcherCustom, Postpone: true,
			Command: devwatch.Command{Shell: "wails dev"},
		},
		{
			ID: "daemon", Name: "Run harbord", Kind: devWatcherCustom, Postpone: true,
			Command: devwatch.Command{Shell: "./bin/harbord --foreground"},
		},
	}
	bubble := &devBubbleWriter{disabled: true}
	controller, err := newDevWatcherController(compiled, nil, bubble, bubble, false)
	if err != nil {
		t.Fatalf("create watcher controller: %v", err)
	}
	defer controller.stop(time.Second)
	for _, taskID := range []string{"desktop", "daemon"} {
		command := controller.tasks[taskID].spec.Command
		if command.PTY {
			t.Fatalf("TUI watcher %q unexpectedly owns a nested pseudo-terminal", taskID)
		}
		if command.Stdout == nil || command.Stderr == nil {
			t.Fatalf("TUI watcher %q did not preserve separate stdout and stderr streams", taskID)
		}
	}

	if _, err := controller.tasks["desktop"].spec.Command.Stdout.Write([]byte("desktop port conflict\n")); err != nil {
		t.Fatalf("write desktop output: %v", err)
	}
	if _, err := controller.tasks["daemon"].spec.Command.Stdout.Write([]byte("daemon healthy\n")); err != nil {
		t.Fatalf("write daemon output: %v", err)
	}
	controller.publishExit("desktop", "Harbor Desktop", &devwatch.Exit{ExitCode: 1}, errors.New("exit status 1"))

	exit := <-controller.exitCh
	if !strings.Contains(exit.output, "desktop port conflict") {
		t.Fatalf("retained output = %q, want desktop diagnostic", exit.output)
	}
	if strings.Contains(exit.output, "daemon healthy") {
		t.Fatalf("retained output = %q, unexpectedly included another watcher", exit.output)
	}
}

// TestShouldRetainDevWatcherFailureOutputLimitsReplayToTUI verifies plain sessions do not duplicate streamed diagnostics.
func TestShouldRetainDevWatcherFailureOutputLimitsReplayToTUI(t *testing.T) {
	if !shouldRetainDevWatcherFailureOutput(&devBubbleWriter{}) {
		t.Fatal("expected Bubble Tea output to retain a failure tail")
	}
	if shouldRetainDevWatcherFailureOutput(io.Discard) {
		t.Fatal("expected plain output to avoid retaining a duplicate failure tail")
	}
}

// TestShouldAttachDevWatcherPTYGivesTheOuterTUIExclusiveTerminalOwnership prevents nested terminal renderers from corrupting its reserved rows.
func TestShouldAttachDevWatcherPTYGivesTheOuterTUIExclusiveTerminalOwnership(t *testing.T) {
	tests := []struct {
		name   string
		writer io.Writer
		goos   string
		want   bool
	}{
		{name: "Bubble Tea on Darwin", writer: &devBubbleWriter{}, goos: "darwin", want: false},
		{name: "Bubble Tea on Linux", writer: &devBubbleWriter{}, goos: "linux", want: false},
		{name: "plain Darwin", writer: io.Discard, goos: "darwin", want: true},
		{name: "plain Linux", writer: io.Discard, goos: "linux", want: true},
		{name: "plain Windows", writer: io.Discard, goos: "windows", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldAttachDevWatcherPTY(test.writer, test.goos); got != test.want {
				t.Fatalf("shouldAttachDevWatcherPTY() = %t, want %t", got, test.want)
			}
		})
	}
}

// TestDevWatcherControllerDuplicateLegacyNamesRunIndependently verifies display collisions do not merge tasks or processes.
func TestDevWatcherControllerDuplicateLegacyNamesRunIndependently(t *testing.T) {
	requireDevWatcherRunnerTestPlatform(t)
	directory := t.TempDir()
	compiled, err := compileDevWatchers(&project.Config{Dev: project.DevConfig{Watches: []project.DevWatch{
		{Name: "Assets", Watch: "-file .css -postpone", Exec: "echo first"},
		{Name: "Assets", Watch: "-file .js -postpone", Exec: "echo second"},
	}}})
	if err != nil {
		t.Fatalf("compile duplicate legacy watchers: %v", err)
	}
	logs := []string{filepath.Join(directory, "first.log"), filepath.Join(directory, "second.log")}
	for index := range compiled {
		compiled[index].WatchChanges = false
		compiled[index].Postpone = true
		compiled[index].Command = devWatcherRunnerHelperCommand("build", map[string]string{
			"GOFORJ_DEV_WATCHER_LOG":     logs[index],
			"GOFORJ_DEV_WATCHER_COUNTER": filepath.Join(directory, fmt.Sprintf("counter-%d", index)),
		})
	}
	controller := newDevWatcherRunnerTestController(t, compiled)
	defer controller.stop(time.Second)

	for _, watcher := range compiled {
		controller.tasks[watcher.ID].request()
	}
	for index, watcher := range compiled {
		waitForDevWatcherRunnerTaskIdle(t, controller.tasks[watcher.ID], logs[index], "build-success:1:")
	}
	if len(controller.tasks) != 2 || compiled[0].ID == compiled[1].ID {
		t.Fatalf("duplicate display names merged internal tasks: IDs %q and %q", compiled[0].ID, compiled[1].ID)
	}
}

// TestDevWatcherControllerDisplayCollisionFollowsStructuredID verifies SPA success cannot trigger a custom namesake.
func TestDevWatcherControllerDisplayCollisionFollowsStructuredID(t *testing.T) {
	requireDevWatcherRunnerTestPlatform(t)
	t.Setenv("FORJ_APP", "")
	directory := t.TempDir()
	compiled, err := compileDevWatchers(&project.Config{Dev: project.DevConfig{
		Apps: map[string]project.DevApp{
			project.DefaultAppName: {
				Run: &project.DevAppCommand{Disabled: true},
				SPAs: map[string]project.DevSPA{
					"portal": {Path: "./cmd/app/frontend"},
				},
			},
		},
		Watches: []project.DevWatch{{Name: "Build App", Include: []string{".md"}, Exec: "echo custom"}},
	}})
	if err != nil {
		t.Fatalf("compile colliding watcher graph: %v", err)
	}
	spaLog := filepath.Join(directory, "spa.log")
	appLog := filepath.Join(directory, "app.log")
	customLog := filepath.Join(directory, "custom.log")
	var spaID string
	var appBuildID string
	var customID string
	for index := range compiled {
		compiled[index].WatchChanges = false
		compiled[index].Postpone = true
		logPath := customLog
		counterName := "custom-count"
		switch compiled[index].Kind {
		case devWatcherSPABuild:
			spaID = compiled[index].ID
			logPath = spaLog
			counterName = "spa-count"
		case devWatcherAppBuild:
			appBuildID = compiled[index].ID
			logPath = appLog
			counterName = "app-count"
		case devWatcherCustom:
			customID = compiled[index].ID
		}
		compiled[index].Command = devWatcherRunnerHelperCommand("build", map[string]string{
			"GOFORJ_DEV_WATCHER_LOG":     logPath,
			"GOFORJ_DEV_WATCHER_COUNTER": filepath.Join(directory, counterName),
		})
	}
	controller := newDevWatcherRunnerTestController(t, compiled)
	defer controller.stop(time.Second)

	controller.tasks[spaID].request()
	waitForDevWatcherRunnerTaskIdle(t, controller.tasks[appBuildID], appLog, "build-success:1:")
	if lines := readDevWatcherRunnerLog(t, customLog); len(lines) != 0 {
		t.Fatalf("structured edge triggered custom namesake %q: %v", customID, lines)
	}
}

// TestDevWatcherControllerSuccessfulBuildRestartsRuntime verifies successful graph edges publish a replacement runtime.
func TestDevWatcherControllerSuccessfulBuildRestartsRuntime(t *testing.T) {
	requireDevWatcherRunnerTestPlatform(t)
	directory := t.TempDir()
	logPath := filepath.Join(directory, "events.log")
	buildCommand := devWatcherRunnerHelperCommand("build", map[string]string{
		"GOFORJ_DEV_WATCHER_LOG":     logPath,
		"GOFORJ_DEV_WATCHER_COUNTER": filepath.Join(directory, "build-count"),
	})
	runtimeCommand := devWatcherRunnerHelperCommand("runtime", map[string]string{
		"GOFORJ_DEV_WATCHER_LOG": logPath,
	})
	compiled := []devCompiledWatcher{
		{
			Name: "Build App", Kind: devWatcherAppBuild, App: "app", Command: buildCommand,
			Postpone: true, OnSuccess: []string{"Run App"},
		},
		{
			Name: "Run App", Kind: devWatcherAppRun, App: "app", Command: runtimeCommand,
			Restart: true,
		},
	}
	controller := newDevWatcherRunnerTestController(t, compiled)
	defer controller.stop(time.Second)

	waitForDevWatcherRunnerLog(t, logPath, func(lines []string) bool {
		return countDevWatcherRunnerLines(lines, "runtime-start:") == 1
	})
	controller.tasks["Build App"].request()
	lines := waitForDevWatcherRunnerLog(t, logPath, func(lines []string) bool {
		return countDevWatcherRunnerLines(lines, "runtime-start:") == 2
	})

	firstRuntime := indexDevWatcherRunnerLine(lines, "runtime-start:", 1)
	buildStarted := indexDevWatcherRunnerLine(lines, "build-start:1:", 1)
	buildSucceeded := indexDevWatcherRunnerLine(lines, "build-success:1:", 1)
	runtimeStopped := indexDevWatcherRunnerLine(lines, "runtime-stop:", 1)
	secondRuntime := indexDevWatcherRunnerLine(lines, "runtime-start:", 2)
	if !(firstRuntime < buildStarted && buildStarted < buildSucceeded && buildSucceeded < runtimeStopped && runtimeStopped < secondRuntime) {
		t.Fatalf("unexpected build-to-runtime ordering: %v", lines)
	}
}

// TestDevWatcherControllerFailedBuildKeepsRuntime verifies a failed upstream command cannot follow success edges.
func TestDevWatcherControllerFailedBuildKeepsRuntime(t *testing.T) {
	requireDevWatcherRunnerTestPlatform(t)
	directory := t.TempDir()
	logPath := filepath.Join(directory, "events.log")
	buildCommand := devWatcherRunnerHelperCommand("build", map[string]string{
		"GOFORJ_DEV_WATCHER_LOG":     logPath,
		"GOFORJ_DEV_WATCHER_COUNTER": filepath.Join(directory, "build-count"),
		"GOFORJ_DEV_WATCHER_FAIL":    "1",
	})
	runtimeCommand := devWatcherRunnerHelperCommand("runtime", map[string]string{
		"GOFORJ_DEV_WATCHER_LOG": logPath,
	})
	compiled := []devCompiledWatcher{
		{
			Name: "Build App", Kind: devWatcherAppBuild, App: "app", Command: buildCommand,
			Postpone: true, OnSuccess: []string{"Run App"},
		},
		{Name: "Run App", Kind: devWatcherAppRun, App: "app", Command: runtimeCommand, Restart: true},
	}
	controller := newDevWatcherRunnerTestController(t, compiled)
	defer controller.stop(time.Second)

	waitForDevWatcherRunnerLog(t, logPath, func(lines []string) bool {
		return countDevWatcherRunnerLines(lines, "runtime-start:") == 1
	})
	controller.tasks["Build App"].request()
	waitForDevWatcherRunnerTaskIdle(t, controller.tasks["Build App"], logPath, "build-failed:1:")
	lines := readDevWatcherRunnerLog(t, logPath)
	if starts := countDevWatcherRunnerLines(lines, "runtime-start:"); starts != 1 {
		t.Fatalf("failed build unexpectedly restarted runtime %d times: %v", starts-1, lines)
	}
	if stops := countDevWatcherRunnerLines(lines, "runtime-stop:"); stops != 0 {
		t.Fatalf("failed build unexpectedly stopped the healthy runtime: %v", lines)
	}
}

// TestDevWatcherControllerLegacyBuildPublishesReadyStamp keeps custom historical builders connected to runtime restarts.
func TestDevWatcherControllerLegacyBuildPublishesReadyStamp(t *testing.T) {
	requireDevWatcherRunnerTestPlatform(t)
	root := t.TempDir()
	originalDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDirectory) })
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	logPath := filepath.Join(root, "events.log")
	buildCommand := devWatcherRunnerHelperCommand("build", map[string]string{
		"GOFORJ_DEV_WATCHER_LOG":     logPath,
		"GOFORJ_DEV_WATCHER_COUNTER": filepath.Join(root, "build-count"),
		"GOFORJ_DEV_WATCHER_BINARY":  filepath.Join("bin", "app"),
	})
	controller := newDevWatcherRunnerTestController(t, []devCompiledWatcher{{
		Name: "Build App", Kind: devWatcherAppBuild, App: "app", Command: buildCommand,
		Postpone: true, Legacy: true,
	}})
	defer controller.stop(time.Second)

	controller.tasks["Build App"].request()
	waitForDevWatcherRunnerTaskIdle(t, controller.tasks["Build App"], logPath, "build-success:1:")
	if _, err := os.Stat(filepath.Join("bin", ".app.ready")); err != nil {
		t.Fatalf("legacy build did not publish runtime readiness: %v", err)
	}
}

// TestDevWatcherControllerReconciliationGatesStructuredRuntime verifies replacement coverage precedes runtime publication.
func TestDevWatcherControllerReconciliationGatesStructuredRuntime(t *testing.T) {
	requireDevWatcherRunnerTestPlatform(t)
	directory := t.TempDir()
	logPath := filepath.Join(directory, "events.log")
	compiled := []devCompiledWatcher{{
		Name: "Run App", Kind: devWatcherAppRun, App: "app",
		Command: devWatcherRunnerHelperCommand("runtime", map[string]string{
			"GOFORJ_DEV_WATCHER_LOG": logPath,
		}),
		Restart: true, Postpone: true,
	}}
	controller, err := newDevWatcherControllerWithOptions(
		compiled,
		nil,
		io.Discard,
		io.Discard,
		false,
		devWatcherControllerOptions{reconcile: true},
	)
	if err != nil {
		t.Fatalf("start reconciling dev watcher controller: %v", err)
	}
	defer controller.stop(time.Second)

	controller.tasks["Run App"].request()
	time.Sleep(100 * time.Millisecond)
	if lines := readDevWatcherRunnerLog(t, logPath); countDevWatcherRunnerLines(lines, "runtime-start:") != 0 {
		t.Fatalf("structured runtime started before reconciliation finished: %v", lines)
	}
	controller.finishReconciliation(true)
	waitForDevWatcherRunnerLog(t, logPath, func(lines []string) bool {
		return countDevWatcherRunnerLines(lines, "runtime-start:") == 1
	})
}

// TestDevWatcherControllerReconciliationReplaysBufferedBuild verifies source changes survive the outer-build barrier.
func TestDevWatcherControllerReconciliationReplaysBufferedBuild(t *testing.T) {
	requireDevWatcherRunnerTestPlatform(t)
	directory := t.TempDir()
	logPath := filepath.Join(directory, "events.log")
	compiled := []devCompiledWatcher{{
		Name: "Build App", Kind: devWatcherAppBuild, App: "app",
		Command: devWatcherRunnerHelperCommand("build", map[string]string{
			"GOFORJ_DEV_WATCHER_LOG":     logPath,
			"GOFORJ_DEV_WATCHER_COUNTER": filepath.Join(directory, "build-count"),
		}),
		Postpone: true,
	}}
	controller, err := newDevWatcherControllerWithOptions(
		compiled,
		nil,
		io.Discard,
		io.Discard,
		false,
		devWatcherControllerOptions{reconcile: true},
	)
	if err != nil {
		t.Fatalf("start reconciling dev watcher controller: %v", err)
	}
	defer controller.stop(time.Second)

	controller.tasks["Build App"].request()
	time.Sleep(100 * time.Millisecond)
	if lines := readDevWatcherRunnerLog(t, logPath); countDevWatcherRunnerLines(lines, "build-start:") != 0 {
		t.Fatalf("paused structured build started before reconciliation finished: %v", lines)
	}
	controller.finishReconciliation(true)
	waitForDevWatcherRunnerTaskIdle(t, controller.tasks["Build App"], logPath, "build-success:1:")
	lines := readDevWatcherRunnerLog(t, logPath)
	if starts := countDevWatcherRunnerLines(lines, "build-start:"); starts != 1 {
		t.Fatalf("expected one replayed build, got %d: %v", starts, lines)
	}
}

// TestDevWatcherControllerSuccessfulReconciliationWaitsForBufferedAppBuild verifies runtime publication follows replayed app work.
func TestDevWatcherControllerSuccessfulReconciliationWaitsForBufferedAppBuild(t *testing.T) {
	requireDevWatcherRunnerTestPlatform(t)
	directory := t.TempDir()
	logPath := filepath.Join(directory, "events.log")
	releasePath := filepath.Join(directory, "release-build")
	compiled := []devCompiledWatcher{
		{
			Name: "Build App", Kind: devWatcherAppBuild, App: "app",
			Command: devWatcherRunnerHelperCommand("build", map[string]string{
				"GOFORJ_DEV_WATCHER_LOG":     logPath,
				"GOFORJ_DEV_WATCHER_COUNTER": filepath.Join(directory, "build-count"),
				"GOFORJ_DEV_WATCHER_RELEASE": releasePath,
			}),
			Postpone: true, OnSuccess: []string{"Run App"},
		},
		{
			Name: "Run App", Kind: devWatcherAppRun, App: "app",
			Command: devWatcherRunnerHelperCommand("runtime", map[string]string{
				"GOFORJ_DEV_WATCHER_LOG": logPath,
			}),
			Restart: true,
		},
	}
	controller, err := newDevWatcherControllerWithOptions(
		compiled,
		nil,
		io.Discard,
		io.Discard,
		false,
		devWatcherControllerOptions{reconcile: true},
	)
	if err != nil {
		t.Fatalf("start reconciling dev watcher controller: %v", err)
	}
	defer controller.stop(time.Second)

	controller.tasks["Build App"].request()
	controller.tasks["Run App"].request()
	controller.finishReconciliation(true)
	lines := waitForDevWatcherRunnerLog(t, logPath, func(lines []string) bool {
		return countDevWatcherRunnerLines(lines, "build-start:1:") == 1
	})
	if starts := countDevWatcherRunnerLines(lines, "runtime-start:"); starts != 0 {
		t.Fatalf("runtime started before buffered app build succeeded: %v", lines)
	}
	if err := os.WriteFile(releasePath, []byte("release"), 0o600); err != nil {
		t.Fatalf("release buffered app build: %v", err)
	}
	lines = waitForDevWatcherRunnerLog(t, logPath, func(lines []string) bool {
		return countDevWatcherRunnerLines(lines, "runtime-start:") == 1
	})
	buildSucceeded := indexDevWatcherRunnerLine(lines, "build-success:1:", 1)
	runtimeStarted := indexDevWatcherRunnerLine(lines, "runtime-start:", 1)
	if buildSucceeded < 0 || runtimeStarted < buildSucceeded {
		t.Fatalf("runtime did not wait for buffered app build success: %v", lines)
	}
}

// TestDevWatcherControllerSuccessfulReconciliationWaitsForAllBufferedSPAs verifies one app build joins every changed frontend.
func TestDevWatcherControllerSuccessfulReconciliationWaitsForAllBufferedSPAs(t *testing.T) {
	requireDevWatcherRunnerTestPlatform(t)
	directory := t.TempDir()
	firstSPALog := filepath.Join(directory, "first-spa.log")
	secondSPALog := filepath.Join(directory, "second-spa.log")
	appLog := filepath.Join(directory, "app.log")
	firstRelease := filepath.Join(directory, "release-first-spa")
	secondRelease := filepath.Join(directory, "release-second-spa")
	compiled := []devCompiledWatcher{
		{
			Name: "Build SPA Admin", Kind: devWatcherSPABuild, App: "app",
			Command: devWatcherRunnerHelperCommand("build", map[string]string{
				"GOFORJ_DEV_WATCHER_LOG":     firstSPALog,
				"GOFORJ_DEV_WATCHER_COUNTER": filepath.Join(directory, "first-spa-count"),
				"GOFORJ_DEV_WATCHER_RELEASE": firstRelease,
			}),
			Postpone: true, OnSuccess: []string{"Build App"},
		},
		{
			Name: "Build SPA Portal", Kind: devWatcherSPABuild, App: "app",
			Command: devWatcherRunnerHelperCommand("build", map[string]string{
				"GOFORJ_DEV_WATCHER_LOG":     secondSPALog,
				"GOFORJ_DEV_WATCHER_COUNTER": filepath.Join(directory, "second-spa-count"),
				"GOFORJ_DEV_WATCHER_RELEASE": secondRelease,
			}),
			Postpone: true, OnSuccess: []string{"Build App"},
		},
		{
			Name: "Build App", Kind: devWatcherAppBuild, App: "app",
			Command: devWatcherRunnerHelperCommand("build", map[string]string{
				"GOFORJ_DEV_WATCHER_LOG":     appLog,
				"GOFORJ_DEV_WATCHER_COUNTER": filepath.Join(directory, "app-build-count"),
			}),
			Postpone: true, OnSuccess: []string{"Run App"},
		},
		{
			Name: "Run App", Kind: devWatcherAppRun, App: "app",
			Command: devWatcherRunnerHelperCommand("runtime", map[string]string{
				"GOFORJ_DEV_WATCHER_LOG": appLog,
			}),
			Restart: true,
		},
	}
	controller, err := newDevWatcherControllerWithOptions(
		compiled,
		nil,
		io.Discard,
		io.Discard,
		false,
		devWatcherControllerOptions{reconcile: true},
	)
	if err != nil {
		t.Fatalf("start reconciling dev watcher controller: %v", err)
	}
	defer controller.stop(time.Second)

	controller.tasks["Build SPA Admin"].request()
	controller.tasks["Build SPA Portal"].request()
	controller.finishReconciliation(true)
	waitForDevWatcherRunnerLog(t, firstSPALog, func(lines []string) bool {
		return countDevWatcherRunnerLines(lines, "build-start:1:") == 1
	})
	waitForDevWatcherRunnerLog(t, secondSPALog, func(lines []string) bool {
		return countDevWatcherRunnerLines(lines, "build-start:1:") == 1
	})
	if lines := readDevWatcherRunnerLog(t, appLog); len(lines) != 0 {
		t.Fatalf("app lifecycle started before buffered SPAs succeeded: %v", lines)
	}

	if err := os.WriteFile(firstRelease, []byte("release"), 0o600); err != nil {
		t.Fatalf("release first buffered SPA build: %v", err)
	}
	waitForDevWatcherRunnerLog(t, firstSPALog, func(lines []string) bool {
		return countDevWatcherRunnerLines(lines, "build-success:1:") == 1
	})
	time.Sleep(100 * time.Millisecond)
	if lines := readDevWatcherRunnerLog(t, appLog); len(lines) != 0 {
		t.Fatalf("first SPA success released app build before second SPA: %v", lines)
	}

	if err := os.WriteFile(secondRelease, []byte("release"), 0o600); err != nil {
		t.Fatalf("release second buffered SPA build: %v", err)
	}
	lines := waitForDevWatcherRunnerLog(t, appLog, func(lines []string) bool {
		return countDevWatcherRunnerLines(lines, "runtime-start:") == 1
	})
	if builds := countDevWatcherRunnerLines(lines, "build-start:"); builds != 1 {
		t.Fatalf("expected one joined app build, got %d: %v", builds, lines)
	}
	if successes := countDevWatcherRunnerLines(lines, "build-success:"); successes != 1 {
		t.Fatalf("expected one successful joined app build, got %d: %v", successes, lines)
	}
	buildSucceeded := indexDevWatcherRunnerLine(lines, "build-success:1:", 1)
	runtimeStarted := indexDevWatcherRunnerLine(lines, "runtime-start:", 1)
	if buildSucceeded < 0 || runtimeStarted < buildSucceeded {
		t.Fatalf("joined app build did not publish runtime in order: %v", lines)
	}
}

// TestDevWatcherControllerSteadySPAsJoinOneAppBuild verifies concurrent frontend changes publish only their joined App edge.
func TestDevWatcherControllerSteadySPAsJoinOneAppBuild(t *testing.T) {
	requireDevWatcherRunnerTestPlatform(t)
	directory := t.TempDir()
	firstSPALog := filepath.Join(directory, "first-spa.log")
	secondSPALog := filepath.Join(directory, "second-spa.log")
	appLog := filepath.Join(directory, "app.log")
	firstRelease := filepath.Join(directory, "release-first-spa")
	secondRelease := filepath.Join(directory, "release-second-spa")
	compiled := []devCompiledWatcher{
		{
			Name: "Build SPA Admin", Kind: devWatcherSPABuild, App: "app",
			Command: devWatcherRunnerHelperCommand("build", map[string]string{
				"GOFORJ_DEV_WATCHER_LOG":     firstSPALog,
				"GOFORJ_DEV_WATCHER_COUNTER": filepath.Join(directory, "first-spa-count"),
				"GOFORJ_DEV_WATCHER_RELEASE": firstRelease,
			}),
			Postpone: true, OnSuccess: []string{"Build App"},
		},
		{
			Name: "Build SPA Portal", Kind: devWatcherSPABuild, App: "app",
			Command: devWatcherRunnerHelperCommand("build", map[string]string{
				"GOFORJ_DEV_WATCHER_LOG":     secondSPALog,
				"GOFORJ_DEV_WATCHER_COUNTER": filepath.Join(directory, "second-spa-count"),
				"GOFORJ_DEV_WATCHER_RELEASE": secondRelease,
			}),
			Postpone: true, OnSuccess: []string{"Build App"},
		},
		{
			Name: "Build App", Kind: devWatcherAppBuild, App: "app",
			Command: devWatcherRunnerHelperCommand("build", map[string]string{
				"GOFORJ_DEV_WATCHER_LOG":     appLog,
				"GOFORJ_DEV_WATCHER_COUNTER": filepath.Join(directory, "app-count"),
			}),
			Postpone: true,
		},
	}
	controller := newDevWatcherRunnerTestController(t, compiled)
	defer controller.stop(time.Second)

	controller.tasks["Build SPA Admin"].request()
	controller.tasks["Build SPA Portal"].request()
	waitForDevWatcherRunnerLog(t, firstSPALog, func(lines []string) bool {
		return countDevWatcherRunnerLines(lines, "build-start:1:") == 1
	})
	waitForDevWatcherRunnerLog(t, secondSPALog, func(lines []string) bool {
		return countDevWatcherRunnerLines(lines, "build-start:1:") == 1
	})
	if err := os.WriteFile(firstRelease, []byte("release"), 0o600); err != nil {
		t.Fatalf("release first steady-state SPA build: %v", err)
	}
	waitForDevWatcherRunnerTaskIdle(t, controller.tasks["Build SPA Admin"], firstSPALog, "build-success:1:")
	time.Sleep(50 * time.Millisecond)
	if lines := readDevWatcherRunnerLog(t, appLog); len(lines) != 0 {
		t.Fatalf("first steady-state SPA released the App build early: %v", lines)
	}

	if err := os.WriteFile(secondRelease, []byte("release"), 0o600); err != nil {
		t.Fatalf("release second steady-state SPA build: %v", err)
	}
	lines := waitForDevWatcherRunnerLog(t, appLog, func(lines []string) bool {
		return countDevWatcherRunnerLines(lines, "build-success:1:") == 1
	})
	if builds := countDevWatcherRunnerLines(lines, "build-start:"); builds != 1 {
		t.Fatalf("joined steady-state SPAs started %d App builds, want 1: %v", builds, lines)
	}
}

// TestDevWatcherControllerSteadySPAFailureSuppressesAppBuild verifies a terminal frontend failure poisons only its active wave.
func TestDevWatcherControllerSteadySPAFailureSuppressesAppBuild(t *testing.T) {
	requireDevWatcherRunnerTestPlatform(t)
	directory := t.TempDir()
	firstSPALog := filepath.Join(directory, "first-spa.log")
	secondSPALog := filepath.Join(directory, "second-spa.log")
	appLog := filepath.Join(directory, "app.log")
	firstRelease := filepath.Join(directory, "release-first-spa")
	compiled := []devCompiledWatcher{
		{
			Name: "Build SPA Admin", Kind: devWatcherSPABuild, App: "app",
			Command: devWatcherRunnerHelperCommand("build", map[string]string{
				"GOFORJ_DEV_WATCHER_LOG":     firstSPALog,
				"GOFORJ_DEV_WATCHER_COUNTER": filepath.Join(directory, "first-spa-count"),
				"GOFORJ_DEV_WATCHER_RELEASE": firstRelease,
			}),
			Postpone: true, OnSuccess: []string{"Build App"},
		},
		{
			Name: "Build SPA Portal", Kind: devWatcherSPABuild, App: "app",
			Command: devWatcherRunnerHelperCommand("build", map[string]string{
				"GOFORJ_DEV_WATCHER_LOG":     secondSPALog,
				"GOFORJ_DEV_WATCHER_COUNTER": filepath.Join(directory, "second-spa-count"),
				"GOFORJ_DEV_WATCHER_FAIL":    "1",
			}),
			Postpone: true, OnSuccess: []string{"Build App"},
		},
		{
			Name: "Build App", Kind: devWatcherAppBuild, App: "app",
			Command: devWatcherRunnerHelperCommand("build", map[string]string{
				"GOFORJ_DEV_WATCHER_LOG":     appLog,
				"GOFORJ_DEV_WATCHER_COUNTER": filepath.Join(directory, "app-count"),
			}),
			Postpone: true,
		},
	}
	controller := newDevWatcherRunnerTestController(t, compiled)
	defer controller.stop(time.Second)

	controller.tasks["Build SPA Admin"].request()
	controller.tasks["Build SPA Portal"].request()
	waitForDevWatcherRunnerLog(t, firstSPALog, func(lines []string) bool {
		return countDevWatcherRunnerLines(lines, "build-start:1:") == 1
	})
	waitForDevWatcherRunnerTaskIdle(t, controller.tasks["Build SPA Portal"], secondSPALog, "build-failed:1:")
	if err := os.WriteFile(firstRelease, []byte("release"), 0o600); err != nil {
		t.Fatalf("release successful steady-state SPA build: %v", err)
	}
	waitForDevWatcherRunnerTaskIdle(t, controller.tasks["Build SPA Admin"], firstSPALog, "build-success:1:")
	time.Sleep(50 * time.Millisecond)
	if lines := readDevWatcherRunnerLog(t, appLog); len(lines) != 0 {
		t.Fatalf("failed steady-state SPA wave published an App build: %v", lines)
	}
}

// TestDevWatcherControllerSteadySPARetrySupersedesFailure verifies a queued newer build can repair a failing frontend wave.
func TestDevWatcherControllerSteadySPARetrySupersedesFailure(t *testing.T) {
	requireDevWatcherRunnerTestPlatform(t)
	directory := t.TempDir()
	spaLog := filepath.Join(directory, "spa.log")
	appLog := filepath.Join(directory, "app.log")
	releasePath := filepath.Join(directory, "release-spa")
	compiled := []devCompiledWatcher{
		{
			Name: "Build SPA Portal", Kind: devWatcherSPABuild, App: "app",
			Command: devWatcherRunnerHelperCommand("build", map[string]string{
				"GOFORJ_DEV_WATCHER_LOG":        spaLog,
				"GOFORJ_DEV_WATCHER_COUNTER":    filepath.Join(directory, "spa-count"),
				"GOFORJ_DEV_WATCHER_RELEASE":    releasePath,
				"GOFORJ_DEV_WATCHER_FAIL_FIRST": "1",
			}),
			Postpone: true, OnSuccess: []string{"Build App"},
		},
		{
			Name: "Build App", Kind: devWatcherAppBuild, App: "app",
			Command: devWatcherRunnerHelperCommand("build", map[string]string{
				"GOFORJ_DEV_WATCHER_LOG":     appLog,
				"GOFORJ_DEV_WATCHER_COUNTER": filepath.Join(directory, "app-count"),
			}),
			Postpone: true,
		},
	}
	controller := newDevWatcherRunnerTestController(t, compiled)
	defer controller.stop(time.Second)
	task := controller.tasks["Build SPA Portal"]

	task.request()
	waitForDevWatcherRunnerLog(t, spaLog, func(lines []string) bool {
		return countDevWatcherRunnerLines(lines, "build-start:1:") == 1
	})
	for range 8 {
		task.request()
	}
	if err := os.WriteFile(releasePath, []byte("release"), 0o600); err != nil {
		t.Fatalf("release failing steady-state SPA build: %v", err)
	}
	waitForDevWatcherRunnerTaskIdle(t, task, spaLog, "build-success:2:")
	lines := waitForDevWatcherRunnerLog(t, appLog, func(lines []string) bool {
		return countDevWatcherRunnerLines(lines, "build-success:1:") == 1
	})
	spaLines := readDevWatcherRunnerLog(t, spaLog)
	if starts := countDevWatcherRunnerLines(spaLines, "build-start:"); starts != 2 {
		t.Fatalf("SPA retry burst started %d builds, want one active and one coalesced: %v", starts, spaLines)
	}
	if builds := countDevWatcherRunnerLines(lines, "build-start:"); builds != 1 {
		t.Fatalf("repaired SPA wave started %d App builds, want 1: %v", builds, lines)
	}
}

// TestDevWatcherControllerFailedReconciliationKeepsGraphGated verifies no downstream event can bypass a failed barrier.
func TestDevWatcherControllerFailedReconciliationKeepsGraphGated(t *testing.T) {
	requireDevWatcherRunnerTestPlatform(t)
	directory := t.TempDir()
	logPath := filepath.Join(directory, "events.log")
	compiled := []devCompiledWatcher{
		{
			Name: "Build App", Kind: devWatcherAppBuild, App: "app",
			Command: devWatcherRunnerHelperCommand("build", map[string]string{
				"GOFORJ_DEV_WATCHER_LOG":     logPath,
				"GOFORJ_DEV_WATCHER_COUNTER": filepath.Join(directory, "build-count"),
			}),
			Postpone: true, OnSuccess: []string{"Run App"},
		},
		{
			Name: "Run App", Kind: devWatcherAppRun, App: "app",
			Command: devWatcherRunnerHelperCommand("runtime", map[string]string{
				"GOFORJ_DEV_WATCHER_LOG": logPath,
			}),
			Restart: true,
		},
	}
	controller, err := newDevWatcherControllerWithOptions(
		compiled,
		nil,
		io.Discard,
		io.Discard,
		false,
		devWatcherControllerOptions{reconcile: true},
	)
	if err != nil {
		t.Fatalf("start reconciling dev watcher controller: %v", err)
	}
	defer controller.stop(time.Second)

	controller.finishReconciliation(false)
	time.Sleep(100 * time.Millisecond)
	if lines := readDevWatcherRunnerLog(t, logPath); countDevWatcherRunnerLines(lines, "runtime-start:") != 0 {
		t.Fatalf("failed reconciliation started structured runtime: %v", lines)
	}
	controller.tasks["Build App"].request()
	time.Sleep(100 * time.Millisecond)
	lines := readDevWatcherRunnerLog(t, logPath)
	if builds := countDevWatcherRunnerLines(lines, "build-start:"); builds != 0 {
		t.Fatalf("failed reconciliation released the build graph: %v", lines)
	}
	if runtimes := countDevWatcherRunnerLines(lines, "runtime-start:"); runtimes != 0 {
		t.Fatalf("failed reconciliation published a runtime: %v", lines)
	}
}

// TestDevWatchEventHasLegacyTrigger verifies compatibility execution remains Create/Write-only.
func TestDevWatchEventHasLegacyTrigger(t *testing.T) {
	tests := []struct {
		name    string
		op      devwatch.Op
		trigger bool
	}{
		{name: "create", op: devwatch.OpCreate, trigger: true},
		{name: "write", op: devwatch.OpWrite, trigger: true},
		{name: "create and rename", op: devwatch.OpCreate | devwatch.OpRename, trigger: true},
		{name: "remove", op: devwatch.OpRemove},
		{name: "rename", op: devwatch.OpRename},
		{name: "remove and rename", op: devwatch.OpRemove | devwatch.OpRename},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := devwatch.Event{Changes: []devwatch.Change{{Op: test.op}}}
			if got := devWatchEventHasLegacyTrigger(event); got != test.trigger {
				t.Fatalf("devWatchEventHasLegacyTrigger() = %t, want %t", got, test.trigger)
			}
		})
	}
}

// TestDevWatchEventContainsProjectEnvFile verifies only root dotenv files delegate to coordinated rebuilds.
func TestDevWatchEventContainsProjectEnvFile(t *testing.T) {
	projectRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve package working directory: %v", err)
	}
	tests := []struct {
		name     string
		path     string
		relative string
		contains bool
	}{
		{name: "root env", path: filepath.Join(projectRoot, ".env"), relative: ".env", contains: true},
		{name: "root named env", path: filepath.Join(projectRoot, ".env.local"), relative: ".env.local", contains: true},
		{name: "nested env", path: filepath.Join(projectRoot, "internal", ".env"), relative: "internal/.env"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := devwatch.Event{Changes: []devwatch.Change{{Path: test.path, RelativePath: test.relative}}}
			if got := devWatchEventContainsProjectEnvFile(event); got != test.contains {
				t.Fatalf("devWatchEventContainsProjectEnvFile() = %t, want %t", got, test.contains)
			}
		})
	}
}

// TestDevWatcherControllerBusyBuildCoalescesFollowUp verifies build bursts queue one rerun without canceling active work.
func TestDevWatcherControllerBusyBuildCoalescesFollowUp(t *testing.T) {
	requireDevWatcherRunnerTestPlatform(t)
	directory := t.TempDir()
	logPath := filepath.Join(directory, "events.log")
	releasePath := filepath.Join(directory, "release")
	buildCommand := devWatcherRunnerHelperCommand("build", map[string]string{
		"GOFORJ_DEV_WATCHER_LOG":     logPath,
		"GOFORJ_DEV_WATCHER_COUNTER": filepath.Join(directory, "build-count"),
		"GOFORJ_DEV_WATCHER_RELEASE": releasePath,
	})
	compiled := []devCompiledWatcher{{
		Name: "Build App", Kind: devWatcherAppBuild, App: "app", Command: buildCommand,
		Postpone: true, Restart: true,
	}}
	controller := newDevWatcherRunnerTestController(t, compiled)
	defer controller.stop(time.Second)
	task := controller.tasks["Build App"]

	task.request()
	waitForDevWatcherRunnerLog(t, logPath, func(lines []string) bool {
		return countDevWatcherRunnerLines(lines, "build-start:1:") == 1
	})
	for range 8 {
		task.request()
	}
	if err := os.WriteFile(releasePath, []byte("release"), 0o600); err != nil {
		t.Fatalf("release busy build: %v", err)
	}
	waitForDevWatcherRunnerTaskIdle(t, task, logPath, "build-success:2:")
	lines := readDevWatcherRunnerLog(t, logPath)
	if starts := countDevWatcherRunnerLines(lines, "build-start:"); starts != 2 {
		t.Fatalf("expected active build plus one coalesced follow-up, got %d: %v", starts, lines)
	}
	if finishes := countDevWatcherRunnerLines(lines, "build-success:"); finishes != 2 {
		t.Fatalf("expected both builds to finish without cancellation, got %d: %v", finishes, lines)
	}
}

// TestDevWatcherControllerShutdownKillsRuntimeTree verifies controller shutdown reaches runtime descendants.
func TestDevWatcherControllerShutdownKillsRuntimeTree(t *testing.T) {
	requireDevWatcherRunnerTestPlatform(t)
	directory := t.TempDir()
	readyPath := filepath.Join(directory, "tree-ready")
	markerPath := filepath.Join(directory, "descendant-survived")
	runtimeCommand := devWatcherRunnerHelperCommand("runtime-tree", map[string]string{
		"GOFORJ_DEV_WATCHER_READY":  readyPath,
		"GOFORJ_DEV_WATCHER_MARKER": markerPath,
	})
	compiled := []devCompiledWatcher{{
		Name: "Run App", Kind: devWatcherAppRun, App: "app", Command: runtimeCommand, Restart: true,
	}}
	controller := newDevWatcherRunnerTestController(t, compiled)
	t.Cleanup(func() { controller.stop(time.Second) })
	waitForDevWatcherRunnerFile(t, readyPath)
	controller.stop(75 * time.Millisecond)

	select {
	case exit := <-controller.exitCh:
		if exit.name != "Run App" {
			t.Fatalf("unexpected controller exit %+v", exit)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for controller shutdown exit")
	}
	time.Sleep(400 * time.Millisecond)
	if _, err := os.Stat(markerPath); err == nil {
		t.Fatal("runtime descendant survived controller shutdown")
	} else if !os.IsNotExist(err) {
		t.Fatalf("inspect descendant marker: %v", err)
	}
}

// newDevWatcherRunnerTestController starts a controller without physical watch subscriptions.
func newDevWatcherRunnerTestController(t *testing.T, compiled []devCompiledWatcher) *devWatcherController {
	t.Helper()
	controller, err := newDevWatcherController(compiled, nil, io.Discard, io.Discard, false)
	if err != nil {
		t.Fatalf("start dev watcher controller: %v", err)
	}
	return controller
}

// devWatcherRunnerHelperCommand builds the shell form accepted by the compiled runner.
func devWatcherRunnerHelperCommand(action string, environment map[string]string) devwatch.Command {
	commandEnvironment := map[string]string{"GOFORJ_DEV_WATCHER_RUNNER_HELPER": "1"}
	for key, value := range environment {
		commandEnvironment[key] = value
	}
	command := devWatcherRunnerShellQuote(os.Args[0]) + " -test.run='^TestDevWatcherRunnerHelper$' -- " + action
	return devwatch.Command{Shell: command, Env: commandEnvironment}
}

// devWatcherRunnerShellQuote protects a test-binary path passed through the POSIX compatibility shell.
func devWatcherRunnerShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// devWatcherRunnerHelperAction finds the helper action after the test flag separator.
func devWatcherRunnerHelperAction(args []string) string {
	for index := len(args) - 1; index >= 0; index-- {
		if args[index] == "--" && index+1 < len(args) {
			return args[index+1]
		}
	}
	return ""
}

// runDevWatcherRunnerBuildHelper records one build and optionally blocks or fails it.
func runDevWatcherRunnerBuildHelper() {
	index, err := incrementDevWatcherRunnerCounter(os.Getenv("GOFORJ_DEV_WATCHER_COUNTER"))
	if err != nil {
		os.Exit(3)
	}
	appendDevWatcherRunnerLog(fmt.Sprintf("build-start:%d:%d", index, os.Getpid()))
	if releasePath := os.Getenv("GOFORJ_DEV_WATCHER_RELEASE"); releasePath != "" && index == 1 {
		for {
			if _, err := os.Stat(releasePath); err == nil {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	if os.Getenv("GOFORJ_DEV_WATCHER_FAIL") == "1" ||
		os.Getenv("GOFORJ_DEV_WATCHER_FAIL_FIRST") == "1" && index == 1 {
		appendDevWatcherRunnerLog(fmt.Sprintf("build-failed:%d:%d", index, os.Getpid()))
		os.Exit(17)
	}
	if binary := os.Getenv("GOFORJ_DEV_WATCHER_BINARY"); binary != "" {
		if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
			os.Exit(18)
		}
		if err := os.WriteFile(binary, []byte("built"), 0o755); err != nil {
			os.Exit(19)
		}
	}
	appendDevWatcherRunnerLog(fmt.Sprintf("build-success:%d:%d", index, os.Getpid()))
}

// runDevWatcherRunnerRuntimeHelper records runtime start and graceful termination.
func runDevWatcherRunnerRuntimeHelper() {
	appendDevWatcherRunnerLog(fmt.Sprintf("runtime-start:%d", os.Getpid()))
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	<-signals
	appendDevWatcherRunnerLog(fmt.Sprintf("runtime-stop:%d", os.Getpid()))
}

// runDevWatcherRunnerTreeHelper starts a descendant and ignores TERM so shutdown must escalate.
func runDevWatcherRunnerTreeHelper() {
	signal.Ignore(syscall.SIGTERM)
	child := exec.Command(os.Args[0], "-test.run=^TestDevWatcherRunnerHelper$", "--", "delayed-marker")
	child.Env = os.Environ()
	if err := child.Start(); err != nil {
		os.Exit(4)
	}
	if err := os.WriteFile(os.Getenv("GOFORJ_DEV_WATCHER_READY"), []byte("ready"), 0o600); err != nil {
		os.Exit(5)
	}
	time.Sleep(time.Hour)
}

// runDevWatcherRunnerDelayedMarkerHelper writes only when a descendant survives shutdown escalation.
func runDevWatcherRunnerDelayedMarkerHelper() {
	signal.Ignore(syscall.SIGTERM)
	time.Sleep(350 * time.Millisecond)
	if err := os.WriteFile(os.Getenv("GOFORJ_DEV_WATCHER_MARKER"), []byte("survived"), 0o600); err != nil {
		os.Exit(6)
	}
}

// incrementDevWatcherRunnerCounter advances the sequential build invocation counter.
func incrementDevWatcherRunnerCounter(path string) (int, error) {
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

// appendDevWatcherRunnerLog appends one atomic lifecycle record from a helper process.
func appendDevWatcherRunnerLog(line string) {
	file, err := os.OpenFile(os.Getenv("GOFORJ_DEV_WATCHER_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		os.Exit(7)
	}
	if _, err := fmt.Fprintln(file, line); err != nil {
		_ = file.Close()
		os.Exit(8)
	}
	if err := file.Close(); err != nil {
		os.Exit(9)
	}
}

// waitForDevWatcherRunnerLog waits until lifecycle records satisfy the supplied condition.
func waitForDevWatcherRunnerLog(t *testing.T, path string, condition func([]string) bool) []string {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		lines := readDevWatcherRunnerLog(t, path)
		if condition(lines) {
			return lines
		}
		time.Sleep(10 * time.Millisecond)
	}
	lines := readDevWatcherRunnerLog(t, path)
	t.Fatalf("timed out waiting for runner log condition: %v", lines)
	return nil
}

// waitForDevWatcherRunnerCondition polls controller state without coupling tests to scheduler timing.
func waitForDevWatcherRunnerCondition(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

// waitForDevWatcherRunnerTaskIdle waits until a command has completed runner bookkeeping.
func waitForDevWatcherRunnerTaskIdle(t *testing.T, task *devWatcherTask, logPath string, expectedLine string) {
	t.Helper()
	waitForDevWatcherRunnerLog(t, logPath, func(lines []string) bool {
		if countDevWatcherRunnerLines(lines, expectedLine) == 0 {
			return false
		}
		task.mu.Lock()
		defer task.mu.Unlock()
		return task.activeCancel == nil && !task.busy && len(task.triggerCh) == 0
	})
}

// readDevWatcherRunnerLog reads complete lifecycle records observed so far.
func readDevWatcherRunnerLog(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read runner log: %v", err)
	}
	rawLines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(rawLines) == 1 && rawLines[0] == "" {
		return nil
	}
	return rawLines
}

// countDevWatcherRunnerLines counts lifecycle records with a given prefix.
func countDevWatcherRunnerLines(lines []string, prefix string) int {
	count := 0
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			count++
		}
	}
	return count
}

// indexDevWatcherRunnerLine returns the index of the requested matching occurrence.
func indexDevWatcherRunnerLine(lines []string, prefix string, occurrence int) int {
	seen := 0
	for index, line := range lines {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		seen++
		if seen == occurrence {
			return index
		}
	}
	return -1
}

// waitForDevWatcherRunnerFile waits for a helper's readiness marker.
func waitForDevWatcherRunnerFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

// requireDevWatcherRunnerTestPlatform skips shell integration where the production wrapper is not POSIX-shaped.
func requireDevWatcherRunnerTestPlatform(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("native watcher command wrapper currently uses POSIX shell syntax")
	}
}
