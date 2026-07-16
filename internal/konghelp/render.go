package konghelp

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/goforj/console"
	"github.com/goforj/str"
)

// sectionHeader centralizes section styling so all formatter variants stay visually consistent.
func sectionHeader(title string) string {
	return console.Colorize(console.ColorBoldWhite, title)
}

// categoryHeader shares the section treatment for command groups and usage headings.
func categoryHeader(category string) string {
	return console.Colorize(console.ColorBoldWhite, category)
}

// helpIdentifier gives arguments and flags a distinct treatment from prose.
func helpIdentifier(value string) string {
	return console.Colorize(console.ColorCyan, value)
}

// helpCommand highlights copyable command text across all formatter variants.
func helpCommand(value string) string {
	return console.Colorize(console.ColorBoldGreen, value)
}

// helpDescription mutes explanatory text so command names remain the visual anchor.
func helpDescription(value string) string {
	return console.Colorize(console.ColorGray, value)
}

// selectedNode falls back to the model root because standalone preboot parsers can leave no selected node.
func selectedNode(ctx *kong.Context) *kong.Node {
	node := ctx.Selected()
	if node == nil {
		node = ctx.Model.Node
	}
	return node
}

// maintainerHelpEnabled exposes hidden diagnostic commands only in explicit framework-development contexts.
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

// commandVisibleInHelp keeps internal test and scenario commands out of normal user help.
func commandVisibleInHelp(child *kong.Node, maintainerHelp bool) bool {
	if child == nil || child.Type != kong.CommandNode {
		return false
	}
	if child.Tag == nil || !child.Tag.Hidden {
		return true
	}
	return maintainerHelp && (strings.HasPrefix(child.Name, "test:") || strings.HasPrefix(child.Name, "scenario:"))
}

// helpRow carries already-derived command metadata through the shared table renderer.
type helpRow struct {
	name string
	help string
}

// PrintCommandHelp renders detailed help for a single command node.
func PrintCommandHelp(out io.Writer, node *kong.Node) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, sectionHeader(node.Help))
	renderHelpRows(out, commandHelpRows(node), "  ")
	if detail := strings.TrimSpace(node.Detail); detail != "" {
		fmt.Fprintln(out)
		renderCommandDetail(out, detail)
	}
	fmt.Fprintln(out)
}

// commandHelpRows filters hidden flags before any formatter renders detailed command help.
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

// renderHelpRows aligns metadata columns so mixed short and long flags remain easy to scan.
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

// renderCommandDetail styles known sections while preserving user-authored help text.
func renderCommandDetail(out io.Writer, detail string) {
	for _, line := range strings.Split(detail, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.EqualFold(strings.TrimSuffix(trimmed, ":"), "examples") {
			fmt.Fprintln(out, console.Colorize(console.ColorBoldWhite, trimmed))
			continue
		}
		fmt.Fprintln(out, line)
	}
}

// renderAlignedCommands normalizes command-list spacing across root help layouts.
func renderAlignedCommands(out io.Writer, cmds []*kong.Node, maxLen int, indent string) {
	sortCommands(cmds)
	for _, cmd := range cmds {
		spacing := strings.Repeat(" ", maxLen-len(cmd.Name)+2)
		fmt.Fprintf(out, "%s%s%s%s\n", indent, helpCommand(cmd.Name), spacing, helpDescription(cmd.Help))
	}
}

// sortCommands makes generated help deterministic across map and registration order changes.
func sortCommands(cmds []*kong.Node) {
	sort.Slice(cmds, func(i, j int) bool {
		return cmds[i].Name < cmds[j].Name
	})
}

// sortedKeys keeps framework command categories stable between runs.
func sortedKeys(m map[string][]*kong.Node) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// maxCommandLen lets root and command-specific tables share one alignment calculation.
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
