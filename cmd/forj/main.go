package main

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/goforj/goforj/internal/build"
	"github.com/goforj/goforj/internal/cmd"
	"github.com/goforj/goforj/version"
	"github.com/goforj/goforj/wire"
)

func main() {
	if build.HandleProfileTool(os.Args[1:]) {
		return
	}

	// Default environment
	setCLIDefaultEnv("APP_ENV", "local")
	setCLIDefaultEnv("APP_NAME", "GoForj")

	// Initialize application
	app, err := wire.InitializeApplication()
	if err != nil {
		fmt.Println("Error initializing application:", err)
		return
	}
	app.Logger().Debug().Msg("App initialized")

	// Setup Kong parser
	parser, err := kong.New(
		app.RootCmd(),
		kong.Name("goforj"),
		kong.Description("GoForj CLI\n  The composable stack for building with Go."),
		kong.Help(cmd.KongHelpFormatter),
		kong.Vars{
			"version": version.String(),
		},
	)
	if err != nil {
		app.Logger().Fatal().Err(err).Msg("Error setting up CLI parser")
		return
	}
	app.RootCmd().RootCmd.RunCmd.Env = localAppEnv()

	args := os.Args[1:]
	if isRootHelp(args) {
		printRootHelp(parser)
		return
	}

	// Parse CLI args
	ctx, err := parser.Parse(args)
	if err != nil {
		if shouldDelegateToAppCommand(args, err) {
			if err := runAppCommandThroughSource(app.RootCmd(), args); err != nil {
				app.Logger().Fatal().Err(err).Msg("Error executing app command")
			}
			return
		}
		parser.FatalIfErrorf(err)
		return
	}

	// Execute command
	err = ctx.Run()
	if err != nil {
		app.Logger().Fatal().Err(err).Msg("Error executing command")
	}
}
