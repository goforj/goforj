package atlas

import (
	"context"
	"fmt"
	"os"

	"github.com/goforj/atlas/install"
)

// DoctorCmd reports local Atlas install health.
type DoctorCmd struct {
	probe func(context.Context, string) mcpProbeResult
}

// NewDoctorCmd creates a DoctorCmd.
func NewDoctorCmd() *DoctorCmd {
	return &DoctorCmd{}
}

// Signature returns the Kong metadata for DoctorCmd.
func (*DoctorCmd) Signature() string {
	return `name:"atlas:doctor" help:"Report Atlas install health"`
}

// Run executes the Atlas install status check.
func (c *DoctorCmd) Run() error {
	ctx := context.Background()
	status, err := install.Status(ctx, install.StatusOptions{
		Root:    ".",
		Project: Project("."),
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "Atlas installed: %t\n", status.Installed)
	fmt.Fprintf(os.Stdout, "Project: %s\n", status.Project)
	if status.GoForjVersion != "" {
		fmt.Fprintf(os.Stdout, "GoForj: %s\n", status.GoForjVersion)
	}
	if status.DocsVersion != "" || status.DocsRevision != "" {
		fmt.Fprintf(os.Stdout, "Docs: %s %s\n", status.DocsVersion, status.DocsRevision)
	}
	fmt.Fprintf(os.Stdout, "Skills: %d/%d present", status.Skills.Present, status.Skills.Expected)
	if status.Skills.Stale {
		fmt.Fprint(os.Stdout, " (stale)")
	}
	fmt.Fprintln(os.Stdout)
	mcpConfigured := false
	for _, agent := range status.Agents {
		fmt.Fprintf(os.Stdout, "Agent %s: configured=%t guidelines=%t mcp_config=%t skills=%t\n", agent.Name, agent.Configured, agent.GuidelinesPresent, agent.MCPPresent, agent.SkillsPresent)
		mcpConfigured = mcpConfigured || (agent.Configured && agent.MCPPresent)
	}
	if mcpConfigured {
		probe := c.probe
		if probe == nil {
			probe = probeMCP
		}
		result := probe(ctx, ".")
		fmt.Fprintf(os.Stdout, "MCP server: executable=%t protocol=%t tools=%d ready=%t\n", result.Executable, result.Protocol, result.Tools, result.Ready)
		if result.Err != nil {
			status.Warnings = append(status.Warnings, "Atlas MCP is not ready: "+result.Err.Error())
		}
	}
	for _, warning := range status.Warnings {
		fmt.Fprintf(os.Stdout, "Warning: %s\n", warning)
	}
	return nil
}
