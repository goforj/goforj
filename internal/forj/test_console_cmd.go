package forj

import (
	"fmt"

	"github.com/goforj/console"
)

// TestConsoleCmd prints semantic console output samples.
type TestConsoleCmd struct{}

func (*TestConsoleCmd) Signature() string {
	return `name:"test:console" help:"Print semantic console output samples" hidden:""`
}

// NewTestConsoleCmd creates a new TestConsoleCmd.
func NewTestConsoleCmd() *TestConsoleCmd {
	return &TestConsoleCmd{}
}

// Run prints all console marks and sample messages.
func (cmd *TestConsoleCmd) Run() error {
	fmt.Printf("Marks: %s %s %s %s %s %s\n",
		console.ActionMark(),
		console.InfoMark(),
		console.SuccessMark(),
		console.WarnMark(),
		console.ErrorMark(),
		console.DebugMark(),
	)

	console.Actionf("Running action example")
	console.Infof("Informational output")
	console.Successf("Success output")
	console.Warnf("Warning output")
	console.Errorf("Error output")
	console.Debugf("Debug output (set FORJ_DEBUG=1 to see this)")
	return nil
}
