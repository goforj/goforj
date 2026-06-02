package forj

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestRenderwarmExcludesLegacyScheduler verifies string literals in test templates do not warm legacy modules.
func TestRenderwarmExcludesLegacyScheduler(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve current file path")
	}
	repoRoot := filepath.Join(filepath.Dir(currentFile), "..", "..")

	mainSource := readRenderwarmTestFile(t, filepath.Join(repoRoot, "tools", "renderwarm", "main.go"))
	goMod := readRenderwarmTestFile(t, filepath.Join(repoRoot, "tools", "renderwarm", "go.mod"))

	for _, source := range []string{mainSource, goMod} {
		if strings.Contains(source, "github.com/goforj/scheduler\"") || strings.Contains(source, "github.com/goforj/scheduler v") {
			t.Fatalf("renderwarm should not include legacy scheduler module:\n%s", source)
		}
	}
	for _, want := range []string{
		`github.com/goforj/scheduler/v2`,
		`github.com/klauspost/compress/zstd`,
	} {
		if !strings.Contains(mainSource, want) {
			t.Fatalf("expected renderwarm main to contain %s", want)
		}
	}
}

// readRenderwarmTestFile reads a renderwarm file for regression assertions.
func readRenderwarmTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
