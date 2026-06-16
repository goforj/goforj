package cmd

import "testing"

type prebootHelpRoot struct {
	Example prebootHelpCommand `cmd:""`
}

type prebootHelpCommand struct{}

func (*prebootHelpCommand) Signature() string {
	return `name:"example" help:"Example command" goforj:"preboot"`
}

func (*prebootHelpCommand) Run() error {
	return nil
}

func TestDispatchPrebootCommandHandlesRootHelpBeforeBoot(t *testing.T) {
	handled, err := DispatchPrebootCommand([]string{"--help"}, &prebootHelpRoot{})
	if err != nil {
		t.Fatalf("DispatchPrebootCommand root help returned error: %v", err)
	}
	if !handled {
		t.Fatal("expected root help to be handled before app boot")
	}
}

func TestCommandHelpRequestedAllowsPositionalArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "bare long help", args: []string{"--help"}, want: true},
		{name: "bare short help", args: []string{"-h"}, want: true},
		{name: "positional then help", args: []string{"Wow", "--help"}, want: true},
		{name: "help then positional", args: []string{"--help", "Wow"}, want: true},
		{name: "help command word alone", args: []string{"help"}, want: true},
		{name: "help word as positional", args: []string{"Wow", "help"}, want: false},
		{name: "passthrough help", args: []string{"--", "--help"}, want: false},
		{name: "positional passthrough help", args: []string{"Wow", "--", "--help"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := commandHelpRequested(tt.args); got != tt.want {
				t.Fatalf("commandHelpRequested(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
