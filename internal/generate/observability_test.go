package generate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// metricsTargetWant keeps endpoint and label expectations readable in generated target assertions.
type metricsTargetWant struct {
	process string
	appName string
	target  string
}

// TestGenerateObservabilityFilesWritesSingleProcessTargetsByDefaultInStandaloneMode verifies default generation follows the combined App process.
func TestGenerateObservabilityFilesWritesSingleProcessTargetsByDefaultInStandaloneMode(t *testing.T) {
	projectDir := observabilityTestProjectDir(t, "http", "jobs", "scheduler")

	t.Setenv("APP_NAME", "Observability Test App")
	t.Setenv("APP_ENV", "staging")
	t.Setenv("OBSERVABILITY_METRICS_TARGET_HOST", "metrics.internal")
	t.Setenv("METRICS_PORT", "9200")
	t.Setenv("API_HTTP_PORT", "3200")

	written, err := GenerateObservabilityFiles(projectDir)
	if err != nil {
		t.Fatalf("GenerateObservabilityFiles returned error: %v", err)
	}
	if written != 1 {
		t.Fatalf("written files = %d, want 1", written)
	}

	targets := readMetricsTargets(t, projectDir)
	want := []metricsTargetWant{
		{process: "app", target: "metrics.internal:9200"},
	}
	assertMetricsTargets(t, targets, "Observability Test App", "staging", want)
}

// TestGenerateObservabilityFilesRejectsInvalidStandaloneMetricsPort verifies a bad scrape listener cannot silently fall back to public HTTP traffic.
func TestGenerateObservabilityFilesRejectsInvalidStandaloneMetricsPort(t *testing.T) {
	projectDir := observabilityTestProjectDir(t, "http")

	t.Setenv("OBSERVABILITY_METRICS_TARGET_MODE", "local-single")
	t.Setenv("METRICS_PORT", "not-a-port")
	t.Setenv("API_HTTP_PORT", "3200")

	_, err := GenerateObservabilityFiles(projectDir)
	if err == nil || !strings.Contains(err.Error(), `invalid METRICS_PORT "not-a-port"`) {
		t.Fatalf("GenerateObservabilityFiles error = %v, want invalid dedicated metrics port", err)
	}
}

// TestGenerateObservabilityFilesWritesLocalMultiTargetsWhenExplicitlySelected keeps process layout tied to the selected commands.
func TestGenerateObservabilityFilesWritesLocalMultiTargetsWhenExplicitlySelected(t *testing.T) {
	projectDir := observabilityTestProjectDir(t, "http", "jobs", "scheduler")

	t.Setenv("APP_NAME", "Observability Test App")
	t.Setenv("APP_ENV", "staging")
	t.Setenv("OBSERVABILITY_METRICS_TARGET_HOST", "metrics.internal")
	t.Setenv("OBSERVABILITY_METRICS_TARGET_MODE", "local-multi")

	written, err := GenerateObservabilityFiles(projectDir)
	if err != nil {
		t.Fatalf("GenerateObservabilityFiles returned error: %v", err)
	}
	if written != 1 {
		t.Fatalf("written files = %d, want 1", written)
	}

	targets := readMetricsTargets(t, projectDir)
	want := []metricsTargetWant{
		{process: "api", appName: "app", target: "metrics.internal:10000"},
		{process: "jobs", appName: "app", target: "metrics.internal:10002"},
		{process: "scheduler", appName: "app", target: "metrics.internal:10001"},
	}
	assertMetricsTargets(t, targets, "Observability Test App", "staging", want)
}

// TestGenerateObservabilityFilesWritesStandaloneTargetsForConventionalApps verifies conventionally discovered Apps receive stable combined-process targets.
func TestGenerateObservabilityFilesWritesStandaloneTargetsForConventionalApps(t *testing.T) {
	projectDir := observabilityTestProjectDir(t, "http", "jobs", "scheduler")
	writeObservabilityAppMarker(t, projectDir, "billing")
	writeObservabilityAppMarker(t, projectDir, "customer-portal")

	t.Setenv("APP_NAME", "Observability Test App")
	t.Setenv("APP_ENV", "local")
	t.Setenv("OBSERVABILITY_METRICS_TARGET_MODE", "local-single")
	t.Setenv("OBSERVABILITY_METRICS_TARGET_HOST", "host.docker.internal")

	written, err := GenerateObservabilityFiles(projectDir)
	if err != nil {
		t.Fatalf("GenerateObservabilityFiles returned error: %v", err)
	}
	if written != 1 {
		t.Fatalf("written files = %d, want 1", written)
	}

	targets := readMetricsTargets(t, projectDir)
	want := []metricsTargetWant{
		{process: "app", appName: "app", target: "host.docker.internal:10000"},
		{process: "app", appName: "billing", target: "host.docker.internal:10010"},
		{process: "app", appName: "customer-portal", target: "host.docker.internal:10020"},
	}
	assertMetricsTargets(t, targets, "Observability Test App", "local", want)
}

