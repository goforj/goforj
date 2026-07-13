package devwatch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultStopTimeout      = 5 * time.Second
	defaultProcessWaitDelay = 5 * time.Second
	defaultPTYDrainTimeout  = 500 * time.Millisecond
	defaultExitBuffer       = 256
)

// StopReason records why the supervisor intentionally stopped a process.
type StopReason string

const (
	// StopReasonCanceled identifies a one-shot command stopped by context cancellation.
	StopReasonCanceled StopReason = "canceled"
	// StopReasonRestart identifies a runtime stopped to publish its replacement.
	StopReasonRestart StopReason = "restart"
	// StopReasonManual identifies a runtime stopped through StopRuntime.
	StopReasonManual StopReason = "stop"
	// StopReasonShutdown identifies a runtime stopped during coordinated supervisor shutdown.
	StopReasonShutdown StopReason = "shutdown"
)

// Stream identifies the output stream delivered to an output hook.
type Stream string

const (
	// StreamStdout identifies output read from the child's standard output stream.
	StreamStdout Stream = "stdout"
	// StreamStderr identifies output read from the child's standard error stream.
	StreamStderr Stream = "stderr"
)

// Output describes one output chunk observed from a child process.
type Output struct {
	Stream Stream
	Data   []byte
}

// Command describes either an argv command or a shell command.
type Command struct {
	Args                []string
	Shell               string
	Dir                 string
	Env                 map[string]string
	ReplaceEnv          bool
	Stdin               io.Reader
	Stdout              io.Writer
	Stderr              io.Writer
	Interactive         bool
	PTY                 bool
	OnOutput            func(Output)
	GracefulStopTimeout time.Duration
	cleanup             func() error
}

// Exit is the stable result emitted whenever a managed process finishes.
type Exit struct {
	Name       string
	PID        int
	ExitCode   int
	Err        error
	StartedAt  time.Time
	FinishedAt time.Time
	StopReason StopReason
}

// OK reports whether the process exited successfully without an intentional stop.
func (e Exit) OK() bool {
	return e.Err == nil && e.ExitCode == 0 && e.StopReason == ""
}

// Intentional reports whether the supervisor requested the process exit.
func (e Exit) Intentional() bool {
	return e.StopReason != ""
}

// SupervisorOptions configures native child-process supervision.
type SupervisorOptions struct {
	StopTimeout time.Duration
	ExitBuffer  int
}

// Supervisor owns one-shot commands and named long-running runtimes.
type Supervisor struct {
	operationMu  sync.Mutex
	mu           sync.Mutex
	runtimes     map[string]*managedProcess
	shuttingDown bool
	stopTimeout  time.Duration
	exits        chan Exit
	exitInput    chan Exit
	exitStop     chan struct{}
	exitDone     chan struct{}
	exitStopOnce sync.Once
}

// managedProcess holds the mutable lifecycle state for one operating-system process.
type managedProcess struct {
	name      string
	command   Command
	cmd       *exec.Cmd
	tree      *processTree
	startedAt time.Time
	done      chan struct{}
	ptyMaster *os.File
	ptyDone   chan struct{}

	mu           sync.Mutex
	leaderExited bool
	exited       bool
	exit         Exit
	stopReason   StopReason
}

// commandEnvironmentValue retains the spelling paired with one normalized environment key.
type commandEnvironmentValue struct {
	key   string
	value string
}

// outputWriter forwards output while giving the dev transcript a copy to inspect.
type outputWriter struct {
	stream Stream
	writer io.Writer
	hook   func(Output)
	mu     *sync.Mutex
}

// NewSupervisor creates a process supervisor with bounded graceful shutdowns.
func NewSupervisor(options SupervisorOptions) *Supervisor {
	stopTimeout := options.StopTimeout
	if stopTimeout <= 0 {
		stopTimeout = defaultStopTimeout
	}
	exitBuffer := options.ExitBuffer
	if exitBuffer <= 0 {
		exitBuffer = defaultExitBuffer
	}
	supervisor := &Supervisor{
		runtimes:    make(map[string]*managedProcess),
		stopTimeout: stopTimeout,
		exits:       make(chan Exit, exitBuffer),
		exitInput:   make(chan Exit, exitBuffer),
		exitStop:    make(chan struct{}),
		exitDone:    make(chan struct{}),
	}
	go supervisor.dispatchExits()
	return supervisor
}

