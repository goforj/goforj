package atlas

import (
	"fmt"
	"os"
	"strings"

	"github.com/goforj/atlas/install"
)

// printResult writes a compact summary of files touched by Atlas.
func printResult(prefix string, result install.Result) {
	if len(result.Agents) > 0 {
		fmt.Fprintf(os.Stderr, "%s for agents: %s\n", prefix, strings.Join(result.Agents, ", "))
	} else {
		fmt.Fprintf(os.Stderr, "%s\n", prefix)
	}
	for _, path := range result.Files {
		fmt.Fprintf(os.Stderr, "  %s\n", path)
	}
}

// printActivationHint explains when Codex applies newly written project MCP configuration.
func printActivationHint(result install.Result) {
	for _, agent := range result.Agents {
		if agent == "codex" {
			fmt.Fprintln(os.Stderr, "Start a new Codex thread to load Atlas MCP tools, then run `forj atlas:doctor` to verify readiness.")
			return
		}
	}
}
