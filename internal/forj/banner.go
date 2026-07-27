package forj

import (
	"fmt"
	"os"
	"strings"

	"github.com/goforj/console"
	"golang.org/x/term"
)

var (
	// The Temper display ramp keeps CLI wordmarks visually aligned with the docs without assigning them an action or navigation color.
	bannerGradientStart = [3]int{255, 198, 133}
	bannerGradientEnd   = [3]int{255, 138, 61}
)

// printNewProjectBanner gives the interactive project flow a recognizable entry point before Bubble Tea takes over.
func printNewProjectBanner() {
	fmt.Println("")
	lines := []string{
		"  ██████╗  ██████╗ ███████╗ ██████╗ ██████╗      ██╗",
		" ██╔════╝ ██╔═══██╗██╔════╝██╔═══██╗██╔══██╗     ██║",
		" ██║  ███╗██║   ██║█████╗  ██║   ██║██████╔╝     ██║",
		" ██║   ██║██║   ██║██╔══╝  ██║   ██║██╔══██╗██   ██║",
		" ╚██████╔╝╚██████╔╝██║     ╚██████╔╝██║  ██║╚█████╔╝",
		"  ╚═════╝  ╚═════╝ ╚═╝      ╚═════╝ ╚═╝  ╚═╝ ╚════╝ ",
	}
	for _, line := range lines {
		fmt.Println(colorizeBannerLine(line))
	}
	fmt.Println("")
}

// colorizeBannerLine preserves a readable bold fallback when true-color output is unavailable.
func colorizeBannerLine(line string) string {
	if !bannerColorsEnabled() {
		return console.Colorize(console.ColorBoldWhite, line)
	}

	return colorizeGradientLine(line, true)
}

// colorizeGradientLine keeps ASCII wordmarks on one shared brand ramp without coupling CLI rendering to web CSS.
func colorizeGradientLine(line string, bold bool) string {
	if !bannerColorsEnabled() {
		return line
	}

	runes := []rune(line)
	paintable := 0
	for _, r := range runes {
		if r != ' ' {
			paintable++
		}
	}
	if paintable == 0 {
		return line
	}

	var b strings.Builder
	if bold {
		b.WriteString("\033[1m")
	}
	index := 0
	for _, r := range runes {
		if r == ' ' {
			b.WriteRune(r)
			continue
		}
		t := float64(index) / float64(maxInt(1, paintable-1))
		c := lerpRGB(bannerGradientStart, bannerGradientEnd, t)
		b.WriteString(fmt.Sprintf("\033[38;2;%d;%d;%dm", c[0], c[1], c[2]))
		b.WriteRune(r)
		index++
	}
	b.WriteString("\033[0m")
	return b.String()
}

// lerpRGB makes the terminal ramp continuous across glyphs instead of rendering visible color bands.
func lerpRGB(a, b [3]int, t float64) [3]int {
	return [3]int{
		int(float64(a[0]) + (float64(b[0])-float64(a[0]))*t),
		int(float64(a[1]) + (float64(b[1])-float64(a[1]))*t),
		int(float64(a[2]) + (float64(b[2])-float64(a[2]))*t),
	}
}

// bannerColorsEnabled honors established terminal color controls so banners remain script-friendly.
func bannerColorsEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if value := os.Getenv("CLICOLOR_FORCE"); value != "" && value != "0" {
		return true
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// maxInt prevents single-glyph gradients from dividing by zero.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
