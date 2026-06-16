package forj

import (
	"os"
	"os/exec"
	"strings"
)

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

func runDevAppHelp(args ...string) (string, error) {
	cmd := exec.Command(activeDevAppBinaryPath(), args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	return string(output), err
}
