package forj

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/goforj/goforj/internal/devwatch"
	"github.com/goforj/goforj/project"
)

type devWatcherController struct {
	ctx        context.Context
	cancel     context.CancelFunc
	supervisor *devwatch.Supervisor
	tasks      map[string]*devWatcherTask
	order      []string
	engines    []*devwatch.Engine
	exitCh     chan watcherExit
	outWriter  io.Writer
	errWriter  io.Writer

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

	runtimeMu           sync.Mutex
	mu                  sync.Mutex
	activeCancel        context.CancelFunc
	runtimeLive         bool
	runtimePID          int
	paused              bool
	pending             bool
	busy                bool
	startAfterReconcile bool
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
	if len(compiled) == 0 {
		return watcherRuntime, nil
	}
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
		watcherRuntime.watchers = append(watcherRuntime.watchers, runningWatcher{id: watcher.ID, name: watcher.Name, native: controller})
		names = append(names, watcher.Name)
	}
	emitWatcherLifecycleSummary(controller.outWriter, session.streamer, names, watcherStateStarted)
	return watcherRuntime, nil
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
	outputMu := &sync.Mutex{}
	outWriter = devWatcherSynchronizedWriter{mu: outputMu, writer: outWriter}
	errWriter = devWatcherSynchronizedWriter{mu: outputMu, writer: errWriter}
	controller := &devWatcherController{
		ctx: ctx, cancel: cancel, supervisor: devwatch.NewSupervisor(devwatch.SupervisorOptions{}),
		tasks: make(map[string]*devWatcherTask, len(compiled)), exitCh: make(chan watcherExit, len(compiled)),
		outWriter: outWriter, errWriter: errWriter, exited: make(map[string]bool, len(compiled)),
		steadySPAWaves: make(map[string]*devWatcherSteadySPAWave), stopDone: make(chan struct{}),
	}
	runtimeApps := compiledDevRuntimeApps(compiled)
	showAppColumn := len(runtimeApps) > 1
	appNameWidth := devAppColumnWidth(runtimeApps)
	immediate := 0
	runtimeNames := make([]string, 0)
	for _, spec := range compiled {
		gatedRuntime := options.reconcile && !spec.Legacy && spec.Kind == devWatcherAppRun
		if !spec.Postpone && !gatedRuntime {
			immediate++
		}
		if spec.Kind == devWatcherAppRun {
			runtimeNames = append(runtimeNames, spec.Name)
		}
	}
	lifecycle := newDevwatchLifecycleState(immediate, runtimeNames)
	for _, spec := range compiled {
		structuredBuild := !spec.Legacy && (spec.Kind == devWatcherAppBuild || spec.Kind == devWatcherSPABuild)
		structuredRuntime := !spec.Legacy && spec.Kind == devWatcherAppRun
		startAfterReconcile := structuredRuntime && !spec.Postpone
		if options.reconcile && structuredRuntime {
			spec.Postpone = true
		}
		configureCompiledDevCommand(&spec, streamer, outWriter, errWriter, soundOnError, lifecycle, appNameWidth, showAppColumn)
		controller.tasks[spec.ID] = &devWatcherTask{
			controller: controller, spec: spec, triggerCh: make(chan struct{}, 1),
			paused: options.reconcile && (structuredBuild || structuredRuntime), startAfterReconcile: startAfterReconcile,
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
	soundOnError bool,
	lifecycle *devwatchLifecycleState,
	appNameWidth int,
	showAppColumn bool,
) {
	triggerCommand := strings.Join(strings.Fields(spec.Command.Shell), " ")
	spec.DisplayCommand = triggerCommand
	appName := spec.App
	stdout := newDevwatchWriterForApp(outWriter, streamer, "stdout", spec.Name, triggerCommand, appName, appNameWidth, showAppColumn, lifecycle)
	stderr := newDevwatchWriterForApp(errWriter, streamer, "stderr", spec.Name, triggerCommand, appName, appNameWidth, showAppColumn, lifecycle)
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
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
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

// finishReconciliation releases buffered source events and publishes runtimes only after the barrier succeeds.
func (c *devWatcherController) finishReconciliation(success bool) {
	if !success {
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
	t.writeExecLog()
	runContext, cancel := context.WithCancel(t.controller.ctx)
	t.setActiveCancel(cancel)
	exit, err := t.controller.supervisor.Run(runContext, t.spec.ID, t.spec.Command)
	t.clearActiveCancel(cancel)
	cancel()
	if t.controller.ctx.Err() != nil || exit.Intentional() {
		return
	}
	success := err == nil && exit.OK()
	if success && t.spec.Kind == devWatcherAppBuild {
		err = publishDevBuildReadyStamp(project.DefaultNamedApp(t.spec.App))
		if err != nil {
			_, _ = fmt.Fprintf(t.controller.errWriter, "forj dev: %v\n", err)
		}
	}
	success = err == nil && exit.OK()
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
	command, err := prepareNativeRuntimeCommand(t.spec)
	if err != nil {
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
	select {
	case c.exitCh <- watcherExit{id: id, name: name, native: processExit, err: err}:
	case <-c.ctx.Done():
		select {
		case c.exitCh <- watcherExit{id: id, name: name, native: processExit, err: err}:
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

// stopAll performs controller shutdown once while allowing every runningWatcher handle to wait on it.
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

// watcherExitOK normalizes legacy execx and native process results for the outer dev loop.
func watcherExitOK(exit watcherExit) bool {
	if exit.native != nil {
		return exit.native.OK()
	}
	return exit.result.OK()
}

// watcherExitCode returns a useful native or compatibility process exit code.
func watcherExitCode(exit watcherExit) int {
	if exit.native != nil {
		return exit.native.ExitCode
	}
	return exit.result.ExitCode
}

// watcherExitError retains start failures even when the operating system did not create a process result.
func watcherExitError(exit watcherExit) error {
	if exit.err != nil {
		return exit.err
	}
	if exit.native != nil && exit.native.Err != nil && !exit.native.Intentional() {
		return exit.native.Err
	}
	return nil
}

// isIntentionalWatcherExit distinguishes coordinated native stops from command failures.
func isIntentionalWatcherExit(exit watcherExit) bool {
	return exit.native != nil && exit.native.Intentional() || errors.Is(exit.err, context.Canceled)
}
