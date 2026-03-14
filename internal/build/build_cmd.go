package build

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/goforj/goforj/internal/logger"
)

// Cmd runs the forj build pipeline.
type Cmd struct {
	logger   *logger.AppLogger
	pipeline Pipeline
	Timings  bool     `help:"Print per-step timings for generate, api index, and go build"`
	SkipWire bool     `help:"Skip running wire before build" hidden:""`
	Root     string   `help:"Project root to build" default:"."`
	Args     []string `arg:"" optional:"" passthrough:"" help:"Arguments passed through to go build"`
}

func NewCmd(logger *logger.AppLogger, apiIndex *APIIndexRunner) *Cmd {
	return &Cmd{
		logger:   logger,
		pipeline: NewPipeline(logger, apiIndex),
	}
}

func (*Cmd) Signature() string {
	return `name:"build" help:"Run generate, API indexing, then go build" group:"build"`
}

func (c *Cmd) Run() error {
	return c.pipeline.Run(c.Root, "build", Step{
		Name: "go build",
		Run:  c.buildBinary,
	}, RunOptions{Timings: c.Timings, SkipWire: c.SkipWire})
}

func (c *Cmd) buildBinary() (string, error) {
	args := c.buildArgs()
	if outIndex := outputArgIndex(args); outIndex >= 0 {
		if err := os.MkdirAll(filepath.Dir(outputPath(args[outIndex])), 0o755); err != nil {
			return "", err
		}
	}

	cmd := exec.Command("go", append([]string{"build"}, args...)...)
	if debugEnabled() {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("go build: %w", err)
		}
		return "", nil
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail != "" {
			return "", fmt.Errorf("go build: %w (%s)", err, detail)
		}
		return "", fmt.Errorf("go build: %w", err)
	}
	return "", nil
}

func (c *Cmd) buildArgs() []string {
	if len(c.Args) == 0 {
		return []string{"-o", "./bin/app", "."}
	}
	return c.Args
}

func outputArgIndex(args []string) int {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-o" {
			return i + 1
		}
	}
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-o=") {
			return i
		}
	}
	return -1
}

func outputPath(arg string) string {
	if strings.HasPrefix(arg, "-o=") {
		return strings.TrimPrefix(arg, "-o=")
	}
	return arg
}
