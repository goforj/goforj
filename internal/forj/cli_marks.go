package forj

import "fmt"

func infoMark() string {
	return colorMark(colorGray, "·")
}

// successMark returns the success indicator.
func successMark() string {
	return colorMark(colorGreen, "✔")
}

// warnMark returns the warning indicator.
func warnMark() string {
	return colorMark(colorYellow, "!")
}

// errorMark returns the error indicator.
func errorMark() string {
	return colorMark(colorRed, "✖")
}

// actionMark returns the action indicator.
func actionMark() string {
	return colorMark(colorCyan, "»")
}

// colorMark wraps a symbol in the provided ANSI color.
func colorMark(color, symbol string) string {
	return fmt.Sprintf("%s%s%s", color, symbol, colorReset)
}
