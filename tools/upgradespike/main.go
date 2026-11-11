// Package main provides a command for disposable framework upgrade experiments.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

// projectFlags retains each explicitly selected reference application.
type projectFlags []string

// String describes the current selections for standard flag help.
func (projects *projectFlags) String() string { return fmt.Sprint([]string(*projects)) }

// Set accepts repeated projects without introducing shell command interpretation.
func (projects *projectFlags) Set(value string) error {
	*projects = append(*projects, value)
	return nil
}

// spikeOptions keeps argument validation separate from processes that render or test applications.
type spikeOptions struct {
	command    string
	renderer   string
	source     string
	revision   string
	comparison string
	workspace  string
	projects   projectFlags
}

// parseSpikeOptions rejects incomplete invocations before preparing any experiment workspace.
func parseSpikeOptions(arguments []string, output io.Writer) (spikeOptions, error) {
	var options spikeOptions
	if len(arguments) == 0 {
		return options, fmt.Errorf("usage: upgradespike reference|prepare|evaluate|auth [options]")
	}
	options.command = arguments[0]
	flags := flag.NewFlagSet(options.command, flag.ContinueOnError)
	flags.SetOutput(output)
	switch options.command {
	case "reference":
		flags.StringVar(&options.renderer, "forj", "", "Renderer executable")
		flags.Var(&options.projects, "project", "Committed application to inspect (repeatable)")
		flags.StringVar(&options.comparison, "from-renderer", "", "Comparison renderer executable")
	case "prepare":
		flags.StringVar(&options.renderer, "forj", "", "Target renderer executable")
		flags.StringVar(&options.source, "source", "", "Committed application checkout")
		flags.StringVar(&options.revision, "revision", "HEAD", "Application commit to freeze")
	case "evaluate":
	case "auth":
		flags.StringVar(&options.renderer, "forj", "", "Target renderer executable")
		flags.StringVar(&options.source, "ditracker", "", "Committed Ditracker checkout")
	default:
		return options, fmt.Errorf("unknown experiment %q", options.command)
	}
	if err := flags.Parse(arguments[1:]); err != nil {
		return options, err
	}
	if options.command == "evaluate" {
		if flags.NArg() != 1 {
			return options, fmt.Errorf("evaluate requires exactly one prepared workspace")
		}
		options.workspace = flags.Arg(0)
	} else {
		if flags.NArg() != 0 {
			return options, fmt.Errorf("unexpected positional arguments")
		}
		if options.renderer == "" {
			return options, fmt.Errorf("--forj is required")
		}
		if options.command == "reference" && len(options.projects) == 0 {
			return options, fmt.Errorf("reference requires at least one --project")
		}
		if options.command != "reference" && options.source == "" {
			return options, fmt.Errorf("application source is required")
		}
		if options.command == "prepare" && options.revision == "" {
			return options, fmt.Errorf("--revision must not be empty")
		}
	}
	return options, nil
}

// absoluteSpikePath resolves existing inputs so subprocess working directories cannot reinterpret their paths.
func absoluteSpikePath(name string) (string, error) {
	absolute, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(absolute)
}

// runSpike coordinates Go-only experiments and writes their result outside application source.
func runSpike(ctx context.Context, arguments []string, output, diagnostics io.Writer) error {
	options, err := parseSpikeOptions(arguments, diagnostics)
	if err != nil {
		return err
	}
	for _, value := range []*string{&options.renderer, &options.source, &options.comparison, &options.workspace} {
		if *value != "" {
			*value, err = absoluteSpikePath(*value)
			if err != nil {
				return err
			}
		}
	}
	for index, project := range options.projects {
		options.projects[index], err = absoluteSpikePath(project)
		if err != nil {
			return err
		}
	}
	var result any
	switch options.command {
	case "reference":
		result, err = runReference(ctx, options.renderer, options.projects, options.comparison)
	case "prepare":
		result, err = prepareTrial(ctx, options.source, options.renderer, options.revision)
	case "evaluate":
		result, err = evaluateTrial(ctx, options.workspace)
	case "auth":
		result, err = runAuthTrial(ctx, options.renderer, options.source)
	}
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// main lets cancellation reach native process-tree cleanup before the experiment exits.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runSpike(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
