package generate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type metricsTargetWant struct {
	process   string
	appTarget string
	target    string
}

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
		{process: "app", target: "metrics.internal:3200"},
	}
	assertMetricsTargets(t, targets, "Observability Test App", "staging", want)
}

func TestGenerateObservabilityFilesWritesLocalMultiTargetsByDefaultInDistributedMode(t *testing.T) {
	projectDir := observabilityTestProjectDir(t, "http", "jobs", "scheduler")

	t.Setenv("APP_NAME", "Observability Test App")
	t.Setenv("APP_ENV", "staging")
	t.Setenv("OBSERVABILITY_METRICS_TARGET_HOST", "metrics.internal")
	t.Setenv("RUNTIME_MODE", "distributed")

	written, err := GenerateObservabilityFiles(projectDir)
	if err != nil {
		t.Fatalf("GenerateObservabilityFiles returned error: %v", err)
	}
	if written != 1 {
		t.Fatalf("written files = %d, want 1", written)
	}

	targets := readMetricsTargets(t, projectDir)
	want := []metricsTargetWant{
		{process: "api", appTarget: "app", target: "metrics.internal:10000"},
		{process: "jobs", appTarget: "app", target: "metrics.internal:10002"},
		{process: "scheduler", appTarget: "app", target: "metrics.internal:10001"},
	}
	assertMetricsTargets(t, targets, "Observability Test App", "staging", want)
}

func TestGenerateObservabilityFilesWritesStandaloneTargetsForConventionalAppTargets(t *testing.T) {
	projectDir := observabilityTestProjectDir(t, "http", "jobs", "scheduler")
	writeObservabilityTargetMarker(t, projectDir, "billing")
	writeObservabilityTargetMarker(t, projectDir, "customer-portal")

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
		{process: "app", appTarget: "app", target: "host.docker.internal:3000"},
		{process: "app", appTarget: "billing", target: "host.docker.internal:3001"},
		{process: "app", appTarget: "customer-portal", target: "host.docker.internal:3002"},
	}
	assertMetricsTargets(t, targets, "Observability Test App", "local", want)
}

func TestGenerateObservabilityFilesWritesLocalMultiTargetsForConventionalAppTargets(t *testing.T) {
	projectDir := observabilityTestProjectDir(t, "http", "jobs", "scheduler")
	writeObservabilityTargetMarker(t, projectDir, "billing")
	writeObservabilityTargetMarker(t, projectDir, "customer-portal")

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
		{process: "api", appTarget: "app", target: "host.docker.internal:10000"},
		{process: "jobs", appTarget: "app", target: "host.docker.internal:10002"},
		{process: "scheduler", appTarget: "app", target: "host.docker.internal:10001"},
		{process: "api", appTarget: "billing", target: "host.docker.internal:10010"},
		{process: "jobs", appTarget: "billing", target: "host.docker.internal:11012"},
		{process: "scheduler", appTarget: "billing", target: "host.docker.internal:10011"},
		{process: "api", appTarget: "customer-portal", target: "host.docker.internal:10020"},
		{process: "jobs", appTarget: "customer-portal", target: "host.docker.internal:10022"},
		{process: "scheduler", appTarget: "customer-portal", target: "host.docker.internal:10021"},
	}
	assertMetricsTargets(t, targets, "Observability Test App", "local", want)
}

func TestGenerateObservabilityFilesFiltersLocalMultiTargetsByTargetComponents(t *testing.T) {
	projectDir := observabilityTestProjectDir(t, "http", "jobs", "scheduler")
	writeObservabilityTargetMarker(t, projectDir, "billing")
	writeObservabilityTargetMarker(t, projectDir, "reporting")
	config := []byte(strings.Join([]string{
		"project_name: Observability Test App",
		"render:",
		"  components:",
		"    web_api: true",
		"    metrics: true",
		"    scheduler: true",
		"    jobs: true",
		"app_targets:",
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
		{process: "api", appTarget: "app", target: "host.docker.internal:10000"},
		{process: "jobs", appTarget: "app", target: "host.docker.internal:10002"},
		{process: "scheduler", appTarget: "app", target: "host.docker.internal:10001"},
		{process: "api", appTarget: "billing", target: "host.docker.internal:10010"},
		{process: "api", appTarget: "reporting", target: "host.docker.internal:10020"},
		{process: "jobs", appTarget: "reporting", target: "host.docker.internal:10022"},
	}
	assertMetricsTargets(t, targets, "Observability Test App", "local", want)
}

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
		{process: "app", target: "host.docker.internal:3300"},
	}
	assertMetricsTargets(t, targets, "Observability Test App", "local", want)
}

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

func TestGenerateObservabilityFilesDoesNotTouchTargetsWhenLocalHostEscapeHatchIsEmpty(t *testing.T) {
	projectDir := observabilityTestProjectDir(t, "http", "jobs")
	targetsPath := filepath.Join(projectDir, "containers", "observability", "vmagent", "metrics-targets.json")
	original := []byte(`["custom"]` + "\n")
	if err := os.WriteFile(targetsPath, original, 0o644); err != nil {
		t.Fatalf("write custom targets: %v", err)
	}

	t.Setenv("OBSERVABILITY_METRICS_TARGET_HOST", "")

	written, err := GenerateObservabilityFiles(projectDir)
	if err != nil {
		t.Fatalf("GenerateObservabilityFiles returned error: %v", err)
	}
	if written != 0 {
		t.Fatalf("written files = %d, want 0", written)
	}

	content, err := os.ReadFile(targetsPath)
	if err != nil {
		t.Fatalf("read metrics-targets.json: %v", err)
	}
	if string(content) != string(original) {
		t.Fatalf("metrics-targets.json was modified:\n%s", content)
	}
}

func TestGenerateObservabilityFilesDoesNotTouchTargetsWhenModeDisabled(t *testing.T) {
	projectDir := observabilityTestProjectDir(t, "http", "jobs")
	targetsPath := filepath.Join(projectDir, "containers", "observability", "vmagent", "metrics-targets.json")
	original := []byte(`["custom"]` + "\n")
	if err := os.WriteFile(targetsPath, original, 0o644); err != nil {
		t.Fatalf("write custom targets: %v", err)
	}

	t.Setenv("OBSERVABILITY_METRICS_TARGET_MODE", "disabled")

	written, err := GenerateObservabilityFiles(projectDir)
	if err != nil {
		t.Fatalf("GenerateObservabilityFiles returned error: %v", err)
	}
	if written != 0 {
		t.Fatalf("written files = %d, want 0", written)
	}

	content, err := os.ReadFile(targetsPath)
	if err != nil {
		t.Fatalf("read metrics-targets.json: %v", err)
	}
	if string(content) != string(original) {
		t.Fatalf("metrics-targets.json was modified:\n%s", content)
	}
}

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

func writeObservabilityTargetMarker(t *testing.T, projectDir string, target string) {
	t.Helper()
	path := filepath.Join(projectDir, "cmd", target, "main.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir target marker %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write target marker %s: %v", path, err)
	}
}

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

func assertMetricsTargets(t *testing.T, targets []metricsTargetEntry, service string, environment string, want []metricsTargetWant) {
	t.Helper()
	if len(targets) != len(want) {
		t.Fatalf("target count = %d, want %d", len(targets), len(want))
	}
	for i, entry := range targets {
		wantAppTarget := want[i].appTarget
		if wantAppTarget == "" {
			wantAppTarget = "app"
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
		if entry.Labels["app_target"] != wantAppTarget {
			t.Fatalf("app_target label = %q, want %q", entry.Labels["app_target"], wantAppTarget)
		}
	}
}
