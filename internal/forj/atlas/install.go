package atlas

import "context"

// InstallCmd installs local agent integration files for a GoForj project.
type InstallCmd struct {
	Agent         []string `help:"Agent to install. Repeatable. Supported: codex, claude, copilot, gemini"`
	AllAgents     bool     `help:"Install all supported agents"`
	Guidelines    bool     `help:"Install generated guideline files"`
	Skills        bool     `help:"Install generated skill or instruction files"`
	MCP           bool     `help:"Install MCP configuration"`
	NoInteraction bool     `name:"no-interaction" help:"Run without interactive prompts"`
}

// NewInstallCmd creates an InstallCmd.
func NewInstallCmd() *InstallCmd {
	return &InstallCmd{}
}

// Signature returns the Kong metadata for InstallCmd.
func (*InstallCmd) Signature() string {
	return `name:"atlas:install" help:"Install Atlas agent integration"`
}

// Run executes the Atlas install workflow.
func (c *InstallCmd) Run() error {
	result, err := RunInstall(context.Background(), InstallOptions{
		Root:          ".",
		Agents:        c.Agent,
		AllAgents:     c.AllAgents,
		Guidelines:    c.Guidelines,
		Skills:        c.Skills,
		MCP:           c.MCP,
		NoInteraction: c.NoInteraction,
	})
	if err != nil {
		return err
	}
	printResult("Installed Atlas", result)
	return nil
}
