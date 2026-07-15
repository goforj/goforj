package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunGenerationTasksAccountsForLegacyArtifactCleanup covers every primitive migration inventory through the production task boundary.
func TestRunGenerationTasksAccountsForLegacyArtifactCleanup(t *testing.T) {
	tests := []struct {
		name       string
		packageDir string
		files      []string
		selection  GenerationSelection
	}{
		{name: "cache", packageDir: filepath.Join("internal", "caches"), files: cacheLegacyGeneratedFiles, selection: GenerationSelection{Cache: true}},
		{name: "events", packageDir: filepath.Join("internal", "events"), files: eventLegacyGeneratedFiles, selection: GenerationSelection{Events: true}},
		{name: "mail", packageDir: filepath.Join("internal", "mail"), files: mailLegacyGeneratedFiles, selection: GenerationSelection{Mail: true}},
		{name: "queue", packageDir: filepath.Join("internal", "queues"), files: queueLegacyGeneratedFiles, selection: GenerationSelection{Queue: true}},
		{name: "storage", packageDir: filepath.Join("internal", "storages"), files: storageLegacyGeneratedFiles, selection: GenerationSelection{Storage: true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/generationtest\n\ngo 1.26\n"), 0o644); err != nil {
				t.Fatalf("write go.mod: %v", err)
			}
			input := generationInput{
				projectDir:  root,
				environment: generationEnvironment{values: map[string]string{}},
			}
			if _, err := runGenerationTasks(input, test.selection); err != nil {
				t.Fatalf("seed generated files: %v", err)
			}

			for _, name := range test.files {
				path := filepath.Join(root, test.packageDir, name)
				if err := os.WriteFile(path, []byte("legacy\n"), 0o644); err != nil {
					t.Fatalf("write legacy artifact %s: %v", name, err)
				}
			}
			run, err := runGenerationTasks(input, test.selection)
			if err != nil {
				t.Fatalf("remove legacy artifacts: %v", err)
			}
			if run.changedFiles != len(test.files) {
				t.Fatalf("changed files = %d, want %d legacy removals", run.changedFiles, len(test.files))
			}
			if !run.dependencyFilesChanged {
				t.Fatal("legacy removals did not mark dependency files changed")
			}
			for _, name := range test.files {
				path := filepath.Join(root, test.packageDir, name)
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("legacy artifact %s still exists: %v", name, err)
				}
			}

			blockedPath := filepath.Join(root, test.packageDir, test.files[0])
			if err := os.MkdirAll(filepath.Join(blockedPath, "child"), 0o755); err != nil {
				t.Fatalf("create blocked legacy artifact: %v", err)
			}
			_, err = runGenerationTasks(input, test.selection)
			if err == nil || !strings.Contains(err.Error(), test.files[0]) {
				t.Fatalf("cleanup error = %v, want path for blocked legacy artifact", err)
			}
		})
	}
}
