package forj

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/testexec"
	"github.com/goforj/goforj/internal/testkit"
)

// TestRunFrameworkTestListCommandSeparatesDiagnostics protects structured discovery from Go download messages.
func TestRunFrameworkTestListCommandSeparatesDiagnostics(t *testing.T) {
	t.Setenv("GOFORJ_FRAMEWORK_TEST_LIST_HELPER", "success")
	command := exec.Command(os.Args[0], "-test.run=^TestFrameworkTestListCommandHelper$")
	output, err := runFrameworkTestListCommand(command)
	if err != nil {
		t.Fatalf("run framework test list command: %v", err)
	}
	const want = `{"Action":"output","Package":"example.com/app","Output":"TestIntegration\n"}`
	if got := strings.TrimSpace(string(output)); got != want {
		t.Fatalf("framework test list = %q, want %q", got, want)
	}
}

// TestRunFrameworkTestListCommandPreservesFailureDetails verifies separated streams remain actionable on failure.
func TestRunFrameworkTestListCommandPreservesFailureDetails(t *testing.T) {
	t.Setenv("GOFORJ_FRAMEWORK_TEST_LIST_HELPER", "failure")
	command := exec.Command(os.Args[0], "-test.run=^TestFrameworkTestListCommandHelper$")
	_, err := runFrameworkTestListCommand(command)
	if err == nil {
		t.Fatal("expected framework test list failure")
	}
	for _, detail := range []string{"example.com/app", "go: helper failed"} {
		if !strings.Contains(err.Error(), detail) {
			t.Fatalf("framework test list failure = %q, want %q", err, detail)
		}
	}
}

// TestFrameworkTestListCommandHelper emits JSON and diagnostics on separate streams for subprocess tests.
func TestFrameworkTestListCommandHelper(t *testing.T) {
	mode := os.Getenv("GOFORJ_FRAMEWORK_TEST_LIST_HELPER")
	if mode == "" {
		return
	}
	_, _ = os.Stdout.WriteString("{\"Action\":\"output\",\"Package\":\"example.com/app\",\"Output\":\"TestIntegration\\n\"}\n")
	_, _ = os.Stderr.WriteString("go: downloading example.com/dependency v1.0.0\n")
	if mode == "failure" {
		_, _ = os.Stderr.WriteString("go: helper failed\n")
		os.Exit(7)
	}
	os.Exit(0)
}

// TestParseFrameworkShard covers the human-facing one-based shard contract.
func TestParseFrameworkShard(t *testing.T) {
	tests := []struct {
		value string
		want  frameworkShard
		err   string
	}{
		{value: "", want: frameworkShard{number: 1, total: 1}},
		{value: "1/1", want: frameworkShard{number: 1, total: 1}},
		{value: "1/6", want: frameworkShard{number: 1, total: 6}},
		{value: "6/6", want: frameworkShard{number: 6, total: 6}},
		{value: "0/1", err: "shard number must be between"},
		{value: "1/0", err: "total must be at least 1"},
		{value: "2/1", err: "shard number must be between"},
		{value: "-1/2", err: "positive integers"},
		{value: "1/-2", err: "positive integers"},
		{value: "1", err: "expected N/M"},
		{value: "1/2/3", err: "expected N/M"},
		{value: "a/2", err: "positive integers"},
		{value: "1/b", err: "positive integers"},
		{value: "/2", err: "positive integers"},
		{value: "1/", err: "positive integers"},
		{value: "1 /2", err: "positive integers"},
		{value: "+1/2", err: "positive integers"},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			got, err := parseFrameworkShard(test.value)
			if test.err != "" {
				if err == nil || !strings.Contains(err.Error(), test.err) {
					t.Fatalf("parseFrameworkShard(%q) error = %v, want %q", test.value, err, test.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFrameworkShard(%q): %v", test.value, err)
			}
			if got != test.want {
				t.Fatalf("parseFrameworkShard(%q) = %#v, want %#v", test.value, got, test.want)
			}
		})
	}
	if _, _, err := selectIntegrationTestTargets([]integrationTestName{{packagePath: "example.com/app", name: "TestOne"}}, frameworkShard{number: 1, total: 2}); err == nil {
		t.Fatal("framework shard count larger than the integration test set should fail")
	}
}

