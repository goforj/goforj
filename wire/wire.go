//go:build wireinject
// +build wireinject

package wire

import (
	"github.com/goforj/goforj/internal/forj"
	"github.com/goforj/goforj/internal/logger"
	"github.com/google/wire"
)

// InitializeApplication initializes the application with all its dependencies.
func InitializeApplication() (App, error) {
	wire.Build(
		forj.WireSet,
		generalSet,
		cmdSet,
		logger.ProvideAppLogger,
		NewApplication,
	)

	return App{}, nil
}
