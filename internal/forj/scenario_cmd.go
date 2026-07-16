package forj

import (
	"fmt"

	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/scenarios"
)

// ScenarioListCmd lists known executable scenario specs.
type ScenarioListCmd struct {
	SpecDir string `help:"Directory containing scenario specs"`
}

// Signature keeps scenario catalog tooling available to maintainers without adding it to normal command discovery.
func (*ScenarioListCmd) Signature() string {
	return `name:"scenario:list" help:"List executable scenario specs (maintainer)" hidden:""`
}

// NewScenarioListCmd constructs the maintainer command without requiring runtime dependencies.
func NewScenarioListCmd() *ScenarioListCmd {
	return &ScenarioListCmd{}
}

// Run prints the validated scenario catalog in a shell-friendly form.
func (c *ScenarioListCmd) Run() error {
	specs, err := scenarios.List(c.SpecDir)
	if err != nil {
		return err
	}
	for _, spec := range specs {
		if _, err := fmt.Printf("%s\t%s\n", spec.ID, spec.Title); err != nil {
			return fmt.Errorf("write scenario catalog: %w", err)
		}
	}
	return nil
}

// ScenarioGenerateCmd generates scenario markdown from executable specs.
type ScenarioGenerateCmd struct {
	SpecDir string   `help:"Directory containing scenario specs"`
	OutDir  string   `help:"Directory to write generated scenario markdown" default:"../goforj-docs/docs/scenarios"`
	Check   bool     `help:"Fail if generated markdown differs from files on disk"`
	All     bool     `help:"Generate all scenarios"`
	IDs     []string `arg:"" optional:"" help:"Scenario IDs to generate"`
}

// Signature keeps scenario generation out of the user-facing command surface.
func (*ScenarioGenerateCmd) Signature() string {
	return `name:"scenario:generate" help:"Generate scenario docs from executable specs (maintainer)" hidden:""`
}

// NewScenarioGenerateCmd constructs the maintainer command with CLI-provided generation options.
func NewScenarioGenerateCmd() *ScenarioGenerateCmd {
	return &ScenarioGenerateCmd{}
}

// Run projects executable scenario specs into their public documentation pages.
func (c *ScenarioGenerateCmd) Run() error {
	return scenarios.Generate(scenarios.GenerateOptions{
		SpecDir: c.SpecDir,
		OutDir:  c.OutDir,
		Check:   c.Check,
		All:     c.All,
		IDs:     c.IDs,
	})
}

// ScenarioTestCmd validates executable scenario specs against a rendered app.
type ScenarioTestCmd struct {
	logger *logger.AppLogger

	SpecDir string   `help:"Directory containing scenario specs"`
	WorkDir string   `help:"Directory for scenario temp apps"`
	Keep    bool     `help:"Keep generated scenario work directories"`
	All     bool     `help:"Test all scenarios"`
	IDs     []string `arg:"" optional:"" help:"Scenario IDs to test"`
}

// Signature keeps executable documentation validation out of normal App workflows.
func (*ScenarioTestCmd) Signature() string {
	return `name:"scenario:test" help:"Validate executable scenario specs (maintainer)" hidden:""`
}

// NewScenarioTestCmd requires the logger used to preserve subprocess diagnostics when a scenario fails.
func NewScenarioTestCmd(logger *logger.AppLogger) *ScenarioTestCmd {
	return &ScenarioTestCmd{logger: logger}
}

// Run validates selected scenarios against fresh rendered Apps.
func (c *ScenarioTestCmd) Run() error {
	return scenarios.Validate(scenarios.ValidateOptions{
		Logger:  c.logger,
		SpecDir: c.SpecDir,
		WorkDir: c.WorkDir,
		Keep:    c.Keep,
		All:     c.All,
		IDs:     c.IDs,
	})
}
