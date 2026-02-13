package forj

import "github.com/alecthomas/kong"

// RootCmd is the root command for the GoForj CLI application.
type RootCmd struct {
	Version              kong.VersionFlag     `help:"Show version information" version:"${version}"`
	BuildCmd             BuildCmd             `cmd:""`
	ApiIndexCmd          ApiIndexCmd          `cmd:""`
	MakeCommandCmd       MakeCommandCmd       `cmd:""`
	MakeControllerCmd    MakeControllerCmd    `cmd:""`
	MakeMigrationCmd     MakeMigrationCmd     `cmd:""`
	NewProjectCmd        NewProjectCmd        `cmd:""`
	DevCmd               DevCmd               `cmd:""`
	DownCmd              DownCmd              `cmd:""`
	BuildBinaryCmd       BuildBinaryCmd       `cmd:""`
	TestRenderCmd        TestRenderCmd        `cmd:""`
	TestRendersCmd       TestRendersCmd       `cmd:""`
	TestIntegrationCmd   TestIntegrationCmd   `cmd:""`
	TestDBIntegrationCmd TestDBIntegrationCmd `cmd:""`
	TestConsoleCmd       TestConsoleCmd       `cmd:""`
	RenderCmd            RenderCmd            `cmd:""`
	RunCmd               RunCmd               `cmd:""`
}

// NewRootCmd creates a new instance of RootCmd with the provided commands.
func NewRootCmd(
	buildCmd *BuildCmd,
	apiIndexCmd *ApiIndexCmd,
	makeMigrationCmd *MakeMigrationCmd,
	makeControllerCmd *MakeControllerCmd,
	makeCommandCmd *MakeCommandCmd,
	newProjectCmd *NewProjectCmd,
	devCmd *DevCmd,
	downCmd *DownCmd,
	buildBinaryCmd *BuildBinaryCmd,
	testRenderCmd *TestRenderCmd,
	testRendersCmd *TestRendersCmd,
	testIntegrationCmd *TestIntegrationCmd,
	testDBIntegrationCmd *TestDBIntegrationCmd,
	testConsoleCmd *TestConsoleCmd,
	rendererCmd *RenderCmd,
	runCmd *RunCmd,
) *RootCmd {
	return &RootCmd{
		BuildCmd:             *buildCmd,
		ApiIndexCmd:          *apiIndexCmd,
		MakeMigrationCmd:     *makeMigrationCmd,
		MakeControllerCmd:    *makeControllerCmd,
		MakeCommandCmd:       *makeCommandCmd,
		NewProjectCmd:        *newProjectCmd,
		DevCmd:               *devCmd,
		DownCmd:              *downCmd,
		BuildBinaryCmd:       *buildBinaryCmd,
		TestRenderCmd:        *testRenderCmd,
		TestRendersCmd:       *testRendersCmd,
		TestIntegrationCmd:   *testIntegrationCmd,
		TestDBIntegrationCmd: *testDBIntegrationCmd,
		TestConsoleCmd:       *testConsoleCmd,
		RenderCmd:            *rendererCmd,
		RunCmd:               *runCmd,
	}
}
