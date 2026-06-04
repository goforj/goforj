package makecmd

import (
	"os"
	"path/filepath"
	"strings"
)

// commandPrefixEnv lets delegated framework commands control help examples.
const commandPrefixEnv = "FORJ_COMMAND_PREFIX"

// commandExamples renders a Kong Help examples block.
func commandExamples(lines ...string) string {
	clean := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			clean = append(clean, line)
		}
	}
	return strings.TrimSpace("Examples:\n  " + strings.Join(clean, "\n  "))
}

// commandExample builds an example using the active CLI entrypoint.
func commandExample(command string, args ...string) string {
	parts := []string{currentCommandPrefix(), strings.TrimSpace(command)}
	for _, arg := range args {
		if arg = strings.TrimSpace(arg); arg != "" {
			parts = append(parts, arg)
		}
	}
	return strings.Join(parts, " ")
}

// currentCommandPrefix returns the user-facing command prefix for help examples.
func currentCommandPrefix() string {
	if prefix := strings.TrimSpace(os.Getenv(commandPrefixEnv)); prefix != "" {
		return prefix
	}
	command := strings.TrimSpace(os.Args[0])
	if command == "" {
		return "app"
	}
	normalized := filepath.ToSlash(command)
	if isGoRunCommandPath(normalized) {
		return "go run ./cmd/app"
	}
	return command
}

// isGoRunCommandPath reports whether os.Args[0] points at a go run build artifact.
func isGoRunCommandPath(command string) bool {
	if strings.Contains(command, "/go-build") && strings.Contains(command, "/exe/") {
		return true
	}
	if strings.Contains(command, "/gocache/") && strings.Contains(command, "-d/") {
		return true
	}
	return false
}
