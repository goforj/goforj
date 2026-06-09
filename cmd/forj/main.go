package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

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
				targets := conventionalAppHelpTargets(inGeneratedApp)
				printTargetUsageHelp(targets)
				if err := printGeneratedAppHelp(targets); err != nil {
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

	sort.Strings(targets)
	return targets
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

// printTargetUsageHelp explains app prefixing once when a project has multiple app targets.
func printTargetUsageHelp(targets []string) {
	if len(targets) <= 1 {
		return
	}
	fmt.Println()
	fmt.Println(console.Colorize(console.ColorBoldWhite, "app usage"))
	renderTargetUsageRow("forj <app> <command>", "Run a command for a specific app")
	renderTargetUsageRow("forj <app> build", "Build a specific app binary")
	renderTargetUsageRow("forj dev", "Build and run all apps in development")
}

// renderTargetUsageRow prints one compact root help example row.
func renderTargetUsageRow(command string, help string) {
	const width = 23
	spacing := strings.Repeat(" ", width-len(command)+2)
	fmt.Printf("  %s%s%s\n",
		console.Colorize(console.ColorBoldGreen, command),
		spacing,
		console.Colorize(console.ColorGray, help),
	)
}

// printGeneratedAppHelp prints generated app help screens in target order after collecting them concurrently.
func printGeneratedAppHelp(targets []string) error {
	results := collectGeneratedAppHelp(targets)
	for _, result := range results {
		if result.err != nil {
			return fmt.Errorf("%s help: %w", result.target, result.err)
		}
	}
	if output, ok := compactGeneratedAppHelp(results); ok {
		fmt.Print(output)
		return nil
	}
	for _, result := range results {
		fmt.Println()
		fmt.Print(result.output)
	}
	return nil
}

type appHelpResult struct {
	target string
	output string
	err    error
}

type appHelpCommand struct {
	section string
	name    string
	help    string
}

type parsedAppHelp struct {
	target   string
	title    string
	baseName string
	commands []appHelpCommand
}

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

// compactGeneratedAppHelp folds identical generated app command surfaces into one shared help block.
func compactGeneratedAppHelp(results []appHelpResult) (string, bool) {
	if len(results) <= 1 {
		return "", false
	}
	parsed := make([]parsedAppHelp, 0, len(results))
	for _, result := range results {
		help, ok := parseGeneratedAppHelp(result.target, result.output)
		if !ok {
			return "", false
		}
		parsed = append(parsed, help)
	}

	shared := sharedAppHelpCommands(parsed)
	if len(shared) == 0 {
		return "", false
	}

	baseName := parsed[0].baseName
	var out strings.Builder
	renderAppHelpBlock(&out, baseName+" · available in all apps", shared)
	for _, help := range parsed {
		delta := appHelpDelta(help.commands, shared)
		if len(delta) == 0 {
			continue
		}
		renderAppHelpBlock(&out, baseName+" · "+help.target, delta)
	}
	return out.String(), true
}

// parseGeneratedAppHelp extracts command rows from the generated app root help format.
func parseGeneratedAppHelp(target string, output string) (parsedAppHelp, bool) {
	lines := strings.Split(stripANSI(output), "\n")
	help := parsedAppHelp{target: strings.TrimSpace(target)}
	currentSection := ""
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if help.title == "" {
			help.title = strings.TrimSpace(line)
			help.baseName = generatedHelpBaseName(help.title)
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			command, ok := parseGeneratedAppHelpCommand(currentSection, line)
			if ok {
				help.commands = append(help.commands, command)
			}
			continue
		}
		currentSection = strings.TrimSpace(line)
	}
	return help, help.title != "" && help.baseName != "" && len(help.commands) > 0
}

// parseGeneratedAppHelpCommand parses one aligned command row from generated app help.
func parseGeneratedAppHelpCommand(section string, line string) (appHelpCommand, bool) {
	section = strings.TrimSpace(section)
	if section == "" {
		return appHelpCommand{}, false
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return appHelpCommand{}, false
	}
	name, help, found := strings.Cut(trimmed, " ")
	if !found || name == "" {
		return appHelpCommand{}, false
	}
	return appHelpCommand{section: section, name: name, help: strings.TrimSpace(help)}, true
}

