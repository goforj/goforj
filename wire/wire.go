//go:build wireinject
// +build wireinject

package wire

import (
	"github.com/goforj/goforj/internal/forj"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/wire"
)

// InitializeApplication initializes the application with all its dependencies.
func InitializeApplication() (App, error) {
	wire.Build(
		forj.WireSet,
		provideEmptyInheritedEnvironment,
		generalSet,
		cmdSet,
		logger.ProvideAppLogger,
		NewApplication,
	)

	return App{}, nil
}

// InitializeApplicationWithEnvironment initializes the application with environment values inherited by the CLI launcher.
func InitializeApplicationWithEnvironment(inheritedEnv map[string]string) (App, error) {
	wire.Build(
		forj.WireSet,
		generalSet,
		cmdSet,
		logger.ProvideAppLogger,
		NewApplication,
	)

	return App{}, nil
}
