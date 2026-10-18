package forj

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/goforj/goforj/internal/build"
	"github.com/goforj/goforj/internal/devwatch"
	"github.com/goforj/goforj/project"
)

type devWatcherController struct {
	ctx          context.Context
	cancel       context.CancelFunc
	supervisor   *devwatch.Supervisor
	tasks        map[string]*devWatcherTask
	order        []string
	engines      []*devwatch.Engine
	exitCh       chan watcherExit
	outWriter    io.Writer
	errWriter    io.Writer
	projectBuild *devWatcherBuildGate
	startup      *devWatcherStartupTracker

	mu                sync.Mutex
	exited            map[string]bool
	reconcileSPAs     map[string]map[string]bool
	reconcileAppBuild map[string]string
	steadySPAWaves    map[string]*devWatcherSteadySPAWave
	stopping          bool
	stopOnce          sync.Once
	stopDone          chan struct{}
	wait              sync.WaitGroup
}

// devWatcherBuildPhase classifies project access so compatible build work can remain concurrent.
type devWatcherBuildPhase uint8

const (
	devWatcherBuildPhaseNone devWatcherBuildPhase = iota
	devWatcherBuildPhasePrepare
	devWatcherBuildPhaseApp
	devWatcherBuildPhaseSPA
)

// devWatcherBuildRequest retains queue identity when several tasks wait for the same compatible phase.
type devWatcherBuildRequest struct {
	phase devWatcherBuildPhase
}

// devWatcherBuildGate allows concurrency within read-only App or independent SPA phases while preparation remains exclusive.
type devWatcherBuildGate struct {
	mu          sync.Mutex
	cond        *sync.Cond
	active      devWatcherBuildPhase
	activeCount int
	waiting     []*devWatcherBuildRequest
}

// devWatcherSteadySPAWave joins frontend work that must precede one conventional App build.
type devWatcherSteadySPAWave struct {
	pending   map[string]bool
	buildName string
	failed    map[string]bool
}

type devWatcherSynchronizedWriter struct {
	mu     *sync.Mutex
	writer io.Writer
}

// Write serializes transcript output shared by concurrent watcher and diagnostic goroutines.
func (w devWatcherSynchronizedWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Write(data)
}

type devWatcherTask struct {
	controller *devWatcherController
	spec       devCompiledWatcher
	triggerCh  chan struct{}
	outputTail *devTaskOutputTail

	runtimeMu           sync.Mutex
	mu                  sync.Mutex
	activeCancel        context.CancelFunc
	runtimeLive         bool
	runtimePID          int
	paused              bool
	pending             bool
	busy                bool
	startAfterReconcile bool
	startupTracked      bool
	startupOnce         sync.Once
}

type devWatcherStartupTracker struct {
	mu       sync.Mutex
	pending  map[string]*devWatcherStartupTask
	done     chan struct{}
	doneOnce sync.Once
	err      error
}

type devWatcherStartupTask struct {
	triggered bool
	finished  bool
}

type devWatcherStartupExpectation struct {
	ID          string
	WaitOutcome bool
}

type devWatcherStartupError struct {
	Watcher string
	Command string
	Err     error
}

// Error renders the watcher and command boundary before the underlying process failure.
func (e *devWatcherStartupError) Error() string {
	if e == nil {
		return ""
	}
	parts := []string{strings.TrimSpace(e.Watcher)}
	if command := strings.TrimSpace(e.Command); command != "" {
		parts = append(parts, "  "+command)
	}
	if e.Err != nil {
		parts = append(parts, "  "+e.Err.Error())
	}
	return strings.Join(parts, "\n")
}

// Unwrap exposes the process error to callers that need structured inspection.
func (e *devWatcherStartupError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// newDevWatcherStartupTracker waits for explicit command outcomes and wrapper trigger markers from the initial generation.
func newDevWatcherStartupTracker(expectations []devWatcherStartupExpectation) *devWatcherStartupTracker {
	tracker := &devWatcherStartupTracker{
		pending: make(map[string]*devWatcherStartupTask, len(expectations)),
		done:    make(chan struct{}),
	}
	for _, expectation := range expectations {
		tracker.pending[expectation.ID] = &devWatcherStartupTask{finished: !expectation.WaitOutcome}
	}
	if len(tracker.pending) == 0 {
		tracker.doneOnce.Do(func() { close(tracker.done) })
	}
	return tracker
}

// noteTrigger records that the child wrapper reached process execution rather than inferring readiness from its output text.
func (t *devWatcherStartupTracker) noteTrigger(id string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	if task := t.pending[id]; task != nil {
		task.triggered = true
	}
	t.completeLocked()
	t.mu.Unlock()
}

// noteOutcome records the first command result while retaining the earliest failure for the transaction summary.
func (t *devWatcherStartupTracker) noteOutcome(id string, err error) {
	if t == nil {
		return
	}
	t.mu.Lock()
	if task := t.pending[id]; task != nil {
		task.finished = true
		if err != nil {
			// A command can fail before its wrapper emits a launch marker; the failure itself is terminal evidence.
			task.triggered = true
			if t.err == nil {
				t.err = err
			}
		}
	}
	t.completeLocked()
	t.mu.Unlock()
}

// completeLocked closes readiness exactly once after every expected task both launched and settled.
func (t *devWatcherStartupTracker) completeLocked() {
	for _, task := range t.pending {
		if !task.triggered || !task.finished {
			return
		}
	}
	t.doneOnce.Do(func() { close(t.done) })
}

// wait blocks until the generation is explicit about readiness or the surrounding dev session stops.
func (t *devWatcherStartupTracker) wait(ctx context.Context) error {
	if t == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.done:
		t.mu.Lock()
		defer t.mu.Unlock()
		return t.err
	}
}

// devWatcherControllerOptions controls transition-only behavior without changing steady-state watcher specs.
type devWatcherControllerOptions struct {
	reconcile bool
}

// devWatcherRuntime keeps handles, controller, and exit stream together so restart paths cannot drain a different generation.
type devWatcherRuntime struct {
	session    *devWatchSession
	watchers   []runningWatcher
	controller *devWatcherController
	exitCh     <-chan watcherExit
}

