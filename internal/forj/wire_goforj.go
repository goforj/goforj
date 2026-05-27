package forj

import (
	"github.com/goforj/goforj/internal/bench"
	"github.com/goforj/goforj/internal/build"
	"github.com/goforj/goforj/internal/forj/makecmd"
	"github.com/goforj/goforj/internal/generate"
	"github.com/google/wire"
)

// WireSet is a Wire set that provides all the commands for the GoForj CLI application.
var WireSet = wire.NewSet(
	build.NewAPIIndexRunner,
	build.NewCmd,
	makecmd.NewCommandCmd,
	makecmd.NewControllerCmd,
	makecmd.NewMigrationCmd,
	generate.NewCmd,
	NewNewProjectCmd,
	NewDevCmd,
	NewDownCmd,
	NewBuildBinaryCmd,
	NewTestRenderCmd,
	NewTestRendersCmd,
	NewTestIntegrationCmd,
	bench.NewInspectOverheadMeasureCmd,
	bench.NewLoggerOverheadMeasureCmd,
	bench.NewHTTPLiveProfileCmd,
	bench.NewHTTPRuntimeProfileCmd,
	bench.NewMetricsOverheadMeasureCmd,
	NewTestConsoleCmd,
	NewTestOpenAPICmd,
	NewRootCmd,
	build.NewRunCmd,
	NewCmd,
)
