package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/testexec"
	"github.com/goforj/goforj/internal/testkit"
)

// TestValidateFrameworkShardRejectsInvalidBounds covers every shard validation failure before CI starts expensive work.
func TestValidateFrameworkShardRejectsInvalidBounds(t *testing.T) {
	tests := []struct {
		name  string
		count int
		index int
		want  string
	}{
		{name: "zero count", count: 0, index: 0, want: "count must be at least 1"},
		{name: "negative count", count: -1, index: 0, want: "count must be at least 1"},
		{name: "negative index", count: 2, index: -1, want: "index must be between 0 and 1"},
		{name: "index equals count", count: 2, index: 2, want: "index must be between 0 and 1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateFrameworkShard(test.count, test.index)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateFrameworkShard(%d, %d) = %v, want %q", test.count, test.index, err, test.want)
			}
		})
	}
	if err := validateFrameworkShard(4, 3); err != nil {
		t.Fatalf("valid framework shard: %v", err)
	}
	if _, _, err := selectIntegrationTestTargets([]integrationTestName{{packagePath: "example.com/app", name: "TestOne"}}, 2, 0); err == nil {
		t.Fatal("framework shard count larger than the integration test set should fail")
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

// TestIntegrationOnlyTestNamesFiltersFilesAndSignatures proves discovery excludes ordinary files and invalid Test-prefixed helpers.
func TestIntegrationOnlyTestNamesFiltersFilesAndSignatures(t *testing.T) {
	rootDir := t.TempDir()
	nestedDir := t.TempDir()
	writeIntegrationDiscoveryFixture(t, filepath.Join(rootDir, "ordinary_test.go"), `package root
import "testing"
func TestOrdinary(t *testing.T) {}
`)
	writeIntegrationDiscoveryFixture(t, filepath.Join(rootDir, "integration_test.go"), `package root
import "testing"
func TestIncluded(t *testing.T) {}
func Testlowercase(t *testing.T) {}
func TestMain(m *testing.M) {}
func TestHelperWithoutParameter() {}
func FuzzIncluded(f *testing.F) {}
func ExampleIncluded() {}
`)
	writeIntegrationDiscoveryFixture(t, filepath.Join(rootDir, "external_integration_test.go"), `package root_test
import "testing"
func TestExternal(t *testing.T) {}
`)
	writeIntegrationDiscoveryFixture(t, filepath.Join(nestedDir, "nested_integration_test.go"), `package nested
import "testing"
func TestNested(t *testing.T) {}
func TestIncluded(t *testing.T) {}
`)
	baseline := []integrationPackageMetadata{
		{ImportPath: "example.com/root", Dir: rootDir, TestGoFiles: []string{"ordinary_test.go"}},
		{ImportPath: "example.com/root/nested", Dir: nestedDir},
	}
	integration := []integrationPackageMetadata{
		{
			ImportPath:   "example.com/root",
			Dir:          rootDir,
			TestGoFiles:  []string{"ordinary_test.go", "integration_test.go"},
			XTestGoFiles: []string{"external_integration_test.go"},
		},
		{ImportPath: "example.com/root/nested", Dir: nestedDir, TestGoFiles: []string{"nested_integration_test.go"}},
	}

	tests, err := integrationOnlyTestNames(baseline, integration)
	if err != nil {
		t.Fatalf("integrationOnlyTestNames: %v", err)
	}
	want := []integrationTestName{
		{packagePath: "example.com/root", name: "ExampleIncluded"},
		{packagePath: "example.com/root", name: "FuzzIncluded"},
		{packagePath: "example.com/root", name: "TestExternal"},
		{packagePath: "example.com/root", name: "TestIncluded"},
		{packagePath: "example.com/root/nested", name: "TestIncluded"},
		{packagePath: "example.com/root/nested", name: "TestNested"},
	}
	if len(tests) != len(want) {
		t.Fatalf("discovered tests = %#v, want %#v", tests, want)
	}
	for index := range want {
		if tests[index] != want[index] {
			t.Fatalf("discovered tests = %#v, want %#v", tests, want)
		}
	}
	assertIntegrationShardsCoverTestsOnce(t, tests, 2)
}

// writeIntegrationDiscoveryFixture writes one parser fixture inside a test-owned temporary directory.
func writeIntegrationDiscoveryFixture(t *testing.T, path, source string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write integration discovery fixture %s: %v", path, err)
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

// TestFrameworkIntegrationDiscoveryIncludesEveryTaggedPackage protects nested and specialized suites from silent CI omissions.
func TestFrameworkIntegrationDiscoveryIncludesEveryTaggedPackage(t *testing.T) {
	modCache, buildCache := testkit.GoCachePaths()
	executor := integrationExecutor{caches: testexec.GoCaches{ModulePath: modCache, BuildPath: buildCache}}

	integrationTests := discoverIntegrationTestsForTest(t, executor, "", "integration")
	assertDiscoveredIntegrationTest(t, integrationTests, "github.com/goforj/goforj/internal/forj", "TestRunnableScenarioPathIntegration")
	assertDiscoveredIntegrationTest(t, integrationTests, "github.com/goforj/goforj/internal/forj/atlas", "TestAtlasMCPServerUsesRenderedProjectInventory")
	assertMissingIntegrationTest(t, integrationTests, "TestAboutCommandTemplateIsWired")
	assertIntegrationShardsCoverTestsOnce(t, integrationTests, 6)

	lighthouseTests := discoverIntegrationTestsForTest(t, executor, "integration", "integration,lighthouse")
	assertDiscoveredIntegrationTest(t, lighthouseTests, "github.com/goforj/goforj/internal/forj", "TestDevwatchStreamIntegration")
	assertDiscoveredIntegrationTest(t, lighthouseTests, "github.com/goforj/goforj/internal/forj", "TestLighthouseAuthBootIntegration")
	assertMissingIntegrationTest(t, lighthouseTests, "TestRunnableScenarioPathIntegration")

	multiAppTests := discoverIntegrationTestsForTest(t, executor, "integration", "integration,multiapp")
	if len(multiAppTests) != 2 {
		t.Fatalf("multi-App discovery returned %d tests, want 2: %#v", len(multiAppTests), multiAppTests)
	}
	assertDiscoveredIntegrationTest(t, multiAppTests, "github.com/goforj/goforj/internal/forj", "TestMakeAppMultiAppRuntimeSmoke")
	assertDiscoveredIntegrationTest(t, multiAppTests, "github.com/goforj/goforj/internal/forj", "TestMakeAppMultiAppSQLiteMigrationsSmoke")
}

// assertIntegrationShardsCoverTestsOnce verifies the CI partition neither loses nor duplicates package-qualified tests.
func assertIntegrationShardsCoverTestsOnce(t *testing.T, tests []integrationTestName, shardCount int) {
	t.Helper()
	seen := make(map[integrationTestName]int, len(tests))
	for shardIndex := 0; shardIndex < shardCount; shardIndex++ {
		targets, selectedCount, err := selectIntegrationTestTargets(tests, shardCount, shardIndex)
		if err != nil {
			t.Fatalf("select integration shard %d/%d: %v", shardIndex, shardCount, err)
		}
		counted := 0
		for _, target := range targets {
			for _, testName := range target.testNames {
				seen[integrationTestName{packagePath: target.packagePath, name: testName}]++
				counted++
			}
		}
		if counted != selectedCount {
			t.Fatalf("integration shard %d selected count = %d, grouped %d", shardIndex, selectedCount, counted)
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

// discoverIntegrationTestsForTest loads both build selections and returns their integration-only difference.
func discoverIntegrationTestsForTest(t *testing.T, executor integrationExecutor, baselineTags, integrationTags string) []integrationTestName {
	t.Helper()
	baseline, err := executor.listFrameworkPackageTests(baselineTags)
	if err != nil {
		t.Fatalf("list baseline framework tests for %q: %v", baselineTags, err)
	}
	integration, err := executor.listFrameworkPackageTests(integrationTags)
	if err != nil {
		t.Fatalf("list integration framework tests for %q: %v", integrationTags, err)
	}
	tests, err := integrationOnlyTestNames(baseline, integration)
	if err != nil {
		t.Fatalf("discover integration tests for %q: %v", integrationTags, err)
	}
	return tests
}

// assertDiscoveredIntegrationTest verifies one package-qualified test remains owned by the expected profile.
func assertDiscoveredIntegrationTest(t *testing.T, tests []integrationTestName, packagePath, testName string) {
	t.Helper()
	for _, discovered := range tests {
		if discovered.packagePath == packagePath && discovered.name == testName {
			return
		}
	}
	t.Fatalf("missing integration test %s/%s from %#v", packagePath, testName, tests)
}

// assertMissingIntegrationTest verifies ordinary or baseline tests do not leak into a specialized profile.
func assertMissingIntegrationTest(t *testing.T, tests []integrationTestName, testName string) {
	t.Helper()
	for _, discovered := range tests {
		if discovered.name == testName {
			t.Fatalf("unexpected integration test %s/%s in %#v", discovered.packagePath, testName, tests)
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