// TestGenerateObservabilityFilesWritesLocalMultiTargetsForConventionalApps verifies role ports remain deterministic across conventionally discovered Apps.
func TestGenerateObservabilityFilesWritesLocalMultiTargetsForConventionalApps(t *testing.T) {
	projectDir := observabilityTestProjectDir(t, "http", "jobs", "scheduler")
	writeObservabilityAppMarker(t, projectDir, "billing")
	writeObservabilityAppMarker(t, projectDir, "customer-portal")

	t.Setenv("APP_NAME", "Observability Test App")
	t.Setenv("APP_ENV", "local")
	t.Setenv("OBSERVABILITY_METRICS_TARGET_MODE", "local-multi")
	t.Setenv("OBSERVABILITY_METRICS_TARGET_HOST", "host.docker.internal")
	t.Setenv("BILLING_METRICS_JOBS_PORT", "11012")

	written, err := GenerateObservabilityFiles(projectDir)
	if err != nil {
		t.Fatalf("GenerateObservabilityFiles returned error: %v", err)
	}
	if written != 1 {
		t.Fatalf("written files = %d, want 1", written)
	}

	targets := readMetricsTargets(t, projectDir)
	want := []metricsTargetWant{
		{process: "api", appName: "app", target: "host.docker.internal:10000"},
		{process: "jobs", appName: "app", target: "host.docker.internal:10002"},
		{process: "scheduler", appName: "app", target: "host.docker.internal:10001"},
		{process: "api", appName: "billing", target: "host.docker.internal:10010"},
		{process: "jobs", appName: "billing", target: "host.docker.internal:11012"},
		{process: "scheduler", appName: "billing", target: "host.docker.internal:10011"},
		{process: "api", appName: "customer-portal", target: "host.docker.internal:10020"},
		{process: "jobs", appName: "customer-portal", target: "host.docker.internal:10022"},
		{process: "scheduler", appName: "customer-portal", target: "host.docker.internal:10021"},
	}
	assertMetricsTargets(t, targets, "Observability Test App", "local", want)
}

// TestGenerateObservabilityFilesFiltersLocalMultiTargetsByAppComponents verifies each App emits only the runtime roles selected for that App.
func TestGenerateObservabilityFilesFiltersLocalMultiTargetsByAppComponents(t *testing.T) {
	projectDir := observabilityTestProjectDir(t, "http", "jobs", "scheduler")
	writeObservabilityAppMarker(t, projectDir, "billing")
	writeObservabilityAppMarker(t, projectDir, "reporting")
	config := []byte(strings.Join([]string{
		"project_name: Observability Test App",
		"render:",
		"  components:",
		"    web_api: true",
		"    metrics: true",
		"    scheduler: true",
		"    jobs: true",
		"apps:",
		"  billing:",
		"    components:",
		"      web_api: true",
		"  reporting:",
		"    components:",
		"      jobs: true",
		"",
	}, "\n"))
	if err := os.WriteFile(filepath.Join(projectDir, ".goforj.yml"), config, 0o644); err != nil {
		t.Fatalf("write .goforj.yml: %v", err)
	}

	t.Setenv("APP_NAME", "Observability Test App")
	t.Setenv("APP_ENV", "local")
	t.Setenv("OBSERVABILITY_METRICS_TARGET_MODE", "local-multi")
	t.Setenv("OBSERVABILITY_METRICS_TARGET_HOST", "host.docker.internal")

	written, err := GenerateObservabilityFiles(projectDir)
	if err != nil {
		t.Fatalf("GenerateObservabilityFiles returned error: %v", err)
	}
	if written != 1 {
		t.Fatalf("written files = %d, want 1", written)
	}

	targets := readMetricsTargets(t, projectDir)
	want := []metricsTargetWant{
		{process: "api", appName: "app", target: "host.docker.internal:10000"},
		{process: "jobs", appName: "app", target: "host.docker.internal:10002"},
		{process: "scheduler", appName: "app", target: "host.docker.internal:10001"},
		{process: "api", appName: "billing", target: "host.docker.internal:10010"},
		{process: "api", appName: "reporting", target: "host.docker.internal:10020"},
		{process: "jobs", appName: "reporting", target: "host.docker.internal:10022"},
	}
	assertMetricsTargets(t, targets, "Observability Test App", "local", want)
}

