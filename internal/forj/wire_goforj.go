package forj

import (
	"github.com/goforj/goforj/internal/apiindex"
	"github.com/goforj/goforj/internal/backup"
	"github.com/goforj/goforj/internal/bench"
	"github.com/goforj/goforj/internal/build"
	"github.com/goforj/goforj/internal/forj/atlas"
	"github.com/goforj/goforj/internal/forj/makeapp"
	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/wire"
)

// ProvideAPIIndexRunner keeps App selection late-bound because CLI dispatch sets FORJ_APP after root wiring.
func ProvideAPIIndexRunner(appLogger *logger.AppLogger) *apiindex.Runner {
	return apiindex.NewRunner(appLogger, build.ActiveApp)
}

// WireSet provides native GoForj CLI dependencies; app-owned generators are rendered into apps.
var WireSet = wire.NewSet(
	ProvideAPIIndexRunner,
	wire.Bind(new(apiindex.Preparer), new(*apiindex.Runner)),
	build.NewCmd,
	apiindex.NewCmd,
	generate.NewCmd,
	NewNewProjectCmd,
	atlas.NewInstallCmd,
	atlas.NewUpdateCmd,
	atlas.NewDoctorCmd,
	atlas.NewListSkillsCmd,
	atlas.NewMakeSkillCmd,
	atlas.NewMCPCmd,
	makeapp.NewCmd,
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
	backup.NewPlanCmd,
	backup.NewListCmd,
	backup.NewCreateCmd,
	backup.NewVerifyCmd,
	backup.NewRestoreCmd,
	backup.NewPruneCmd,
	backup.NewStatusCmd,
	NewCmd,
)
