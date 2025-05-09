package goforj

import "github.com/goforj/goforj/internal/logger"

// RenderCmd is the command to run the scaffolder
type RenderCmd struct {
	logger   *logger.AppLogger
	renderer *ProjectRenderer
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
	err := c.renderer.Render()
	if err != nil {
		return err
	}
	return nil
}
