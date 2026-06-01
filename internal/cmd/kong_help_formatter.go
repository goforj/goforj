package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/str"
)

// sectionHeader formats a top-level help section title.
func sectionHeader(title string) string {
	return console.Colorize(console.ColorBoldWhite, fmt.Sprintf("› %s", title))
}

// categoryHeader formats a command category label.
func categoryHeader(category string) string {
	return console.Colorize(console.ColorBoldWhite, category)
}

// helpIdentifier formats positional and flag names in command help.
func helpIdentifier(value string) string {
	return console.Colorize(console.ColorCyan, value)
}

// helpCommand formats command names in command lists.
func helpCommand(value string) string {
	return console.Colorize(console.ColorBoldGreen, value)
}

// helpDescription formats muted descriptive help text.
func helpDescription(value string) string {
	return console.Colorize(console.ColorGray, value)
}

// KongHelpFormatter is a custom help formatter for Kong CLI that resembles Laravel's artisan help output.
func KongHelpFormatter(options kong.HelpOptions, ctx *kong.Context) error {
	out := os.Stdout
	node := ctx.Selected()
	if node == nil {
		node = ctx.Model.Node
	}
	maintainerHelp := maintainerHelpEnabled()

	// If the selected node is a command, print its flags/help. Standalone
	// skip-boot commands are both the selected command and the parser root.
	if node.Type == kong.CommandNode && (node != ctx.Model.Node || len(node.Children) == 0) {
		printCommandHelp(out, node)
		return nil
	}

	if len(ctx.Model.Help) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, sectionHeader(ctx.Model.Help))
		fmt.Fprintln(out)
	}

	sections := make(map[string][]*kong.Node)

	for _, child := range node.Children {
		if !commandVisibleInHelp(child, maintainerHelp) {
			continue
		}

		name := child.Name

		if child.Tag != nil && str.Of(child.Tag.Group).TrimSpace().String() != "" {
			group := str.Of(child.Tag.Group).TrimSpace().String()
			sections[group] = append(sections[group], child)
			continue
		}

		switch {
		case strings.HasPrefix(name, "make:"):
			sections["make"] = append(sections["make"], child)
		case strings.HasPrefix(name, "test:"):
			sections["test"] = append(sections["test"], child)
		default:
			if !strings.Contains(name, ":") {
				sections["app"] = append(sections["app"], child)
			} else {
				prefix := strings.SplitN(name, ":", 2)[0]
				sections[prefix] = append(sections[prefix], child)
			}
		}
	}

	maxLen := maxCommandLen(sections)
	printed := make(map[string]bool)
	order := []string{"app", "build", "make", "test"}
	for _, section := range order {
		if len(sections[section]) == 0 {
			continue
		}
		fmt.Fprintln(out, categoryHeader(section))
		renderAlignedCommands(out, sections[section], maxLen, "  ")
		printed[section] = true
	}

	rest := sortedKeys(sections)
	for _, section := range rest {
		if printed[section] {
			continue
		}
		fmt.Fprintln(out, categoryHeader(section))
		renderAlignedCommands(out, sections[section], maxLen, "  ")
	}

	return nil
}

// maintainerHelpEnabled reports whether hidden maintainer commands should be shown.
func maintainerHelpEnabled() bool {
	v := str.Of(os.Getenv("FORJ_DEV")).TrimSpace().ToLower().String()
	if v == "1" || v == "true" || v == "yes" || v == "on" {
		return true
	}
	for _, arg := range os.Args[1:] {
		if arg == "--dev" || arg == "--dev=true" || arg == "--x" || arg == "--x=true" {
			return true
		}
	}
	return false
}

// commandVisibleInHelp reports whether a command node belongs in normal or maintainer help.
func commandVisibleInHelp(child *kong.Node, maintainerHelp bool) bool {
	if child == nil || child.Type != kong.CommandNode {
		return false
	}
	if child.Tag == nil || !child.Tag.Hidden {
		return true
	}
	return maintainerHelp && (strings.HasPrefix(child.Name, "test:") || strings.HasPrefix(child.Name, "scenario:"))
}

