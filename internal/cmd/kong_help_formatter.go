package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/lipgloss"
	"github.com/goforj/str"
)

const (
	colorReset = "\033[0m"
	colorLime  = "\033[1;38;5;113m"
)

// Shadow-styled section header with emoji
func sectionHeader(title string) string {
	style := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF"))
	return style.Render(fmt.Sprintf("› %s", title))
}

// Shadow-styled and bold App category header
func categoryHeader(category string) string {
	style := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF"))
	return style.Render(category)
}

// KongHelpFormatter is a custom help formatter for Kong CLI that resembles Laravel's artisan help output.
func KongHelpFormatter(options kong.HelpOptions, ctx *kong.Context) error {
	out := os.Stdout
	node := ctx.Selected()
	if node == nil {
		node = ctx.Model.Node
	}
	maintainerHelp := maintainerHelpEnabled()

	// If the selected node is a specific command (not root), print its flags/help
	if node.Type == kong.CommandNode && node != ctx.Model.Node {
		fmt.Fprintln(out)
		fmt.Fprintln(out, sectionHeader(node.Help))

		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

		// Print positional arguments
		for _, pos := range node.Positional {
			fmt.Fprintf(w, "  %s\t%s\n", pos.Name, pos.Help)
		}

		// Print flags
		for _, flag := range node.Flags {
			if flag.Hidden {
				continue
			}
			name := "--" + flag.Name
			if flag.Short != 0 {
				name = fmt.Sprintf("-%c, %s", flag.Short, name)
			}
			fmt.Fprintf(w, "  %s\t%s\n", name, flag.Help)
		}
		w.Flush()
		fmt.Fprintln(out)
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

func commandVisibleInHelp(child *kong.Node, maintainerHelp bool) bool {
	if child == nil || child.Type != kong.CommandNode {
		return false
	}
	if child.Tag == nil || !child.Tag.Hidden {
		return true
	}
	return maintainerHelp && strings.HasPrefix(child.Name, "test:")
}

// Renders aligned command names and descriptions
func renderAlignedCommands(out *os.File, cmds []*kong.Node, maxLen int, indent string) {
	sortCommands(cmds)
	for _, cmd := range cmds {
		spacing := strings.Repeat(" ", maxLen-len(cmd.Name)+2)
		fmt.Fprintf(out, "%s%s%s%s%s\n", indent, colorLime, cmd.Name, colorReset, spacing+cmd.Help)
	}
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
