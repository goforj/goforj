package forj

import (
	"github.com/google/wire"
)

// WireSet is a Wire set that provides all the commands for the GoForj CLI application.
var WireSet = wire.NewSet(
	NewMakeCommandCmd,
	NewMakeControllerCmd,
	NewMakeMigrationCmd,
	NewNewProjectCmd,
	NewDevCmd,
	NewWgoCmd,
	NewDownCmd,
	NewBuildBinaryCmd,
	NewTestRenderCmd,
	NewTestRendersCmd,
	NewTestIntegrationCmd,
	NewTestConsoleCmd,
	NewRootCmd,
	NewRunCmd,
	NewCmd,
)
