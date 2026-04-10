package testkit

import (
	"fmt"
	"strings"

	"github.com/goforj/goforj/internal/console"
)

func ConsoleLogf(silent bool) Logf {
	if silent {
		return nil
	}
	return console.Infof
}

func PrintSection(title string) {
	title = strings.TrimSpace(title)
	if title == "" {
		return
	}
	fmt.Printf("\n%s %s\n", console.Colorize(console.ColorCyan, "◇"), console.Colorize(console.ColorBoldWhite, title))
}

func PrintSubsection(title string) {
	title = strings.TrimSpace(title)
	if title == "" {
		return
	}
	fmt.Printf("%s\n", console.Colorize(console.ColorGray, "  "+title))
}
