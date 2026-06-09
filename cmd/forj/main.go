package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/goforj/goforj/internal/build"
	"github.com/goforj/goforj/internal/cmd"
	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/project"
	"github.com/goforj/goforj/version"
	"github.com/goforj/goforj/wire"
	"golang.org/x/term"
)

var cliDefaultedEnv = map[string]bool{}
var cliNativeCommandNames []string

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
	cliNativeCommandNames = nativeCommandNames(parser.Model.Node)

	args := os.Args[1:]
	inGeneratedApp := isGeneratedAppDir()
	targetContext := ""
	if target, remaining, ok := resolveTargetPrefix(args, inGeneratedApp); ok {
		if shouldRunTargetNativeCommand(remaining) {
			applySourceTargetEnv(target)
			targetContext = target
			args = remaining
		} else if runTargetBinary(target, remaining) {
			return
		} else {
			applySourceTargetEnv(target)
			targetContext = target
			args = remaining
		}
	}
	app.RootCmd().RootCmd.RunCmd.Env = delegatedAppEnv()

	if isRootHelp(args) {
		if targetContext != "" {
			if err := runAppCommandThroughSource(app.RootCmd(), appRootHelpArgs()); err != nil {
				if code, ok := build.ChildExitCode(err); ok {
					os.Exit(code)
				}
				console.Fatalf("%v", err)
			}
		} else {
			printRootHelp(parser)
			if inGeneratedApp {
				if err := printGeneratedAppHelp(app.RootCmd(), conventionalAppHelpTargets(inGeneratedApp)); err != nil {
					if code, ok := build.ChildExitCode(err); ok {
						os.Exit(code)
					}
					console.Fatalf("%v", err)
				}
			}
		}
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

// resolveTargetPrefix strips a conventional target prefix while preserving native command precedence.
func resolveTargetPrefix(args []string, inGeneratedApp bool) (string, []string, bool) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "", args, false
	}
	target := strings.TrimSpace(args[0])
	if !project.IsSafeAppTargetName(target) || isNativeCommandName(target) {
		return "", args, false
	}
	if regularFileExists(filepath.Join(".", "bin", target)) {
		return target, args[1:], true
	}
	if inGeneratedApp && isConventionalSourceTarget(target) {
		return target, args[1:], true
	}
	return "", args, false
}

// shouldRunTargetNativeCommand keeps framework-owned commands target-scoped instead of delegating them to app binaries.
func shouldRunTargetNativeCommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	return isNativeCommandName(args[0])
}

// conventionalAppHelpTargets discovers the app targets that should be visible from `forj --help`.
func conventionalAppHelpTargets(inGeneratedApp bool) []string {
	if !inGeneratedApp {
		return nil
	}

	seen := map[string]struct{}{}
	targets := []string{}
	appendConventionalAppHelpTarget(&targets, seen, project.DefaultAppTargetName)

	appendConventionalAppHelpTargetsFromDir(&targets, seen, "cmd")
	appendConventionalBinaryHelpTargets(&targets, seen, filepath.Join(".", "bin"))

	if len(targets) <= 1 {
		return targets
	}
	named := append([]string(nil), targets[1:]...)
	sort.Strings(named)
	return append([]string{project.DefaultAppTargetName}, named...)
}

// appendConventionalAppHelpTargetsFromDir treats cmd/<target>/main.go as a source-owned app target.
func appendConventionalAppHelpTargetsFromDir(targets *[]string, seen map[string]struct{}, root string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		target := entry.Name()
		if !regularFileExists(filepath.Join(root, target, "main.go")) {
			continue
		}
		appendConventionalAppHelpTarget(targets, seen, target)
	}
}

// appendConventionalBinaryHelpTargets keeps binary-only targets visible after a build.
func appendConventionalBinaryHelpTargets(targets *[]string, seen map[string]struct{}, root string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		appendConventionalAppHelpTarget(targets, seen, entry.Name())
	}
}

// appendConventionalAppHelpTarget applies the same safety and native-command precedence rules as target dispatch.
func appendConventionalAppHelpTarget(targets *[]string, seen map[string]struct{}, target string) {
	target = strings.TrimSpace(target)
	if target == "" || !project.IsSafeAppTargetName(target) || project.IsReservedAppTargetName(target) {
		return
	}
	if target != project.DefaultAppTargetName && isNativeCommandName(target) {
		return
	}
	if _, ok := seen[target]; ok {
		return
	}
	seen[target] = struct{}{}
	*targets = append(*targets, target)
}

// isConventionalSourceTarget reports whether cmd/<target>/main.go defines an app target.
func isConventionalSourceTarget(target string) bool {
	target = strings.TrimSpace(target)
	if !project.IsSafeAppTargetName(target) {
		return false
	}
	return regularFileExists(filepath.Join(".", "cmd", target, "main.go"))
}

// isNativeCommandName reports whether name belongs to the framework CLI grammar.
func isNativeCommandName(name string) bool {
	name = strings.TrimSpace(name)
	for _, native := range cliNativeCommandNames {
		if native == name {
			return true
		}
	}
	return false
}