// TestGenerateObservabilityFilesWritesSingleProcessTargetInLocalSingleMode verifies explicit combined-process mode follows the dedicated metrics listener.
func TestGenerateObservabilityFilesWritesSingleProcessTargetInLocalSingleMode(t *testing.T) {
	projectDir := observabilityTestProjectDir(t, "http")

	t.Setenv("APP_NAME", "Observability Test App")
	t.Setenv("APP_ENV", "local")
	t.Setenv("OBSERVABILITY_METRICS_TARGET_MODE", "local-single")
	t.Setenv("OBSERVABILITY_METRICS_TARGET_HOST", "host.docker.internal")
	t.Setenv("METRICS_PORT", "9300")
	t.Setenv("API_HTTP_PORT", "3300")

	written, err := GenerateObservabilityFiles(projectDir)
	if err != nil {
		t.Fatalf("GenerateObservabilityFiles returned error: %v", err)
	}
	if written != 1 {
		t.Fatalf("written files = %d, want 1", written)
	}

	targets := readMetricsTargets(t, projectDir)
	want := []metricsTargetWant{
		{process: "app", target: "host.docker.internal:9300"},
	}
	assertMetricsTargets(t, targets, "Observability Test App", "local", want)
}

// TestGenerateObservabilityFilesWritesComposeTargetsUsingSharedPort verifies Compose roles share the configured metrics port.
func TestGenerateObservabilityFilesWritesComposeTargetsUsingSharedPort(t *testing.T) {
	projectDir := observabilityTestProjectDir(t, "http", "jobs", "scheduler")

	t.Setenv("APP_NAME", "Observability Test App")
	t.Setenv("APP_ENV", "prod")
	t.Setenv("OBSERVABILITY_METRICS_TARGET_MODE", "compose")
	t.Setenv("METRICS_PORT", "9400")
	t.Setenv("OBSERVABILITY_API_METRICS_HOST", "api")
	t.Setenv("OBSERVABILITY_JOBS_METRICS_HOST", "jobs-worker")
	t.Setenv("OBSERVABILITY_SCHEDULER_METRICS_HOST", "scheduler")

	written, err := GenerateObservabilityFiles(projectDir)
	if err != nil {
		t.Fatalf("GenerateObservabilityFiles returned error: %v", err)
	}
	if written != 1 {
		t.Fatalf("written files = %d, want 1", written)
	}

	targets := readMetricsTargets(t, projectDir)
	want := []metricsTargetWant{
		{process: "api", target: "api:9400"},
		{process: "jobs", target: "jobs-worker:9400"},
		{process: "scheduler", target: "scheduler:9400"},
	}
	assertMetricsTargets(t, targets, "Observability Test App", "prod", want)
}

// TestGenerateObservabilityFilesAddsOptionalDeploymentMetadata verifies every target from one generated plan receives the same bounded deployment identity.
func TestGenerateObservabilityFilesAddsOptionalDeploymentMetadata(t *testing.T) {
	projectDir := observabilityTestProjectDir(t, "http", "jobs")

	t.Setenv("APP_NAME", "Observability Test App")
	t.Setenv("APP_ENV", "prod")
	t.Setenv("APP_VERSION", " v1.24.0 ")
	t.Setenv("APP_REVISION", " sha256:0123456789abcdef ")
	t.Setenv("OBSERVABILITY_METRICS_TARGET_MODE", "compose")
	t.Setenv("OBSERVABILITY_API_METRICS_HOST", "api")
	t.Setenv("OBSERVABILITY_JOBS_METRICS_HOST", "jobs")

	if _, err := GenerateObservabilityFiles(projectDir); err != nil {
		t.Fatalf("GenerateObservabilityFiles returned error: %v", err)
	}

	for _, target := range readMetricsTargets(t, projectDir) {
		if got := target.Labels["release"]; got != "v1.24.0" {
			t.Errorf("release label = %q, want %q", got, "v1.24.0")
		}
		if got := target.Labels["revision"]; got != "sha256:0123456789abcdef" {
			t.Errorf("revision label = %q, want %q", got, "sha256:0123456789abcdef")
		}
	}
}

