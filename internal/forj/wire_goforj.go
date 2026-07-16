package forj

import (
	"github.com/goforj/goforj/internal/apiindex"
	"github.com/goforj/goforj/internal/build"
	"github.com/goforj/wire"
)

// ProvideAPIIndexRunner keeps App selection late-bound because CLI dispatch sets FORJ_APP after root wiring.
func ProvideAPIIndexRunner() *apiindex.Runner {
	return apiindex.NewRunner(build.ActiveApp)
}

// WireSet provides native GoForj CLI dependencies; app-owned generators are rendered into apps.
var WireSet = wire.NewSet(
	ProvideAPIIndexRunner,
	NewProjectAuthoringCommands,
	NewBuildCommands,
	NewRuntimeCommands,
	NewAtlasCommands,
	NewTestCommands,
	NewBenchmarkCommands,
	NewScenarioCommands,
	NewBackupCommands,
	NewRootCmd,
)
