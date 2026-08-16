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
	Guidelines    *bool
	Skills        *bool
	MCP           *bool
	NoInteraction bool
	DryRun        bool
}

// RunInstall writes Atlas guidance, skills, and MCP config for a rendered project.
func RunInstall(ctx context.Context, opts InstallOptions) (install.Result, error) {
	root := firstNonEmpty(opts.Root, ".")
	installOptions := install.Options{
		Root:          root,
		Project:       Project(root),
		Agents:        opts.Agents,
		AllAgents:     opts.AllAgents,
		NoInteraction: opts.NoInteraction,
		DryRun:        opts.DryRun,
	}
	applySurfaceSelections(&installOptions, opts.Guidelines, opts.Skills, opts.MCP)
	result, err := install.NewInstaller().Install(ctx, installOptions)
	if err != nil || opts.DryRun {
		return result, err
	}
	_, err = reconcileInstalledGuidance(root, result)
	if err != nil {
		return result, err
	}
	return result, nil
}

// applySurfaceSelections preserves omitted CLI flags while forwarding explicit enable and disable choices to Atlas.
func applySurfaceSelections(opts *install.Options, guidelines *bool, skills *bool, mcp *bool) {
	opts.GuidelinesSelection = guidelines
	opts.SkillsSelection = skills
	opts.MCPSelection = mcp
}

// reconcileInstalledGuidance applies Atlas's typed request through GoForj's native projection writer.
func reconcileInstalledGuidance(root string, result install.Result) (ReconcileGuidanceResult, error) {
	return ReconcileGuidanceIntent(root, result.Guidance)
}
