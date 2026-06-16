package atlas

import (
	"context"

	"github.com/goforj/atlas/agents"
)

// AgentOption describes one local AI agent GoForj can configure.
type AgentOption struct {
	Name        string
	DisplayName string
	Detected    bool
}

// AgentOptions returns supported agents with project/system detection state.
func AgentOptions(ctx context.Context, root string) []AgentOption {
	root = firstNonEmpty(root, ".")
	options := []AgentOption{}
	for _, agent := range agents.Builtins() {
		options = append(options, AgentOption{
			Name:        agent.Name(),
			DisplayName: agent.DisplayName(),
			Detected:    agent.DetectProject(root) || agent.DetectSystem(ctx),
		})
	}
	return options
}

// RecommendedAgents returns detected agents or Codex when none are detected.
func RecommendedAgents(ctx context.Context, root string) []string {
	options := AgentOptions(ctx, root)
	names := []string{}
	for _, option := range options {
		if option.Detected {
			names = append(names, option.Name)
		}
	}
	if len(names) == 0 {
		return []string{"codex"}
	}
	return names
}

// DisplayName returns the user-facing name for a supported agent.
func DisplayName(name string) string {
	if agent, ok := agents.ByName(name); ok {
		return agent.DisplayName()
	}
	return name
}
