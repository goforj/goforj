package cmd

import (
	"github.com/goforj/goforj/internal/logger"
)

// HelloWorldCmd is a test command.
type HelloWorldCmd struct {
	logger *logger.AppLogger
}

// NewHelloWorldCmd creates a new HelloWorldCmd.
func NewHelloWorldCmd(logger *logger.AppLogger) *HelloWorldCmd {
	return &HelloWorldCmd{
		logger: logger,
	}
}

// Run executes the command.
func (c *HelloWorldCmd) Run() error {
	c.logger.Info().Msg("Hello world!")

	return nil
}
