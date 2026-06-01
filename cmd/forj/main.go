package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/goforj/goforj/internal/build"
	"github.com/goforj/goforj/internal/cmd"
	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/version"
	"github.com/goforj/goforj/wire"
)

var cliDefaultedEnv = map[string]bool{}

// main initializes the framework CLI and delegates unknown App commands when appropriate.
func main() {
	if build.HandleProfileTool(os.Args[1:]) {
		return
	}

	// Default environment
	setCLIDefaultEnv("APP_ENV", "local")
	setCLIDefaultEnv("APP_NAME", "GoForj")

	// Initialize application
	app, err := wire.InitializeApplication()
	if err != nil {
		console.Fatalf("initializing application: %v", err)
	}
	app.Logger().Debug().Msg("App initialized")

	// Setup Kong parser
	parser, err := kong.New(
		app.RootCmd(),
		kong.Name("goforj"),
		kong.Description("GoForj CLI\n  The composable stack for building with Go."),
		kong.Help(cmd.KongHelpFormatter),
		kong.Vars{
			"version": version.String(),
		},
	)
	if err != nil {
		console.Fatalf("setting up CLI parser: %v", err)
	}
	app.RootCmd().RootCmd.RunCmd.Env = delegatedAppEnv()

	args := os.Args[1:]
	inGeneratedApp := isGeneratedAppDir()
	if isRootHelp(args) {
		printRootHelp(parser, inGeneratedApp)
		return
	}

	// Parse CLI args
	ctx, err := parser.Parse(args)
	if err != nil {
		if shouldDelegateToAppCommand(args, err, inGeneratedApp) {
			if err := runAppCommandThroughSource(app.RootCmd(), args); err != nil {
				if code, ok := build.ChildExitCode(err); ok {
					os.Exit(code)
				}
				console.Fatalf("%v", err)
			}
			return
		}
		console.Fatalf("%v", err)
	}

	// Execute command
	err = ctx.Run()
	if err != nil {
		if code, ok := build.ChildExitCode(err); ok {
			os.Exit(code)
		}
		console.Fatalf("%v", err)
	}
}

// setCLIDefaultEnv sets a framework default and records that it should not leak into delegated App commands.
func setCLIDefaultEnv(key, value string) {
	if _, ok := os.LookupEnv(key); ok {
		return
	}
	_ = os.Setenv(key, value)
	cliDefaultedEnv[key] = true
}

// isRootHelp reports whether args request the root help screen.
func isRootHelp(args []string) bool {
	return len(args) == 0 || len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help")
}

// printRootHelp prints native GoForj help and notes App delegation when running inside a generated App.
func printRootHelp(parser *kong.Kong, inGeneratedApp bool) {
	ctx, _ := kong.Trace(parser, []string{})
	ctx.PrintUsage(false)

	if inGeneratedApp {
		fmt.Println()
		fmt.Println("› App commands")
		fmt.Println()
		fmt.Println("Unknown commands are delegated to this app.")
		fmt.Println("Use `forj run <command>` to force app command execution.")
	}
}

// shouldDelegateToAppCommand reports whether an unresolved native command should run as an App command.
func shouldDelegateToAppCommand(args []string, parseErr error, inGeneratedApp bool) bool {
	if !inGeneratedApp || parseErr == nil || len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return false
	}
	message := strings.ToLower(parseErr.Error())
	return strings.Contains(message, "unexpected argument") || strings.Contains(message, "unknown command")
}

// runAppCommandThroughSource runs a generated App command through the same source path as forj run.
func runAppCommandThroughSource(root *cmd.RootCmd, args []string) error {
	run := &root.RootCmd.RunCmd
	run.Root = "."
	run.Args = append([]string(nil), args...)
	run.Env = delegatedAppEnv()
	return run.Run()
}

// delegatedAppEnv returns the environment passed to source-run generated App commands.
func delegatedAppEnv() []string {
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
	if _, ok := os.LookupEnv("FORJ_COMMAND_PREFIX"); !ok {
		env = append(env, "FORJ_COMMAND_PREFIX=forj")
	}
	return env
}

// isGeneratedAppDir reports whether the current directory looks like a generated App root.
func isGeneratedAppDir() bool {
	return regularFileExists(".goforj.yml") && regularFileExists("go.mod")
}

// regularFileExists reports whether path exists and is not a directory.
func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
