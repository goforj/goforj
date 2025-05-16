package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/lipgloss"
)

// Shadow-styled section header with emoji
func sectionHeader(title, emoji string) string {
	style := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D7D7D"))
	return style.Render(fmt.Sprintf("%s %s ›\n", emoji, title))
}

// Shadow-styled and bold App category header
func categoryHeader(category string) string {
	style := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D7D7D"))
	return style.Render(category)
}

// Custom Help Printer with structured output
func KongHelpFormatter(options kong.HelpOptions, ctx *kong.Context) error {
	out := os.Stdout
	node := ctx.Selected()
	if node == nil {
		node = ctx.Model.Node
	}

	if len(ctx.Model.Help) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, sectionHeader(ctx.Model.Help, ""))
	}

	application := make(map[string][]*kong.Node)
	generators := []*kong.Node{}
	migrations := []*kong.Node{}

	for _, child := range node.Children {
		if child.Type != kong.CommandNode || (child.Tag != nil && child.Tag.Hidden) {
			continue
		}

		name := child.Name

		switch {
		case strings.HasPrefix(name, "make:"):
			generators = append(generators, child)
		case strings.HasPrefix(name, "migrate:") || name == "migrate":
			migrations = append(migrations, child)
		default:
			if !strings.Contains(name, ":") {
				application["_ungrouped"] = append(application["_ungrouped"], child)
			} else {
				prefix := strings.SplitN(name, ":", 2)[0]
				application[prefix] = append(application[prefix], child)
			}
		}
	}

	// Generators Section
	if len(generators) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, sectionHeader("Generators", "🛠 "))
		renderAlignedCommands(out, generators)
		fmt.Fprintln(out) // Newline after section
	}

	// Migrations Section
	if len(migrations) > 0 {
		fmt.Fprintln(out, sectionHeader("Migrations", "🧱"))
		renderAlignedCommands(out, migrations)
		fmt.Fprintln(out) // Newline after section
	}

	// Application Section
	if len(application) > 0 {
		fmt.Fprintln(out, sectionHeader("App", "🚀"))

		// Flatten all commands and calculate max command length
		var allAppCommands []struct {
			Category string
			Name     string
			Help     string
		}
		maxLen := 0

		// Handle ungrouped commands first
		if ungrouped, exists := application["_ungrouped"]; exists {
			sortCommands(ungrouped)
			for _, cmd := range ungrouped {
				fmt.Fprintf(out, "  %s  %s\n", cmd.Name, cmd.Help)
			}
			fmt.Fprintln(out) // Newline after ungrouped section
			delete(application, "_ungrouped")
		}

		prefixes := sortedKeys(application)
		for _, prefix := range prefixes {
			for _, cmd := range application[prefix] {
				entry := struct {
					Category string
					Name     string
					Help     string
				}{Category: prefix, Name: cmd.Name, Help: cmd.Help}
				allAppCommands = append(allAppCommands, entry)
				if len(cmd.Name) > maxLen {
					maxLen = len(cmd.Name)
				}
			}
		}

		// Render by category with manual alignment
		currentCategory := ""
		for _, cmd := range allAppCommands {
			if cmd.Category != currentCategory {
				fmt.Fprintln(out, categoryHeader(cmd.Category))
				currentCategory = cmd.Category
			}
			// Manually pad based on maxLen
			spacing := strings.Repeat(" ", maxLen-len(cmd.Name)+2) // +2 for gap
			fmt.Fprintf(out, "  %s%s%s\n", cmd.Name, spacing, cmd.Help)
		}
	}

	return nil
}

// Renders aligned command names and descriptions
func renderAlignedCommands(out *os.File, cmds []*kong.Node) {
	sortCommands(cmds)
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, cmd := range cmds {
		fmt.Fprintf(w, "  %s\t%s\n", cmd.Name, cmd.Help)
	}
	w.Flush()
}

// Sort commands alphabetically
func sortCommands(cmds []*kong.Node) {
	sort.Slice(cmds, func(i, j int) bool {
		return cmds[i].Name < cmds[j].Name
	})
}

// Sorted keys helper
func sortedKeys(m map[string][]*kong.Node) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
