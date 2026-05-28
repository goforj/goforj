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

func (*ScenarioListCmd) Signature() string {
	return `name:"scenario:list" help:"List executable scenario specs (maintainer)" hidden:""`
}

func NewScenarioListCmd() *ScenarioListCmd {
	return &ScenarioListCmd{}
}

func (c *ScenarioListCmd) Run() error {
	specs, err := scenarios.List(c.SpecDir)
	if err != nil {
		return err
	}
	for _, spec := range specs {
		fmt.Printf("%s\t%s\n", spec.ID, spec.Title)
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

func (*ScenarioGenerateCmd) Signature() string {
	return `name:"scenario:generate" help:"Generate scenario docs from executable specs (maintainer)" hidden:""`
}

func NewScenarioGenerateCmd() *ScenarioGenerateCmd {
	return &ScenarioGenerateCmd{}
}

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

func (*ScenarioTestCmd) Signature() string {
	return `name:"scenario:test" help:"Validate executable scenario specs (maintainer)" hidden:""`
}

func NewScenarioTestCmd(logger *logger.AppLogger) *ScenarioTestCmd {
	return &ScenarioTestCmd{logger: logger}
}

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
