package build

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goforj/goforj/internal/logger"
)

// Cmd runs the forj build pipeline.
type Cmd struct {
	logger   *logger.AppLogger
	pipeline Pipeline
	Timings  bool `help:"Print per-step timings for generate, api index, and go build"`
	SkipWire bool `help:"Skip running wire before build" hidden:""`
	AutoRun  bool   `help:"Build binary so launching it with no args runs the app runtime command"`
	DefaultLaunch string `help:"Set compiled default command used when the built binary is launched without args"`

	// Profile flags.
	Profile bool `help:"Profile compile time for this build"`
	Top     int  `help:"Limit profile results" default:"12"`

	Root           string   `help:"Project root to build" default:"."`
	Args           []string `arg:"" optional:"" passthrough:"" help:"Arguments passed through to go build"`
	compileProfile CompileProfileReport
	lastBuildStatus string
	goGetFunc      func([]string) error
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
	if err := c.validateLaunchDefaults(); err != nil {
		return err
	}
	if err := c.pipeline.Run(c.Root, "build", Step{
		Name: "go build",
		Run:  c.buildBinary,
	}, RunOptions{Timings: c.Timings, SkipWire: c.SkipWire}); err != nil {
		return err
	}
	if c.Profile {
		return c.printProfile()
	}
	return nil
}

func (c *Cmd) buildBinary() (string, error) {
	args := c.buildArgs()
	if outIndex := outputArgIndex(args); outIndex >= 0 {
		if err := os.MkdirAll(filepath.Dir(outputPath(args[outIndex])), 0o755); err != nil {
			return "", err
		}
	}
	if c.Profile {
		return c.buildBinaryWithProfile(args)
	}
	return c.runPlainGoBuild(args)
}

func (c *Cmd) buildArgs() []string {
	defaultLaunch := c.effectiveDefaultLaunch()
	modulePath := ""
	if defaultLaunch != "" {
		modulePath = c.modulePath()
	}
	if len(c.Args) == 0 {
		args := []string{"-o", "./bin/app"}
		if defaultLaunch != "" {
			args = append(args, "-ldflags", c.defaultLaunchLdflags(modulePath, defaultLaunch))
		}
		return append(args, ".")
	}
	args := append([]string{}, c.Args...)
	if defaultLaunch == "" {
		return args
	}
	return c.withDefaultLaunchLdflags(args, modulePath, defaultLaunch)
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

func (c *Cmd) validateLaunchDefaults() error {
	if c.AutoRun && strings.TrimSpace(c.DefaultLaunch) != "" && strings.TrimSpace(c.DefaultLaunch) != "run" {
		return fmt.Errorf("--auto-run and --default-launch must agree; got default-launch=%q", c.DefaultLaunch)
	}
	if launch := strings.TrimSpace(c.DefaultLaunch); launch != "" && strings.ContainsAny(launch, " \t\r\n") {
		return fmt.Errorf("--default-launch must be a single command token, got %q", c.DefaultLaunch)
	}
	if launch := c.effectiveDefaultLaunch(); launch != "" && c.modulePath() == "" {
		return fmt.Errorf("could not resolve module path from %s/go.mod for default launch %q", strings.TrimSpace(c.Root), launch)
	}
	return nil
}

func (c *Cmd) effectiveDefaultLaunch() string {
	if launch := strings.TrimSpace(c.DefaultLaunch); launch != "" {
		return launch
	}
	if c.AutoRun {
		return "run"
	}
	return ""
}

func (c *Cmd) defaultLaunchLdflags(modulePath, defaultLaunch string) string {
	return fmt.Sprintf("-X %s/internal/cmd.DefaultLaunchCommand=%s", modulePath, defaultLaunch)
}

func (c *Cmd) withDefaultLaunchLdflags(args []string, modulePath, defaultLaunch string) []string {
	ldflagsValue := c.defaultLaunchLdflags(modulePath, defaultLaunch)
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-ldflags" {
			args[i+1] = strings.TrimSpace(args[i+1] + " " + ldflagsValue)
			return args
		}
	}
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-ldflags=") {
			current := strings.TrimPrefix(args[i], "-ldflags=")
			args[i] = "-ldflags=" + strings.TrimSpace(current+" "+ldflagsValue)
			return args
		}
	}
	return append([]string{"-ldflags", ldflagsValue}, args...)
}

func (c *Cmd) modulePath() string {
	root := c.Root
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	f, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}
