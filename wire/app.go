package wire

import (
	"context"
	"github.com/goforj/goforj/internal/cmd"
	"github.com/goforj/goforj/internal/logger"
)

// App is the root App resource
type App struct {
	context context.Context
	rootCmd *cmd.RootCmd
	logger  *logger.AppLogger
}

func (a *App) RootCmd() *cmd.RootCmd {
	return a.rootCmd
}

func (a *App) Logger() *logger.AppLogger {
	return a.logger
}

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
