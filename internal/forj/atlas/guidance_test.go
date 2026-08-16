package atlas

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/atlas/agents"
	"github.com/goforj/atlas/config"
	"github.com/goforj/atlas/files"
	"github.com/goforj/goforj/project"
)

// TestReconcileAgentGuidanceUsesStableCodexFallback proves baseline guidance does not depend on host detection or Atlas installation.
func TestReconcileAgentGuidanceUsesStableCodexFallback(t *testing.T) {
	root := writeGuidanceTestProject(t)
	result, err := ReconcileAgentGuidance(root, project.AgentGuidanceBaseline)
	if err != nil {
		t.Fatalf("ReconcileAgentGuidance(): %v", err)
	}
	content := readGuidanceTestFile(t, filepath.Join(root, "AGENTS.md"))
	if !strings.Contains(content, "`forj make:*`") || !strings.Contains(content, "flat, self-contained, and portable") || !strings.Contains(content, "can stand on its own") || !strings.Contains(content, "<!-- "+files.DefaultMarker+":start -->") {
		t.Fatalf("AGENTS.md missing canonical baseline:\n%s", content)
	}
	if len(result.Updated) != 1 || result.Updated[0] != filepath.Join(root, "AGENTS.md") {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Stat(config.FilePath(root)); !os.IsNotExist(err) {
		t.Fatalf("baseline reconciliation created Atlas installation state: %v", err)
	}
}

// TestReconcileAgentGuidanceSupportsEveryNativeTarget prevents baseline ownership from becoming Codex-specific.
func TestReconcileAgentGuidanceSupportsEveryNativeTarget(t *testing.T) {
	root := writeGuidanceTestProject(t)
	names := []string{}
	for _, agent := range agents.Builtins() {
		names = append(names, agent.Name())
	}
	if err := config.Save(root, config.Config{
		Version:  config.CurrentVersion,
		Features: config.Features{Guidelines: true},
		Agents:   names,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ReconcileAgentGuidance(root, project.AgentGuidanceBaseline); err != nil {
		t.Fatal(err)
	}
	for _, agent := range agents.Builtins() {
		content := readGuidanceTestFile(t, agent.GuidelinesPath(root))
		if !strings.Contains(content, "<!-- "+files.DefaultMarker+":start -->") {
			t.Fatalf("%s projection missing managed marker", agent.Name())
		}
	}
}

// TestReconcileAgentGuidancePreservesUserContentAndRemovesOnlyManagedBlocks protects native instruction ownership.
func TestReconcileAgentGuidancePreservesUserContentAndRemovesOnlyManagedBlocks(t *testing.T) {
	root := writeGuidanceTestProject(t)
	path := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(path, []byte("# Local Rules\n\nKeep this.\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := ReconcileAgentGuidance(root, project.AgentGuidanceBaseline); err != nil {
		t.Fatal(err)
	}
	if _, err := ReconcileAgentGuidance(root, project.AgentGuidanceNone); err != nil {
		t.Fatal(err)
	}
	content := readGuidanceTestFile(t, path)
	if content != "# Local Rules\n\nKeep this.\n" {
		t.Fatalf("user guidance changed:\n%s", content)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %o, want 640", info.Mode().Perm())
	}
}

// TestReconcileAgentGuidanceUsesCommittedTargets prevents render behavior from changing with the developer machine.
func TestReconcileAgentGuidanceUsesCommittedTargets(t *testing.T) {
	root := writeGuidanceTestProject(t)
	if err := config.Save(root, config.Config{
		Version:  config.CurrentVersion,
		Features: config.Features{Guidelines: true},
		Agents:   []string{"claude"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(files.Block(files.DefaultMarker, "stale")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReconcileAgentGuidance(root, project.AgentGuidanceBaseline); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("unselected Codex projection remains: %v", err)
	}
	content := readGuidanceTestFile(t, filepath.Join(root, "CLAUDE.md"))
	if !strings.Contains(content, "`forj make:*`") {
		t.Fatalf("CLAUDE.md missing baseline:\n%s", content)
	}
}

// TestInferAgentGuidanceUsesOnlyManagedLegacyEvidence avoids claiming user-authored instructions as GoForj-owned.
func TestInferAgentGuidanceUsesOnlyManagedLegacyEvidence(t *testing.T) {
	root := writeGuidanceTestProject(t)
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# User instructions\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	guidance, err := InferAgentGuidance(root)
	if err != nil {
		t.Fatal(err)
	}
	if guidance != project.AgentGuidanceNone {
		t.Fatalf("user-authored guidance inferred as %q", guidance)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(files.Block(files.DefaultMarker, "legacy")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	guidance, err = InferAgentGuidance(root)
	if err != nil {
		t.Fatal(err)
	}
	if guidance != project.AgentGuidanceBaseline {
		t.Fatalf("managed guidance inferred as %q", guidance)
	}
}

// TestRunInstallSynchronizesDurableGuidance proves optional Atlas installation cannot drift from later GoForj renders.
func TestRunInstallSynchronizesDurableGuidance(t *testing.T) {
	root := writeGuidanceTestProject(t)
	if _, err := RunInstall(t.Context(), InstallOptions{
		Root:       root,
		Agents:     []string{"codex"},
		Guidelines: true,
	}); err != nil {
		t.Fatalf("RunInstall(): %v", err)
	}
	projectConfig, err := project.LoadProjectConfigAt(root)
	if err != nil {
		t.Fatal(err)
	}
	if projectConfig.Render.AgentGuidance != project.AgentGuidanceBaseline {
		t.Fatalf("installed guidance = %q", projectConfig.Render.AgentGuidance)
	}
	if _, err := RunInstall(t.Context(), InstallOptions{
		Root:   root,
		Agents: []string{"codex"},
		Skills: true,
	}); err != nil {
		t.Fatalf("RunInstall(skills only): %v", err)
	}
	projectConfig, err = project.LoadProjectConfigAt(root)
	if err != nil {
		t.Fatal(err)
	}
	if projectConfig.Render.AgentGuidance != project.AgentGuidanceNone {
		t.Fatalf("skills-only guidance = %q", projectConfig.Render.AgentGuidance)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("skills-only install retained managed AGENTS.md: %v", err)
	}
}

// writeGuidanceTestProject creates the minimum metadata required by the canonical Atlas composer.
func writeGuidanceTestProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	content := "project_name: invoices\nmodule_name: example.com/invoices\nrender:\n  components: [cli, web_api]\n  agent_guidance: baseline\n  goforj_version: 0.1.0\n"
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// readGuidanceTestFile reads one native projection with test-local failure reporting.
func readGuidanceTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