// TestGenerateObservabilityFilesRejectsInvalidDeploymentMetadata prevents free-form metadata from expanding scrape-target cardinality.
func TestGenerateObservabilityFilesRejectsInvalidDeploymentMetadata(t *testing.T) {
	for _, test := range []struct {
		name  string
		key   string
		value string
	}{
		{name: "release whitespace", key: "APP_VERSION", value: "release candidate"},
		{name: "revision whitespace", key: "APP_REVISION", value: "abc 123"},
		{name: "oversized release", key: "APP_VERSION", value: strings.Repeat("a", maxDeploymentMetadataLabelLength+1)},
		{name: "oversized revision", key: "APP_REVISION", value: strings.Repeat("a", maxDeploymentMetadataLabelLength+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			projectDir := observabilityTestProjectDir(t, "http")
			t.Setenv("OBSERVABILITY_METRICS_TARGET_HOST", "metrics.internal")
			t.Setenv(test.key, test.value)

			_, err := GenerateObservabilityFiles(projectDir)
			if err == nil || !strings.Contains(err.Error(), test.key) {
				t.Fatalf("GenerateObservabilityFiles error = %v, want %s validation error", err, test.key)
			}
		})
	}
}

// TestGenerateObservabilityFilesIgnoresStaleJobsRoleWhenDisabled verifies stale worker packages cannot restore a Jobs metrics target.
func TestGenerateObservabilityFilesIgnoresStaleJobsRoleWhenDisabled(t *testing.T) {
	projectDir := observabilityTestProjectDir(t, "http", "jobs")
	config := []byte(strings.Join([]string{
		"project_name: Observability Test App",
		"render:",
		"  components: [web_api, metrics, observability]",
		"",
	}, "\n"))
	if err := os.WriteFile(filepath.Join(projectDir, ".goforj.yml"), config, 0o644); err != nil {
		t.Fatalf("write .goforj.yml: %v", err)
	}

	t.Setenv("APP_NAME", "Observability Test App")
	t.Setenv("APP_ENV", "prod")
	t.Setenv("OBSERVABILITY_METRICS_TARGET_MODE", "compose")
	t.Setenv("METRICS_PORT", "9400")
	t.Setenv("OBSERVABILITY_API_METRICS_HOST", "api")
	t.Setenv("OBSERVABILITY_JOBS_METRICS_HOST", "stale-jobs-worker")

	written, err := GenerateObservabilityFiles(projectDir)
	if err != nil {
		t.Fatalf("GenerateObservabilityFiles returned error: %v", err)
	}
	if written != 1 {
		t.Fatalf("written files = %d, want 1", written)
	}

	targets := readMetricsTargets(t, projectDir)
	want := []metricsTargetWant{
		{process: "api", target: "api:9400"},
	}
	assertMetricsTargets(t, targets, "Observability Test App", "prod", want)
}

// TestGenerateObservabilityFilesReturnsPlanningErrors verifies invalid project state cannot be mistaken for a successful target refresh.
func TestGenerateObservabilityFilesReturnsPlanningErrors(t *testing.T) {
	tests := []struct {
		name      string
		roles     []string
		setup     func(t *testing.T, projectDir string)
		wantError string
	}{
		{
			name:  "malformed project config",
			roles: []string{"http"},
			setup: func(t *testing.T, projectDir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(projectDir, ".goforj.yml"), []byte("render: [\n"), 0o644); err != nil {
					t.Fatalf("write malformed config: %v", err)
				}
			},
			wantError: "load project observability config",
		},
		{
			name: "role inspection error",
			setup: func(t *testing.T, projectDir string) {
				t.Helper()
				internalDir := filepath.Join(projectDir, "internal")
				if err := os.MkdirAll(internalDir, 0o755); err != nil {
					t.Fatalf("mkdir internal: %v", err)
				}
				if err := os.Symlink("http", filepath.Join(internalDir, "http")); err != nil {
					t.Fatalf("create role symlink loop: %v", err)
				}
			},
			wantError: "inspect observability api role",
		},
		{
			name:  "app discovery error",
			roles: []string{"http"},
			setup: func(t *testing.T, projectDir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(projectDir, "cmd"), []byte("not a directory\n"), 0o644); err != nil {
					t.Fatalf("write invalid command root: %v", err)
				}
			},
			wantError: "discover observability apps",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectDir := observabilityTestProjectDir(t, test.roles...)
			targetsPath := filepath.Join(projectDir, "containers", "observability", "vmagent", "metrics-targets.json")
			original := []byte("[\"existing\"]\n")
			if err := os.WriteFile(targetsPath, original, 0o644); err != nil {
				t.Fatalf("write existing targets: %v", err)
			}
			test.setup(t, projectDir)

			written, err := GenerateObservabilityFiles(projectDir)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("GenerateObservabilityFiles error = %v, want containing %q", err, test.wantError)
			}
			if written != 0 {
				t.Fatalf("written files = %d, want 0", written)
			}
			content, readErr := os.ReadFile(targetsPath)
			if readErr != nil {
				t.Fatalf("read existing targets: %v", readErr)
			}
			if string(content) != string(original) {
				t.Fatalf("metrics-targets.json was modified after planning error:\n%s", content)
			}
		})
	}
}

