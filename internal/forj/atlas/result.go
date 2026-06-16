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
