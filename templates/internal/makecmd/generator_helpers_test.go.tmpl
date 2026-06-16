package makecmd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestGeneratedPackageHelpersUseCompactLowercaseSegments(t *testing.T) {
	if got := generatedPackageSegment("BillingPortal"); got != "billingportal" {
		t.Fatalf("package segment = %q, want billingportal", got)
	}
	if got := generatedPackageName(filepath.Join("internal", "billing_portal"), "fallback"); got != "billingportal" {
		t.Fatalf("package name = %q, want billingportal", got)
	}
	if got := generatedPackagePathParts([]string{"BillingPortal", "UsageReports"}); !reflect.DeepEqual(got, []string{"billingportal", "usagereports"}) {
		t.Fatalf("package path parts = %#v, want [billingportal usagereports]", got)
	}
	if got := generatedPackagePathPartsFromPath("Internal/BillingPortal"); !reflect.DeepEqual(got, []string{"internal", "billingportal"}) {
		t.Fatalf("package path parts from path = %#v, want [internal billingportal]", got)
	}
	if got := generatedPackageRef(filepath.Join("internal", "billing_portal"), "fallback"); got != "billingportal" {
		t.Fatalf("package ref = %q, want billingportal", got)
	}
}

func TestActiveAppPaths(t *testing.T) {
	t.Setenv("FORJ_APP", "")
	t.Setenv("FORJ_APP", "")

	if got := activeAppFile("commands.go"); got != filepath.Join("app", "commands.go") {
		t.Fatalf("default app file = %q, want app/commands.go", got)
	}
	if got := activeAppWireFile("inject_cmd_app.go"); got != filepath.Join("app", "wire", "inject_cmd_app.go") {
		t.Fatalf("default app wire file = %q, want app/wire/inject_cmd_app.go", got)
	}

	t.Setenv("FORJ_APP", "reporting")
	if got := activeAppFile("commands.go"); got != filepath.Join("app", "reporting", "commands.go") {
		t.Fatalf("named app file = %q, want app/reporting/commands.go", got)
	}
	if got := activeAppWireFile("inject_cmd_app.go"); got != filepath.Join("app", "reporting", "wire", "inject_cmd_app.go") {
		t.Fatalf("named app wire file = %q, want app/reporting/wire/inject_cmd_app.go", got)
	}

	t.Setenv("FORJ_APP", "../reporting")
	if got := activeAppFile("commands.go"); got != filepath.Join("app", "commands.go") {
		t.Fatalf("unsafe app file = %q, want app/commands.go", got)
	}
}

func TestInsertIntoCallBlockExpandsSingleLineCalls(t *testing.T) {
	lines := []string{
		"package wire",
		"",
		"var appCommandSet = wire.NewSet()",
	}
	got := strings.Join(insertIntoCallBlock(lines, "var appCommandSet = wire.NewSet(", "\treports.NewSyncCmd,"), "\n")
	for _, want := range []string{
		"var appCommandSet = wire.NewSet(",
		"\treports.NewSyncCmd,",
		")",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected expanded call to contain %q, got:\n%s", want, got)
		}
	}
}

func writeMakeCmdTestFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readMakeCmdTestFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}

func assertMakeCmdTestContains(t *testing.T, path string, needles []string) {
	t.Helper()
	body := readMakeCmdTestFile(t, path)
	for _, needle := range needles {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %s to contain %q, got:\n%s", path, needle, body)
		}
	}
}

func assertMakeCmdTestNotContains(t *testing.T, path string, needles []string) {
	t.Helper()
	body := readMakeCmdTestFile(t, path)
	for _, needle := range needles {
		if strings.Contains(body, needle) {
			t.Fatalf("expected %s not to contain %q, got:\n%s", path, needle, body)
		}
	}
}

func assertMakeCmdTestFileMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected %s to be removed", path)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat %s: %v", path, err)
	}
}

func assertMakeCmdTestGlob(t *testing.T, pattern string) {
	t.Helper()
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob %s: %v", pattern, err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected at least one file matching %s", pattern)
	}
}