// startDevWatcherRuntime derives startup controls from the session so watcher policy has one source of truth.
func startDevWatcherRuntime(session *devWatchSession) (*devWatcherRuntime, error) {
	watcherRuntime := &devWatcherRuntime{session: session}
	compiled, err := compileDevWatchers(session.config)
	if err != nil {
		return nil, err
	}
	compiled = isolateDevRuntimeEnvironments(compiled, session.inheritedEnv)
	controller, err := newDevWatcherControllerWithOptions(
		compiled,
		session.streamer,
		session.outWriter,
		session.errWriter,
		session.config.Dev.SoundOnWatchError,
		devWatcherControllerOptions{reconcile: session.reconcile},
	)
	if err != nil {
		return nil, err
	}
	watcherRuntime.controller = controller
	watcherRuntime.exitCh = controller.exitCh
	watcherRuntime.watchers = make([]runningWatcher, 0, len(compiled))
	names := make([]string, 0, len(compiled))
	for _, watcher := range compiled {
		watcherRuntime.watchers = append(watcherRuntime.watchers, runningWatcher{id: watcher.ID, name: watcher.Name})
		names = append(names, watcher.Name)
	}
	emitWatcherLifecycleSummary(controller.outWriter, session.streamer, names, watcherStateStarted)
	return watcherRuntime, nil
}

// isolateDevRuntimeEnvironments starts App runtimes from the original launcher environment so the
// framework's own dotenv state cannot become inherited configuration for a nested application.
func isolateDevRuntimeEnvironments(compiled []devCompiledWatcher, inherited processEnvironment) []devCompiledWatcher {
	isolated := make([]devCompiledWatcher, len(compiled))
	copy(isolated, compiled)
	for index := range isolated {
		if isolated[index].Legacy || isolated[index].Kind != devWatcherAppRun || isolated[index].FullProcessOverride {
			continue
		}
		isolated[index].Command.Env = mergeDevRuntimeEnvironment(
			inherited,
			isolated[index].Command.Env,
			runtime.GOOS == "windows",
		)
		isolated[index].Command.ReplaceEnv = true
	}
	return isolated
}

// mergeDevRuntimeEnvironment applies explicit App settings after the launcher snapshot while respecting
// Windows' case-insensitive environment namespace.
func mergeDevRuntimeEnvironment(inherited processEnvironment, overrides map[string]string, caseInsensitive bool) map[string]string {
	type value struct {
		key   string
		value string
	}
	values := make(map[string]value, len(inherited)+len(overrides))
	normalize := func(key string) string {
		if caseInsensitive {
			return strings.ToUpper(key)
		}
		return key
	}
	merge := func(source map[string]string) {
		keys := make([]string, 0, len(source))
		for key := range source {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			values[normalize(key)] = value{key: key, value: source[key]}
		}
	}
	merge(inherited)
	merge(overrides)

	environment := make(map[string]string, len(values))
	for _, entry := range values {
		environment[entry.key] = entry.value
	}
	return environment
}

// newDevWatcherController establishes physical coverage before starting any configured command.
func newDevWatcherController(
	compiled []devCompiledWatcher,
	streamer *devwatchStreamer,
	outWriter io.Writer,
	errWriter io.Writer,
	soundOnError bool,
) (*devWatcherController, error) {
	return newDevWatcherControllerWithOptions(
		compiled,
		streamer,
		outWriter,
		errWriter,
		soundOnError,
		devWatcherControllerOptions{},
	)
}

// newDevWatcherControllerWithOptions gates replacement runtimes until their new build graph is reconciled.
func newDevWatcherControllerWithOptions(
	compiled []devCompiledWatcher,
	streamer *devwatchStreamer,
	outWriter io.Writer,
	errWriter io.Writer,
	soundOnError bool,
	options devWatcherControllerOptions,
) (*devWatcherController, error) {
	var err error
	compiled, err = normalizeCompiledDevWatchers(compiled)
	if err != nil {
		return nil, err
	}
	if err := validateCompiledDevWatchers(compiled); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	retainFailureOutput := shouldRetainDevWatcherFailureOutput(outWriter)
	attachPTY := shouldAttachDevWatcherPTY(outWriter, runtime.GOOS)
	outputMu := &sync.Mutex{}
	outWriter = devWatcherSynchronizedWriter{mu: outputMu, writer: outWriter}
	errWriter = devWatcherSynchronizedWriter{mu: outputMu, writer: errWriter}
	controller := &devWatcherController{
		ctx: ctx, cancel: cancel, supervisor: devwatch.NewSupervisor(devwatch.SupervisorOptions{}),
		tasks: make(map[string]*devWatcherTask, len(compiled)), exitCh: make(chan watcherExit, len(compiled)),
		outWriter: outWriter, errWriter: errWriter, exited: make(map[string]bool, len(compiled)),
		projectBuild: newDevWatcherBuildGate(), steadySPAWaves: make(map[string]*devWatcherSteadySPAWave),
		stopDone: make(chan struct{}),
	}
	runtimeApps := compiledDevRuntimeApps(compiled)
	showAppColumn := len(runtimeApps) > 1
	appNameWidth := devAppColumnWidth(runtimeApps)
	immediate := 0
	startupExpectations := make([]devWatcherStartupExpectation, 0)
	runtimeNames := make([]string, 0)
	for _, spec := range compiled {
		gatedRuntime := options.reconcile && !spec.Legacy && spec.Kind == devWatcherAppRun
		if !spec.Postpone && !gatedRuntime {
			immediate++
			startupExpectations = append(startupExpectations, devWatcherStartupExpectation{
				ID: spec.ID, WaitOutcome: spec.Kind != devWatcherCustom,
			})
		} else if gatedRuntime && !spec.Postpone {
			startupExpectations = append(startupExpectations, devWatcherStartupExpectation{ID: spec.ID, WaitOutcome: true})
		}
		if spec.Kind == devWatcherAppRun {
			runtimeNames = append(runtimeNames, spec.Name)
		}
	}
	controller.startup = newDevWatcherStartupTracker(startupExpectations)
	lifecycle := newDevwatchLifecycleState(immediate, runtimeNames)
	lifecycle.separators = asDevOutputController(outWriter) == nil || devLifecycleDetailedOutput(compiled)
	for _, spec := range compiled {
		structuredBuild := !spec.Legacy && (spec.Kind == devWatcherAppBuild || spec.Kind == devWatcherSPABuild)
		structuredRuntime := !spec.Legacy && spec.Kind == devWatcherAppRun
		startAfterReconcile := structuredRuntime && !spec.Postpone
		if options.reconcile && structuredRuntime {
			spec.Postpone = true
		}
		var outputTail *devTaskOutputTail
		if retainFailureOutput {
			outputTail = newDevTaskOutputTail(40)
		}
		configureCompiledDevCommand(&spec, streamer, outWriter, errWriter, outputTail, soundOnError, lifecycle, controller.startup, appNameWidth, showAppColumn, attachPTY)
		controller.tasks[spec.ID] = &devWatcherTask{
			controller: controller, spec: spec, triggerCh: make(chan struct{}, 1),
			outputTail: outputTail, paused: options.reconcile && (structuredBuild || structuredRuntime),
			startAfterReconcile: startAfterReconcile, startupTracked: controller.startup.pending[spec.ID] != nil,
		}
		controller.order = append(controller.order, spec.ID)
	}
	engines, err := controller.startEngines(compiled)
	if err != nil {
		cancel()
		controller.supervisor.Close()
		return nil, err
	}
	controller.engines = engines
	controller.startWorkers()
	return controller, nil
}

