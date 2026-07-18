package build

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/goforj/goforj/internal/apiindex"
	"github.com/goforj/goforj/internal/logger"
	"golang.org/x/term"
)

// RunCmd runs a generated app from source after framework generation steps complete.
type RunCmd struct {
	pipeline Pipeline
	Timings  bool `help:"Print per-step timings for generate, API indexing, compilation, and app start"`
	// APIIndexStrict fails before app start when API indexing reports warnings or errors.
	APIIndexStrict    bool     `name:"api-index-strict" help:"Fail when API indexing reports warnings or errors"`
	Root              string   `help:"Project root to run" default:"."`
	Args              []string `arg:"" optional:"" passthrough:"" help:"Arguments passed through to the compiled app"`
	Env               []string `kong:"-"`
	PreserveTTY       bool     `kong:"-"`
	waitCh            chan error
	process           *os.Process
	outputGate        *firstOutputGate
	transientProgress bool
}

// NewRunCmd creates the command that compiles and launches an app from its selected source package.
func NewRunCmd(logger *logger.AppLogger, apiIndex apiindex.Preparer) *RunCmd {
	return &RunCmd{
		pipeline: NewPipeline(logger, apiIndex),
	}
}

// Signature returns CLI metadata for the compiled app command.
func (*RunCmd) Signature() string {
	return `name:"run" help:"Run generate, API indexing, then compile and start the app"`
}

// Run executes generation, API indexing, and the generated app command.
func (c *RunCmd) Run() error {
	root, err := resolveProjectRoot(c.Root)
	if err != nil {
		return err
	}
	c.waitCh = nil
	c.process = nil
	c.outputGate = nil
	c.transientProgress = shouldUseTransientRunProgress(c.Timings)
	runArgs, err := c.runArgsAt(root)
	if err != nil {
		return err
	}
	if err := c.pipeline.Run(root, "run", Step{
		Name: c.launchCommand(runArgs),
		Run:  c.runBinary,
	}, RunOptions{
		Timings:                  c.Timings,
		APIIndexStrict:           c.APIIndexStrict,
		TransientProgress:        c.transientProgress,
		ClearProgressBeforeFinal: shouldClearRunProgressBeforeFinal(c.transientProgress, c.shouldPreserveTTY()),
	}); err != nil {
		return c.handlePipelineError(err)
	}
	if c.outputGate != nil {
		c.outputGate.Release()
	}
	if err := c.waitForRunProcess(); err != nil {
		if code, ok := exitCodeFromError(err); ok {
			return ChildExitError{Code: code, Err: err}
		}
		return fmt.Errorf("run app: %w", err)
	}
	return nil
}

// runBinary starts the source app process without waiting for it to finish.
func (c *RunCmd) runBinary(root string) (string, error) {
	args, err := c.runArgsAt(root)
	if err != nil {
		return "", err
	}
	prepared, err := c.preflightBinary(root, args[0])
	if err != nil {
		return "", err
	}
	cmd := exec.Command(prepared.executable, args[1:]...)
	cmd.Dir = root
	if c.shouldPreserveTTY() {
		cmd.Stdin = os.Stdin
	}
	var gate *firstOutputGate
	if c.transientProgress && !c.shouldPreserveTTY() {
		gate = newFirstOutputGate()
		c.outputGate = gate
		cmd.Stdout = gate.Writer(os.Stdout)
		cmd.Stderr = gate.Writer(os.Stderr)
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if c.Env != nil {
		cmd.Env = c.Env
	}
	if err := cmd.Start(); err != nil {
		prepared.cleanup()
		return "", fmt.Errorf("start compiled app: %w", err)
	}
	c.process = cmd.Process
	waitCh := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		prepared.cleanup()
		waitCh <- err
	}()
	c.waitCh = waitCh
	if gate != nil {
		select {
		case <-gate.First():
		case err := <-waitCh:
			doneCh := make(chan error, 1)
			doneCh <- err
			c.waitCh = doneCh
			return "exited", nil
		case <-time.After(2 * time.Second):
		}
	}
	return "started", nil
}

// preparedRunBinary owns the temporary executable produced by compilation preflight.
type preparedRunBinary struct {
	executable string
}

// cleanup removes the one-shot executable after start failure or process exit.
func (p preparedRunBinary) cleanup() {
	_ = os.RemoveAll(filepath.Dir(p.executable))
}

// preflightBinary proves the selected run package compiles before a candidate API contract can be published.
func (c *RunCmd) preflightBinary(root string, packagePath string) (preparedRunBinary, error) {
	outputDir, err := os.MkdirTemp("", "forj-run-preflight-")
	if err != nil {
		return preparedRunBinary{}, fmt.Errorf("prepare app compilation: %w", err)
	}
	prepared := preparedRunBinary{executable: filepath.Join(outputDir, "app")}

	cmd := exec.Command("go", "build", "-o", prepared.executable, packagePath)
	cmd.Dir = root
	if c.Env != nil {
		cmd.Env = c.Env
	}
	output, err := cmd.CombinedOutput()
	if err == nil {
		return prepared, nil
	}
	prepared.cleanup()
	if detail := strings.TrimSpace(string(output)); detail != "" {
		printBuildFailureDetail(detail)
	}
	return preparedRunBinary{}, fmt.Errorf("compile app target: %w", err)
}