// TestValidateShardSuiteRejectsIgnoredPartitions keeps non-framework commands from silently duplicating work.
func TestValidateShardSuiteRejectsIgnoredPartitions(t *testing.T) {
	if err := validateShardSuite("framework", "1/2"); err != nil {
		t.Fatalf("framework shard: %v", err)
	}
	for _, suite := range []string{"all", "rendered"} {
		for _, shardValue := range []string{"1/1", "1/2"} {
			if err := validateShardSuite(suite, shardValue); err == nil {
				t.Fatalf("%s accepted framework shard %s", suite, shardValue)
			}
		}
		if err := validateShardSuite(suite, ""); err != nil {
			t.Fatalf("%s default shard: %v", suite, err)
		}
	}
}

// TestIntegrationForjBinaryValidatesExplicitPath covers CI binary reuse without weakening path validation.
func TestIntegrationForjBinaryValidatesExplicitPath(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		t.Setenv("FORJ_INTEGRATION_FORJ_PATH", filepath.Join(t.TempDir(), "missing"))
		if _, _, err := integrationForjBinary(testexec.GoCaches{}); err == nil || !strings.Contains(err.Error(), "validate FORJ_INTEGRATION_FORJ_PATH") {
			t.Fatalf("missing integration binary error = %v", err)
		}
	})
	t.Run("directory", func(t *testing.T) {
		t.Setenv("FORJ_INTEGRATION_FORJ_PATH", t.TempDir())
		if _, _, err := integrationForjBinary(testexec.GoCaches{}); err == nil || !strings.Contains(err.Error(), "is a directory") {
			t.Fatalf("directory integration binary error = %v", err)
		}
	})
	t.Run("file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "forj")
		if err := os.WriteFile(path, []byte("fixture"), 0o700); err != nil {
			t.Fatalf("write integration binary fixture: %v", err)
		}
		t.Setenv("FORJ_INTEGRATION_FORJ_PATH", path)
		got, cleanup, err := integrationForjBinary(testexec.GoCaches{})
		if err != nil {
			t.Fatalf("reuse integration binary: %v", err)
		}
		if got != path {
			t.Fatalf("integration binary path = %q, want %q", got, path)
		}
		cleanup()
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("explicit integration binary should remain after cleanup: %v", err)
		}
	})
}

