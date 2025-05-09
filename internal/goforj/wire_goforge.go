package goforj

import (
	"github.com/google/wire"
)

// WireSet is a Wire set that provides all the commands for the GoForge CLI application.
var WireSet = wire.NewSet(
	NewMakeCommandCmd,
	NewMakeControllerCmd,
	NewMakeMigrationCmd,
	NewNewProjectCmd,
	NewDevCmd,
	NewBuildBinaryCmd,
	NewRootCmd,
	NewRunCmd,
	NewCmd,
)
