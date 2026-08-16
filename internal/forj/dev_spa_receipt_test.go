package forj

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goforj/goforj/internal/appassets"
	"github.com/goforj/goforj/internal/devwatch"
)

// TestRecordDevSPABuildSharesSuccessfulOutputWithForjBuild verifies dev and build use the same freshness receipt.
func TestRecordDevSPABuildSharesSuccessfulOutputWithForjBuild(t *testing.T) {
	root := t.TempDir()
	frontend := filepath.Join("cmd", "app", "frontend")
	writeDevSPAReceiptTestFile(t, root, filepath.Join(frontend, "src", "app.ts"), "source")
	writeDevSPAReceiptTestFile(t, root, filepath.Join(frontend, "dist", "app.js"), "built")
	watcher := devCompiledWatcher{
		Kind:    devWatcherSPABuild,
		App:     "app",
		Asset:   "frontend",
		Command: devwatch.Command{Dir: frontend, Shell: "npm run build"},
	}
	recordDevSPABuild(root, watcher)

	current, err := appassets.Current(root, appassets.Asset{
		App:     watcher.App,
		Name:    watcher.Asset,
		Root:    watcher.Command.Dir,
		Prepare: generatedFrontendNPMInstallCommand,
		Command: watcher.Command.Shell,
	})
	if err != nil {
		t.Fatalf("inspect dev SPA receipt: %v", err)
	}
	if !current {
		t.Fatal("successful dev SPA output was not current for a later build")
	}
}

// writeDevSPAReceiptTestFile creates one frontend fixture beneath an explicit Project root.
func writeDevSPAReceiptTestFile(t *testing.T, root string, relative string, contents string) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create %s parent: %v", relative, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", relative, err)
	}
}
