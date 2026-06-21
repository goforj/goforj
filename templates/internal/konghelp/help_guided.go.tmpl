package konghelp

import (
	"fmt"
	"io"
	"strings"

	"github.com/alecthomas/kong"
)

// renderGuidedFormatter puts examples first for CLIs where discovery matters more than command density.
func renderGuidedFormatter(out io.Writer, _ kong.HelpOptions, ctx *kong.Context) {
	node := selectedNode(ctx)
	maintainerHelp := maintainerHelpEnabled()

	if node.Type == kong.CommandNode && (node != ctx.Model.Node || len(node.Children) == 0) {
		PrintGuidedCommandHelp(out, node)
		return
	}

	printGuidedRootHelp(out, ctx.Model.Help, node, maintainerHelp)
}

// printGuidedRootHelp leads with examples so first-time users can copy a valid command immediately.
func printGuidedRootHelp(out io.Writer, modelHelp string, node *kong.Node, maintainerHelp bool) {
	name := strings.TrimSpace(node.Name)
	if name != "" {
		fmt.Fprintln(out, sectionHeader(name))
	}
	if help := strings.TrimSpace(modelHelp); help != "" {
		fmt.Fprintln(out, helpDescription(firstHelpLine(help)))
	}

	if examples := examplesFromDetail(node.Detail); len(examples) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, categoryHeader("Examples"))
		renderExamples(out, examples)
	}

	if name != "" {
		fmt.Fprintln(out)
		fmt.Fprintln(out, categoryHeader("Usage"))
		fmt.Fprintf(out, "  %s %s\n", helpCommand(name), helpDescription("<command> [flags]"))
	}

	commands := visibleCommandChildren(node, maintainerHelp)
	if len(commands) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, categoryHeader("Common commands"))
		renderAlignedCommands(out, commands, maxCommandLen(commands), "  ")
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, categoryHeader("Flags"))
	renderHelpRows(out, []helpRow{helpRow{name: "-h, --help", help: "Show help"}}, "  ")

	fmt.Fprintln(out)
	fmt.Fprintln(out, categoryHeader("Learn more"))
	if name != "" {
		fmt.Fprintf(out, "  %s\n", helpDescription(name+" <command> --help"))
	}
}

// PrintGuidedCommandHelp renders examples-first command help.
func PrintGuidedCommandHelp(out io.Writer, node *kong.Node) {
	fmt.Fprintln(out, sectionHeader(node.FullPath()))
	if help := strings.TrimSpace(node.Help); help != "" {
		fmt.Fprintln(out, helpDescription(help))
	}

	if examples := examplesFromDetail(node.Detail); len(examples) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, categoryHeader("Examples"))
		renderExamples(out, examples)
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, categoryHeader("Usage"))
	fmt.Fprintf(out, "  %s\n", helpCommand(commandUsage(node)))

	if len(node.Positional) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, categoryHeader("Arguments"))
		renderExternalCLIPositionals(out, node.Positional)
	}

	rows := commandHelpRows(node)
	if len(rows) > len(node.Positional) {
		fmt.Fprintln(out)
		fmt.Fprintln(out, categoryHeader("Flags"))
		rows = withHelpRow(rows[len(node.Positional):])
		renderHelpRows(out, rows, "  ")
	} else {
		fmt.Fprintln(out)
		fmt.Fprintln(out, categoryHeader("Flags"))
		renderHelpRows(out, []helpRow{helpRow{name: "-h, --help", help: "Show help"}}, "  ")
	}
}

// withHelpRow ensures the guided layout always exposes the universal help affordance.
func withHelpRow(rows []helpRow) []helpRow {
	for _, row := range rows {
		if strings.Contains(row.name, "--help") {
			return rows
		}
	}
	return append(rows, helpRow{name: "-h, --help", help: "Show help"})
}

// commandUsage builds a stable usage line from Kong metadata instead of duplicating signatures by hand.
func commandUsage(node *kong.Node) string {
	var parts []string
	path := strings.TrimSpace(node.FullPath())
	if path != "" {
		parts = append(parts, path)
	}
	for _, positional := range node.Positional {
		parts = append(parts, positional.Summary())
	}
	hasVisibleFlags := false
	for _, flag := range node.Flags {
		if !flag.Hidden {
			hasVisibleFlags = true
			break
		}
	}
	if hasVisibleFlags {
		parts = append(parts, "[flags]")
	}
	return strings.Join(parts, " ")
}

// renderExamples keeps copied examples visually distinct from surrounding prose.
func renderExamples(out io.Writer, examples []string) {
	for _, example := range examples {
		example = strings.TrimSpace(example)
		if example == "" {
			continue
		}
		fmt.Fprintf(out, "  %s\n", helpCommand(example))
	}
}

// examplesFromDetail reuses Kong Help detail text as the single source for examples.
func examplesFromDetail(detail string) []string {
	var examples []string
	inExamples := false
	for _, line := range strings.Split(detail, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if inExamples && len(examples) > 0 {
				break
			}
			continue
		}
		if strings.EqualFold(strings.TrimSuffix(trimmed, ":"), "examples") {
			inExamples = true
			continue
		}
		if !inExamples {
			continue
		}
		if strings.HasSuffix(trimmed, ":") {
			break
		}
		examples = append(examples, strings.TrimPrefix(trimmed, "$ "))
	}
	return examples
}
