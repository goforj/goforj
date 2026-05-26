package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alecthomas/kong"
)

const localAppHelpTimeout = 2 * time.Second

var errNoLocalApp = errors.New("local app binary not found")

var cliDefaultedEnv = map[string]bool{}

func setCLIDefaultEnv(key, value string) {
	if _, ok := os.LookupEnv(key); ok {
		return
	}
	_ = os.Setenv(key, value)
	cliDefaultedEnv[key] = true
}

func localAppBinary() (string, bool) {
	path := filepath.Join(".", "bin", "app")
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", false
	}
	if info.Mode()&0111 == 0 {
		return "", false
	}
	return path, true
}

func localAppHelp() (string, bool) {
	path, ok := localAppBinary()
	if !ok {
		return "", false
	}

	for _, args := range [][]string{{"--help"}, {}} {
		if help, ok := localAppHelpForArgs(path, args); ok {
			return help, true
		}
	}
	return "", false
}

func localAppHelpForArgs(path string, args []string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), localAppHelpTimeout)
	defer cancel()

	var out bytes.Buffer
	command := exec.CommandContext(ctx, path, args...)
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
		fmt.Println("› App commands (./bin/app)")
		fmt.Println()
		fmt.Println(help)
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

func shouldPassThroughToLocalApp(args []string, parseErr error) bool {
	if parseErr == nil || len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return false
	}
	if _, ok := localAppBinary(); !ok {
		return false
	}

	message := strings.ToLower(parseErr.Error())
	return strings.Contains(message, "unexpected argument") ||
		strings.Contains(message, "unknown command")
}

func runLocalApp(args []string) error {
	path, ok := localAppBinary()
	if !ok {
		return errNoLocalApp
	}

	command := exec.Command(path, args...)
	command.Env = localAppEnv()
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
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
