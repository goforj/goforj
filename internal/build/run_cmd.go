package build

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/goforj/goforj/internal/logger"
	"golang.org/x/term"
)

type RunCmd struct {
	pipeline          Pipeline
	Timings           bool     `help:"Print per-step timings for generate, api index, and go run"`
	Root              string   `help:"Project root to run" default:"."`
	Args              []string `arg:"" optional:"" passthrough:"" help:"Arguments passed through to the app after go run ."`
	Env               []string `kong:"-"`
	waitCh            chan error
	process           *os.Process
	outputGate        *firstOutputGate
	transientProgress bool
}

func NewRunCmd(logger *logger.AppLogger, apiIndex *APIIndexRunner) *RunCmd {
	return &RunCmd{
		pipeline: NewPipeline(logger, apiIndex),
	}
}

func (*RunCmd) Signature() string {
	return `name:"run" help:"Run generate, API indexing, then go run ."`
}

func (c *RunCmd) Run() error {
	c.waitCh = nil
	c.process = nil
	c.outputGate = nil
	c.transientProgress = shouldUseTransientRunProgress(c.Timings)
	if err := c.pipeline.Run(c.Root, "run", Step{
		Name: c.launchCommand(c.runArgs()),
		Run:  c.runBinary,
	}, RunOptions{Timings: c.Timings, TransientProgress: c.transientProgress}); err != nil {
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
		return fmt.Errorf("go run: %w", err)
	}
	return nil
}

func (c *RunCmd) runBinary() (string, error) {
	args := c.runArgs()
	cmd := exec.Command("go", append([]string{"run"}, args...)...)
	var gate *firstOutputGate
	if c.transientProgress {
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

func (c *RunCmd) waitForRunProcess() error {
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

func (c *RunCmd) runArgs() []string {
	args := make([]string, 0, len(c.Args)+1)
	args = append(args, ".")
	args = append(args, c.Args...)
	return args
}

func (c *RunCmd) launchCommand(args []string) string {
	return "go run " + strings.Join(args, " ")
}

func shouldUseTransientRunProgress(timings bool) bool {
	if buildProgressEnabled() || debugEnabled() || timings {
		return false
	}
	return term.IsTerminal(int(os.Stderr.Fd()))
}

func clearInterruptEcho() {
	if term.IsTerminal(int(os.Stderr.Fd())) {
		fmt.Fprint(os.Stderr, "\r\x1b[2K")
	}
}

type firstOutputGate struct {
	first       chan struct{}
	release     chan struct{}
	firstOnce   sync.Once
	releaseOnce sync.Once
}

func newFirstOutputGate() *firstOutputGate {
	return &firstOutputGate{
		first:   make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (g *firstOutputGate) First() <-chan struct{} {
	return g.first
}

func (g *firstOutputGate) Release() {
	g.releaseOnce.Do(func() {
		close(g.release)
	})
}

func (g *firstOutputGate) Writer(dst io.Writer) io.Writer {
	return firstOutputWriter{gate: g, dst: dst}
}

func (g *firstOutputGate) signalFirst() {
	g.firstOnce.Do(func() {
		close(g.first)
	})
}

type firstOutputWriter struct {
	gate *firstOutputGate
	dst  io.Writer
}

func (w firstOutputWriter) Write(p []byte) (int, error) {
	w.gate.signalFirst()
	<-w.gate.release
	return w.dst.Write(p)
}