// handlePipelineError releases output and terminates any app started before a later publication error surfaced.
func (c *RunCmd) handlePipelineError(pipelineErr error) error {
	if c.outputGate != nil {
		c.outputGate.Release()
	}
	if err := c.terminateStartedProcess(); err != nil {
		return errors.Join(pipelineErr, fmt.Errorf("clean up started app after pipeline failure: %w", err))
	}
	return pipelineErr
}

// terminateStartedProcess kills the exact compiled app child and drains Cmd.Wait so failures cannot leak it or leave a zombie.
func (c *RunCmd) terminateStartedProcess() error {
	if c.process == nil {
		return nil
	}
	process := c.process
	if err := process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	if c.waitCh != nil {
		<-c.waitCh
	}
	c.process = nil
	c.waitCh = nil
	return nil
}

// shouldPreserveTTY reports whether the generated app command should keep terminal streams attached.
func (c *RunCmd) shouldPreserveTTY() bool {
	return c.PreserveTTY || len(c.Args) > 0
}

// shouldClearRunProgressBeforeFinal reports whether progress should clear before the app owns the TTY.
func shouldClearRunProgressBeforeFinal(transientProgress bool, preserveTTY bool) bool {
	return transientProgress && preserveTTY
}

// waitForRunProcess waits for the app process and forwards interrupts to it.
func (c *RunCmd) waitForRunProcess() error {
	if c.waitCh == nil {
		return errors.New("wait for app process: process was not started")
	}
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	return c.waitForRunProcessSignals(signals)
}

// waitForRunProcessSignals keeps signal delivery injectable while preserving the delegated App's final status.
func (c *RunCmd) waitForRunProcessSignals(signals <-chan os.Signal) error {
	var forwarded bool
	for {
		select {
		case err := <-c.waitCh:
			c.waitCh = nil
			c.process = nil
			return err
		case sig := <-signals:
			if c.outputGate != nil {
				c.outputGate.Release()
			}
			clearInterruptEcho()
			if c.process != nil && !forwarded {
				forwarded = true
				_ = c.process.Signal(sig)
			}
		}
	}
}

// ChildExitError reports that the app process exited non-zero after its output was already streamed.
type ChildExitError struct {
	Code int
	Err  error
}

// Error returns the child process failure message.
func (e ChildExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("child process exited with code %d", e.Code)
	}
	return e.Err.Error()
}

// Unwrap returns the underlying process error.
func (e ChildExitError) Unwrap() error {
	return e.Err
}

// ChildExitCode extracts the child process exit code from an error returned by RunCmd.
func ChildExitCode(err error) (int, bool) {
	var childErr ChildExitError
	if errors.As(err, &childErr) {
		return childErr.Code, true
	}
	return 0, false
}

// exitCodeFromError extracts an exit code from an exec process error.
func exitCodeFromError(err error) (int, bool) {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return 128 + int(status.Signal()), true
		}
		return exitErr.ExitCode(), true
	}
	return 0, false
}

// runArgs returns the selected source package followed by arguments for its compiled app.
func (c *RunCmd) runArgs() ([]string, error) {
	return c.runArgsAt(c.Root)
}

// runArgsAt returns App arguments against the validated project root shared by every pipeline step.
func (c *RunCmd) runArgsAt(root string) ([]string, error) {
	packagePath, err := resolveDefaultAppPackage(root)
	if err != nil {
		return nil, err
	}
	args := make([]string, 0, len(c.Args)+1)
	args = append(args, packagePath)
	args = append(args, c.Args...)
	return args, nil
}

// launchCommand formats the command shown in pipeline progress.
func (c *RunCmd) launchCommand(args []string) string {
	return "compile and start " + strings.Join(args, " ")
}

// shouldUseTransientRunProgress reports whether pipeline output should clear before app output.
func shouldUseTransientRunProgress(timings bool) bool {
	if buildProgressEnabled() || debugEnabled() || timings {
		return false
	}
	return term.IsTerminal(int(os.Stderr.Fd()))
}

// clearInterruptEcho removes the terminal interrupt line before forwarded app shutdown output.
func clearInterruptEcho() {
	if term.IsTerminal(int(os.Stderr.Fd())) {
		fmt.Fprint(os.Stderr, "\r\x1b[2K")
	}
}

// firstOutputGate holds app output until transient pipeline progress can be cleared.
type firstOutputGate struct {
	first       chan struct{}
	release     chan struct{}
	firstOnce   sync.Once
	releaseOnce sync.Once
}

// newFirstOutputGate creates a gate for delaying the first app output.
func newFirstOutputGate() *firstOutputGate {
	return &firstOutputGate{
		first:   make(chan struct{}),
		release: make(chan struct{}),
	}
}

// First is closed when the app writes its first byte of output.
func (g *firstOutputGate) First() <-chan struct{} {
	return g.first
}

// Release allows buffered app output to continue to the terminal.
func (g *firstOutputGate) Release() {
	g.releaseOnce.Do(func() {
		close(g.release)
	})
}

// Writer wraps dst so writes signal first output and wait for release.
func (g *firstOutputGate) Writer(dst io.Writer) io.Writer {
	return firstOutputWriter{gate: g, dst: dst}
}

// signalFirst records that app output has started.
func (g *firstOutputGate) signalFirst() {
	g.firstOnce.Do(func() {
		close(g.first)
	})
}

// firstOutputWriter delays app output until its gate is released.
type firstOutputWriter struct {
	gate *firstOutputGate
	dst  io.Writer
}

// Write signals first output, waits for release, then forwards bytes.
func (w firstOutputWriter) Write(p []byte) (int, error) {
	w.gate.signalFirst()
	<-w.gate.release
	return w.dst.Write(p)
}
