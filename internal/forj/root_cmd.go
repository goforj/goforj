package forj

import "github.com/alecthomas/kong"

// RootCmd is the root command for the GoForj CLI application.
type RootCmd struct {
	Version              kong.VersionFlag     `help:"Show version information" version:"${version}"`
	BuildCmd             BuildCmd             `cmd:"" name:"build" help:"Run all build pipelines" group:"build"`
	ApiIndexCmd          ApiIndexCmd          `cmd:"" name:"build:api-index" help:"Build API index metadata from source" group:"build"`
	MakeCommandCmd       MakeCommandCmd       `cmd:"" name:"make:command" help:"Generate a new CLI command"`
	MakeControllerCmd    MakeControllerCmd    `cmd:"" name:"make:controller" help:"Generate a new controller"`
	MakeMigrationCmd     MakeMigrationCmd     `cmd:"" name:"make:migration" help:"Generate a new migration"`
	NewProjectCmd        NewProjectCmd        `cmd:"" name:"new" help:"New project command"`
	DevCmd               DevCmd               `cmd:"" name:"dev" help:"Run development watchers"`
	DownCmd              DownCmd              `cmd:"" name:"down" help:"Bring down development resources"`
	BuildBinaryCmd       BuildBinaryCmd       `cmd:"" name:"build:binary" help:"Build the GoForj binary" hidden:""`
	TestRenderCmd        TestRenderCmd        `cmd:"" name:"test:render" help:"Render full project and run build/tests" hidden:""`
	TestRendersCmd       TestRendersCmd       `cmd:"" name:"test:renders" help:"Runs all combinations of project configurations to test rendering" hidden:""`
	TestIntegrationCmd   TestIntegrationCmd   `cmd:"" name:"test:integration" help:"Run integration tests" hidden:""`
	TestDBIntegrationCmd TestDBIntegrationCmd `cmd:"" name:"test:db-integration" help:"Run DB integration test workflow" hidden:""`
	TestConsoleCmd       TestConsoleCmd       `cmd:"" name:"test:console" help:"Print semantic console output samples" hidden:""`
	RenderCmd            RenderCmd            `cmd:"" name:"render" help:"Run the project renderer" hidden:""`
	RunCmd               RunCmd               `cmd:"" name:"run" help:"Runs go run main.go when project detects changes from the 'App' watcher"`
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
