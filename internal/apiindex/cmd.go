package apiindex

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// defaultRunner captures the immediate-publication operation used by the standalone command.
type defaultRunner interface {
	RunDefault(options Options) (string, error)
}

// Cmd runs API indexing independently from the complete build pipeline.
type Cmd struct {
	runner defaultRunner
	// Strict fails when API indexing reports warnings or errors.
	Strict bool `help:"Fail when API indexing reports warnings or errors"`
	// Tags selects additional Go build tags for this indexing invocation.
	Tags   string `help:"Select Go build tags as a comma-separated list"`
	stdout io.Writer
}

// NewCmd creates the standalone command around the API-index runner.
func NewCmd(runner *Runner) *Cmd {
	return &Cmd{runner: runner, stdout: os.Stdout}
}

// Signature returns CLI metadata that keeps API indexing in the build command group.
func (*Cmd) Signature() string {
	return `name:"build:api-index" help:"Build API index and OpenAPI artifacts" group:"build"`
}

// Run generates API artifacts for the active app without running the other build steps.
func (c *Cmd) Run() error {
	buildTags := parseBuildTags(c.Tags)
	if strings.TrimSpace(c.Tags) != "" && len(buildTags) == 0 {
		return fmt.Errorf("build:api-index --tags requires at least one non-empty tag")
	}
	status, err := c.runner.RunDefault(Options{Strict: c.Strict, BuildTags: buildTags})
	if err != nil {
		return fmt.Errorf("%s: %w", status, err)
	}
	_, err = fmt.Fprintln(c.stdout, status)
	return err
}
