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

// RunInstall writes Atlas-owned files and reconciles native guidance through GoForj.
func RunInstall(ctx context.Context, opts InstallOptions) (install.Result, error) {
	root := firstNonEmpty(opts.Root, ".")
	request := install.HostRequest{
		Root:          root,
		Project:       Project(root),
		Agents:        opts.Agents,
		AllAgents:     opts.AllAgents,
		Guidelines:    opts.Guidelines,
		Skills:        opts.Skills,
		MCP:           opts.MCP,
		NoInteraction: opts.NoInteraction,
		DryRun:        opts.DryRun,
	}
	hostResult, err := install.NewHostInstaller().Install(ctx, request)
	result := install.Result{Agents: hostResult.Agents, Files: hostResult.Files}
	if err != nil {
		return result, err
	}
	if opts.DryRun {
		return includePlannedGuidance(root, result, hostResult.Guidance)
	}
	_, err = reconcileInstalledGuidance(root, hostResult.Guidance)
	if err != nil {
		return result, err
	}
	return result, nil
}

// includePlannedGuidance adds host-owned native projections to Atlas dry-run output.
func includePlannedGuidance(root string, result install.Result, intent install.GuidanceReconciliation) (install.Result, error) {
	plan, err := PlanGuidanceIntent(root, intent)
	if err != nil {
		return result, err
	}
	result.Files = append(result.Files, plan.Updated...)
	return result, nil
}

// reconcileInstalledGuidance applies Atlas's typed request through GoForj's native projection writer.
func reconcileInstalledGuidance(root string, intent install.GuidanceReconciliation) (ReconcileGuidanceResult, error) {
	return ReconcileGuidanceIntent(root, intent)
}