// configureCompiledDevCommand attaches existing transcript formatting and error alerts to native children.
func configureCompiledDevCommand(
	spec *devCompiledWatcher,
	streamer *devwatchStreamer,
	outWriter io.Writer,
	errWriter io.Writer,
	outputTail *devTaskOutputTail,
	soundOnError bool,
	lifecycle *devwatchLifecycleState,
	startup *devWatcherStartupTracker,
	appNameWidth int,
	showAppColumn bool,
	attachPTY bool,
) {
	triggerCommand := strings.Join(strings.Fields(spec.Command.Shell), " ")
	spec.DisplayCommand = triggerCommand
	appName := spec.App
	orchestrationOut := outWriter
	orchestrationErr := errWriter
	if outputTail != nil {
		outWriter = io.MultiWriter(outWriter, outputTail)
		errWriter = io.MultiWriter(errWriter, outputTail)
	}
	var onTrigger func()
	if startup != nil {
		onTrigger = func() { startup.noteTrigger(spec.ID) }
	}
	stdout := setDevwatchOrchestrationOutput(
		newDevwatchWriterForApp(outWriter, streamer, "stdout", spec.Name, appName, appNameWidth, showAppColumn, lifecycle, onTrigger),
		orchestrationOut,
	)
	stderr := setDevwatchOrchestrationOutput(
		newDevwatchWriterForApp(errWriter, streamer, "stderr", spec.Name, appName, appNameWidth, showAppColumn, lifecycle, onTrigger),
		orchestrationErr,
	)
	if spec.Kind == devWatcherAppRun && !spec.Legacy && !spec.FullProcessOverride {
		spec.NativeRuntimeCommand = spec.Command.Shell
	}
	var wrappedCommand string
	if spec.Kind == devWatcherAppRun && !spec.Legacy && spec.FullProcessOverride {
		wrappedCommand = buildFullProcessRuntimeExec(spec.Command.Shell)
	} else if spec.Kind == devWatcherAppRun && spec.NativeRuntimeCommand != "" {
		wrappedCommand = buildNativeRuntimeExec(spec.Command.Shell)
	} else {
		wrappedCommand = buildWatcherExec(spec.Command.Shell)
	}
	// GoForj dev commands have always used the project's Bash contract, and the lifecycle wrappers rely on it on every OS.
	spec.Command.Args = []string{"bash", "-c", wrappedCommand}
	spec.Command.Shell = ""
	spec.Command.Stdout = stdout
	spec.Command.Stderr = stderr
	if attachPTY {
		spec.Command.PTY = true
		// A pseudo-terminal combines both streams; one transcript sink prevents every line from appearing twice.
		spec.Command.Stderr = nil
	}
	if soundOnError {
		hook := errorSoundHook(true)
		spec.Command.OnOutput = func(output devwatch.Output) {
			hook(string(output.Data))
		}
	}
}

// shouldAttachDevWatcherPTY keeps nested terminal renderers away from the alternate-screen TUI while preserving plain-session compatibility.
func shouldAttachDevWatcherPTY(outWriter io.Writer, goos string) bool {
	if asDevOutputController(outWriter) != nil {
		return false
	}
	return goos == "linux" || goos == "darwin"
}

// shouldRetainDevWatcherFailureOutput limits replay to alternate-screen sessions where live diagnostics disappear on exit.
func shouldRetainDevWatcherFailureOutput(outWriter io.Writer) bool {
	_, ok := outWriter.(*devBubbleWriter)
	return ok
}

// compiledDevRuntimeApps returns distinct app labels in watcher order.
func compiledDevRuntimeApps(compiled []devCompiledWatcher) []string {
	apps := make([]string, 0)
	seen := make(map[string]bool)
	for _, watcher := range compiled {
		if watcher.Kind != devWatcherAppRun || watcher.App == "" || seen[watcher.App] {
			continue
		}
		seen[watcher.App] = true
		apps = append(apps, watcher.App)
	}
	return apps
}

// startEngines groups notification watches together and preserves each configured polling interval.
func (c *devWatcherController) startEngines(compiled []devCompiledWatcher) ([]*devwatch.Engine, error) {
	groups := make(map[time.Duration][]devwatch.Spec)
	order := make([]time.Duration, 0)
	for _, watcher := range compiled {
		if !watcher.WatchChanges {
			continue
		}
		if _, exists := groups[watcher.PollInterval]; !exists {
			order = append(order, watcher.PollInterval)
		}
		groups[watcher.PollInterval] = append(groups[watcher.PollInterval], watcher.Watch)
	}
	engines := make([]*devwatch.Engine, 0, len(groups))
	for _, pollInterval := range order {
		backend := devwatch.BackendAuto
		if pollInterval > 0 {
			backend = devwatch.BackendPoll
		}
		engine, err := devwatch.NewEngine(devwatch.EngineConfig{
			Watchers: groups[pollInterval], Backend: backend, PollInterval: pollInterval,
		})
		if err != nil {
			closeDevWatchEngines(engines)
			return nil, err
		}
		if err := engine.Start(c.ctx); err != nil {
			closeDevWatchEngines(engines)
			return nil, err
		}
		engines = append(engines, engine)
	}
	return engines, nil
}

// closeDevWatchEngines releases a partially started collection after startup failure.
func closeDevWatchEngines(engines []*devwatch.Engine) {
	for _, engine := range engines {
		_ = engine.Close()
	}
}

// startWorkers connects shared filesystem events, watcher health, and process exits to task workers.
func (c *devWatcherController) startWorkers() {
	for _, name := range c.order {
		task := c.tasks[name]
		c.wait.Add(1)
		go task.run()
	}
	for _, engine := range c.engines {
		c.wait.Add(1)
		go c.forwardEngineEvents(engine)
		c.wait.Add(1)
		go c.forwardEngineErrors(engine)
	}
	c.wait.Add(1)
	go c.forwardProcessExits()
	for _, name := range c.order {
		task := c.tasks[name]
		if !task.spec.Postpone {
			task.request()
		}
	}
}

