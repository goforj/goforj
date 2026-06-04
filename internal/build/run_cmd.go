package build

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/goforj/goforj/internal/logger"
	"golang.org/x/term"
)

// RunCmd runs a generated app from source after framework generation steps complete.
type RunCmd struct {
	pipeline          Pipeline
	Timings           bool     `help:"Print per-step timings for generate, api index, and go run"`
	Root              string   `help:"Project root to run" default:"."`
	Args              []string `arg:"" optional:"" passthrough:"" help:"Arguments passed through to the app after go run ./cmd/app"`
	Env               []string `kong:"-"`
	PreserveTTY       bool     `kong:"-"`
	waitCh            chan error
	process           *os.Process
	outputGate        *firstOutputGate
	stderrFilter      *goRunExitStatusFilter
	transientProgress bool
}

// NewRunCmd creates the source-run command.
func NewRunCmd(logger *logger.AppLogger, apiIndex *APIIndexRunner) *RunCmd {
	return &RunCmd{
		pipeline: NewPipeline(logger, apiIndex),
	}
}

// Signature returns CLI metadata for the source-run command.
func (*RunCmd) Signature() string {
	return `name:"run" help:"Run generate, API indexing, then go run ./cmd/app"`
}

// Run executes generation, API indexing, and the generated app command.
func (c *RunCmd) Run() error {
	c.waitCh = nil
	c.process = nil
	c.outputGate = nil
	c.stderrFilter = nil
	c.transientProgress = shouldUseTransientRunProgress(c.Timings)
	if err := c.pipeline.Run(c.Root, "run", Step{
		Name: c.launchCommand(c.runArgs()),
		Run:  c.runBinary,
	}, RunOptions{
		Timings:                  c.Timings,
		TransientProgress:        c.transientProgress,
		ClearProgressBeforeFinal: shouldClearRunProgressBeforeFinal(c.transientProgress, c.shouldPreserveTTY()),
	}); err != nil {
		if c.outputGate != nil {
			c.outputGate.Release()
		}
		return err
	}
	if c.outputGate != nil {
		c.outputGate.Release()
	}
	if c.waitCh == nil {
		return nil
	}
	if err := c.waitForRunProcess(); err != nil {
		if code, ok := exitCodeFromError(err); ok {
			return ChildExitError{Code: code, Err: err}
		}
		return fmt.Errorf("go run: %w", err)
	}
	return nil
}

// runBinary starts the source app process without waiting for it to finish.
func (c *RunCmd) runBinary() (string, error) {
	args := c.runArgs()
	cmd := exec.Command("go", append([]string{"run"}, args...)...)
	if c.shouldPreserveTTY() {
		cmd.Stdin = os.Stdin
	}
	var gate *firstOutputGate
	if c.transientProgress && !c.shouldPreserveTTY() {
		gate = newFirstOutputGate()
		c.outputGate = gate
		cmd.Stdout = gate.Writer(os.Stdout)
		filter := newGoRunExitStatusFilter(gate.Writer(os.Stderr))
		c.stderrFilter = filter
		cmd.Stderr = filter
	} else {
		cmd.Stdout = os.Stdout
		filter := newGoRunExitStatusFilter(os.Stderr)
		c.stderrFilter = filter
		cmd.Stderr = filter
	}
	if c.Env != nil {
		cmd.Env = c.Env
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("go run: %w", err)
	}
	c.process = cmd.Process
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
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
	defer c.closeStderrFilter()

	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	var forwarded bool
	for {
		select {
		case err := <-c.waitCh:
			if forwarded {
				return nil
			}
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

// closeStderrFilter flushes any pending stderr output held by the go run filter.
func (c *RunCmd) closeStderrFilter() {
	if c.stderrFilter == nil {
		return
	}
	_ = c.stderrFilter.Close()
	c.stderrFilter = nil
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
		return exitErr.ExitCode(), true
	}
	return 0, false
}

// runArgs returns the full go run argument list for the app command.
func (c *RunCmd) runArgs() []string {
	args := make([]string, 0, len(c.Args)+1)
	args = append(args, defaultRunPackage(c.Root))
	args = append(args, c.Args...)
	return args
}

// launchCommand formats the command shown in pipeline progress.
func (c *RunCmd) launchCommand(args []string) string {
	return "go run " + strings.Join(args, " ")
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

// defaultRunPackage keeps `forj run` pointed at the generated app entrypoint when one exists.
func defaultRunPackage(root string) string {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	if info, err := os.Stat(filepath.Join(root, "cmd", "app")); err == nil && info.IsDir() {
		return "./cmd/app"
	}
	return "."
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

// goRunExitStatusFilter removes Go tool exit-status echoes while preserving app stderr.
type goRunExitStatusFilter struct {
	dst io.Writer
	buf bytes.Buffer
}

// newGoRunExitStatusFilter creates an stderr writer for source-run app commands.
func newGoRunExitStatusFilter(dst io.Writer) *goRunExitStatusFilter {
	return &goRunExitStatusFilter{dst: dst}
}

// Write buffers stderr until complete lines can be checked for Go tool exit-status echoes.
func (w *goRunExitStatusFilter) Write(p []byte) (int, error) {
	for _, b := range p {
		_ = w.buf.WriteByte(b)
		if b != '\n' {
			continue
		}
		if err := w.flushLine(); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// Close flushes a final partial line if stderr did not end in a newline.
func (w *goRunExitStatusFilter) Close() error {
	if w.buf.Len() == 0 {
		return nil
	}
	return w.flushLine()
}

// flushLine writes the buffered line unless it is a Go tool exit-status echo.
func (w *goRunExitStatusFilter) flushLine() error {
	line := w.buf.String()
	w.buf.Reset()
	if isGoRunExitStatusLine(line) {
		return nil
	}
	_, err := io.WriteString(w.dst, line)
	return err
}

// isGoRunExitStatusLine reports whether line is exactly a Go tool exit-status echo.
func isGoRunExitStatusLine(line string) bool {
	value := strings.TrimSpace(line)
	if !strings.HasPrefix(value, "exit status ") {
		return false
	}
	code := strings.TrimSpace(strings.TrimPrefix(value, "exit status "))
	if code == "" {
		return false
	}
	_, err := strconv.Atoi(code)
	return err == nil
}
