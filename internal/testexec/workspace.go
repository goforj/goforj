// Package testexec runs maintainer commands inside isolated generated workspaces.
package testexec

import (
	"fmt"
	"os"
	"strings"

	"github.com/goforj/execx"
	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
)

// Workspace owns the execution policy shared by commands run in one generated project.
type Workspace struct {
	logger *logger.AppLogger
	silent bool
	dir    string
	caches GoCaches
}

// GoCaches names the module and build cache paths that isolate Go subprocesses.
type GoCaches struct {
	// ModulePath isolates downloaded modules shared by commands in one test run.
	ModulePath string
	// BuildPath isolates compiled artifacts shared by commands in one test run.
	BuildPath string
}

// StreamingStep describes a long-running command whose output should remain visible outside silent runs.
type StreamingStep struct {
	// Name identifies the command in progress and failure messages.
	Name string
	// Command contains the executable followed by its arguments.
	Command []string
	// Environment overrides inherited values for this command only.
	Environment map[string]string
}

// workspaceCommand carries the execution differences between ordinary and streaming commands through one policy path.
type workspaceCommand struct {
	name        string
	args        []string
	environment map[string]string
	stream      bool
}

// NewWorkspace binds command output and Go cache policy to one generated project directory.
func NewWorkspace(log *logger.AppLogger, silent bool, dir string, caches GoCaches) *Workspace {
	return &Workspace{
		logger: log,
		silent: silent,
		dir:    dir,
		caches: caches,
	}
}

// Run executes one named validation step using the workspace's shared policy.
func (workspace *Workspace) Run(name string, args ...string) error {
	return workspace.execute(workspaceCommand{name: name, args: args})
}

// RunStreaming executes a long-running step with command output attached to non-silent runs.
func (workspace *Workspace) RunStreaming(step StreamingStep) error {
	return workspace.execute(workspaceCommand{
		name:        step.Name,
		args:        step.Command,
		environment: step.Environment,
		stream:      true,
	})
}

// execute keeps cache, output, and failure policy identical across every workspace command.
func (workspace *Workspace) execute(command workspaceCommand) error {
	cmd := execx.Command(command.args[0], command.args[1:]...).
		Dir(workspace.dir).
		EnvAppend(map[string]string{
			"GOMODCACHE": workspace.caches.ModulePath,
			"GOCACHE":    workspace.caches.BuildPath,
		}).
		EnvAppend(command.environment)

	if !workspace.silent {
		cmd = cmd.ShadowPrint(execx.WithFormatter(formatShadowEvent))
		if command.stream {
			cmd = cmd.StdoutWriter(os.Stdout).StderrWriter(os.Stderr)
		}
	}

	res, err := cmd.Run()
	if err != nil || !res.OK() {
		if !workspace.silent {
			console.Errorf("%s failed", command.name)
		}
		failure := commandFailure(command.name, res, err)
		workspace.logger.Error().
			Str("step", command.name).
			Str("stdout", strings.TrimSpace(res.Stdout)).
			Err(failure).
			Msg("Step failed")
		return failure
	}
	return nil
}

// commandFailure retains captured subprocess output because package failures are otherwise difficult to diagnose in CI.
func commandFailure(name string, result execx.Result, runErr error) error {
	detail := commandOutput(result)
	if runErr != nil {
		if detail != "" {
			return fmt.Errorf("%s: %w (%s)", name, runErr, detail)
		}
		return fmt.Errorf("%s: %w", name, runErr)
	}
	if detail != "" {
		return fmt.Errorf("%s failed with exit code %d (%s)", name, result.ExitCode, detail)
	}
	return fmt.Errorf("%s failed with exit code %d", name, result.ExitCode)
}

// commandOutput keeps stdout and stderr together because Go test failures can split package context across both streams.
func commandOutput(result execx.Result) string {
	output := make([]string, 0, 2)
	if stdout := strings.TrimSpace(result.Stdout); stdout != "" {
		output = append(output, stdout)
	}
	if stderr := strings.TrimSpace(result.Stderr); stderr != "" {
		output = append(output, stderr)
	}
	return strings.Join(output, "\n")
}

// formatShadowEvent keeps maintainer command progress consistent with the rest of the CLI.
func formatShadowEvent(event execx.ShadowEvent) string {
	switch event.Phase {
	case execx.ShadowBefore:
		return fmt.Sprintf("%s %s", console.ActionMark(), event.Command)
	case execx.ShadowAfter:
		return fmt.Sprintf("%s %s (%s)", console.InfoMark(), event.Command, event.Duration)
	default:
		return fmt.Sprintf("%s %s", console.InfoMark(), event.Command)
	}
}
