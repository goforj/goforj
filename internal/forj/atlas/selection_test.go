package atlas

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRecommendedAgentsSelectsOnePreferredInstalledAgent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, dir := range []string{".codex", ".claude", ".gemini"} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	recommended := RecommendedAgents(context.Background(), t.TempDir())
	if len(recommended) != 1 || recommended[0] != "codex" {
		t.Fatalf("recommended agents = %v, want [codex]", recommended)
	}
}

func TestAgentOptionsIgnoresParentProjectAgentFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("# Parent project rules\n"), 0o644); err != nil {
		t.Fatalf("write parent project fixture: %v", err)
	}
	for _, option := range AgentOptions(context.Background(), root) {
		if option.Name == "claude" && option.Detected {
			t.Fatal("new-project selection inherited Claude from the parent project")
		}
	}
}
