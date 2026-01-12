package forj

import "github.com/alecthomas/kong"

// RootCmd is the root command for the GoForj CLI application.
type RootCmd struct {
	Version           kong.VersionFlag  `help:"Show version information" version:"${version}"`
	MakeCommandCmd    MakeCommandCmd    `cmd:"" name:"make:command" help:"Generate a new CLI command"`
	MakeControllerCmd MakeControllerCmd `cmd:"" name:"make:controller" help:"Generate a new controller"`
	MakeMigrationCmd  MakeMigrationCmd  `cmd:"" name:"make:migration" help:"Generate a new migration"`
	NewProjectCmd     NewProjectCmd     `cmd:"" name:"new" help:"New project command"`
	DevCmd            DevCmd            `cmd:"" name:"dev" help:"Run development watchers"`
	WgoCmd            WgoCmd            `cmd:"" name:"wgo" help:"Run wgo with GoForj defaults"`
	DownCmd           DownCmd           `cmd:"" name:"down" help:"Bring down development resources"`
	BuildBinaryCmd    BuildBinaryCmd    `cmd:"" name:"build" help:"Build the GoForj binary" hidden:""`
	TestRenderCmd     TestRenderCmd     `cmd:"" name:"test:render" help:"Render full project and run build/tests" hidden:""`
	TestRendersCmd    TestRendersCmd    `cmd:"" name:"test:renders" help:"Runs all combinations of project configurations to test rendering" hidden:""`
	RenderCmd         RenderCmd         `cmd:"" name:"render" help:"Run the project renderer" hidden:""`
	RunCmd            RunCmd            `cmd:"" name:"run" help:"Runs go run main.go when project detects changes from the 'App' watcher"`
}

// NewRootCmd creates a new instance of RootCmd with the provided commands.
func NewRootCmd(
	makeMigrationCmd *MakeMigrationCmd,
	makeControllerCmd *MakeControllerCmd,
	makeCommandCmd *MakeCommandCmd,
	newProjectCmd *NewProjectCmd,
	devCmd *DevCmd,
	wgoCmd *WgoCmd,
	downCmd *DownCmd,
	buildBinaryCmd *BuildBinaryCmd,
	testRenderCmd *TestRenderCmd,
	testRendersCmd *TestRendersCmd,
	rendererCmd *RenderCmd,
	runCmd *RunCmd,
) *RootCmd {
	return &RootCmd{
		MakeMigrationCmd:  *makeMigrationCmd,
		MakeControllerCmd: *makeControllerCmd,
		MakeCommandCmd:    *makeCommandCmd,
		NewProjectCmd:     *newProjectCmd,
		DevCmd:            *devCmd,
		WgoCmd:            *wgoCmd,
		DownCmd:           *downCmd,
		BuildBinaryCmd:    *buildBinaryCmd,
		TestRenderCmd:     *testRenderCmd,
		TestRendersCmd:    *testRendersCmd,
		RenderCmd:         *rendererCmd,
		RunCmd:            *runCmd,
	}
}
