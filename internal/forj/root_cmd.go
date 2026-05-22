package forj

import (
	"github.com/alecthomas/kong"
	"github.com/goforj/goforj/internal/bench"
	"github.com/goforj/goforj/internal/build"
	"github.com/goforj/goforj/internal/generate"
)

// RootCmd is the root command for the GoForj CLI application.
type RootCmd struct {
	Version                   kong.VersionFlag          `help:"Show version information" version:"${version}"`
	Dev                       bool                      `name:"dev" aliases:"x" env:"FORJ_DEV" help:"Show developer/maintainer commands in help output" hidden:""`
	BuildCmd                  build.Cmd                 `cmd:""`
	MakeCommandCmd            MakeCommandCmd            `cmd:""`
	MakeControllerCmd         MakeControllerCmd         `cmd:""`
	MakeMigrationCmd          MakeMigrationCmd          `cmd:""`
	GenerateCmd               generate.Cmd              `cmd:""`
	NewProjectCmd             NewProjectCmd             `cmd:""`
	DevCmd                    DevCmd                    `cmd:""`
	DownCmd                   DownCmd                   `cmd:""`
	BuildBinaryCmd            BuildBinaryCmd            `cmd:""`
	TestRenderCmd             TestRenderCmd             `cmd:""`
	TestRendersCmd            TestRendersCmd            `cmd:""`
	TestIntegrationCmd        TestIntegrationCmd        `cmd:""`
	InspectOverheadMeasureCmd bench.InspectOverheadMeasureCmd `cmd:""`
	LoggerOverheadMeasureCmd  bench.LoggerOverheadMeasureCmd  `cmd:""`
	HTTPLiveProfileCmd        bench.HTTPLiveProfileCmd        `cmd:""`
	HTTPRuntimeProfileCmd     bench.HTTPRuntimeProfileCmd     `cmd:""`
	MetricsOverheadMeasureCmd bench.MetricsOverheadMeasureCmd `cmd:""`
	TestConsoleCmd            TestConsoleCmd            `cmd:""`
	TestOpenAPICmd            TestOpenAPICmd            `cmd:""`
	RenderCmd                 RenderCmd                 `cmd:""`
	RunCmd                    build.RunCmd              `cmd:""`
}

// NewRootCmd creates a new instance of RootCmd with the provided commands.
func NewRootCmd(
	buildCmd *build.Cmd,
	makeMigrationCmd *MakeMigrationCmd,
	makeControllerCmd *MakeControllerCmd,
	makeCommandCmd *MakeCommandCmd,
	generateCmd *generate.Cmd,
	newProjectCmd *NewProjectCmd,
	devCmd *DevCmd,
	downCmd *DownCmd,
	buildBinaryCmd *BuildBinaryCmd,
	testRenderCmd *TestRenderCmd,
	testRendersCmd *TestRendersCmd,
	testIntegrationCmd *TestIntegrationCmd,
	inspectOverheadMeasureCmd *bench.InspectOverheadMeasureCmd,
	loggerOverheadMeasureCmd *bench.LoggerOverheadMeasureCmd,
	httpLiveProfileCmd *bench.HTTPLiveProfileCmd,
	httpRuntimeProfileCmd *bench.HTTPRuntimeProfileCmd,
	metricsOverheadMeasureCmd *bench.MetricsOverheadMeasureCmd,
	testConsoleCmd *TestConsoleCmd,
	testOpenAPICmd *TestOpenAPICmd,
	rendererCmd *RenderCmd,
	runCmd *build.RunCmd,
) *RootCmd {
	return &RootCmd{
		BuildCmd:                  *buildCmd,
		MakeMigrationCmd:          *makeMigrationCmd,
		MakeControllerCmd:         *makeControllerCmd,
		MakeCommandCmd:            *makeCommandCmd,
		GenerateCmd:               *generateCmd,
		NewProjectCmd:             *newProjectCmd,
		DevCmd:                    *devCmd,
		DownCmd:                   *downCmd,
		BuildBinaryCmd:            *buildBinaryCmd,
		TestRenderCmd:             *testRenderCmd,
		TestRendersCmd:            *testRendersCmd,
		TestIntegrationCmd:        *testIntegrationCmd,
		InspectOverheadMeasureCmd: *inspectOverheadMeasureCmd,
		LoggerOverheadMeasureCmd:  *loggerOverheadMeasureCmd,
		HTTPLiveProfileCmd:        *httpLiveProfileCmd,
		HTTPRuntimeProfileCmd:     *httpRuntimeProfileCmd,
		MetricsOverheadMeasureCmd: *metricsOverheadMeasureCmd,
		TestConsoleCmd:            *testConsoleCmd,
		TestOpenAPICmd:            *testOpenAPICmd,
		RenderCmd:                 *rendererCmd,
		RunCmd:                    *runCmd,
	}
}