// forwardEngineEvents routes a logical debounce batch to its named task.
func (c *devWatcherController) forwardEngineEvents(engine *devwatch.Engine) {
	defer c.wait.Done()
	for {
		select {
		case <-c.ctx.Done():
			return
		case event, ok := <-engine.Events():
			if !ok {
				return
			}
			if task := c.tasks[event.Watcher]; task != nil {
				if task.spec.Legacy && !devWatchEventHasLegacyTrigger(event) {
					continue
				}
				if task.spec.Verbose {
					writeLegacyWatchEvent(c.errWriter, task.spec, event)
				}
				if task.spec.Kind == devWatcherAppBuild && !task.spec.Legacy && devWatchEventContainsProjectEnvFile(event) {
					// The outer dev loop reloads process environment before rebuilding all apps.
					continue
				}
				task.request()
			}
		}
	}
}

// devWatchEventHasLegacyTrigger preserves wgo's Create/Write-only execution behavior.
func devWatchEventHasLegacyTrigger(event devwatch.Event) bool {
	for _, change := range event.Changes {
		if change.Op&(devwatch.OpCreate|devwatch.OpWrite) != 0 {
			return true
		}
	}
	return false
}

// writeLegacyWatchEvent preserves wgo's opt-in file-event diagnostics.
func writeLegacyWatchEvent(out io.Writer, spec devCompiledWatcher, event devwatch.Event) {
	for _, change := range event.Changes {
		_, _ = fmt.Fprintf(out, "%s%s %s\n", legacyWatchLogPrefix(spec), devWatchOperationLabel(change.Op), change.RelativePath)
	}
}

// devWatchOperationLabel renders coalesced native operations in wgo-style uppercase form.
func devWatchOperationLabel(operation devwatch.Op) string {
	labels := make([]string, 0, 4)
	if operation&devwatch.OpCreate != 0 {
		labels = append(labels, "CREATE")
	}
	if operation&devwatch.OpWrite != 0 {
		labels = append(labels, "WRITE")
	}
	if operation&devwatch.OpRemove != 0 {
		labels = append(labels, "REMOVE")
	}
	if operation&devwatch.OpRename != 0 {
		labels = append(labels, "RENAME")
	}
	return strings.Join(labels, "|")
}

// legacyWatchLogPrefix resolves the compatibility logger prefix exactly once per emitted line.
func legacyWatchLogPrefix(spec devCompiledWatcher) string {
	if spec.LogPrefixSet {
		return spec.LogPrefix
	}
	return "[wgo] "
}

// devWatchEventContainsProjectEnvFile delegates batches the coordinated root-env rebuild already subsumes.
func devWatchEventContainsProjectEnvFile(event devwatch.Event) bool {
	projectRoot, err := filepath.Abs(".")
	if err != nil {
		return false
	}
	for _, change := range event.Changes {
		name := filepath.Base(change.RelativePath)
		if name != ".env" && !strings.HasPrefix(name, ".env.") {
			continue
		}
		absolutePath, err := filepath.Abs(change.Path)
		if err == nil && filepath.Clean(filepath.Dir(absolutePath)) == filepath.Clean(projectRoot) {
			return true
		}
	}
	return false
}

// forwardEngineErrors keeps incomplete coverage visible instead of silently dropping fsnotify failures.
func (c *devWatcherController) forwardEngineErrors(engine *devwatch.Engine) {
	defer c.wait.Done()
	for {
		select {
		case <-c.ctx.Done():
			return
		case err, ok := <-engine.Errors():
			if !ok {
				return
			}
			if err != nil {
				_, _ = fmt.Fprintf(c.errWriter, "forj dev watcher coverage degraded: %v\n", err)
			}
		}
	}
}

// forwardProcessExits surfaces unexpected app runtime exits while ignoring coordinated restarts and shutdowns.
func (c *devWatcherController) forwardProcessExits() {
	defer c.wait.Done()
	for {
		select {
		case <-c.ctx.Done():
			return
		case exit := <-c.supervisor.Exits():
			c.handleProcessExit(exit)
		}
	}
}

// handleProcessExit ignores delayed completions from runtimes already superseded by a newer process.
func (c *devWatcherController) handleProcessExit(exit devwatch.Exit) {
	task := c.tasks[exit.Name]
	if task == nil || task.spec.Kind != devWatcherAppRun || exit.Intentional() {
		return
	}
	if !task.markRuntimeStopped(exit.PID) {
		return
	}
	exitErr := exit.Err
	if exitErr == nil {
		exitErr = fmt.Errorf("runtime %q exited unexpectedly with code %d", task.spec.Name, exit.ExitCode)
	}
	if !task.spec.Legacy {
		_, _ = fmt.Fprintf(c.errWriter, "forj dev: %s exited; waiting for the next successful build: %v\n", task.spec.Name, exitErr)
		return
	}
	c.publishExit(task.spec.ID, task.spec.Name, &exit, exitErr)
}

// request coalesces duplicate events and interrupts only restart-style custom commands.
func (t *devWatcherTask) request() {
	structuredLifecycle := !t.spec.Legacy &&
		(t.spec.Kind == devWatcherAppBuild || t.spec.Kind == devWatcherSPABuild || t.spec.Kind == devWatcherAppRun)
	if structuredLifecycle {
		t.controller.mu.Lock()
		defer t.controller.mu.Unlock()
	}
	t.mu.Lock()
	if t.paused {
		t.pending = true
		t.mu.Unlock()
		return
	}
	if t.spec.Kind == devWatcherSPABuild && !t.spec.Legacy {
		t.controller.registerStructuredSPARequestLocked(t)
	}
	activeCancel := t.activeCancel
	restartActive := activeCancel != nil && t.spec.Restart && t.spec.Kind == devWatcherCustom
	t.mu.Unlock()
	if restartActive {
		activeCancel()
	}
	t.enqueue()
}

