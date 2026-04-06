package build

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/goforj/goforj/internal/logger"
)

type RunCmd struct {
	pipeline Pipeline
	Timings  bool     `help:"Print per-step timings for generate, api index, and go run"`
	Root     string   `help:"Project root to run" default:"."`
	Args     []string `arg:"" optional:"" passthrough:"" help:"Arguments passed through to the app after go run ."`
	waitCh   chan error
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
	if err := c.pipeline.Run(c.Root, "run", Step{
		Name: c.launchCommand(c.runArgs()),
		Run:  c.runBinary,
	}, RunOptions{Timings: c.Timings}); err != nil {
		return err
	}
	if c.waitCh == nil {
		return nil
	}
	if err := <-c.waitCh; err != nil {
		return fmt.Errorf("go run: %w", err)
	}
	return nil
}

func (c *RunCmd) runBinary() (string, error) {
	args := c.runArgs()
	cmd := exec.Command("go", append([]string{"run"}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("go run: %w", err)
	}
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()
	c.waitCh = waitCh
	return "started", nil
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