// TestGenerateObservabilityFilesExcludesOnlyDisabledComposeRole verifies one role opt-out cannot suppress unrelated scrape targets.
func TestGenerateObservabilityFilesExcludesOnlyDisabledComposeRole(t *testing.T) {
	projectDir := observabilityTestProjectDir(t, "http", "jobs", "scheduler")

	t.Setenv("APP_NAME", "Observability Test App")
	t.Setenv("APP_ENV", "prod")
	t.Setenv("OBSERVABILITY_METRICS_TARGET_MODE", "compose")
	t.Setenv("METRICS_PORT", "9400")
	t.Setenv("OBSERVABILITY_API_METRICS_HOST", "api")
	t.Setenv("OBSERVABILITY_JOBS_METRICS_HOST", "")
	t.Setenv("OBSERVABILITY_SCHEDULER_METRICS_HOST", "scheduler")

	written, err := GenerateObservabilityFiles(projectDir)
	if err != nil {
		t.Fatalf("GenerateObservabilityFiles returned error: %v", err)
	}
	if written != 1 {
		t.Fatalf("written files = %d, want 1", written)
	}

	targets := readMetricsTargets(t, projectDir)
	want := []metricsTargetWant{
		{process: "api", target: "api:9400"},
		{process: "scheduler", target: "scheduler:9400"},
	}
	assertMetricsTargets(t, targets, "Observability Test App", "prod", want)
}

// TestGenerateObservabilityFilesPublishesManagedEmptyTargets verifies stale entries disappear when a managed plan no longer has scrape sources.
func TestGenerateObservabilityFilesPublishesManagedEmptyTargets(t *testing.T) {
	tests := []struct {
		name  string
		roles []string
		setup func(t *testing.T)
	}{
		{
			name: "no rendered roles",
			setup: func(t *testing.T) {
				t.Helper()
			},
		},
		{
			name:  "all compose roles excluded",
			roles: []string{"http", "jobs", "scheduler"},
			setup: func(t *testing.T) {
				t.Helper()
				t.Setenv("OBSERVABILITY_METRICS_TARGET_MODE", "compose")
				t.Setenv("OBSERVABILITY_API_METRICS_HOST", "")
				t.Setenv("OBSERVABILITY_JOBS_METRICS_HOST", "")
				t.Setenv("OBSERVABILITY_SCHEDULER_METRICS_HOST", "")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectDir := observabilityTestProjectDir(t, test.roles...)
			targetsPath := filepath.Join(projectDir, "containers", "observability", "vmagent", "metrics-targets.json")
			if err := os.WriteFile(targetsPath, []byte("[\"stale\"]\n"), 0o644); err != nil {
				t.Fatalf("write stale targets: %v", err)
			}
			test.setup(t)

			written, err := GenerateObservabilityFiles(projectDir)
			if err != nil {
				t.Fatalf("GenerateObservabilityFiles returned error: %v", err)
			}
			if written != 1 {
				t.Fatalf("written files = %d, want 1", written)
			}
			content, err := os.ReadFile(targetsPath)
			if err != nil {
				t.Fatalf("read generated targets: %v", err)
			}
			if string(content) != "[]\n" {
				t.Fatalf("metrics-targets.json = %q, want empty JSON array", content)
			}

			written, err = GenerateObservabilityFiles(projectDir)
			if err != nil {
				t.Fatalf("second GenerateObservabilityFiles returned error: %v", err)
			}
			if written != 0 {
				t.Fatalf("second written files = %d, want 0", written)
			}
		})
	}
}