// registerStructuredSPARequestLocked joins a request to reconciliation or the current steady-state App wave.
func (c *devWatcherController) registerStructuredSPARequestLocked(task *devWatcherTask) {
	if remaining, active := c.reconcileSPAs[task.spec.App]; active {
		remaining[task.spec.ID] = true
		return
	}
	buildName := c.structuredSPAAppBuildName(task.spec)
	if buildName == "" {
		return
	}
	wave := c.steadySPAWaves[task.spec.App]
	if wave == nil {
		wave = &devWatcherSteadySPAWave{
			pending: make(map[string]bool), failed: make(map[string]bool), buildName: buildName,
		}
		c.steadySPAWaves[task.spec.App] = wave
	}
	wave.pending[task.spec.ID] = true
	delete(wave.failed, task.spec.ID)
}

// structuredSPAAppBuildName finds the conventional App build edge shared by an App's frontend tasks.
func (c *devWatcherController) structuredSPAAppBuildName(spec devCompiledWatcher) string {
	for _, successor := range spec.OnSuccess {
		next := c.tasks[successor]
		if next != nil && next.spec.Kind == devWatcherAppBuild && next.spec.App == spec.App && !next.spec.Legacy {
			return successor
		}
	}
	return ""
}

// enqueue coalesces a task request after its pause and reconciliation state has been resolved.
func (t *devWatcherTask) enqueue() {
	select {
	case t.triggerCh <- struct{}{}:
	default:
	}
}

// run serializes command execution so build bursts coalesce into at most one follow-up build.
func (t *devWatcherTask) run() {
	defer t.controller.wait.Done()
	for {
		select {
		case <-t.controller.ctx.Done():
			t.cancelActive()
			return
		case <-t.triggerCh:
			t.mu.Lock()
			if t.paused {
				t.pending = true
				t.mu.Unlock()
				continue
			}
			if t.spec.Kind == devWatcherAppBuild || t.spec.Kind == devWatcherSPABuild {
				t.busy = true
			}
			t.mu.Unlock()
			if t.spec.Kind == devWatcherAppRun {
				t.runRuntime()
				continue
			}
			t.runCommand()
		}
	}
}

// quiesceBuilds pauses graph build tasks and waits for active work to finish without stopping runtimes.
func (c *devWatcherController) quiesceBuilds(ctx context.Context) error {
	buildTasks := make([]*devWatcherTask, 0)
	for _, name := range c.order {
		task := c.tasks[name]
		if task.spec.Kind != devWatcherAppBuild && task.spec.Kind != devWatcherSPABuild {
			continue
		}
		task.mu.Lock()
		task.paused = true
		task.mu.Unlock()
		buildTasks = append(buildTasks, task)
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		idle := true
		for _, task := range buildTasks {
			task.mu.Lock()
			active := task.activeCancel != nil || task.busy
			task.mu.Unlock()
			if active || len(task.triggerCh) > 0 {
				idle = false
				break
			}
		}
		if idle {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.ctx.Done():
			return c.ctx.Err()
		case <-ticker.C:
		}
	}
}

// resumeBuilds replays one coalesced request for events observed while an outer build was active.
func (c *devWatcherController) resumeBuilds() {
	for _, name := range c.order {
		task := c.tasks[name]
		if task.spec.Kind != devWatcherAppBuild && task.spec.Kind != devWatcherSPABuild {
			continue
		}
		task.mu.Lock()
		pending := task.pending
		task.pending = false
		task.paused = false
		task.mu.Unlock()
		if pending {
			task.request()
		}
	}
}

