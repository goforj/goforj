package build

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadCompileProfileAggregatesAndSorts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compile.log")
	if err := appendCompileProfile(path, "example.com/a", 120*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := appendCompileProfile(path, "example.com/b", 40*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := appendCompileProfile(path, "example.com/a", 30*time.Millisecond); err != nil {
		t.Fatal(err)
	}

	report, err := loadCompileProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(report.Entries))
	}
	if report.Entries[0].Package != "example.com/a" || report.Entries[0].DurationMS != 150 || report.Entries[0].Invocations != 2 {
		t.Fatalf("unexpected first entry: %#v", report.Entries[0])
	}
}

func TestPrintCompileProfile(t *testing.T) {
	var out bytes.Buffer
	report := CompileProfileReport{
		BaselineTotalMS: 200,
		ProfiledTotalMS: 240,
		Entries: []CompileProfileEntry{
			{Package: "example.com/a", DurationMS: 150, Invocations: 2, ImportChain: []string{"./wire", "example.com/a"}},
			{Package: "example.com/b", DurationMS: 40, Invocations: 1},
		},
	}
	if err := printCompileProfile(&out, report, 10); err != nil {
		t.Fatal(err)
	}
	output := out.String()
	for _, expected := range []string{
		"Baseline build total: 200ms",
		"Profiled build total: 240ms",
		"Compile time (packages compiled in this build):",
		"example.com/a",
		"150ms",
		"(2x)",
		"└─ ./wire",
		"└─ example.com/a",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected output to contain %q, got %q", expected, output)
		}
	}
}

func TestNormalizeCompileProfile(t *testing.T) {
	report := normalizeCompileProfile(CompileProfileReport{
		Entries: []CompileProfileEntry{
			{Package: "example.com/a", DurationMS: 200},
			{Package: "example.com/b", DurationMS: 50},
		},
	}, 500, 1000)
	if report.BaselineTotalMS != 500 || report.ProfiledTotalMS != 1000 {
		t.Fatalf("unexpected totals: %#v", report)
	}
	if report.Entries[0].DurationMS != 100 || report.Entries[1].DurationMS != 25 {
		t.Fatalf("unexpected normalized entries: %#v", report.Entries)
	}
}

func TestCompilePackageName(t *testing.T) {
	got := compilePackageName([]string{"-o", "/tmp/out.a", "-p", "example.com/app/internal/http"})
	if got != "example.com/app/internal/http" {
		t.Fatalf("package = %q", got)
	}
}

func TestHandleProfileToolIgnoresNormalArgs(t *testing.T) {
	if HandleProfileTool([]string{"build"}) {
		t.Fatal("expected normal args not to trigger profile tool")
	}
}

func TestLoadCompileProfileMissingFile(t *testing.T) {
	if _, err := loadCompileProfile(filepath.Join(t.TempDir(), "missing.log")); err == nil {
		t.Fatal("expected missing file error")
	}
}

func TestAppendCompileProfileNoPathIsNoop(t *testing.T) {
	if err := appendCompileProfile("", "example.com/a", time.Millisecond); err != nil {
		t.Fatal(err)
	}
}

func TestAppendCompileProfileWritesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compile.log")
	if err := appendCompileProfile(path, "example.com/a", 5*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "example.com/a\t5") {
		t.Fatalf("unexpected file contents: %q", string(data))
	}
}

func TestImportChainToTarget(t *testing.T) {
	loaded := importLoadResult{
		roots: []string{"example.com/app/wire"},
		packages: map[string]goListPackage{
			"example.com/app/wire":             {ImportPath: "example.com/app/wire", Imports: []string{"example.com/app/internal/dbconns"}},
			"example.com/app/internal/dbconns": {ImportPath: "example.com/app/internal/dbconns", Imports: []string{"github.com/glebarez/sqlite"}},
			"github.com/glebarez/sqlite":       {ImportPath: "github.com/glebarez/sqlite", Imports: []string{"modernc.org/sqlite/lib"}},
			"modernc.org/sqlite/lib":           {ImportPath: "modernc.org/sqlite/lib"},
		},
	}
	chain := importChainToTarget(loaded, "modernc.org/sqlite/lib")
	want := []string{"example.com/app/wire", "example.com/app/internal/dbconns", "github.com/glebarez/sqlite", "modernc.org/sqlite/lib"}
	if strings.Join(chain, "|") != strings.Join(want, "|") {
		t.Fatalf("chain = %#v, want %#v", chain, want)
	}
}
