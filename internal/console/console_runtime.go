package console

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

type getenvFunc func(string) string
type isTerminalFunc func(int) bool
type exitFunc func(int)

// Config configures a Console instance.
type Config struct {
	Stdout io.Writer
	Stderr io.Writer

	// ColorEnabled forces color on/off when non-nil. Nil means auto-detect.
	ColorEnabled *bool
	// DebugEnabled forces debug on/off when non-nil. Nil means env-based.
	DebugEnabled *bool

	Getenv     getenvFunc
	IsTerminal isTerminalFunc
	Exit       exitFunc
}

// Console prints semantic CLI output with configurable colors and marks.
type Console struct {
	stdout io.Writer
	stderr io.Writer

	colorEnabled *bool
	debugEnabled *bool

	getenv getenvFunc
	isTTY  isTerminalFunc
	exit   exitFunc
}

// New creates a console with optional runtime overrides.
func New(cfg Config) *Console {
	c := &Console{
		stdout:       cfg.Stdout,
		stderr:       cfg.Stderr,
		colorEnabled: cfg.ColorEnabled,
		debugEnabled: cfg.DebugEnabled,
		getenv:       cfg.Getenv,
		isTTY:        cfg.IsTerminal,
		exit:         cfg.Exit,
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.stderr == nil {
		c.stderr = os.Stderr
	}
	if c.getenv == nil {
		c.getenv = os.Getenv
	}
	if c.isTTY == nil {
		c.isTTY = term.IsTerminal
	}
	if c.exit == nil {
		c.exit = os.Exit
	}
	return c
}

func (c *Console) ActionMark() string  { return c.mark(ColorGray, "·") }
func (c *Console) InfoMark() string    { return c.mark(ColorGray, "·") }
func (c *Console) SuccessMark() string { return c.mark(ColorGreen, "✔") }
func (c *Console) WarnMark() string    { return c.mark(ColorYellow, "!") }
func (c *Console) ErrorMark() string   { return c.mark(ColorRed, "✖") }
func (c *Console) DebugMark() string   { return c.mark(ColorGray, "?") }

func (c *Console) Actionf(format string, args ...any) {
	fmt.Fprintf(c.stdout, "%s %s\n", c.ActionMark(), fmt.Sprintf(format, args...))
}

func (c *Console) Infof(format string, args ...any) {
	fmt.Fprintf(c.stdout, "%s %s\n", c.InfoMark(), fmt.Sprintf(format, args...))
}

func (c *Console) Successf(format string, args ...any) {
	fmt.Fprintf(c.stdout, "%s %s\n", c.SuccessMark(), fmt.Sprintf(format, args...))
}

func (c *Console) Warnf(format string, args ...any) {
	fmt.Fprintf(c.stdout, "%s %s\n", c.WarnMark(), fmt.Sprintf(format, args...))
}

func (c *Console) Errorf(format string, args ...any) {
	fmt.Fprintf(c.stderr, "%s %s\n", c.ErrorMark(), fmt.Sprintf(format, args...))
}

func (c *Console) Fatalf(format string, args ...any) {
	c.Errorf(format, args...)
	c.exit(1)
}

func (c *Console) Debugf(format string, args ...any) {
	if !c.isDebugEnabled() {
		return
	}
	fmt.Fprintf(c.stdout, "%s %s\n", c.DebugMark(), fmt.Sprintf(format, args...))
}

func (c *Console) Colorize(color, value string) string {
	if !c.shouldColor() {
		return value
	}
	return fmt.Sprintf("%s%s%s", color, value, ColorReset)
}

func (c *Console) mark(color, symbol string) string {
	if !c.shouldColor() {
		return symbol
	}
	return fmt.Sprintf("%s%s%s", color, symbol, ColorReset)
}

func (c *Console) isDebugEnabled() bool {
	if c.debugEnabled != nil {
		return *c.debugEnabled
	}
	for _, key := range []string{"FORJ_DEBUG", "DEBUG"} {
		value := c.getenv(key)
		if value != "" && value != "0" {
			return true
		}
	}
	return false
}

func (c *Console) shouldColor() bool {
	if c.colorEnabled != nil {
		return *c.colorEnabled
	}
	if c.getenv("NO_COLOR") != "" {
		return false
	}
	if c.forceColor() {
		return true
	}
	out, ok := c.stdout.(*os.File)
	if !ok {
		return false
	}
	return c.isTTY(int(out.Fd()))
}

func (c *Console) forceColor() bool {
	value := c.getenv("CLICOLOR_FORCE")
	return value != "" && value != "0"
}
