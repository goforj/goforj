package testkit

import (
	"fmt"
	"strings"

	"github.com/goforj/console"
)

// ConsoleLogf centralizes console logf behavior so callers follow the same contract.
func ConsoleLogf(silent bool) Logf {
	if silent {
		return nil
	}
	return console.Infof
}

// PrintSection centralizes print section behavior so callers follow the same contract.
func PrintSection(title string) {
	title = strings.TrimSpace(title)
	if title == "" {
		return
	}
	fmt.Printf("\n%s %s\n", console.Colorize(console.ColorCyan, "◇"), console.Colorize(console.ColorBoldWhite, title))
}

// PrintSubsection centralizes print subsection behavior so callers follow the same contract.
func PrintSubsection(title string) {
	title = strings.TrimSpace(title)
	if title == "" {
		return
	}
	fmt.Printf("%s\n", console.Colorize(console.ColorGray, "  "+title))
}
