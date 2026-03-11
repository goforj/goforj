package forj

import "github.com/goforj/goforj/internal/build"

// ApiIndexCmd builds a canonical API index manifest from project source.
type ApiIndexCmd struct {
	runner *build.APIIndexRunner

	Root        string `help:"Project root to index" default:"."`
	Out         string `help:"Output path for API index manifest" default:"build/api_index.json"`
	Diagnostics string `help:"Output path for diagnostics" default:"build/api_index.diagnostics.json"`
	OpenAPI     string `help:"Optional output path for generated OpenAPI document" default:"build/openapi.json"`
}

func (*ApiIndexCmd) Signature() string {
	return `name:"build:api-index" help:"Build API index metadata from source" group:"build"`
}

// NewApiIndexCmd creates a new API index command.
func NewApiIndexCmd(runner *build.APIIndexRunner) *ApiIndexCmd {
	return &ApiIndexCmd{runner: runner}
}

// Run executes API indexing.
func (c *ApiIndexCmd) Run() error {
	return c.run(true)
}

func (c *ApiIndexCmd) RunQuiet() error {
	return c.run(false)
}

func (c *ApiIndexCmd) run(emitLog bool) error {
	return c.runner.Run(c.Root, c.Out, c.Diagnostics, c.OpenAPI, emitLog)
}