// TestGoDrivenIntegrationDiscoveryAndSharding proves Go owns the inventory and every shard owns an exact partition.
func TestGoDrivenIntegrationDiscoveryAndSharding(t *testing.T) {
	t.Setenv("GOPROXY", "off")
	rootDir := t.TempDir()
	writeIntegrationDiscoveryFixture(t, filepath.Join(rootDir, "go.mod"), "module example.com/discovery\n\ngo 1.25.0\n")
	writeIntegrationDiscoveryFixture(t, filepath.Join(rootDir, "root.go"), `package discovery

func Empty() {}
func NoOutput() {}
`)
	writeIntegrationDiscoveryFixture(t, filepath.Join(rootDir, "ordinary_test.go"), `package discovery

import "testing"

func TestOrdinary(t *testing.T) {}
`)
	writeIntegrationDiscoveryFixture(t, filepath.Join(rootDir, "integration_test.go"), `//go:build integration

package discovery

import (
	"fmt"
	"os"
	. "testing"
)

func TestMain(m *M) { os.Exit(m.Run()) }
func TestRootIntegration(t *T) {}
func Testlowercase(t *T) {}
func FuzzRootIntegration(f *F) {}
func BenchmarkIgnored(b *B) {}
func Example() {
	fmt.Println("integration")
	// Output: integration
}
func ExampleEmpty() {
	// Output:
}
func ExampleNoOutput() {}
`)
	writeIntegrationDiscoveryFixture(t, filepath.Join(rootDir, "external_integration_test.go"), `//go:build integration

package discovery_test

import . "testing"

func TestExternalIntegration(t *T) {}
func TestDuplicate(t *T) {}
`)
	writeIntegrationDiscoveryFixture(t, filepath.Join(rootDir, "nested", "ordinary_test.go"), `package nested

import "testing"

func TestNestedOrdinary(t *testing.T) {}
`)
	writeIntegrationDiscoveryFixture(t, filepath.Join(rootDir, "nested", "integration_test.go"), `//go:build integration

package nested

import "testing"

func TestNestedIntegration(t *testing.T) {}
func TestDuplicate(t *testing.T) {}
`)
	writeIntegrationDiscoveryFixture(t, filepath.Join(rootDir, "lighthouse_integration_test.go"), `//go:build integration && lighthouse

package discovery

import "testing"

func TestLighthouseOnly(t *testing.T) {}
`)
	writeIntegrationDiscoveryFixture(t, filepath.Join(rootDir, "multiapp_integration_test.go"), `//go:build integration && multiapp

package discovery

import "testing"

func TestMultiAppOnly(t *testing.T) {}
`)

	modCache, buildCache := testkit.GoCachePaths()
	executor := integrationExecutor{caches: testexec.GoCaches{ModulePath: modCache, BuildPath: buildCache}}
	baseline := listGoTestsForTest(t, executor, rootDir, "")
	integration := listGoTestsForTest(t, executor, rootDir, "integration")
	lighthouse := listGoTestsForTest(t, executor, rootDir, "integration,lighthouse")
	multiApp := listGoTestsForTest(t, executor, rootDir, "integration,multiapp")

	integrationOnly := integrationTestsForTest(t, baseline, integration)
	assertIntegrationTestsEqual(t, integrationOnly, []integrationTestName{
		{packagePath: "example.com/discovery", name: "Example"},
		{packagePath: "example.com/discovery", name: "ExampleEmpty"},
		{packagePath: "example.com/discovery", name: "FuzzRootIntegration"},
		{packagePath: "example.com/discovery", name: "TestDuplicate"},
		{packagePath: "example.com/discovery", name: "TestExternalIntegration"},
		{packagePath: "example.com/discovery", name: "TestRootIntegration"},
		{packagePath: "example.com/discovery/nested", name: "TestDuplicate"},
		{packagePath: "example.com/discovery/nested", name: "TestNestedIntegration"},
	})
	assertIntegrationTestsEqual(t, integrationTestsForTest(t, integration, lighthouse), []integrationTestName{
		{packagePath: "example.com/discovery", name: "TestLighthouseOnly"},
	})
	assertIntegrationTestsEqual(t, integrationTestsForTest(t, integration, multiApp), []integrationTestName{
		{packagePath: "example.com/discovery", name: "TestMultiAppOnly"},
	})
	assertIntegrationShardsCoverTestsOnce(t, integrationOnly, 3)
}

// writeIntegrationDiscoveryFixture writes one Go discovery fixture inside a test-owned temporary module.
func writeIntegrationDiscoveryFixture(t *testing.T, path, source string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create integration discovery fixture directory %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write integration discovery fixture %s: %v", path, err)
	}
}

// listGoTestsForTest loads one tagged inventory from a test-owned temporary module.
func listGoTestsForTest(t *testing.T, executor integrationExecutor, rootDir, tags string) []integrationTestName {
	t.Helper()
	tests, err := executor.listGoTests(rootDir, "./...", tags)
	if err != nil {
		t.Fatalf("list Go tests for tags %q: %v", tags, err)
	}
	return tests
}

