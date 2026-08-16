package atlas

import (
	"context"

	"github.com/goforj/atlas/install"
)

// UpdateCmd refreshes previously installed Atlas integration files.
type UpdateCmd struct {
	Agent         []string `help:"Agent to update. Repeatable. Supported: codex, claude, copilot, gemini"`
	AllAgents     bool     `help:"Update all supported agents"`
	Discover      bool     `help:"Re-detect the preferred local agent and update the committed selection"`
	Guidelines    *bool    `help:"Update generated guideline files"`
	Skills        *bool    `help:"Update generated skill or instruction files"`
	MCP           *bool    `help:"Update MCP configuration"`
	NoInteraction bool     `name:"no-interaction" help:"Run without interactive prompts"`
	DryRun        bool     `name:"dry-run" help:"Show files that would be written without changing the project"`
}

// NewUpdateCmd creates an UpdateCmd.
func NewUpdateCmd() *UpdateCmd {
	return &UpdateCmd{}
}

// Signature returns the Kong metadata for UpdateCmd.
func (*UpdateCmd) Signature() string {
	return `name:"atlas:update" help:"Update Atlas agent integration"`
}

// Run executes the Atlas update workflow.
func (c *UpdateCmd) Run() error {
	ctx := context.Background()
	opts := install.Options{
		Root:          ".",
		Project:       Project("."),
		Agents:        c.Agent,
		AllAgents:     c.AllAgents,
		Discover:      c.Discover,
		NoInteraction: c.NoInteraction,
		DryRun:        c.DryRun,
	}
	applySurfaceSelections(&opts, c.Guidelines, c.Skills, c.MCP)
	result, err := install.NewUpdater().Update(ctx, opts)
	if err != nil {
		return err
	}
	if c.DryRun {
		printResult("Would update Atlas", result)
		return nil
	}
	if _, err := reconcileInstalledGuidance(".", result); err != nil {
		return err
	}
	printResult("Updated Atlas", result)
	return nil
}
