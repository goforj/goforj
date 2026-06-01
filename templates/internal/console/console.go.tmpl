package console

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

const (
	// ColorReset resets ANSI color styling.
	ColorReset = "\033[0m"
	// ColorBoldWhite is a bold white ANSI color.
	ColorBoldWhite = "\033[1;97m"
	// ColorGray is a muted gray ANSI color.
	ColorGray = "\033[90m"
	// ColorGreen is a green ANSI color.
	ColorGreen = "\033[32m"
	// ColorBoldGreen is a bold green ANSI color.
	ColorBoldGreen = "\033[1;32m"
	// ColorYellow is a yellow ANSI color.
	ColorYellow = "\033[33m"
	// ColorRed is a red ANSI color.
	ColorRed = "\033[31m"
	// ColorCyan is a cyan ANSI color.
	ColorCyan = "\033[36m"
)

// ActionMark returns the action indicator.
func ActionMark() string {
	return colorMark(ColorCyan, "»")
}

// InfoMark returns the info indicator.
func InfoMark() string {
	return colorMark(ColorGray, "·")
}

// SuccessMark returns the success indicator.
func SuccessMark() string {
	return colorMark(ColorGreen, "✔")
}

// WarnMark returns the warning indicator.
func WarnMark() string {
	return colorMark(ColorYellow, "!")
}

// ErrorMark returns the error indicator.
func ErrorMark() string {
	return colorMark(ColorRed, "✖")
}

// DebugMark returns the debug indicator.
func DebugMark() string {
	return colorMark(ColorGray, "?")
}

// Actionf prints an action message.
func Actionf(format string, args ...any) {
	fmt.Printf("%s %s\n", ActionMark(), fmt.Sprintf(format, args...))
}

// Infof prints an informational message.
func Infof(format string, args ...any) {
	fmt.Printf("%s %s\n", InfoMark(), fmt.Sprintf(format, args...))
}

// Successf prints a success message.
func Successf(format string, args ...any) {
	fmt.Printf("%s %s\n", SuccessMark(), fmt.Sprintf(format, args...))
}

// Warnf prints a warning message.
func Warnf(format string, args ...any) {
	fmt.Printf("%s %s\n", WarnMark(), fmt.Sprintf(format, args...))
}

// Errorf prints an error message.
func Errorf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s %s\n", ErrorMark(), fmt.Sprintf(format, args...))
}

// Fatalf prints an error message and exits with status 1.
func Fatalf(format string, args ...any) {
	Errorf(format, args...)
	os.Exit(1)
}

// Debugf prints a debug message when debug mode is enabled.
func Debugf(format string, args ...any) {
	if !debugEnabled() {
		return
	}
	fmt.Printf("%s %s\n", DebugMark(), fmt.Sprintf(format, args...))
}

// Colorize applies an ANSI color to a string when color output is enabled.
func Colorize(color, value string) string {
	if !shouldColor() {
		return value
	}
	return fmt.Sprintf("%s%s%s", color, value, ColorReset)
}

// colorMark wraps a symbol in the provided ANSI color.
func colorMark(color, symbol string) string {
	if !shouldColor() {
		return symbol
	}
	return fmt.Sprintf("%s%s%s", color, symbol, ColorReset)
}

// debugEnabled reports whether debug output is enabled.
func debugEnabled() bool {
	for _, key := range []string{"FORJ_DEBUG", "APP_DEBUG", "DEBUG"} {
		value := os.Getenv(key)
		if value != "" && value != "0" {
			return true
		}
	}
	return false
}

// shouldColor reports whether ANSI color should be used.
func shouldColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if forceColor() {
		return true
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func forceColor() bool {
	for _, key := range []string{"CLICOLOR_FORCE"} {
		value := os.Getenv(key)
		if value != "" && value != "0" {
			return true
		}
	}
	return false
}
