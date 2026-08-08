package forj

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestRenderwarmUsesCurrentTemplateDependencies verifies warm builds exclude legacy modules and pin current sibling releases.
func TestRenderwarmUsesCurrentTemplateDependencies(t *testing.T) {
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
		`github.com/goforj/cache/driver/rediscache`,
		`github.com/goforj/console`,
		`github.com/goforj/scheduler/v2`,
		`github.com/klauspost/compress/zstd`,
	} {
		if !strings.Contains(mainSource, want) {
			t.Fatalf("expected renderwarm main to contain %s", want)
		}
	}
	if !strings.Contains(goMod, "github.com/goforj/godump v1.9.1") {
		t.Fatalf("renderwarm should pin the host-validated godump release:\n%s", goMod)
	}
	if !strings.Contains(goMod, "github.com/goforj/cache/driver/rediscache v0.4.0") {
		t.Fatalf("renderwarm should pin the current Redis cache driver release:\n%s", goMod)
	}
	for _, want := range []string{
		"github.com/goforj/console v0.2.0",
		"github.com/goforj/str v1.3.0",
		"github.com/goforj/str/v2 v2.0.1",
	} {
		if !strings.Contains(goMod, want) {
			t.Fatalf("renderwarm should pin %q:\n%s", want, goMod)
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
