package goforj

// RootCmd is the root command for the GoForj CLI application.
type RootCmd struct {
	MakeCommandCmd    MakeCommandCmd    `cmd:"" name:"make:command" help:"Generate a new CLI command"`
	MakeControllerCmd MakeControllerCmd `cmd:"" name:"make:controller" help:"Generate a new controller"`
	MakeMigrationCmd  MakeMigrationCmd  `cmd:"" name:"make:migration" help:"Generate a new migration"`
	NewProjectCmd     NewProjectCmd     `cmd:"" name:"new" help:"New project command"`
	DevCmd            DevCmd            `cmd:"" name:"dev" help:"Run development watchers"`
	BuildBinaryCmd    BuildBinaryCmd    `cmd:"" name:"build" help:"Build the GoForj binary" hidden:""`
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
	buildBinaryCmd *BuildBinaryCmd,
	rendererCmd *RenderCmd,
	runCmd *RunCmd,
) *RootCmd {
	return &RootCmd{
		MakeMigrationCmd:  *makeMigrationCmd,
		MakeControllerCmd: *makeControllerCmd,
		MakeCommandCmd:    *makeCommandCmd,
		NewProjectCmd:     *newProjectCmd,
		DevCmd:            *devCmd,
		BuildBinaryCmd:    *buildBinaryCmd,
		RenderCmd:         *rendererCmd,
		RunCmd:            *runCmd,
	}
}
