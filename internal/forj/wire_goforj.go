package forj

import (
	"github.com/goforj/goforj/internal/bench"
	"github.com/goforj/goforj/internal/build"
	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/wire"
)

// WireSet provides native GoForj CLI dependencies; app-owned generators are rendered into apps.
var WireSet = wire.NewSet(
	build.NewAPIIndexRunner,
	build.NewCmd,
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
	NewScenarioListCmd,
	NewScenarioGenerateCmd,
	NewScenarioTestCmd,
	NewRootCmd,
	build.NewRunCmd,
	NewCmd,
)
