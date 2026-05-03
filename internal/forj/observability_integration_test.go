//go:build integration

package forj

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

func TestRenderedObservabilityStack(t *testing.T) {
	projectDir := t.TempDir()

	testkit.RenderProjectWithForj(t, projectDir, testkit.RenderProjectRequest{
		Config: project.Config{
			ProjectName:  "Observability Test App",
			GoModuleName: "example.com/observabilitytestapp",
			UpdatedAt:    "2026-04-21 00:00:00 UTC",
			Render: project.RenderConfig{
				Components: project.Components{
					CLI:           true,
					WebAPI:        true,
					Metrics:       true,
					Mail:          true,
					Docker:        true,
					DatabaseMySQL: true,
					Scheduler:     true,
					Jobs:          true,
					Observability: true,
					Grafana:       true,
				},
				QueueDriver: "redis",
			},
		},
	})

	buildCmd := exec.Command("go", "build", ".")
	buildCmd.Dir = projectDir
	buildCmd.Env = testkit.IntegrationGoProcessEnv(t, nil)
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build rendered app: %v\n%s", err, out)
	}

	t.Setenv("APP_NAME", "Observability Test App")
	t.Setenv("APP_ENV", "local")
	t.Setenv("OBSERVABILITY_METRICS_TARGET_HOST", "host.docker.internal")
	t.Setenv("METRICS_API_PORT", "9100")
	t.Setenv("METRICS_JOBS_PORT", "9101")
	t.Setenv("METRICS_SCHEDULER_PORT", "9102")
	if _, err := generate.GenerateObservabilityFiles(projectDir); err != nil {
		t.Fatalf("generate observability files: %v", err)
	}

	composeText := readRenderedFile(t, projectDir, "docker-compose.yml")
	for _, token := range []string{
		"victoriametrics:",
		"vmagent:",
		"grafana:",
		"grafana-seed:",
		"./containers/observability/vmagent:/etc/vmagent:ro",
		"./containers/observability/grafana/provisioning:/etc/grafana/provisioning:ro",
		"./containers/observability/grafana/dashboards:/var/lib/grafana/dashboards:ro",
		"./containers/observability/grafana/seed-dashboards.sh:/seed-dashboards.sh:ro",
	} {
		if !strings.Contains(composeText, token) {
			t.Fatalf("docker-compose.yml missing %q\n%s", token, composeText)
		}
	}

	prometheusYAML := readRenderedFile(t, projectDir, "containers/observability/vmagent/prometheus.yml")
	for _, token := range []string{
		"file_sd_configs:",
		"/etc/vmagent/metrics-targets.json",
	} {
		if !strings.Contains(prometheusYAML, token) {
			t.Fatalf("vmagent config missing %q\n%s", token, prometheusYAML)
		}
	}

	metricsTargetsJSON := readRenderedFile(t, projectDir, "containers/observability/vmagent/metrics-targets.json")
	for _, token := range []string{
		"host.docker.internal:9100",
		"host.docker.internal:9101",
		"host.docker.internal:9102",
		`"process": "api"`,
		`"process": "jobs"`,
		`"process": "scheduler"`,
		`"service": "Observability Test App"`,
		`"environment": "local"`,
	} {
		if !strings.Contains(metricsTargetsJSON, token) {
			t.Fatalf("metrics targets missing %q\n%s", token, metricsTargetsJSON)
		}
	}

	envText := readRenderedFile(t, projectDir, ".env")
	for _, token := range []string{
		"METRICS_PORT=9100",
		"METRICS_API_PORT=9100",
		"METRICS_JOBS_PORT=9101",
		"METRICS_SCHEDULER_PORT=9102",
		"OBSERVABILITY_METRICS_TARGET_MODE=auto",
		"OBSERVABILITY_METRICS_TARGET_HOST=host.docker.internal",
	} {
		if !strings.Contains(envText, token) {
			t.Fatalf(".env missing %q\n%s", token, envText)
		}
	}

	datasourceYAML := readRenderedFile(t, projectDir, "containers/observability/grafana/provisioning/datasources/datasource.yml")
	if !strings.Contains(datasourceYAML, "http://victoriametrics:8428") {
		t.Fatalf("grafana datasource missing victoria url\n%s", datasourceYAML)
	}

	grafanaSeedScript := readRenderedFile(t, projectDir, "containers/observability/grafana/seed-dashboards.sh")
	for _, token := range []string{
		"/api/user/stars/dashboard/uid/",
		"/api/org/preferences",
		"goforj-platform-overview",
		"goforj-cache-overview",
		"goforj-storage-overview",
		"goforj-events-overview",
		"goforj-http-overview",
		"goforj-auth-overview",
		"goforj-mail-overview",
		"goforj-database-overview",
		"goforj-queue-overview",
		"goforj-scheduler-overview",
	} {
		if !strings.Contains(grafanaSeedScript, token) {
			t.Fatalf("grafana seed script missing %q\n%s", token, grafanaSeedScript)
		}
	}

	httpDashboardJSON := readRenderedFile(t, projectDir, "containers/observability/grafana/dashboards/http-overview.json")
	for _, token := range []string{
		"HTTP Overview",
		"http_requests_by_route_total",
		"http_request_duration_by_route_seconds_bucket",
		"label_values(http_requests_by_route_total, route)",
	} {
		if !strings.Contains(httpDashboardJSON, token) {
			t.Fatalf("http dashboard missing %q\n%s", token, httpDashboardJSON)
		}
	}

	authDashboardJSON := readRenderedFile(t, projectDir, "containers/observability/grafana/dashboards/auth-overview.json")
	for _, token := range []string{
		"Auth Overview",
		"auth_flows_total",
		"auth_flow_duration_seconds_bucket",
		"oauth_callback",
		"request_auth",
	} {
		if !strings.Contains(authDashboardJSON, token) {
			t.Fatalf("auth dashboard missing %q\n%s", token, authDashboardJSON)
		}
	}

	cacheDashboardJSON := readRenderedFile(t, projectDir, "containers/observability/grafana/dashboards/cache-overview.json")
	for _, token := range []string{
		"Cache Overview",
		"cache_operations_total",
		"cache_operation_duration_seconds_bucket",
	} {
		if !strings.Contains(cacheDashboardJSON, token) {
			t.Fatalf("cache dashboard missing %q\n%s", token, cacheDashboardJSON)
		}
	}

	storageDashboardJSON := readRenderedFile(t, projectDir, "containers/observability/grafana/dashboards/storage-overview.json")
	for _, token := range []string{
		"Storage Overview",
		"storage_operations_total",
		"storage_operation_duration_seconds_bucket",
	} {
		if !strings.Contains(storageDashboardJSON, token) {
			t.Fatalf("storage dashboard missing %q\n%s", token, storageDashboardJSON)
		}
	}

	databaseDashboardJSON := readRenderedFile(t, projectDir, "containers/observability/grafana/dashboards/database-overview.json")
	for _, token := range []string{
		"Database Overview",
		"database_operations_total",
		"database_operation_duration_seconds_bucket",
		"database_queries_by_fingerprint_total",
		"database_query_fingerprint_info",
		"database_slow_queries_total",
		"database_slow_query_duration_seconds_bucket",
		"label_values(database_queries_by_fingerprint_total, fingerprint)",
		"Queries / sec",
		"Top Query Fingerprints By Volume",
		"P95 Slow Query Latency By Fingerprint",
		"Query Fingerprint Lookup",
	} {
		if !strings.Contains(databaseDashboardJSON, token) {
			t.Fatalf("database dashboard missing %q\n%s", token, databaseDashboardJSON)
		}
	}

	queueDashboardJSON := readRenderedFile(t, projectDir, "containers/observability/grafana/dashboards/queue-overview.json")
	for _, token := range []string{
		"Queue Overview",
		"queue_jobs_by_job_total",
		"queue_jobs_inflight_by_job",
		"queue_job_duration_by_queue_seconds_bucket",
		"queue_job_duration_by_job_seconds_bucket",
		"job_name",
	} {
		if !strings.Contains(queueDashboardJSON, token) {
			t.Fatalf("queue dashboard missing %q\n%s", token, queueDashboardJSON)
		}
	}

	schedulerDashboardJSON := readRenderedFile(t, projectDir, "containers/observability/grafana/dashboards/scheduler-overview.json")
	for _, token := range []string{
		"Scheduler Overview",
		"scheduler_runs_by_job_total",
		"scheduler_events_by_job_total",
		"scheduler_job_duration_by_job_seconds_bucket",
		"job_name",
	} {
		if !strings.Contains(schedulerDashboardJSON, token) {
			t.Fatalf("scheduler dashboard missing %q\n%s", token, schedulerDashboardJSON)
		}
	}

	schedulerRegistryGo := readRenderedFile(t, projectDir, "internal/scheduler/scheduler_registry.go")
	if !strings.Contains(schedulerRegistryGo, "s.metrics.RecordSchedulerJob(event)") {
		t.Fatalf("scheduler registry missing metrics observer hook\n%s", schedulerRegistryGo)
	}

	eventsDashboardJSON := readRenderedFile(t, projectDir, "containers/observability/grafana/dashboards/events-overview.json")
	for _, token := range []string{
		"Events Overview",
		"events_publishes_total",
		"events_publish_duration_seconds_bucket",
	} {
		if !strings.Contains(eventsDashboardJSON, token) {
			t.Fatalf("events dashboard missing %q\n%s", token, eventsDashboardJSON)
		}
	}

	mailDashboardJSON := readRenderedFile(t, projectDir, "containers/observability/grafana/dashboards/mail-overview.json")
	for _, token := range []string{
		"Mail Overview",
		"mail_sends_total",
		"mail_send_duration_seconds_bucket",
	} {
		if !strings.Contains(mailDashboardJSON, token) {
			t.Fatalf("mail dashboard missing %q\n%s", token, mailDashboardJSON)
		}
	}

	platformDashboardJSON := readRenderedFile(t, projectDir, "containers/observability/grafana/dashboards/platform-overview.json")
	for _, token := range []string{
		"Platform Overview",
		"http_requests_total",
		"queue_jobs_by_job_total",
		"scheduler_runs_by_job_total",
		"job_name",
		"database_operations_total",
		"cache_operations_total",
		"storage_operations_total",
		"events_publishes_total",
		"mail_sends_total",
	} {
		if !strings.Contains(platformDashboardJSON, token) {
			t.Fatalf("platform dashboard missing %q\n%s", token, platformDashboardJSON)
		}
	}

	observabilityReadme := readRenderedFile(t, projectDir, "internal/observability/README.md")
	if !strings.Contains(observabilityReadme, "http://localhost:8428") ||
		!strings.Contains(observabilityReadme, "http://localhost:3001") ||
		!strings.Contains(observabilityReadme, "grafana-seed") {
		t.Fatalf("observability readme missing local URLs\n%s", observabilityReadme)
	}
}

func readRenderedFile(t *testing.T, root string, rel string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read rendered file %s: %v", rel, err)
	}
	return string(body)
}
