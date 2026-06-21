package konghelp

import (
	"fmt"
	"io"
	"strings"

	"github.com/alecthomas/kong"
)

// renderExternalCLIFormatter favors a compact command list for product-facing CLIs.
func renderExternalCLIFormatter(out io.Writer, _ kong.HelpOptions, ctx *kong.Context) {
	node := selectedNode(ctx)
	maintainerHelp := maintainerHelpEnabled()

	if node.Type == kong.CommandNode && (node != ctx.Model.Node || len(node.Children) == 0) {
		PrintExternalCLICommandHelp(out, node)
		return
	}

	printExternalCLIRootHelp(out, ctx.Model.Help, node, maintainerHelp)
}

// printExternalCLIRootHelp avoids framework categories when the binary is the product surface.
func printExternalCLIRootHelp(out io.Writer, modelHelp string, node *kong.Node, maintainerHelp bool) {
	name := strings.TrimSpace(node.Name)
	if name != "" {
		fmt.Fprintln(out, sectionHeader(name))
	}
	if help := strings.TrimSpace(modelHelp); help != "" {
		fmt.Fprintln(out, helpDescription(firstHelpLine(help)))
	}
	if name != "" {
		fmt.Fprintln(out)
		fmt.Fprintln(out, categoryHeader("Usage"))
		fmt.Fprintf(out, "  %s %s\n", helpCommand(name), helpDescription("<command> [flags]"))
	}

	commands := visibleCommandChildren(node, maintainerHelp)
	if len(commands) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, categoryHeader("Commands"))
		renderAlignedCommands(out, commands, maxCommandLen(commands), "  ")
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, helpDescription("Run \""+name+" <command> --help\" for command details."))
}

// PrintExternalCLICommandHelp renders detailed command help with explicit usage and flags.
func PrintExternalCLICommandHelp(out io.Writer, node *kong.Node) {
	fmt.Fprintln(out, sectionHeader(node.Name))
	if help := strings.TrimSpace(node.Help); help != "" {
		fmt.Fprintln(out, helpDescription(help))
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, categoryHeader("Usage"))
	fmt.Fprintf(out, "  %s\n", helpCommand(commandUsage(node)))

	if len(node.Positional) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, categoryHeader("Arguments"))
		renderExternalCLIPositionals(out, node.Positional)
	}
	if rows := commandHelpRows(node); len(rows) > len(node.Positional) {
		fmt.Fprintln(out)
		fmt.Fprintln(out, categoryHeader("Flags"))
		renderHelpRows(out, rows[len(node.Positional):], "  ")
	}
	if detail := strings.TrimSpace(node.Detail); detail != "" {
		fmt.Fprintln(out)
		renderCommandDetail(out, detail)
	}
}

// renderExternalCLIPositionals separates arguments from flags so command help scans like common Unix tools.
func renderExternalCLIPositionals(out io.Writer, positionals []*kong.Positional) {
	rows := make([]helpRow, 0, len(positionals))
	for _, positional := range positionals {
		rows = append(rows, helpRow{name: positional.Name, help: positional.Help})
	}
	renderHelpRows(out, rows, "  ")
}

// visibleCommandChildren applies the same hidden-command policy before sorting root command lists.
func visibleCommandChildren(node *kong.Node, maintainerHelp bool) []*kong.Node {
	commands := make([]*kong.Node, 0, len(node.Children))
	for _, child := range node.Children {
		if commandVisibleInHelp(child, maintainerHelp) {
			commands = append(commands, child)
		}
	}
	sortCommands(commands)
	return commands
}

// firstHelpLine keeps root help concise when Kong detail text spans multiple lines.
func firstHelpLine(help string) string {
	for _, line := range strings.Split(help, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}