// printCommandHelp renders detailed help for a single command node.
func printCommandHelp(out io.Writer, node *kong.Node) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, sectionHeader(node.Help))
	renderHelpRows(out, commandHelpRows(node), "  ")
	if detail := strings.TrimSpace(node.Detail); detail != "" {
		fmt.Fprintln(out)
		renderCommandDetail(out, detail)
	}
	fmt.Fprintln(out)
}

// helpRow represents one positional argument or flag row.
type helpRow struct {
	name string
	help string
}

// commandHelpRows returns visible positional arguments and flags for a command node.
func commandHelpRows(node *kong.Node) []helpRow {
	rows := make([]helpRow, 0, len(node.Positional)+len(node.Flags))
	for _, pos := range node.Positional {
		rows = append(rows, helpRow{name: pos.Name, help: pos.Help})
	}
	for _, flag := range node.Flags {
		if flag.Hidden {
			continue
		}
		name := "--" + flag.Name
		if flag.Short != 0 {
			name = fmt.Sprintf("-%c, %s", flag.Short, name)
		}
		rows = append(rows, helpRow{name: name, help: flag.Help})
	}
	return rows
}

// renderHelpRows prints aligned command help rows.
func renderHelpRows(out io.Writer, rows []helpRow, indent string) {
	maxLen := 0
	for _, row := range rows {
		if len(row.name) > maxLen {
			maxLen = len(row.name)
		}
	}
	for _, row := range rows {
		spacing := strings.Repeat(" ", maxLen-len(row.name)+2)
		fmt.Fprintf(out, "%s%s%s%s\n", indent, helpIdentifier(row.name), spacing, helpDescription(row.help))
	}
}

// renderCommandDetail prints extended command help with styled section labels.
func renderCommandDetail(out io.Writer, detail string) {
	for _, line := range strings.Split(detail, "\n") {
		trimmed := strings.TrimSpace(line)
		if isHelpDetailSection(trimmed) {
			fmt.Fprintln(out, console.Colorize(console.ColorBoldWhite, trimmed))
			continue
		}
		fmt.Fprintln(out, line)
	}
}

// isHelpDetailSection reports whether a detail line should be styled as a section label.
func isHelpDetailSection(line string) bool {
	return strings.EqualFold(strings.TrimSuffix(line, ":"), "examples")
}

// renderAlignedCommands prints command names and descriptions in aligned columns.
func renderAlignedCommands(out io.Writer, cmds []*kong.Node, maxLen int, indent string) {
	sortCommands(cmds)
	for _, cmd := range cmds {
		spacing := strings.Repeat(" ", maxLen-len(cmd.Name)+2)
		fmt.Fprintf(out, "%s%s%s%s\n", indent, helpCommand(cmd.Name), spacing, helpDescription(cmd.Help))
	}
}

// sortCommands sorts command nodes alphabetically by name.
func sortCommands(cmds []*kong.Node) {
	sort.Slice(cmds, func(i, j int) bool {
		return cmds[i].Name < cmds[j].Name
	})
}

// sortedKeys returns map keys sorted alphabetically.
func sortedKeys(m map[string][]*kong.Node) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// maxCommandLen returns the longest command name across command groups.
func maxCommandLen(groups ...interface{}) int {
	maxLen := 0
	for _, group := range groups {
		switch v := group.(type) {
		case []*kong.Node:
			for _, cmd := range v {
				if l := len(cmd.Name); l > maxLen {
					maxLen = l
				}
			}
		case map[string][]*kong.Node:
			for _, cmds := range v {
				for _, cmd := range cmds {
					if l := len(cmd.Name); l > maxLen {
						maxLen = l
					}
				}
			}
		}
	}
	return maxLen
}
