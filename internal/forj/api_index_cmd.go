package forj

import (
	"context"
	"path/filepath"

	"github.com/goforj/goforj/internal/apix"
	"github.com/goforj/goforj/internal/logger"
)

// ApiIndexCmd builds a canonical API index manifest from project source.
type ApiIndexCmd struct {
	logger *logger.AppLogger

	Root        string `help:"Project root to index" default:"."`
	Out         string `help:"Output path for API index manifest" default:"build/api_index.json"`
	Diagnostics string `help:"Output path for diagnostics" default:"build/api_index.diagnostics.json"`
	OpenAPI     string `help:"Optional output path for generated OpenAPI document" default:"build/openapi.json"`
}

func (*ApiIndexCmd) Signature() string {
	return `name:"build:api-index" help:"Build API index metadata from source" group:"build"`
}

// NewApiIndexCmd creates a new API index command.
func NewApiIndexCmd(logger *logger.AppLogger) *ApiIndexCmd {
	return &ApiIndexCmd{logger: logger}
}

// Run executes API indexing.
func (c *ApiIndexCmd) Run() error {
	root, err := filepath.Abs(c.Root)
	if err != nil {
		return err
	}

	manifest, err := apix.Run(context.Background(), apix.IndexOptions{
		Root:            root,
		OutPath:         c.Out,
		DiagnosticsPath: c.Diagnostics,
		OpenAPIPath:     c.OpenAPI,
	})
	if err != nil {
		return err
	}

	c.logger.Info().
		Any("operations", len(manifest.Operations)).
		Any("schemas", len(manifest.Schemas)).
		Any("diagnostics", len(manifest.Diagnostics)).
		Any("out", c.Out).
		Any("diagnostics_out", c.Diagnostics).
		Any("openapi_out", c.OpenAPI).
		Msg("API index generated")
	return nil
}
