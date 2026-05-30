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
)

const localAppHelpTimeout = 10 * time.Second

var errNoGeneratedApp = errors.New("generated app not found")

var cliDefaultedEnv = map[string]bool{}

func setCLIDefaultEnv(key, value string) {
	if _, ok := os.LookupEnv(key); ok {
		return
	}
	_ = os.Setenv(key, value)
	cliDefaultedEnv[key] = true
}

func isGeneratedAppDir() bool {
	if !regularFileExists(".goforj.yml") {
		return false
	}
	return regularFileExists("go.mod")
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

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

func localAppEnv() []string {
	env := os.Environ()
	if len(cliDefaultedEnv) == 0 {
		return env
	}

	filtered := env[:0]
	for _, entry := range env {
		key, _, ok := strings.Cut(entry, "=")
		if ok && cliDefaultedEnv[key] {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}