// Exits returns every one-shot and runtime completion record in finish order until Close is called.
func (s *Supervisor) Exits() <-chan Exit {
	return s.exits
}

// Close releases exit-delivery resources after callers have stopped runtimes and awaited one-shot commands.
func (s *Supervisor) Close() {
	s.exitStopOnce.Do(func() {
		close(s.exitStop)
	})
	<-s.exitDone
}

// Run executes one command and gracefully stops its process tree when the context is canceled.
func (s *Supervisor) Run(ctx context.Context, name string, command Command) (Exit, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		err = errors.Join(err, cleanupCommand(command))
		exit := newStartFailure(name, err)
		s.emitExit(exit)
		return exit, err
	}
	process, err := startManagedProcess(name, command)
	if err != nil {
		exit := newStartFailure(name, err)
		s.emitExit(exit)
		return exit, err
	}
	process.beginWait(s.emitExit)
	select {
	case <-process.done:
		exit := process.result()
		return exit, exit.Err
	case <-ctx.Done():
		stopErr := process.stop(context.Background(), StopReasonCanceled, s.commandStopTimeout(command))
		select {
		case <-process.done:
			exit := process.result()
			return exit, ctx.Err()
		default:
		}
		exit := Exit{
			Name:       name,
			PID:        process.cmd.Process.Pid,
			ExitCode:   -1,
			Err:        stopErr,
			StartedAt:  process.startedAt,
			FinishedAt: time.Now(),
			StopReason: StopReasonCanceled,
		}
		return exit, ctx.Err()
	}
}

// StartRuntime starts a named long-running process without replacing an existing runtime.
func (s *Supervisor) StartRuntime(ctx context.Context, name string, command Command) (int, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	return s.startRuntime(ctx, normalizeRuntimeName(name), command)
}

// RuntimeRunning reports whether the supervisor still owns a live or not-yet-reaped named runtime.
func (s *Supervisor) RuntimeRunning(name string) bool {
	return s.runtime(normalizeRuntimeName(name)) != nil
}

// RestartRuntime gracefully stops a named runtime before starting its replacement.
func (s *Supervisor) RestartRuntime(ctx context.Context, name string, command Command) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	name = normalizeRuntimeName(name)
	s.operationMu.Lock()
	defer s.operationMu.Unlock()

	process := s.runtime(name)
	if process != nil {
		if err := process.stop(ctx, StopReasonRestart, s.commandStopTimeout(process.command)); err != nil {
			return 0, errors.Join(fmt.Errorf("restart runtime %q: %w", name, err), cleanupCommand(command))
		}
		s.forgetRuntime(name, process)
	}
	return s.startRuntime(ctx, name, command)
}

// StopRuntime gracefully stops one named runtime and its process tree.
func (s *Supervisor) StopRuntime(ctx context.Context, name string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	name = normalizeRuntimeName(name)
	s.operationMu.Lock()
	defer s.operationMu.Unlock()

	process := s.runtime(name)
	if process == nil {
		return nil
	}
	if err := process.stop(ctx, StopReasonManual, s.commandStopTimeout(process.command)); err != nil {
		return fmt.Errorf("stop runtime %q: %w", name, err)
	}
	s.forgetRuntime(name, process)
	return nil
}

// Shutdown gracefully stops all active runtimes in parallel.
func (s *Supervisor) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.operationMu.Lock()
	defer s.operationMu.Unlock()

	s.mu.Lock()
	s.shuttingDown = true
	processes := make([]*managedProcess, 0, len(s.runtimes))
	for _, process := range s.runtimes {
		processes = append(processes, process)
	}
	s.mu.Unlock()

	errorsCh := make(chan error, len(processes))
	var waitGroup sync.WaitGroup
	for _, process := range processes {
		waitGroup.Add(1)
		go func(process *managedProcess) {
			defer waitGroup.Done()
			if err := process.stop(ctx, StopReasonShutdown, s.commandStopTimeout(process.command)); err != nil {
				errorsCh <- fmt.Errorf("stop runtime %q: %w", process.name, err)
				return
			}
			s.forgetRuntime(process.name, process)
		}(process)
	}
	waitGroup.Wait()
	close(errorsCh)

	var shutdownErrors []error
	for err := range errorsCh {
		shutdownErrors = append(shutdownErrors, err)
	}
	return errors.Join(shutdownErrors...)
}