// generatedHelpBaseName returns the app name before the optional target qualifier.
func generatedHelpBaseName(title string) string {
	title = strings.TrimSpace(title)
	if before, _, ok := strings.Cut(title, " · "); ok {
		return strings.TrimSpace(before)
	}
	if before, _, ok := strings.Cut(title, " | "); ok {
		return strings.TrimSpace(before)
	}
	return title
}

// sharedAppHelpCommands returns commands that are exactly present in every parsed app target.
func sharedAppHelpCommands(parsed []parsedAppHelp) []appHelpCommand {
	counts := map[string]int{}
	commands := map[string]appHelpCommand{}
	for _, help := range parsed {
		seen := map[string]struct{}{}
		for _, command := range help.commands {
			key := appHelpCommandKey(command)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			counts[key]++
			commands[key] = command
		}
	}
	shared := make([]appHelpCommand, 0)
	for key, count := range counts {
		if count == len(parsed) {
			shared = append(shared, commands[key])
		}
	}
	sortAppHelpCommands(shared)
	return shared
}

// appHelpDelta removes shared commands from one target's command list.
func appHelpDelta(commands []appHelpCommand, shared []appHelpCommand) []appHelpCommand {
	sharedKeys := map[string]struct{}{}
	for _, command := range shared {
		sharedKeys[appHelpCommandKey(command)] = struct{}{}
	}
	delta := make([]appHelpCommand, 0)
	for _, command := range commands {
		if _, ok := sharedKeys[appHelpCommandKey(command)]; ok {
			continue
		}
		delta = append(delta, command)
	}
	sortAppHelpCommands(delta)
	return delta
}

// renderAppHelpBlock writes one compact app help block with the same simple grouping as generated help.
func renderAppHelpBlock(out *strings.Builder, title string, commands []appHelpCommand) {
	if len(commands) == 0 {
		return
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, console.Colorize(console.ColorBoldWhite, title))
	fmt.Fprintln(out)
	sections := map[string][]appHelpCommand{}
	maxLen := 0
	for _, command := range commands {
		sections[command.section] = append(sections[command.section], command)
		if len(command.name) > maxLen {
			maxLen = len(command.name)
		}
	}
	for _, section := range sortedStringKeys(sections) {
		fmt.Fprintln(out, console.Colorize(console.ColorBoldWhite, section))
		for _, command := range sections[section] {
			spacing := strings.Repeat(" ", maxLen-len(command.name)+2)
			fmt.Fprintf(out, "  %s%s%s\n",
				console.Colorize(console.ColorBoldGreen, command.name),
				spacing,
				console.Colorize(console.ColorGray, command.help),
			)
		}
	}
}

// sortAppHelpCommands keeps compact help deterministic by section then command name.
func sortAppHelpCommands(commands []appHelpCommand) {
	sort.Slice(commands, func(i, j int) bool {
		if commands[i].section != commands[j].section {
			return commands[i].section < commands[j].section
		}
		return commands[i].name < commands[j].name
	})
}

// sortedStringKeys returns sorted map keys for compact help rendering.
func sortedStringKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// appHelpCommandKey identifies a command row exactly, including its section and description.
func appHelpCommandKey(command appHelpCommand) string {
	return command.section + "\x00" + command.name + "\x00" + command.help
}

// stripANSI removes color escape sequences before parsing generated help output.
func stripANSI(value string) string {
	return ansiEscapePattern.ReplaceAllString(value, "")
}

// collectGeneratedAppHelp shells out per target so help rendering can run in parallel without sharing parser state.
func collectGeneratedAppHelp(targets []string) []appHelpResult {
	results := make([]appHelpResult, len(targets))
	multi := len(targets) > 1
	var wait sync.WaitGroup
	for index, target := range targets {
		wait.Add(1)
		go func(index int, target string, multi bool) {
			defer wait.Done()
			output, err := runAppHelpForTarget(target, multi)
			results[index] = appHelpResult{target: target, output: output, err: err}
		}(index, target, multi)
	}
	wait.Wait()
	return results
}

// runAppHelpForTarget invokes the selected target through the root binary so source and built targets share one path.
func runAppHelpForTarget(target string, multi bool) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		target = project.DefaultAppTargetName
	}
	command := exec.Command(os.Args[0], target, "--help")
	command.Env = withTargetEnv(delegatedAppEnv(), target)
	if multi {
		command.Env = append(command.Env, "FORJ_MULTI_APP_HELP=1")
	}
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err != nil {
		return output.String(), err
	}
	return output.String(), nil
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
