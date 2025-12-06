package main

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/forj/forj/internal/cmd"
	"github.com/forj/forj/wire"
)

func main() {
	// Default environment
	_ = os.Setenv("APP_ENV", "local")
	_ = os.Setenv("APP_NAME", "GoForj")

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
		kong.Name("forj"),
		kong.Description("🛠  GoForj CLI ❯ Scaffolding, Automation, and Developer Productivity for Go Applications"),
		kong.Help(cmd.KongHelpFormatter),
	)
	if err != nil {
		app.Logger().Fatal().Err(err).Msg("Error setting up CLI parser")
		return
	}

	args := os.Args[1:]
	if len(args) == 0 {
		ctx, _ := parser.Parse([]string{"--help"})
		ctx.PrintUsage(false)
		return
	}

	// Parse CLI args
	ctx, err := parser.Parse(args)
	if err != nil {
		parser.FatalIfErrorf(err)
		return
	}

	// Execute command
	err = ctx.Run()
	if err != nil {
		app.Logger().Fatal().Err(err).Msg("Error executing command")
	}
}
