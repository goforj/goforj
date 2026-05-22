package forj

import (
	"github.com/goforj/goforj/internal/build"
	"github.com/goforj/goforj/internal/generate"
	"github.com/google/wire"
)

// WireSet is a Wire set that provides all the commands for the GoForj CLI application.
var WireSet = wire.NewSet(
	build.NewAPIIndexRunner,
	build.NewCmd,
	NewMakeCommandCmd,
	NewMakeControllerCmd,
	NewMakeMigrationCmd,
	generate.NewCmd,
	NewNewProjectCmd,
	NewDevCmd,
	NewDownCmd,
	NewBuildBinaryCmd,
	NewTestRenderCmd,
	NewTestRendersCmd,
	NewTestIntegrationCmd,
	NewTestInspectOverheadCmd,
	NewTestLoggerOverheadCmd,
	NewTestHTTPLiveProfileCmd,
	NewTestHTTPRuntimeProfileCmd,
	NewTestMetricsOverheadCmd,
	NewTestConsoleCmd,
	NewTestOpenAPICmd,
	NewRootCmd,
	build.NewRunCmd,
	NewCmd,
)
