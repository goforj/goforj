package build

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/goforj/goforj/internal/logger"
)

type RunCmd struct {
	pipeline Pipeline
	Root     string   `help:"Project root to run" default:"."`
	Args     []string `arg:"" optional:"" passthrough:"" help:"Arguments passed through to go run"`
}

func NewRunCmd(logger *logger.AppLogger, apiIndex *APIIndexRunner) *RunCmd {
	return &RunCmd{
		pipeline: NewPipeline(logger, apiIndex),
	}
}

func (*RunCmd) Signature() string {
	return `name:"run" help:"Run generate, API indexing, then go run"`
}

func (c *RunCmd) Run() error {
	return c.pipeline.Run(c.Root, "run", Step{
		Name: "go run",
		Run:  c.runBinary,
	})
}

func (c *RunCmd) runBinary() error {
	args := c.runArgs()
	cmd := exec.Command("go", append([]string{"run"}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go run: %w", err)
	}
	return nil
}

func (c *RunCmd) runArgs() []string {
	if len(c.Args) == 0 {
		return []string{"."}
	}
	return c.Args
}
