package cmd

import "github.com/goforj/goforj/internal/forj"

// RootCmd is the root command for the application.
type RootCmd struct {
	RootCmd forj.RootCmd `kong:"embed"`

	HelloWorldCmd HelloWorldCmd `cmd:"" name:"hello:world" help:"Hello world command" hidden:""`
}

// NewRootCmd creates a new RootCmd with the given commands.
func NewRootCmd(
	goForgeRootCmd *forj.RootCmd,
	helloWorldCmd *HelloWorldCmd,
) *RootCmd {
	return &RootCmd{
		RootCmd:       *goForgeRootCmd,
		HelloWorldCmd: *helloWorldCmd,
	}
}
