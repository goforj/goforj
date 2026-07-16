package compileprofile

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLoadAggregatesAndSorts protects the package ranking contract consumed by the CLI report.
func TestLoadAggregatesAndSorts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compile.log")
	if err := Record(path, "example.com/a", 120*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := Record(path, "example.com/b", 40*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := Record(path, "example.com/a", 30*time.Millisecond); err != nil {
		t.Fatal(err)
	}

	report, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(report.entries))
	}
	if report.entries[0].packageName != "example.com/a" || report.entries[0].durationMS != 150 || report.entries[0].invocations != 2 {
		t.Fatalf("unexpected first entry: %#v", report.entries[0])
	}
}

// TestReportPrint preserves the human-readable output expected by profiling users.
func TestReportPrint(t *testing.T) {
	var out bytes.Buffer
	report := Report{
		baselineTotalMS: 200,
		profiledTotalMS: 240,
		entries: []entry{
			{packageName: "example.com/a", durationMS: 150, invocations: 2, importChain: []string{"./wire", "example.com/a"}},
			{packageName: "example.com/b", durationMS: 40, invocations: 1},
		},
	}
	if err := report.Print(&out, 10); err != nil {
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

// TestReportPrintReturnsImportChainWriterFailure keeps the public writer contract intact through nested chain formatting.
func TestReportPrintReturnsImportChainWriterFailure(t *testing.T) {
	want := errors.New("injected import chain write failure")
	report := Report{entries: []entry{{
		packageName: "example.com/a",
		durationMS:  10,
		invocations: 1,
		importChain: []string{"example.com/root", "example.com/a"},
	}}}
	err := report.Print(importChainFailWriter{err: want}, 1)
	if !errors.Is(err, want) {
		t.Fatalf("Print() error = %v, want injected writer failure", err)
	}
}

// TestAnnotateImportChainsReturnsDiscoveryFailure prevents incomplete dependency explanations from looking successful.
func TestAnnotateImportChainsReturnsDiscoveryFailure(t *testing.T) {
	report := Report{}
	err := report.AnnotateImportChains(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("AnnotateImportChains() accepted an unreadable project root")
	}
}

// TestReportNormalize verifies that instrumentation overhead is scaled back to the uncached baseline.
func TestReportNormalize(t *testing.T) {
	report := Report{
		entries: []entry{
			{packageName: "example.com/a", durationMS: 200},
			{packageName: "example.com/b", durationMS: 50},
		},
	}
	report.NormalizeTimings(500, 1000)
	if report.baselineTotalMS != 500 || report.profiledTotalMS != 1000 {
		t.Fatalf("unexpected totals: %#v", report)
	}
	if report.entries[0].durationMS != 100 || report.entries[1].durationMS != 25 {
		t.Fatalf("unexpected normalized entries: %#v", report.entries)
	}
}

// TestLoadMissingFile keeps missing profile logs visible to build orchestration.
func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.log")); err == nil {
		t.Fatal("expected missing file error")
	}
}

// TestRecordNoPathIsNoop preserves normal compiler execution when profile collection is disabled.
func TestRecordNoPathIsNoop(t *testing.T) {
	if err := Record("", "example.com/a", time.Millisecond); err != nil {
		t.Fatal(err)
	}
}

// TestRecordWritesFile protects the append-only protocol shared by the compiler tool and report loader.
func TestRecordWritesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compile.log")
	if err := Record(path, "example.com/a", 5*time.Millisecond); err != nil {
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

// TestImportChainToTarget verifies that dependency explanations follow the shortest project-rooted path.
func TestImportChainToTarget(t *testing.T) {
	loaded := importLoadResult{
		roots: []string{"example.com/app/wire"},
		packages: map[string]goListPackage{
			"example.com/app/wire":              {ImportPath: "example.com/app/wire", Imports: []string{"example.com/app/internal/database"}},
			"example.com/app/internal/database": {ImportPath: "example.com/app/internal/database", Imports: []string{"github.com/glebarez/sqlite"}},
			"github.com/glebarez/sqlite":        {ImportPath: "github.com/glebarez/sqlite", Imports: []string{"modernc.org/sqlite/lib"}},
			"modernc.org/sqlite/lib":            {ImportPath: "modernc.org/sqlite/lib"},
		},
	}
	chain := importChainToTarget(loaded, "modernc.org/sqlite/lib")
	want := []string{"example.com/app/wire", "example.com/app/internal/database", "github.com/glebarez/sqlite", "modernc.org/sqlite/lib"}
	if strings.Join(chain, "|") != strings.Join(want, "|") {
		t.Fatalf("chain = %#v, want %#v", chain, want)
	}
}

// importChainFailWriter fails only nested import-chain lines so Print must propagate the deepest writer boundary.
type importChainFailWriter struct {
	err error
}

// Write accepts report headings and rejects the first import-chain line.
func (w importChainFailWriter) Write(data []byte) (int, error) {
	if bytes.Contains(data, []byte("└─")) {
		return 0, w.err
	}
	return len(data), nil
}
