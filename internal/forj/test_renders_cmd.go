package forj

import (
	"os"

	"github.com/goforj/goforj/internal/rendercheck"
)

// TestRendersCmd exposes generated-project render coverage through the framework CLI.
type TestRendersCmd struct {
	// Profile selects the render coverage strategy.
	Profile string `help:"Render profile to run" enum:"smoke,pr,full" default:"pr"`

	// Full runs the exhaustive core matrix plus sentinel combinations. Prefer --profile=full for new usage.
	Full bool `help:"Run the full component matrix"`

	// RunTests executes rendered Go test packages after render/build validation.
	RunTests bool `help:"Run rendered Go test packages after render/build" short:"t"`

	// List prints the selected combinations without rendering them.
	List bool `help:"List selected render combinations without running them"`
}

// Signature exposes the render matrix command for framework validation.
func (*TestRendersCmd) Signature() string {
	return `name:"test:renders" help:"Runs all combinations of project configurations to test rendering" hidden:""`
}

// NewTestRendersCmd creates a render matrix command within the shared command graph.
func NewTestRendersCmd() *TestRendersCmd {
	return &TestRendersCmd{}
}

// Run keeps command parsing at the CLI boundary while render validation remains independently testable.
func (cmd *TestRendersCmd) Run() error {
	suite := rendercheck.NewSuite(cmd.Profile, cmd.Full)
	if cmd.List {
		return suite.List(os.Stdout)
	}
	return suite.Run(cmd.RunTests)
}
