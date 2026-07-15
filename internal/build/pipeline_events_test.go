package build

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goforj/goforj/internal/logger"
)

// TestPipelineGenerationIgnoresStaleEventsDirectoryWhenComponentDisabled verifies builds select Events from config rather than filesystem residue.
func TestPipelineGenerationIgnoresStaleEventsDirectoryWhenComponentDisabled(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{
		filepath.Join(root, "internal", "storages"),
		filepath.Join(root, "internal", "events"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("create fixture directory %s: %v", path, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte("project_name: Test\nmodule_name: example.test/app\nrender:\n  component_contract: 1\n  components:\n    cli: true\n    storage: true\n    events: false\n"), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("STORAGE_DRIVER=local\nSTORAGE_SUPPORTED_DRIVERS=local\nEVENTS_DRIVER=unknown\nEVENTS_SUPPORTED_DRIVERS=unknown\n"), 0o644); err != nil {
		t.Fatalf("write environment: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })

	pipeline := NewPipeline(logger.NewSilentLogger(), nil)
	if _, err := pipeline.generateProjectFiles(); err != nil {
		t.Fatalf("generate project files: %v", err)
	}
	if _, err := os.Stat(filepath.Join("internal", "events", "manager_gen.go")); !os.IsNotExist(err) {
		t.Fatalf("Events-disabled build generated stale Events package: %v", err)
	}
}

// TestPipelineGenerationIgnoresStaleStorageDirectoryWhenComponentDisabled verifies builds select Storage from config rather than filesystem or environment residue.
func TestPipelineGenerationIgnoresStaleStorageDirectoryWhenComponentDisabled(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "storages"), 0o755); err != nil {
		t.Fatalf("create stale Storage directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte("project_name: Test\nmodule_name: example.test/app\nrender:\n  component_contract: 1\n  components:\n    cli: true\n    storage: false\n    events: false\n"), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("STORAGE_DRIVER=unknown\nSTORAGE_SUPPORTED_DRIVERS=unknown\n"), 0o644); err != nil {
		t.Fatalf("write stale Storage environment: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })

	pipeline := NewPipeline(logger.NewSilentLogger(), nil)
	if _, err := pipeline.generateProjectFiles(); err != nil {
		t.Fatalf("generate project files: %v", err)
	}
	if _, err := os.Stat(filepath.Join("internal", "storages", "manager_gen.go")); !os.IsNotExist(err) {
		t.Fatalf("Storage-disabled build generated stale Storage package: %v", err)
	}
}