// finishReconciliation releases the gated graph after the barrier settles while publishing runtimes only on success.
func (c *devWatcherController) finishReconciliation(success bool) {
	if !success {
		c.mu.Lock()
		pending := make([]*devWatcherTask, 0)
		c.reconcileSPAs = make(map[string]map[string]bool)
		c.reconcileAppBuild = make(map[string]string)
		for _, name := range c.order {
			task := c.tasks[name]
			if task.spec.Legacy {
				continue
			}
			task.mu.Lock()
			wasPending := task.pending
			task.pending = false
			task.paused = false
			task.mu.Unlock()
			if wasPending && (task.spec.Kind == devWatcherAppBuild || task.spec.Kind == devWatcherSPABuild) {
				pending = append(pending, task)
			}
		}
		c.mu.Unlock()
		for _, task := range pending {
			task.request()
		}
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	appBuilds := make(map[string]string)
	pendingSPAs := make(map[string]map[string]bool)
	pendingApps := make(map[string]bool)
	for _, name := range c.order {
		task := c.tasks[name]
		if task.spec.Legacy {
			continue
		}
		switch task.spec.Kind {
		case devWatcherAppBuild:
			appBuilds[task.spec.App] = name
			task.mu.Lock()
			if task.pending {
				pendingApps[task.spec.App] = true
			}
			task.mu.Unlock()
		case devWatcherSPABuild:
			task.mu.Lock()
			pending := task.pending
			task.mu.Unlock()
			if pending {
				if pendingSPAs[task.spec.App] == nil {
					pendingSPAs[task.spec.App] = map[string]bool{}
				}
				pendingSPAs[task.spec.App][name] = true
			}
		}
	}
	c.reconcileSPAs = make(map[string]map[string]bool)
	c.reconcileAppBuild = make(map[string]string)
	for app, spas := range pendingSPAs {
		if buildName := appBuilds[app]; buildName != "" {
			c.reconcileSPAs[app] = spas
			c.reconcileAppBuild[app] = buildName
			pendingApps[app] = true
		}
	}
	for _, name := range c.order {
		task := c.tasks[name]
		if task.spec.Legacy {
			continue
		}
		if task.spec.Kind == devWatcherAppRun {
			task.mu.Lock()
			pending := task.pending
			task.pending = false
			task.paused = false
			task.mu.Unlock()
			if !pendingApps[task.spec.App] && (task.startAfterReconcile || pending) {
				task.enqueue()
			}
			continue
		}
		if task.spec.Kind != devWatcherAppBuild && task.spec.Kind != devWatcherSPABuild {
			continue
		}
		task.mu.Lock()
		pending := task.pending
		if task.spec.Kind == devWatcherAppBuild && len(pendingSPAs[task.spec.App]) > 0 {
			task.pending = false
			task.mu.Unlock()
			continue
		}
		task.pending = false
		task.paused = false
		task.mu.Unlock()
		if pending {
			task.enqueue()
		}
	}
}

// completeReconciliationSPA holds the App edge until every latest SPA execution in reconciliation has succeeded.
func (c *devWatcherController) completeReconciliationSPA(task *devWatcherTask, success bool) (bool, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	remaining, active := c.reconcileSPAs[task.spec.App]
	if !active || !remaining[task.spec.ID] {
		return false, ""
	}
	if !success {
		return true, ""
	}
	task.mu.Lock()
	followUp := task.pending || len(task.triggerCh) > 0
	task.mu.Unlock()
	if followUp {
		return true, ""
	}
	delete(remaining, task.spec.ID)
	if len(remaining) > 0 {
		return true, ""
	}
	delete(c.reconcileSPAs, task.spec.App)
	buildName := c.reconcileAppBuild[task.spec.App]
	delete(c.reconcileAppBuild, task.spec.App)
	return true, buildName
}

// completeSteadySPA closes one frontend's latest execution and releases one App build when its wave succeeds.
func (c *devWatcherController) completeSteadySPA(task *devWatcherTask, success bool) (bool, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	wave := c.steadySPAWaves[task.spec.App]
	if wave == nil || !wave.pending[task.spec.ID] {
		return false, ""
	}
	task.mu.Lock()
	followUp := task.pending || len(task.triggerCh) > 0
	task.mu.Unlock()
	if followUp {
		return true, ""
	}
	delete(wave.pending, task.spec.ID)
	if success {
		delete(wave.failed, task.spec.ID)
	} else {
		wave.failed[task.spec.ID] = true
	}
	if len(wave.pending) > 0 {
		return true, ""
	}
	delete(c.steadySPAWaves, task.spec.App)
	if len(wave.failed) > 0 {
		return true, ""
	}
	return true, wave.buildName
}

// runCommand executes one build/custom command and follows only successful graph edges.
func (t *devWatcherTask) runCommand() {
	defer t.markCommandIdle()
	transitionKey, transitionLine := t.commandTransition()
	if transitionLine != "" {
		setDevTransition(t.controller.outWriter, transitionKey, transitionLine)
		defer clearDevTransition(t.controller.outWriter, transitionKey)
	}
	t.writeExecLog()
	var exit devwatch.Exit
	var err error
	if t.spec.Kind == devWatcherAppBuild && t.spec.PhasedBuild {
		exit, err = t.runPhasedAppBuild()
	} else {
		if unlock := t.lockProjectBuild(); unlock != nil {
			defer unlock()
		}
		exit, err = t.runSupervisedCommand(t.spec.Command)
	}
	if t.controller.ctx.Err() != nil || exit.Intentional() {
		return
	}
	success := err == nil && exit.OK()
	if success && t.spec.Kind == devWatcherSPABuild {
		recordDevSPABuild(".", t.spec)
	}
	if success && t.spec.Kind == devWatcherAppBuild {
		err = publishDevBuildReadyStamp(project.AppForName(t.spec.App))
		if err != nil {
			_, _ = fmt.Fprintf(t.controller.errWriter, "forj dev: %v\n", err)
		}
	}
	success = err == nil && exit.OK()
	if success {
		t.reportStartup(nil)
	} else {
		t.reportStartup(devWatcherCommandFailure(exit, err))
	}
	handledSPA := false
	reconciliationSPA := false
	buildName := ""
	if t.spec.Kind == devWatcherSPABuild && !t.spec.Legacy {
		reconciliationSPA, buildName = t.controller.completeReconciliationSPA(t, success)
		handledSPA = reconciliationSPA
		if !handledSPA {
			handledSPA, buildName = t.controller.completeSteadySPA(t, success)
		}
		if buildName != "" {
			if build := t.controller.tasks[buildName]; build != nil {
				if reconciliationSPA {
					build.mu.Lock()
					build.pending = false
					build.paused = false
					build.mu.Unlock()
				}
				build.request()
			}
		}
	}
	if success && !handledSPA {
		for _, successor := range t.spec.OnSuccess {
			if next := t.controller.tasks[successor]; next != nil {
				next.request()
			}
		}
	}
	if t.spec.Exit {
		t.controller.publishExit(t.spec.ID, t.spec.Name, &exit, err)
	}
}

// reportStartup resolves one task once so later file events cannot alter the generation's initial result.
func (t *devWatcherTask) reportStartup(err error) {
	if !t.startupTracked {
		return
	}
	t.startupOnce.Do(func() {
		if err != nil {
			err = &devWatcherStartupError{
				Watcher: t.spec.Name,
				Command: t.spec.DisplayCommand,
				Err:     err,
			}
		}
		t.controller.startup.noteOutcome(t.spec.ID, err)
	})
}

// devWatcherCommandFailure preserves native start errors and gives nonzero exits an actionable process result.
func devWatcherCommandFailure(exit devwatch.Exit, err error) error {
	if err != nil {
		return err
	}
	if !exit.OK() {
		return fmt.Errorf("exited with code %d", exit.ExitCode)
	}
	return nil
}

// commandTransition gives structured build work a stable TUI owner across concurrent watcher executions.
func (t *devWatcherTask) commandTransition() (string, string) {
	switch t.spec.Kind {
	case devWatcherAppBuild:
		return "watcher:" + t.spec.ID, "Building " + t.spec.App
	case devWatcherSPABuild:
		return "watcher:" + t.spec.ID, "Building " + t.spec.App + " frontend"
	default:
		return "", ""
	}
}

// runPhasedAppBuild stabilizes generated inputs exclusively before joining concurrent App analysis and compilation.
func (t *devWatcherTask) runPhasedAppBuild() (devwatch.Exit, error) {
	t.controller.projectBuild.acquire(devWatcherBuildPhasePrepare)
	exit, err := t.runSupervisedCommand(devCommandWithBuildPhase(t.spec.Command, build.DevBuildPhasePrepare))
	t.controller.projectBuild.release(devWatcherBuildPhasePrepare)
	if err != nil || !exit.OK() || exit.Intentional() || t.controller.ctx.Err() != nil {
		return exit, err
	}

	t.controller.projectBuild.acquire(devWatcherBuildPhaseApp)
	exit, err = t.runSupervisedCommand(devCommandWithBuildPhase(t.spec.Command, build.DevBuildPhaseCompile))
	t.controller.projectBuild.release(devWatcherBuildPhaseApp)
	return exit, err
}

// runSupervisedCommand exposes one cancellable subprocess phase to shutdown and restart bookkeeping.
func (t *devWatcherTask) runSupervisedCommand(command devwatch.Command) (devwatch.Exit, error) {
	runContext, cancel := context.WithCancel(t.controller.ctx)
	t.setActiveCancel(cancel)
	exit, err := t.controller.supervisor.Run(runContext, t.spec.ID, command)
	t.clearActiveCancel(cancel)
	cancel()
	return exit, err
}

// devCommandWithBuildPhase clones command environment before assigning the private dev build phase.
func devCommandWithBuildPhase(command devwatch.Command, phase string) devwatch.Command {
	command.Env = copyDevWatchEnv(command.Env)
	command.Env["FORJ_COMMAND_ORIGIN"] = "dev_command"
	command.Env[build.DevBuildPhaseEnvironment] = phase
	return command
}

// lockProjectBuild keeps custom App builds conservative and lets independent SPA builds share their phase.
func (t *devWatcherTask) lockProjectBuild() func() {
	switch t.spec.Kind {
	case devWatcherAppBuild:
		t.controller.projectBuild.acquire(devWatcherBuildPhasePrepare)
		return func() { t.controller.projectBuild.release(devWatcherBuildPhasePrepare) }
	case devWatcherSPABuild:
		t.controller.projectBuild.acquire(devWatcherBuildPhaseSPA)
		return func() { t.controller.projectBuild.release(devWatcherBuildPhaseSPA) }
	default:
		return nil
	}
}

// newDevWatcherBuildGate creates a FIFO phase gate for project preparation, App compilation, and SPA publication.
func newDevWatcherBuildGate() *devWatcherBuildGate {
	gate := &devWatcherBuildGate{}
	gate.cond = sync.NewCond(&gate.mu)
	return gate
}

// acquire joins compatible work without jumping ahead of an earlier incompatible phase.
func (g *devWatcherBuildGate) acquire(phase devWatcherBuildPhase) {
	g.mu.Lock()
	request := &devWatcherBuildRequest{phase: phase}
	g.waiting = append(g.waiting, request)
	g.cond.Broadcast()
	for !g.canAcquire(request) {
		g.cond.Wait()
	}
	g.removeWaiting(request)
	if g.active == devWatcherBuildPhaseNone {
		g.active = phase
	}
	g.activeCount++
	g.mu.Unlock()
}

// release advances the waiting phase only after every compatible build has left the current phase.
func (g *devWatcherBuildGate) release(phase devWatcherBuildPhase) {
	g.mu.Lock()
	if g.active != phase || g.activeCount == 0 {
		g.mu.Unlock()
		panic("release inactive dev build phase")
	}
	g.activeCount--
	if g.activeCount == 0 {
		g.active = devWatcherBuildPhaseNone
		g.cond.Broadcast()
	}
	g.mu.Unlock()
}

// canAcquire preserves phase concurrency while stopping new arrivals when the other phase is waiting.
func (g *devWatcherBuildGate) canAcquire(request *devWatcherBuildRequest) bool {
	if g.active != devWatcherBuildPhaseNone && (g.active != request.phase || !request.phase.shareable()) {
		return false
	}
	for _, waiting := range g.waiting {
		if waiting == request {
			return true
		}
		if waiting.phase != request.phase || !request.phase.shareable() {
			return false
		}
	}
	return false
}

// removeWaiting removes the granted request while retaining FIFO order for incompatible phases.
func (g *devWatcherBuildGate) removeWaiting(request *devWatcherBuildRequest) {
	for index, waiting := range g.waiting {
		if waiting != request {
			continue
		}
		g.waiting = append(g.waiting[:index], g.waiting[index+1:]...)
		return
	}
}

// shareable reports whether independent work in one phase may safely overlap.
func (p devWatcherBuildPhase) shareable() bool {
	return p == devWatcherBuildPhaseApp || p == devWatcherBuildPhaseSPA
}

// markCommandIdle closes the trigger-to-process race observed by outer build quiescing.
func (t *devWatcherTask) markCommandIdle() {
	t.mu.Lock()
	t.busy = false
	t.mu.Unlock()
}

// runRuntime starts or gracefully replaces one app runtime after a successful upstream build.
func (t *devWatcherTask) runRuntime() {
	t.writeExecLog()
	t.mu.Lock()
	runtimeLive := t.runtimeLive
	t.mu.Unlock()
	transitionLine := formatDevRuntimeTransition("Starting", t.spec)
	if runtimeLive {
		transitionLine = formatDevRuntimeTransition("Restarting", t.spec)
	}
	transitionKey := "runtime:" + t.spec.ID
	setDevTransition(t.controller.outWriter, transitionKey, transitionLine)
	defer clearDevTransition(t.controller.outWriter, transitionKey)
	command, err := prepareNativeRuntimeCommand(t.spec)
	if err != nil {
		t.reportStartup(err)
		if runtimeLive {
			_, _ = fmt.Fprintf(t.controller.errWriter, "forj dev: replacement for %s is not ready; keeping the current runtime: %v\n", t.spec.Name, err)
		} else {
			_, _ = fmt.Fprintf(t.controller.errWriter, "forj dev: %s is not ready; waiting for the next successful build: %v\n", t.spec.Name, err)
		}
		return
	}
	t.runtimeMu.Lock()
	defer t.runtimeMu.Unlock()
	t.mu.Lock()
	runtimeLive = t.runtimeLive
	t.mu.Unlock()
	var pid int
	if runtimeLive {
		pid, err = t.controller.supervisor.RestartRuntime(t.controller.ctx, t.spec.ID, command)
	} else {
		pid, err = t.controller.supervisor.StartRuntime(t.controller.ctx, t.spec.ID, command)
	}
	if err != nil {
		t.reportStartup(err)
		stillRunning := t.controller.supervisor.RuntimeRunning(t.spec.ID)
		t.mu.Lock()
		t.runtimeLive = stillRunning
		if !stillRunning {
			t.runtimePID = 0
		}
		t.mu.Unlock()
		if t.controller.ctx.Err() == nil {
			if stillRunning {
				_, _ = fmt.Fprintf(t.controller.errWriter, "forj dev: replacement for %s failed; keeping the current runtime: %v\n", t.spec.Name, err)
			} else {
				_, _ = fmt.Fprintf(t.controller.errWriter, "forj dev: could not start %s; waiting for the next successful build: %v\n", t.spec.Name, err)
			}
		}
		return
	}
	t.mu.Lock()
	t.runtimeLive = true
	t.runtimePID = pid
	t.mu.Unlock()
	t.reportStartup(nil)
}

// formatDevRuntimeTransition presents conventional App names without leaking the lowercase default identifier.
func formatDevRuntimeTransition(action string, spec devCompiledWatcher) string {
	label := strings.TrimSpace(spec.Name)
	if appName := strings.TrimSpace(spec.App); appName != "" {
		label = devAppWatcherName(strings.TrimSpace(action), appName)
	} else {
		label = strings.TrimSpace(action) + " " + label
	}
	return joinDevLifecycleFields(label)
}

// waitStartup waits for explicit wrapper and process results from the generation's initial runnable tasks.
func (c *devWatcherController) waitStartup(ctx context.Context) error {
	return c.startup.wait(ctx)
}

// prepareNativeRuntimeCommand snapshots recognized App binaries before replacement can disturb a healthy runtime.
func prepareNativeRuntimeCommand(spec devCompiledWatcher) (devwatch.Command, error) {
	command := spec.Command
	if spec.Kind != devWatcherAppRun || spec.FullProcessOverride || spec.NativeRuntimeCommand == "" {
		return command, nil
	}
	target, ok := devExecutableTarget(spec.NativeRuntimeCommand)
	if !ok {
		return command, nil
	}
	sourcePath := target
	if !filepath.IsAbs(sourcePath) {
		directory := command.Dir
		if directory == "" {
			directory = "."
		}
		sourcePath = filepath.Join(directory, sourcePath)
	}
	prepared, err := devwatch.PrepareExecutable(sourcePath)
	if err != nil {
		return devwatch.Command{}, err
	}
	preparedExec := buildPreparedNativeRuntimeExec(spec.NativeRuntimeCommand, target, prepared.Path())
	command.Args = []string{"bash", "-c", preparedExec}
	command.Shell = ""
	boundCommand, err := prepared.Bind(command)
	if err != nil {
		return devwatch.Command{}, errors.Join(err, prepared.Cleanup())
	}
	return boundCommand, nil
}

// writeExecLog preserves the fork's execution-only logging flags without exposing them to native config.
func (t *devWatcherTask) writeExecLog() {
	if !t.spec.Legacy || (!t.spec.Verbose && !t.spec.ExecLog) {
		return
	}
	message := strings.TrimSpace(t.spec.ExecMessage)
	if message != "" {
		message += " "
	}
	_, _ = fmt.Fprintf(t.controller.errWriter, "%s%sEXECUTING %s\n", legacyWatchLogPrefix(t.spec), message, t.spec.DisplayCommand)
}

// markRuntimeStopped clears ownership only when the completion belongs to the currently published runtime.
func (t *devWatcherTask) markRuntimeStopped(pid int) bool {
	t.runtimeMu.Lock()
	defer t.runtimeMu.Unlock()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.runtimePID != pid {
		return false
	}
	t.runtimeLive = false
	t.runtimePID = 0
	return true
}

// setActiveCancel exposes cancellation to restart-style event delivery.
func (t *devWatcherTask) setActiveCancel(cancel context.CancelFunc) {
	t.mu.Lock()
	t.activeCancel = cancel
	t.mu.Unlock()
}

// clearActiveCancel avoids canceling a later execution after one command finishes.
func (t *devWatcherTask) clearActiveCancel(cancel context.CancelFunc) {
	t.mu.Lock()
	t.activeCancel = nil
	t.mu.Unlock()
}

// cancelActive interrupts a one-shot command during controller shutdown.
func (t *devWatcherTask) cancelActive() {
	t.mu.Lock()
	cancel := t.activeCancel
	t.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// publishExit emits at most one terminal event per internal watcher while retaining its display name.
func (c *devWatcherController) publishExit(id string, name string, processExit *devwatch.Exit, err error) {
	c.mu.Lock()
	if c.exited[id] {
		c.mu.Unlock()
		return
	}
	c.exited[id] = true
	c.mu.Unlock()
	output := ""
	if task := c.tasks[id]; task != nil && task.outputTail != nil {
		output = task.outputTail.String()
	}
	select {
	case c.exitCh <- watcherExit{id: id, name: name, process: processExit, err: err, output: output}:
	case <-c.ctx.Done():
		select {
		case c.exitCh <- watcherExit{id: id, name: name, process: processExit, err: err, output: output}:
		default:
		}
	}
}

// stop cancels event delivery, terminates process trees, and publishes one synthetic exit per remaining watcher.
func (c *devWatcherController) stop(timeout time.Duration) {
	c.stopOnce.Do(func() {
		go c.stopAll(timeout)
	})
	<-c.stopDone
}

// stopAll closes one native generation before publishing terminal events for each logical watcher handle.
func (c *devWatcherController) stopAll(timeout time.Duration) {
	defer close(c.stopDone)
	c.mu.Lock()
	c.stopping = true
	c.mu.Unlock()
	c.cancel()
	for _, engine := range c.engines {
		_ = engine.Close()
	}
	shutdownContext := context.Background()
	var cancel context.CancelFunc
	if timeout > 0 {
		shutdownContext, cancel = context.WithTimeout(shutdownContext, timeout)
		defer cancel()
	}
	_ = c.supervisor.Shutdown(shutdownContext)
	c.wait.Wait()
	c.supervisor.Close()
	for _, id := range c.order {
		task := c.tasks[id]
		c.publishExit(id, task.spec.Name, nil, nil)
	}
}

// watcherExitOK treats synthetic controller exits as successful while preserving native process status.
func watcherExitOK(exit watcherExit) bool {
	if exit.process == nil {
		return true
	}
	return exit.process.OK()
}

// watcherExitCode keeps synthetic controller exits at zero while preserving native process status.
func watcherExitCode(exit watcherExit) int {
	if exit.process == nil {
		return 0
	}
	return exit.process.ExitCode
}

// watcherExitError retains start failures even when the operating system did not create a process result.
func watcherExitError(exit watcherExit) error {
	if exit.err != nil {
		return exit.err
	}
	if exit.process != nil && exit.process.Err != nil && !exit.process.Intentional() {
		return exit.process.Err
	}
	return nil
}

// unexpectedWatcherExitError keeps the failing watcher identifiable after the shared runtime shuts down.
func unexpectedWatcherExitError(exit watcherExit) error {
	var exitErr error
	if err := watcherExitError(exit); err != nil {
		exitErr = fmt.Errorf("dev watcher %q exited: %w", exit.name, err)
	} else if !watcherExitOK(exit) {
		exitErr = fmt.Errorf("dev watcher %q exited with code %d", exit.name, watcherExitCode(exit))
	}
	if exitErr == nil || strings.TrimSpace(exit.output) == "" {
		return exitErr
	}
	return fmt.Errorf("%w\n\nLast watcher output:\n%s", exitErr, strings.TrimSpace(exit.output))
}

// isIntentionalWatcherExit distinguishes coordinated native stops from command failures.
func isIntentionalWatcherExit(exit watcherExit) bool {
	return exit.process != nil && exit.process.Intentional() || errors.Is(exit.err, context.Canceled)
}
