package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	appcmd "github.com/goforj/goforj/internal/cmd"
	"golang.org/x/term"
)

// localAppHelpTimeout bounds source-run help discovery for generated apps.
const localAppHelpTimeout = 10 * time.Second

// errNoGeneratedApp indicates that the current directory cannot receive delegated app commands.
var errNoGeneratedApp = errors.New("generated app not found")

// cliDefaultedEnv tracks environment values supplied by the framework CLI before app delegation.
var cliDefaultedEnv = map[string]bool{}

// localAppStdoutIsTerminal reports whether delegated app output should inherit terminal color behavior.
var localAppStdoutIsTerminal = func() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// setCLIDefaultEnv sets a default environment variable and tracks that GoForj supplied it.
func setCLIDefaultEnv(key, value string) {
	if _, ok := os.LookupEnv(key); ok {
		return
	}
	_ = os.Setenv(key, value)
	cliDefaultedEnv[key] = true
}

// isGeneratedAppDir reports whether the current directory has generated App markers.
func isGeneratedAppDir() bool {
	if !regularFileExists(".goforj.yml") {
		return false
	}
	return regularFileExists("go.mod")
}

// regularFileExists reports whether path exists and is not a directory.
func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// localAppHelp returns generated App help from the current source tree when available.
func localAppHelp() (string, bool) {
	if !isGeneratedAppDir() {
		return "", false
	}

	for _, args := range [][]string{{"--help"}, {}} {
		if help, ok := localAppHelpForArgs(args); ok {
			return help, true
		}
	}
	return "", false
}

// localAppHelpForArgs runs the generated App with args and captures non-empty help output.
func localAppHelpForArgs(args []string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), localAppHelpTimeout)
	defer cancel()

	var out bytes.Buffer
	command := exec.CommandContext(ctx, "go", append([]string{"run", "."}, args...)...)
	command.Env = localAppEnv()
	command.Stdout = &out
	command.Stderr = &out

	if err := command.Run(); err != nil {
		return "", false
	}

	help := strings.TrimSpace(out.String())
	if help == "" {
		return "", false
	}
	return help, true
}

// printRootHelp prints native GoForj help plus generated App help when available.
func printRootHelp(parser *kong.Kong) {
	ctx, _ := kong.Trace(parser, []string{})
	ctx.PrintUsage(false)

	if help, ok := localAppHelp(); ok {
		fmt.Println()
		fmt.Println("› App commands (current source)")
		fmt.Println()
		fmt.Println(help)
	} else if isGeneratedAppDir() {
		fmt.Println()
		fmt.Println("› App commands")
		fmt.Println()
		fmt.Println("Unknown commands are delegated to this app through `forj run <command>`.")
		fmt.Println("Use `forj run <command>` to force app command execution.")
	}
}

// isRootHelp reports whether args request the root help screen.
func isRootHelp(args []string) bool {
	if len(args) == 0 {
		return true
	}
	if len(args) != 1 {
		return false
	}
	switch args[0] {
	case "--help", "-h", "help":
		return true
	default:
		return false
	}
}

// shouldDelegateToAppCommand reports whether a parse error should fall through to the generated App.
func shouldDelegateToAppCommand(args []string, parseErr error) bool {
	if parseErr == nil || len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return false
	}
	if !isGeneratedAppDir() {
		return false
	}

	message := strings.ToLower(parseErr.Error())
	return strings.Contains(message, "unexpected argument") ||
		strings.Contains(message, "unknown command")
}

// runAppCommandThroughSource runs a generated App command through the source-aware run path.
func runAppCommandThroughSource(root *appcmd.RootCmd, args []string) error {
	if !isGeneratedAppDir() {
		return errNoGeneratedApp
	}
	run := &root.RootCmd.RunCmd
	run.Root = "."
	run.Args = append([]string(nil), args...)
	run.Env = localAppEnv()
	return run.Run()
}

// localAppEnv returns the environment used for generated App source execution.
func localAppEnv() []string {
	env := os.Environ()
	if len(cliDefaultedEnv) != 0 {
		filtered := env[:0]
		for _, entry := range env {
			key, _, ok := strings.Cut(entry, "=")
			if ok && cliDefaultedEnv[key] {
				continue
			}
			filtered = append(filtered, entry)
		}
		env = filtered
	}

	env = withLocalAppColorEnv(env)
	env = withLocalAppCommandPrefixEnv(env)
	return env
}

// withLocalAppColorEnv preserves color for delegated app commands when the parent stdout is a terminal.
func withLocalAppColorEnv(env []string) []string {
	if !localAppStdoutIsTerminal() || envListHasKey(env, "NO_COLOR") || envListHasKey(env, "CLICOLOR_FORCE") {
		return env
	}
	return append(env, "CLICOLOR_FORCE=1")
}

// withLocalAppCommandPrefixEnv lets generated app help show the delegated forj entrypoint.
func withLocalAppCommandPrefixEnv(env []string) []string {
	if envListHasKey(env, "FORJ_COMMAND_PREFIX") {
		return env
	}
	return append(env, "FORJ_COMMAND_PREFIX=forj")
}

// envListHasKey reports whether env contains a variable by name.
func envListHasKey(env []string, key string) bool {
	for _, entry := range env {
		if entry == key {
			return true
		}
		name, _, ok := strings.Cut(entry, "=")
		if ok && name == key {
			return true
		}
	}
	return false
}