// startRuntime starts and registers a runtime while the caller holds operationMu.
func (s *Supervisor) startRuntime(ctx context.Context, name string, command Command) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return 0, errors.Join(err, cleanupCommand(command))
	}
	name = normalizeRuntimeName(name)
	if name == "" {
		return 0, errors.Join(errors.New("runtime name is required"), cleanupCommand(command))
	}

	s.mu.Lock()
	if s.shuttingDown {
		s.mu.Unlock()
		return 0, errors.Join(errors.New("process supervisor is shutting down"), cleanupCommand(command))
	}
	if _, exists := s.runtimes[name]; exists {
		s.mu.Unlock()
		return 0, errors.Join(fmt.Errorf("runtime %q is already running", name), cleanupCommand(command))
	}
	s.mu.Unlock()

	process, err := startManagedProcess(name, command)
	if err != nil {
		return 0, fmt.Errorf("start runtime %q: %w", name, err)
	}
	s.mu.Lock()
	if s.shuttingDown {
		s.mu.Unlock()
		process.beginWait(s.emitExit)
		_ = process.stop(context.Background(), StopReasonShutdown, s.commandStopTimeout(command))
		return 0, errors.New("process supervisor is shutting down")
	}
	s.runtimes[name] = process
	s.mu.Unlock()

	process.beginWait(func(exit Exit) {
		s.forgetRuntime(name, process)
		s.emitExit(exit)
	})
	return process.cmd.Process.Pid, nil
}

// runtime returns a registered runtime without surrendering ownership before its exit is confirmed.
func (s *Supervisor) runtime(name string) *managedProcess {
	name = normalizeRuntimeName(name)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runtimes[name]
}

// forgetRuntime removes a naturally exited runtime without disturbing a replacement.
func (s *Supervisor) forgetRuntime(name string, process *managedProcess) {
	name = normalizeRuntimeName(name)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runtimes[name] == process {
		delete(s.runtimes, name)
	}
}

// normalizeRuntimeName gives every lifecycle operation the same canonical map key.
func normalizeRuntimeName(name string) string {
	return strings.TrimSpace(name)
}

// commandStopTimeout resolves a command override against the supervisor default.
func (s *Supervisor) commandStopTimeout(command Command) time.Duration {
	if command.GracefulStopTimeout > 0 {
		return command.GracefulStopTimeout
	}
	return s.stopTimeout
}

// emitExit hands completion records to an unbounded dispatcher so slow observers cannot block process reaping.
func (s *Supervisor) emitExit(exit Exit) {
	select {
	case s.exitInput <- exit:
	case <-s.exitStop:
	}
}

// dispatchExits decouples process reaping from the public channel while retaining every queued record.
func (s *Supervisor) dispatchExits() {
	defer close(s.exitDone)
	defer close(s.exits)
	queue := make([]Exit, 0)
	for {
		var output chan Exit
		var next Exit
		if len(queue) > 0 {
			output = s.exits
			next = queue[0]
		}
		select {
		case exit := <-s.exitInput:
			queue = append(queue, exit)
		case output <- next:
			queue = queue[1:]
		case <-s.exitStop:
			return
		}
	}
}

