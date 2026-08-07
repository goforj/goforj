package wire

import (
	"context"
	"github.com/goforj/goforj/internal/cmd"
	"github.com/goforj/goforj/internal/logger"
)

// App is the root application resource.
type App struct {
	context context.Context
	rootCmd *cmd.RootCmd
	logger  *logger.AppLogger
}

// RootCmd returns the fully wired root command.
func (a *App) RootCmd() *cmd.RootCmd {
	return a.rootCmd
}

// Logger returns the application-scoped logger.
func (a *App) Logger() *logger.AppLogger {
	return a.logger
}

// NewApplication constructs the application with its required dependencies.
func NewApplication(
	logger *logger.AppLogger,
	rootCmd *cmd.RootCmd,
) App {
	return App{
		context: context.Background(),
		logger:  logger,
		rootCmd: rootCmd,
	}
}
