package cmd

import "github.com/goforj/goforj/internal/goforj"

// RootCmd is the root command for the application.
type RootCmd struct {
	goforj.RootCmd

	HelloWorldCmd HelloWorldCmd `cmd:"" name:"hello:world" help:"Hello world command" hidden:""`
}

// NewRootCmd creates a new RootCmd with the given commands.
func NewRootCmd(
	goForgeRootCmd *goforj.RootCmd,
	helloWorldCmd *HelloWorldCmd,
) *RootCmd {
	return &RootCmd{
		RootCmd:       *goForgeRootCmd,
		HelloWorldCmd: *helloWorldCmd,
	}
}
