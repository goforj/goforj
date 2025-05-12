package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/alecthomas/kong"
	"github.com/fatih/color"
)

// HelpGroup is a struct that represents a group of help commands.
type HelpGroup struct {
	Name   string
	Prefix string
	Nodes  []*kong.Node
}

// KongHelpFormatter is a custom help formatter for Kong CLI.
// It formats the help output in a more user-friendly way.
// It groups commands and options into sections.
func KongHelpFormatter(options kong.HelpOptions, ctx *kong.Context) error {
	out := os.Stdout

	node := ctx.Selected()
	if node == nil {
		node = ctx.Model.Node
	}

	gray := color.New(color.FgHiBlack).FprintfFunc()
	bold := color.New(color.FgWhite, color.Bold).FprintfFunc()

	// Print the description if available
	if out != nil && len(ctx.Model.Help) > 0 {
		fmt.Fprintln(out)
		bold(out, "%s\n", ctx.Model.Help)
	}

	// Handle leaf node options
	if len(node.Children) == 0 && len(node.Flags) > 0 {
		gray(out, "\nOptions ❯\n")

		// Setup tabwriter for dynamic alignment
		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		for _, flag := range node.Flags {
			if flag.Tag != nil && flag.Tag.Hidden {
				continue
			}

			var names []string
			if flag.Short != 0 {
				names = append(names, fmt.Sprintf("-%c", flag.Short))
			}
			if flag.Name != "" {
				names = append(names, "--"+flag.Name)
			}

			// Write names and help with tab separation
			fmt.Fprintf(w, "  %s\t%s\n", strings.Join(names, ", "), flag.Help)
		}
		w.Flush()
		return nil
	}

	// Group subcommands
	groups := []HelpGroup{
		{Name: "🛠  Generators", Prefix: "make:"},
		{Name: "🛠  Dev", Prefix: "dev"},
		{Name: "🧱 Migrations", Prefix: "migrate"},
		{Name: "🚀 App", Prefix: ""},
	}

	// Organize child commands into groups
	for _, child := range node.Children {
		if child.Type != kong.CommandNode || (child.Tag != nil && child.Tag.Hidden) {
			continue
		}

		matched := false
		for i := range groups {
			if groups[i].Prefix != "" && strings.HasPrefix(child.Name, groups[i].Prefix) {
				groups[i].Nodes = append(groups[i].Nodes, child)
				matched = true
				break
			}
		}
		if !matched {
			for i := range groups {
				if groups[i].Prefix == "" {
					groups[i].Nodes = append(groups[i].Nodes, child)
					break
				}
			}
		}
	}

	// Render grouped commands
	for _, group := range groups {
		if len(group.Nodes) == 0 {
			continue
		}

		sort.Slice(group.Nodes, func(i, j int) bool {
			return group.Nodes[i].Name < group.Nodes[j].Name
		})

		fmt.Fprintln(out)
		gray(out, "%s ❯\n", group.Name)

		// Setup tabwriter for dynamic alignment
		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		for _, cmd := range group.Nodes {
			fmt.Fprintf(w, "  %s\t%s\n", cmd.Name, cmd.Help)
		}
		w.Flush()
	}

	return nil
}
