package konghelp

import (
	"fmt"
	"io"
	"strings"

	"github.com/alecthomas/kong"
)

// renderFrameworkFormatter preserves the app-operation grouping expected by framework CLIs.
func renderFrameworkFormatter(out io.Writer, _ kong.HelpOptions, ctx *kong.Context) {
	node := selectedNode(ctx)
	maintainerHelp := maintainerHelpEnabled()

	// If the selected node is a command, print its flags/help. Standalone
	// preboot commands are both the selected command and the parser root.
	if node.Type == kong.CommandNode && (node != ctx.Model.Node || len(node.Children) == 0) {
		PrintCommandHelp(out, node)
		return
	}

	printRootHelpHeader(out, ctx.Model.Help, node.Name)

	sections := make(map[string][]*kong.Node)
	for _, child := range node.Children {
		if !commandVisibleInHelp(child, maintainerHelp) {
			continue
		}

		section := commandNamespace(child)
		sections[section] = append(sections[section], child)
	}

	maxLen := maxCommandLen(sections)
	for _, section := range sortedKeys(sections) {
		fmt.Fprintln(out, categoryHeader(section))
		renderAlignedCommands(out, sections[section], maxLen, "  ")
	}
}

// printRootHelpHeader keeps custom Kong Help text authoritative when commands provide it.
func printRootHelpHeader(out io.Writer, modelHelp string, modelName string) {
	if help := strings.TrimSpace(modelHelp); help != "" {
		for index, line := range strings.Split(help, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if index == 0 {
				fmt.Fprintln(out, sectionHeader(line))
				continue
			}
			fmt.Fprintln(out, helpDescription(line))
		}
		fmt.Fprintln(out)
		return
	}
	if name := strings.TrimSpace(modelName); name != "" {
		fmt.Fprintln(out, sectionHeader(name))
		fmt.Fprintln(out)
	}
}

// commandNamespace groups category-style commands without requiring every command to set a group tag.
func commandNamespace(child *kong.Node) string {
	if child == nil {
		return "app"
	}
	if child.Tag != nil {
		if group := strings.TrimSpace(child.Tag.Group); group != "" {
			return group
		}
	}
	name := strings.TrimSpace(child.Name)
	if name == "migrate" {
		return "migrate"
	}
	if prefix, _, ok := strings.Cut(name, ":"); ok && prefix != "" {
		return prefix
	}
	return "app"
}