// integrationTestsForTest returns the runnable tests added beyond one baseline inventory.
func integrationTestsForTest(t *testing.T, baseline, integration []integrationTestName) []integrationTestName {
	t.Helper()
	tests, err := integrationOnlyTestNames(baseline, integration)
	if err != nil {
		t.Fatalf("integration-only tests: %v", err)
	}
	return tests
}

// assertIntegrationTestsEqual compares the complete deterministic package-qualified inventory.
func assertIntegrationTestsEqual(t *testing.T, got, want []integrationTestName) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("integration tests = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("integration tests = %#v, want %#v", got, want)
		}
	}
}

// TestFrameworkProfileTagsKeepsSpecializedTestsOutOfTheDefaultProfile verifies tagged jobs own only their added test files.
func TestFrameworkProfileTagsKeepsSpecializedTestsOutOfTheDefaultProfile(t *testing.T) {
	tests := []struct {
		profile      string
		wantBaseline string
		wantTags     string
	}{
		{profile: "integration", wantBaseline: "", wantTags: "integration"},
		{profile: "lighthouse", wantBaseline: "integration", wantTags: "integration,lighthouse"},
		{profile: "multiapp", wantBaseline: "integration", wantTags: "integration,multiapp"},
	}
	for _, test := range tests {
		t.Run(test.profile, func(t *testing.T) {
			baseline, tags, err := frameworkProfileTags(test.profile)
			if err != nil {
				t.Fatalf("frameworkProfileTags(%q): %v", test.profile, err)
			}
			if baseline != test.wantBaseline || tags != test.wantTags {
				t.Fatalf("frameworkProfileTags(%q) = (%q, %q), want (%q, %q)", test.profile, baseline, tags, test.wantBaseline, test.wantTags)
			}
		})
	}
	if _, _, err := frameworkProfileTags("unknown"); err == nil {
		t.Fatal("unknown framework profile should fail")
	}
}

// TestFrameworkIntegrationDiscoveryPartitionsCurrentProfiles protects nested and specialized suites without tracking test names.
func TestFrameworkIntegrationDiscoveryPartitionsCurrentProfiles(t *testing.T) {
	modCache, buildCache := testkit.GoCachePaths()
	executor := integrationExecutor{caches: testexec.GoCaches{ModulePath: modCache, BuildPath: buildCache}}

	baseline := listFrameworkTestsForTest(t, executor, "")
	integrationInventory := listFrameworkTestsForTest(t, executor, "integration")
	lighthouseInventory := listFrameworkTestsForTest(t, executor, "integration,lighthouse")
	multiAppInventory := listFrameworkTestsForTest(t, executor, "integration,multiapp")
	integrationTests := integrationTestsForTest(t, baseline, integrationInventory)
	lighthouseTests := integrationTestsForTest(t, integrationInventory, lighthouseInventory)
	multiAppTests := integrationTestsForTest(t, integrationInventory, multiAppInventory)

	assertIntegrationPackagePresent(t, integrationTests, "github.com/goforj/goforj/internal/forj")
	assertIntegrationPackagePresent(t, integrationTests, "github.com/goforj/goforj/internal/forj/atlas")
	assertIntegrationShardsCoverTestsOnce(t, integrationTests, 6)
	assertIntegrationProfilesDisjoint(t, integrationTests, lighthouseTests, multiAppTests)
}

// listFrameworkTestsForTest loads one live repository inventory through the production package pattern.
func listFrameworkTestsForTest(t *testing.T, executor integrationExecutor, tags string) []integrationTestName {
	t.Helper()
	tests, err := executor.listFrameworkTests(tags)
	if err != nil {
		t.Fatalf("list framework tests for tags %q: %v", tags, err)
	}
	return tests
}

// assertIntegrationPackagePresent verifies recursive discovery retains a package without naming one of its tests.
func assertIntegrationPackagePresent(t *testing.T, tests []integrationTestName, packagePath string) {
	t.Helper()
	for _, testName := range tests {
		if testName.packagePath == packagePath {
			return
		}
	}
	t.Fatalf("integration inventory does not include package %s: %#v", packagePath, tests)
}

