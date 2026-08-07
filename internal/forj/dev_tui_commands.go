package forj

import (
	"os"
	"os/exec"
	"strings"
)

// loadDevAppCommands centralizes load dev app commands lookup for the surrounding workflow.
func loadDevAppCommands() ([]devAppCommandOption, string) {
	output, err := runDevAppHelp("--help")
	if err != nil {
		return nil, "app help unavailable"
	}
	commands := parseDevAppHelpCommands(output)
	if len(commands) == 0 {
		return nil, "no app commands detected"
	}
	for i := range commands {
		commands[i].AcceptsArgs = loadDevAppCommandAcceptsArgs(commands[i].Name)
	}
	return commands, ""
}

// loadDevAppCommandAcceptsArgs centralizes load dev app command accepts args lookup for the surrounding workflow.
func loadDevAppCommandAcceptsArgs(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	output, err := runDevAppHelp(name, "--help")
	if err != nil {
		return false
	}
	return parseDevAppCommandAcceptsArgs(output)
}

// runDevAppHelp centralizes run dev app help behavior so callers follow the same contract.
func runDevAppHelp(args ...string) (string, error) {
	cmd := exec.Command(activeDevAppBinaryPath(), args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	return string(output), err
}
