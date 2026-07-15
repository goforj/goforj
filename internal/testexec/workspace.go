// Package testexec runs maintainer commands inside isolated generated workspaces.
package testexec

import (
	"fmt"
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
	module string
	build  string
}

// NewGoCaches records the module and build cache paths selected for one test run.
func NewGoCaches(module, build string) GoCaches {
	return GoCaches{module: module, build: build}
}

// ModulePath returns the module cache path shared by commands in a test run.
func (caches GoCaches) ModulePath() string {
	return caches.module
}

// BuildPath returns the build cache path shared by commands in a test run.
func (caches GoCaches) BuildPath() string {
	return caches.build
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
	cmd := execx.Command(args[0], args[1:]...).
		Dir(workspace.dir).
		EnvAppend(map[string]string{
			"GOMODCACHE": workspace.caches.module,
			"GOCACHE":    workspace.caches.build,
		})

	if !workspace.silent {
		cmd = cmd.ShadowPrint(execx.WithFormatter(formatShadowEvent))
	}

	res, err := cmd.Run()
	if err != nil || !res.OK() {
		if !workspace.silent {
			console.Errorf("%s failed", name)
		}
		if err == nil {
			err = fmt.Errorf("command failed with exit code %d", res.ExitCode)
		}
		errMsg := strings.TrimSpace(res.Stderr)
		if errMsg == "" {
			errMsg = err.Error()
		}
		if errMsg == "" {
			errMsg = "command failed"
		}
		workspace.logger.Error().
			Str("step", name).
			Str("stdout", strings.TrimSpace(res.Stdout)).
			Err(fmt.Errorf("%s", errMsg)).
			Msg("Step failed")
		return err
	}
	return nil
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
