package forj

import (
	"fmt"
	"github.com/goforj/goforj/internal/logger"
)

// RenderCmd is the command to run the scaffolder
type RenderCmd struct {
	logger   *logger.AppLogger
	renderer *ProjectRenderer

	Components []string `help:"Components to render"`
}

func (*RenderCmd) Signature() string {
	return `name:"render" help:"Run the project renderer" hidden:""`
}

// NewCmd creates a new RenderCmd
func NewCmd(logger *logger.AppLogger, renderer *ProjectRenderer) *RenderCmd {
	return &RenderCmd{
		logger:   logger,
		renderer: renderer,
	}
}

// Run executes the command.
func (c *RenderCmd) Run() error {
	i := ComponentRenderInput{}
	cmp := &i.components
	if len(c.Components) == 0 {
		c.logger.Info().Msg("No CLI components specified, using .goforj.yml render configuration")
		i.renderAll = true
	} else {
		c.logger.Info().Any("components", c.Components).Msg("Rendering specified components")
		for _, name := range c.Components {
			switch name {
			case "cli":
				cmp.CLI = true
			case "docker":
				cmp.Docker = true
			case "auth":
				cmp.Auth = true
			case "web_api":
				cmp.WebAPI = true
			case "web_ui":
				cmp.WebUI = true
			case "database":
				cmp.DatabaseMySQL = true
			case "database_mysql":
				cmp.DatabaseMySQL = true
			case "database_postgres":
				cmp.DatabasePostgres = true
			case "database_sqlite":
				cmp.DatabaseSQLite = true
			case "scheduler":
				cmp.Scheduler = true
			case "jobs":
				cmp.Jobs = true
			default:
				return fmt.Errorf("unknown component: %s", name)
			}
		}
	}

	err := c.renderer.Render(i)
	if err != nil {
		return err
	}
	return nil
}
