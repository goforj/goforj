package atlas

import (
	"context"

	"github.com/goforj/atlas/install"
)

// InstallOptions controls project-local Atlas files written by GoForj.
type InstallOptions struct {
	Root          string
	Agents        []string
	AllAgents     bool
	Guidelines    bool
	Skills        bool
	MCP           bool
	NoInteraction bool
}

// RunInstall writes Atlas guidance, skills, and MCP config for a rendered project.
func RunInstall(ctx context.Context, opts InstallOptions) (install.Result, error) {
	root := firstNonEmpty(opts.Root, ".")
	return install.NewInstaller().Install(ctx, install.Options{
		Root:          root,
		Project:       Project(root),
		Agents:        opts.Agents,
		AllAgents:     opts.AllAgents,
		Guidelines:    opts.Guidelines,
		Skills:        opts.Skills,
		MCP:           opts.MCP,
		NoInteraction: opts.NoInteraction,
	})
}