// assertIntegrationProfilesDisjoint verifies specialized tag layers never rerun another profile's top-level test.
func assertIntegrationProfilesDisjoint(t *testing.T, profiles ...[]integrationTestName) {
	t.Helper()
	seen := make(map[integrationTestName]int)
	for profileIndex, profile := range profiles {
		for _, testName := range profile {
			if previous, exists := seen[testName]; exists {
				t.Fatalf("integration test %#v appears in profiles %d and %d", testName, previous, profileIndex)
			}
			seen[testName] = profileIndex
		}
	}
}

// assertIntegrationShardsCoverTestsOnce verifies the CI partition neither loses nor duplicates package-qualified tests.
func assertIntegrationShardsCoverTestsOnce(t *testing.T, tests []integrationTestName, shardCount int) {
	t.Helper()
	seen := make(map[integrationTestName]int, len(tests))
	for shardNumber := 1; shardNumber <= shardCount; shardNumber++ {
		shard := frameworkShard{number: shardNumber, total: shardCount}
		targets, selectedCount, err := selectIntegrationTestTargets(tests, shard)
		if err != nil {
			t.Fatalf("select integration shard %d/%d: %v", shardNumber, shardCount, err)
		}
		counted := 0
		for _, target := range targets {
			for _, testName := range target.testNames {
				seen[integrationTestName{packagePath: target.packagePath, name: testName}]++
				counted++
			}
		}
		if counted != selectedCount {
			t.Fatalf("integration shard %d selected count = %d, grouped %d", shardNumber, selectedCount, counted)
		}
	}
	if len(seen) != len(tests) {
		t.Fatalf("integration shards covered %d tests, want %d", len(seen), len(tests))
	}
	for _, testName := range tests {
		if seen[testName] != 1 {
			t.Fatalf("integration test %#v ran in %d shards, want exactly one", testName, seen[testName])
		}
	}
}

// TestApplyRenderedIntegrationEnvironmentPublishesTimezone verifies database variants start outside UTC.
func TestApplyRenderedIntegrationEnvironmentPublishesTimezone(t *testing.T) {
	for _, variant := range []string{"mysql", "postgres"} {
		t.Run(variant, func(t *testing.T) {
			projectDir := t.TempDir()
			envPath := filepath.Join(projectDir, ".env")
			if err := os.WriteFile(envPath, []byte("TZ=UTC\n"), 0o644); err != nil {
				t.Fatalf("write env: %v", err)
			}
			if err := applyRenderedIntegrationEnvironment(projectDir, dbIntegrationVariantSpecs[variant].testEnv); err != nil {
				t.Fatalf("apply integration environment: %v", err)
			}
			body, err := os.ReadFile(envPath)
			if err != nil {
				t.Fatalf("read env: %v", err)
			}
			if !strings.Contains(string(body), "TZ=America/Los_Angeles") {
				t.Fatalf("expected non-UTC timezone, got:\n%s", body)
			}
		})
	}
}

// TestApplyRenderedIntegrationEnvironmentSkipsMissingTimezone verifies unrelated variants need no env file mutation.
func TestApplyRenderedIntegrationEnvironmentSkipsMissingTimezone(t *testing.T) {
	if err := applyRenderedIntegrationEnvironment(t.TempDir(), nil); err != nil {
		t.Fatalf("empty integration environment: %v", err)
	}
}

// TestApplyRenderedIntegrationEnvironmentReturnsEnvWriteFailure verifies setup errors are not hidden.
func TestApplyRenderedIntegrationEnvironmentReturnsEnvWriteFailure(t *testing.T) {
	err := applyRenderedIntegrationEnvironment(t.TempDir(), map[string]string{"TZ": "America/Los_Angeles"})
	if err == nil {
		t.Fatal("expected missing env file error")
	}
}
