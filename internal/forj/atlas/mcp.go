package atlas

import (
	"github.com/goforj/atlas/mcp"
	"github.com/goforj/goforj/version"
)

// MCPCmd starts the Atlas MCP server over stdio.
type MCPCmd struct{}

// NewMCPCmd creates an MCPCmd.
func NewMCPCmd() *MCPCmd {
	return &MCPCmd{}
}

// Signature returns the Kong metadata for MCPCmd.
func (*MCPCmd) Signature() string {
	return `name:"atlas:mcp" help:"Start the Atlas MCP server" hidden:""`
}

// Run starts Atlas MCP over stdio. Stdout must stay reserved for MCP JSON-RPC.
func (*MCPCmd) Run() error {
	return mcp.ServeStdio(mcp.Server{
		Project: Project("."),
		Version: version.String(),
	})
}