// runTargetBinary delegates `forj <target> ...` to ./bin/<target> when that target exists.
func runTargetBinary(target string, args []string) bool {
	if err := runTargetBinaryCommand(target, args); err != nil {
		if errors.Is(err, errTargetBinaryNotFound) {
			return false
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		console.Fatalf("%v", err)
	}
	return true
}

// errTargetBinaryNotFound lets target-prefix dispatch fall back to source-mode handling.
var errTargetBinaryNotFound = errors.New("target binary not found")

// runTargetBinaryCommand delegates to ./bin/<target> while leaving exit handling to the caller.
func runTargetBinaryCommand(target string, args []string) error {
	binPath := filepath.Join(".", "bin", strings.TrimSpace(target))
	if !regularFileExists(binPath) {
		return errTargetBinaryNotFound
	}
	command := exec.Command(binPath, args...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Env = targetCommandEnv(target)
	if err := command.Run(); err != nil {
		return err
	}
	return nil
}

// applySourceTargetEnv makes native source-mode commands operate against the selected target.
func applySourceTargetEnv(target string) {
	target = strings.TrimSpace(target)
	_ = os.Setenv("FORJ_COMMAND_PREFIX", "forj "+target)
	_ = os.Setenv("FORJ_APP_TARGET", target)
	_ = os.Setenv("APP_TARGET", target)
}

// targetCommandEnv marks delegated target commands, with the explicit CLI target taking precedence.
func targetCommandEnv(target string) []string {
	return withTargetEnv(os.Environ(), strings.TrimSpace(target))
}

// withTargetEnv overlays target identity onto an environment slice.
func withTargetEnv(env []string, target string) []string {
	updates := map[string]string{
		"FORJ_COMMAND_PREFIX": "forj " + target,
		"FORJ_APP_TARGET":     target,
		"APP_TARGET":          target,
	}
	out := make([]string, 0, len(env)+len(updates))
	seen := map[string]struct{}{}
	for _, entry := range env {
		key, _, ok := strings.Cut(entry, "=")
		if ok {
			if value, replace := updates[key]; replace {
				out = append(out, key+"="+value)
				seen[key] = struct{}{}
				continue
			}
		}
		out = append(out, entry)
	}
	for key, value := range updates {
		if _, ok := seen[key]; ok {
			continue
		}
		out = append(out, key+"="+value)
	}
	return out
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

// appRootHelpArgs returns the generated App args used for root help delegation.
func appRootHelpArgs() []string {
	return []string{"--help"}
}

// printGeneratedAppHelp prints each generated app target help screen under a small target header.
func printGeneratedAppHelp(root *cmd.RootCmd, targets []string) error {
	for _, target := range targets {
		fmt.Println()
		fmt.Printf("App target: %s\n", target)
		if err := runAppHelpForTarget(root, target); err != nil {
			return err
		}
	}
	return nil
}

// runAppHelpForTarget prefers source-mode help so root help is not coupled to stale target binaries.
func runAppHelpForTarget(root *cmd.RootCmd, target string) error {
	target = strings.TrimSpace(target)
	if target == "" || target == project.DefaultAppTargetName {
		return runAppCommandThroughSource(root, appRootHelpArgs())
	}
	if isConventionalSourceTarget(target) {
		return runAppCommandThroughSourceWithEnv(root, appRootHelpArgs(), withTargetEnv(delegatedAppEnv(), target))
	}
	return runTargetBinaryCommand(target, appRootHelpArgs())
}

// printRootHelp prints native GoForj help.
func printRootHelp(parser *kong.Kong) {
	ctx, _ := kong.Trace(parser, []string{})
	ctx.PrintUsage(false)
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
	return runAppCommandThroughSourceWithEnv(root, args, delegatedAppEnv())
}

// runAppCommandThroughSourceWithEnv runs source-mode app commands with an explicit environment overlay.
func runAppCommandThroughSourceWithEnv(root *cmd.RootCmd, args []string, env []string) error {
	run := &root.RootCmd.RunCmd
	run.Root = "."
	run.Args = append([]string(nil), args...)
	run.Env = env
	run.PreserveTTY = true
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
	if len(cliNativeCommandNames) > 0 {
		env = append(env, "FORJ_NATIVE_COMMAND_NAMES="+strings.Join(cliNativeCommandNames, ","))
	}
	if shouldForceDelegatedAppColor(term.IsTerminal(int(os.Stdout.Fd()))) {
		env = append(env, "CLICOLOR_FORCE=1")
	}
	return env
}

// nativeCommandNames returns every native command name and alias exposed by the framework CLI.
func nativeCommandNames(node *kong.Node) []string {
	seen := map[string]struct{}{}
	collectNativeCommandNames(node, seen)
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// collectNativeCommandNames walks the Kong command tree and records command names and aliases.
func collectNativeCommandNames(node *kong.Node, names map[string]struct{}) {
	if node == nil {
		return
	}
	if node.Type == kong.CommandNode {
		if clean := strings.TrimSpace(node.Name); clean != "" {
			names[clean] = struct{}{}
		}
		for _, alias := range node.Aliases {
			if clean := strings.TrimSpace(alias); clean != "" {
				names[clean] = struct{}{}
			}
		}
	}
	for _, child := range node.Children {
		collectNativeCommandNames(child, names)
	}
}

// shouldForceDelegatedAppColor reports whether delegated app commands should preserve terminal color.
func shouldForceDelegatedAppColor(stdoutTTY bool) bool {
	if !stdoutTTY {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if _, ok := os.LookupEnv("CLICOLOR_FORCE"); ok {
		return false
	}
	return true
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