// startManagedProcess validates, configures, and starts one operating-system process.
func startManagedProcess(name string, command Command) (*managedProcess, error) {
	cmd, err := buildExecCommand(command)
	if err != nil {
		return nil, errors.Join(err, cleanupCommand(command))
	}
	tree, err := newProcessTree(cmd)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("prepare process tree: %w", err), cleanupCommand(command))
	}
	ptyWriter := commandPTYWriter(cmd.Stdout, cmd.Stderr)
	var ptyMaster *os.File
	var ptySlave *os.File
	if command.PTY {
		ptyMaster, ptySlave, err = openDevWatchPTY()
		if err != nil {
			tree.release()
			return nil, errors.Join(fmt.Errorf("open process pseudo-terminal: %w", err), cleanupCommand(command))
		}
		cmd.Stdout = ptySlave
		cmd.Stderr = ptySlave
	}
	startedAt := time.Now()
	if err := cmd.Start(); err != nil {
		tree.release()
		if ptyMaster != nil {
			_ = ptyMaster.Close()
		}
		if ptySlave != nil {
			_ = ptySlave.Close()
		}
		return nil, errors.Join(err, cleanupCommand(command))
	}
	if err := tree.attach(cmd); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		tree.release()
		if ptyMaster != nil {
			_ = ptyMaster.Close()
		}
		if ptySlave != nil {
			_ = ptySlave.Close()
		}
		return nil, errors.Join(fmt.Errorf("attach process tree: %w", err), cleanupCommand(command))
	}
	process := &managedProcess{
		name:      name,
		command:   command,
		cmd:       cmd,
		tree:      tree,
		startedAt: startedAt,
		done:      make(chan struct{}),
		ptyMaster: ptyMaster,
	}
	if ptyMaster != nil {
		_ = ptySlave.Close()
		process.ptyDone = make(chan struct{})
		go func() {
			_, _ = io.Copy(ptyWriter, ptyMaster)
			_ = ptyMaster.Close()
			close(process.ptyDone)
		}()
	}
	return process, nil
}

// commandPTYWriter selects from resolved exec writers so interactive defaults and hooks are preserved.
func commandPTYWriter(stdout io.Writer, stderr io.Writer) io.Writer {
	if stdout != nil {
		return stdout
	}
	if stderr != nil {
		return stderr
	}
	return io.Discard
}

// buildExecCommand translates the neutral command description into os/exec configuration.
func buildExecCommand(command Command) (*exec.Cmd, error) {
	if len(command.Args) > 0 && strings.TrimSpace(command.Shell) != "" {
		return nil, errors.New("dev process command cannot define both args and shell")
	}
	if len(command.Args) == 0 && strings.TrimSpace(command.Shell) == "" {
		return nil, errors.New("dev process command requires args or shell")
	}

	var cmd *exec.Cmd
	if len(command.Args) > 0 {
		if strings.TrimSpace(command.Args[0]) == "" {
			return nil, errors.New("dev process command executable is required")
		}
		cmd = exec.Command(command.Args[0], command.Args[1:]...)
	} else {
		name, args := shellArgs(command.Shell)
		cmd = exec.Command(name, args...)
	}
	cmd.Dir = command.Dir
	cmd.Env = commandEnvironment(command)
	cmd.Stdin = command.Stdin
	cmd.Stdout = command.Stdout
	cmd.Stderr = command.Stderr
	cmd.WaitDelay = defaultProcessWaitDelay
	if command.Interactive {
		if cmd.Stdin == nil {
			cmd.Stdin = os.Stdin
		}
		if cmd.Stdout == nil {
			cmd.Stdout = os.Stdout
		}
		if cmd.Stderr == nil {
			cmd.Stderr = os.Stderr
		}
	}
	if command.OnOutput != nil {
		outputMu := &sync.Mutex{}
		cmd.Stdout = &outputWriter{stream: StreamStdout, writer: cmd.Stdout, hook: command.OnOutput, mu: outputMu}
		cmd.Stderr = &outputWriter{stream: StreamStderr, writer: cmd.Stderr, hook: command.OnOutput, mu: outputMu}
	}
	return cmd, nil
}

// commandEnvironment merges explicit values over the inherited environment deterministically.
func commandEnvironment(command Command) []string {
	return mergeCommandEnvironment(os.Environ(), command.Env, command.ReplaceEnv, runtime.GOOS == "windows")
}

// mergeCommandEnvironment applies deterministic overrides using Windows' case-insensitive key rules when requested.
func mergeCommandEnvironment(inherited []string, overrides map[string]string, replace bool, caseInsensitive bool) []string {
	values := make(map[string]commandEnvironmentValue)
	normalize := func(key string) string {
		if caseInsensitive {
			return strings.ToUpper(key)
		}
		return key
	}
	if !replace {
		for _, item := range inherited {
			key, value, ok := strings.Cut(item, "=")
			if ok {
				values[normalize(key)] = commandEnvironmentValue{key: key, value: value}
			}
		}
	}
	overrideKeys := make([]string, 0, len(overrides))
	for key := range overrides {
		overrideKeys = append(overrideKeys, key)
	}
	sort.Strings(overrideKeys)
	for _, key := range overrideKeys {
		values[normalize(key)] = commandEnvironmentValue{key: key, value: overrides[key]}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		value := values[key]
		environment = append(environment, value.key+"="+value.value)
	}
	return environment
}

