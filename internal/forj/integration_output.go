package forj

import (
	"fmt"
	"strings"

	"github.com/goforj/goforj/internal/console"
)

func printIntegrationSection(title string) {
	title = strings.TrimSpace(title)
	if title == "" {
		return
	}
	fmt.Printf("\n%s %s\n", console.Colorize(console.ColorCyan, "◇"), console.Colorize(console.ColorBoldWhite, title))
}

func printIntegrationSubsection(title string) {
	title = strings.TrimSpace(title)
	if title == "" {
		return
	}
	fmt.Printf("%s\n", console.Colorize(console.ColorGray, "  "+title))
}

func summarizeIntegrationFailure(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return "command failed"
	}
	lines := strings.Split(output, "\n")
	summary := []string{}
	seen := map[string]bool{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "--- FAIL:") ||
			strings.HasPrefix(trimmed, "FAIL\t") ||
			strings.HasPrefix(trimmed, "FAIL ") ||
			strings.Contains(trimmed, "build forj binary:") ||
			strings.Contains(trimmed, "wire generate:") ||
			strings.Contains(trimmed, "no required module provides package") ||
			strings.Contains(trimmed, "executable file not found in $PATH") {
			if !seen[trimmed] {
				summary = append(summary, trimmed)
				seen[trimmed] = true
			}
		}
		if len(summary) >= 8 {
			break
		}
	}
	if len(summary) == 0 {
		limit := 8
		if len(lines) < limit {
			limit = len(lines)
		}
		return strings.Join(lines[:limit], "\n")
	}
	return strings.Join(summary, "\n")
}
