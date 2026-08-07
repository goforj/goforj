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

// AgentOptions returns supported agents with machine-local installation state.
func AgentOptions(ctx context.Context, _ string) []AgentOption {
	options := []AgentOption{}
	for _, agent := range agents.Builtins() {
		options = append(options, AgentOption{
			Name:        agent.Name(),
			DisplayName: agent.DisplayName(),
			Detected:    agent.DetectSystem(ctx),
		})
	}
	return options
}

// RecommendedAgents returns one preferred installed agent or Codex when none are detected.
func RecommendedAgents(ctx context.Context, root string) []string {
	options := AgentOptions(ctx, root)
	for _, option := range options {
		if option.Detected {
			return []string{option.Name}
		}
	}
	return []string{"codex"}
}

// DisplayName returns the user-facing name for a supported agent.
func DisplayName(name string) string {
	if agent, ok := agents.ByName(name); ok {
		return agent.DisplayName()
	}
	return name
}