// TestGenerateObservabilityFilesPreservesUnmanagedTargets verifies opt-outs remain authoritative before empty managed plans are published.
func TestGenerateObservabilityFilesPreservesUnmanagedTargets(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T)
	}{
		{
			name: "disabled mode",
			setup: func(t *testing.T) {
				t.Helper()
				t.Setenv("OBSERVABILITY_METRICS_TARGET_MODE", "disabled")
			},
		},
		{
			name: "empty local host escape hatch",
			setup: func(t *testing.T) {
				t.Helper()
				t.Setenv("OBSERVABILITY_METRICS_TARGET_MODE", "local-single")
				t.Setenv("OBSERVABILITY_METRICS_TARGET_HOST", "")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectDir := observabilityTestProjectDir(t)
			targetsPath := filepath.Join(projectDir, "containers", "observability", "vmagent", "metrics-targets.json")
			original := []byte("[\"custom\"]\n")
			if err := os.WriteFile(targetsPath, original, 0o644); err != nil {
				t.Fatalf("write custom targets: %v", err)
			}
			test.setup(t)

			written, err := GenerateObservabilityFiles(projectDir)
			if err != nil {
				t.Fatalf("GenerateObservabilityFiles returned error: %v", err)
			}
			if written != 0 {
				t.Fatalf("written files = %d, want 0", written)
			}
			content, err := os.ReadFile(targetsPath)
			if err != nil {
				t.Fatalf("read custom targets: %v", err)
			}
			if string(content) != string(original) {
				t.Fatalf("metrics-targets.json was modified:\n%s", content)
			}
		})
	}
}

// observabilityTestProjectDir creates the minimum rendered layout needed to exercise target discovery.
func observabilityTestProjectDir(t *testing.T, roles ...string) string {
	t.Helper()
	projectDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectDir, "containers", "observability", "vmagent"), 0o755); err != nil {
		t.Fatalf("mkdir vmagent dir: %v", err)
	}
	for _, role := range roles {
		dir := role
		if role == "scheduler" {
			dir = "schedules"
		}
		if err := os.MkdirAll(filepath.Join(projectDir, "internal", dir), 0o755); err != nil {
			t.Fatalf("mkdir %s dir: %v", role, err)
		}
	}
	return projectDir
}

// writeObservabilityAppMarker uses the command entrypoint convention so tests cover filesystem-driven App discovery.
func writeObservabilityAppMarker(t *testing.T, projectDir string, appName string) {
	t.Helper()
	path := filepath.Join(projectDir, "cmd", appName, "main.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir app marker %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write app marker %s: %v", path, err)
	}
}

// readMetricsTargets decodes the generated discovery file for behavior-focused assertions.
func readMetricsTargets(t *testing.T, projectDir string) []metricsTargetEntry {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(projectDir, "containers", "observability", "vmagent", "metrics-targets.json"))
	if err != nil {
		t.Fatalf("read metrics-targets.json: %v", err)
	}
	var targets []metricsTargetEntry
	if err := json.Unmarshal(content, &targets); err != nil {
		t.Fatalf("unmarshal metrics-targets.json: %v", err)
	}
	return targets
}

// assertMetricsTargets compares endpoint ordering and the labels relied on by dashboards and alerts.
func assertMetricsTargets(t *testing.T, targets []metricsTargetEntry, service string, environment string, want []metricsTargetWant) {
	t.Helper()
	if len(targets) != len(want) {
		t.Fatalf("target count = %d, want %d", len(targets), len(want))
	}
	for i, entry := range targets {
		wantApp := want[i].appName
		if wantApp == "" {
			wantApp = "app"
		}
		if len(entry.Targets) != 1 || entry.Targets[0] != want[i].target {
			t.Fatalf("targets[%d] = %#v, want %q", i, entry.Targets, want[i].target)
		}
		if entry.Labels["process"] != want[i].process {
			t.Fatalf("process label = %q, want %q", entry.Labels["process"], want[i].process)
		}
		if entry.Labels["service"] != service {
			t.Fatalf("service label = %q, want %q", entry.Labels["service"], service)
		}
		if entry.Labels["environment"] != environment {
			t.Fatalf("environment label = %q, want %q", entry.Labels["environment"], environment)
		}
		if entry.Labels["app"] != wantApp {
			t.Fatalf("app label = %q, want %q", entry.Labels["app"], wantApp)
		}
		if _, ok := entry.Labels["release"]; ok {
			t.Fatalf("release label = %q, want omitted without APP_VERSION", entry.Labels["release"])
		}
		if _, ok := entry.Labels["revision"]; ok {
			t.Fatalf("revision label = %q, want omitted without APP_REVISION", entry.Labels["revision"])
		}
	}
}