// Write forwards output before invoking the hook with an immutable chunk copy.
func (w *outputWriter) Write(data []byte) (int, error) {
	if w.mu != nil {
		w.mu.Lock()
		defer w.mu.Unlock()
	}
	written := len(data)
	var err error
	if w.writer != nil {
		written, err = w.writer.Write(data)
	}
	if w.hook != nil && written > 0 {
		copyOfData := append([]byte(nil), data[:written]...)
		w.hook(Output{Stream: w.stream, Data: copyOfData})
	}
	return written, err
}

// beginWait reaps the child and invokes the completion callback exactly once.
func (p *managedProcess) beginWait(onExit func(Exit)) {
	go func() {
		err := p.cmd.Wait()
		p.mu.Lock()
		p.leaderExited = true
		p.mu.Unlock()
		cleanupErr := p.tree.cleanupAfterExit(p.cmd)
		if p.ptyDone != nil {
			timer := time.NewTimer(defaultPTYDrainTimeout)
			defer timer.Stop()
			select {
			case <-p.ptyDone:
			case <-timer.C:
				_ = p.ptyMaster.Close()
			}
		}
		cleanupErr = errors.Join(cleanupErr, cleanupCommand(p.command))
		exit := p.finish(errors.Join(err, cleanupErr))
		onExit(exit)
	}()
}

// cleanupCommand releases resources whose ownership was transferred to one command lifecycle.
func cleanupCommand(command Command) error {
	if command.cleanup == nil {
		return nil
	}
	return command.cleanup()
}

// finish records a child exit before releasing stop and wait callers.
func (p *managedProcess) finish(err error) Exit {
	p.mu.Lock()
	p.exited = true
	p.exit = Exit{
		Name:       p.name,
		PID:        p.cmd.Process.Pid,
		ExitCode:   p.cmd.ProcessState.ExitCode(),
		Err:        err,
		StartedAt:  p.startedAt,
		FinishedAt: time.Now(),
		StopReason: p.stopReason,
	}
	exit := p.exit
	close(p.done)
	p.mu.Unlock()
	return exit
}

// result waits for and returns the immutable process completion record.
func (p *managedProcess) result() Exit {
	<-p.done
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exit
}

// stop requests graceful process-tree termination and escalates after the timeout.
func (p *managedProcess) stop(ctx context.Context, reason StopReason, timeout time.Duration) error {
	if ctx == nil {
		ctx = context.Background()
	}
	p.mu.Lock()
	if p.exited {
		p.mu.Unlock()
		return nil
	}
	if !p.leaderExited && p.stopReason == "" {
		p.stopReason = reason
	}
	p.mu.Unlock()

	terminateErr := p.tree.terminate(p.cmd)
	var contextErr error
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-p.done:
			return ignoreProcessDoneError(terminateErr)
		case <-timer.C:
		case <-ctx.Done():
			contextErr = ctx.Err()
		}
	}

	killErr := p.tree.kill(p.cmd)
	waitTimeout := timeout
	if waitTimeout <= 0 {
		waitTimeout = defaultStopTimeout
	}
	waitTimer := time.NewTimer(waitTimeout)
	defer waitTimer.Stop()
	select {
	case <-p.done:
		return errors.Join(contextErr, ignoreProcessDoneError(terminateErr), ignoreProcessDoneError(killErr))
	case <-waitTimer.C:
		return errors.Join(
			contextErr,
			ignoreProcessDoneError(terminateErr),
			ignoreProcessDoneError(killErr),
			fmt.Errorf("process %q did not exit after forced shutdown", p.name),
		)
	case <-ctx.Done():
		return errors.Join(ctx.Err(), ignoreProcessDoneError(terminateErr), ignoreProcessDoneError(killErr))
	}
}

// newStartFailure creates an exit-shaped record for commands that never started.
func newStartFailure(name string, err error) Exit {
	now := time.Now()
	return Exit{
		Name:       name,
		ExitCode:   -1,
		Err:        err,
		StartedAt:  now,
		FinishedAt: now,
	}
}
