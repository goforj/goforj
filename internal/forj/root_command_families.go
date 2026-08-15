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
)

// projectAuthoringCommands carries commands that create or reconcile generated project structure.
type projectAuthoringCommands struct {
	generate   generate.Cmd
	newProject NewProjectCmd
	makeApp    makeapp.Cmd
	render     RenderCmd
}

// NewProjectAuthoringCommands constructs the project-authoring family from its shared renderer dependencies.
func NewProjectAuthoringCommands(appLogger *logger.AppLogger, renderer *ProjectRenderer) *projectAuthoringCommands {
	return &projectAuthoringCommands{
		generate:   *generate.NewCmd(),
		newProject: *NewNewProjectCmd(appLogger, renderer),
		makeApp:    *makeapp.NewCmd(appLogger, renderer),
		render:     *NewCmd(appLogger, renderer),
	}
}

// buildCommands carries commands that compile GoForj or prepare build artifacts.
type buildCommands struct {
	build       build.Cmd
	apiIndex    apiindex.Cmd
	buildBinary BuildBinaryCmd
}

// NewBuildCommands constructs the build family around one API index runner.
func NewBuildCommands(appLogger *logger.AppLogger, runner *apiindex.Runner) *buildCommands {
	return &buildCommands{
		build:       *build.NewCmd(appLogger, runner),
		apiIndex:    *apiindex.NewCmd(runner),
		buildBinary: *NewBuildBinaryCmd(appLogger),
	}
}

// runtimeCommands carries commands that start, supervise, or stop an App runtime.
type runtimeCommands struct {
	dev       DevCmd
	devStatus DevStatusCmd
	down      DownCmd
	run       build.RunCmd
}

// NewRuntimeCommands constructs the runtime family around its shared logger and API index runner.
func NewRuntimeCommands(appLogger *logger.AppLogger, runner *apiindex.Runner) *runtimeCommands {
	return &runtimeCommands{
		dev:       DevCmd{},
		devStatus: DevStatusCmd{},
		down:      *NewDownCmd(appLogger),
		run:       *build.NewRunCmd(appLogger, runner),
	}
}

// atlasCommands carries the Atlas support commands as one composition unit.
type atlasCommands struct {
	install    atlas.InstallCmd
	update     atlas.UpdateCmd
	doctor     atlas.DoctorCmd
	listSkills atlas.ListSkillsCmd
	makeSkill  atlas.MakeSkillCmd
	mcp        atlas.MCPCmd
	eval       atlas.EvalCmd
}

// NewAtlasCommands constructs the complete dependency-free Atlas command family.
func NewAtlasCommands() *atlasCommands {
	return &atlasCommands{
		install:    *atlas.NewInstallCmd(),
		update:     *atlas.NewUpdateCmd(),
		doctor:     *atlas.NewDoctorCmd(),
		listSkills: *atlas.NewListSkillsCmd(),
		makeSkill:  *atlas.NewMakeSkillCmd(),
		mcp:        *atlas.NewMCPCmd(),
		eval:       atlas.EvalCmd{},
	}
}

// testCommands carries framework test and diagnostic commands as one composition unit.
type testCommands struct {
	render      TestRenderCmd
	renders     TestRendersCmd
	integration TestIntegrationCmd
	console     TestConsoleCmd
	openAPI     TestOpenAPICmd
}

// NewTestCommands constructs the complete test family from its shared logger.
func NewTestCommands(appLogger *logger.AppLogger) *testCommands {
	return &testCommands{
		render:      *NewTestRenderCmd(appLogger),
		renders:     *NewTestRendersCmd(),
		integration: *NewTestIntegrationCmd(appLogger),
		console:     *NewTestConsoleCmd(),
		openAPI:     *NewTestOpenAPICmd(appLogger),
	}
}

// benchmarkCommands carries runtime measurement and profiling commands as one composition unit.
type benchmarkCommands struct {
	inspectOverhead bench.InspectOverheadMeasureCmd
	loggerOverhead  bench.LoggerOverheadMeasureCmd
	httpLive        bench.HTTPLiveProfileCmd
	httpRuntime     bench.HTTPRuntimeProfileCmd
	metricsOverhead bench.MetricsOverheadMeasureCmd
}

// NewBenchmarkCommands constructs the complete benchmark family from its shared logger.
func NewBenchmarkCommands(appLogger *logger.AppLogger) *benchmarkCommands {
	return &benchmarkCommands{
		inspectOverhead: *bench.NewInspectOverheadMeasureCmd(appLogger),
		loggerOverhead:  *bench.NewLoggerOverheadMeasureCmd(appLogger),
		httpLive:        *bench.NewHTTPLiveProfileCmd(appLogger),
		httpRuntime:     *bench.NewHTTPRuntimeProfileCmd(appLogger),
		metricsOverhead: *bench.NewMetricsOverheadMeasureCmd(appLogger),
	}
}

// scenarioCommands carries scenario inventory, generation, and execution commands as one composition unit.
type scenarioCommands struct {
	list     ScenarioListCmd
	generate ScenarioGenerateCmd
	test     ScenarioTestCmd
}

// NewScenarioCommands constructs the complete scenario family from its shared logger.
func NewScenarioCommands(appLogger *logger.AppLogger) *scenarioCommands {
	return &scenarioCommands{
		list:     *NewScenarioListCmd(),
		generate: *NewScenarioGenerateCmd(),
		test:     *NewScenarioTestCmd(appLogger),
	}
}

// backupCommands carries backup planning and execution commands as one composition unit.
type backupCommands struct {
	plan    backup.PlanCmd
	list    backup.ListCmd
	create  backup.CreateCmd
	verify  backup.VerifyCmd
	restore backup.RestoreCmd
	prune   backup.PruneCmd
	status  backup.StatusCmd
}

// NewBackupCommands constructs the complete dependency-free backup command family.
func NewBackupCommands() *backupCommands {
	return &backupCommands{
		plan:    *backup.NewPlanCmd(),
		list:    *backup.NewListCmd(),
		create:  *backup.NewCreateCmd(),
		verify:  *backup.NewVerifyCmd(),
		restore: *backup.NewRestoreCmd(),
		prune:   *backup.NewPruneCmd(),
		status:  *backup.NewStatusCmd(),
	}
}
