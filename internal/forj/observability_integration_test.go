//go:build integration

package forj

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

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
			},
		},
	})

	renderedDefaultTargets := readRenderedFile(t, projectDir, "containers/observability/vmagent/metrics-targets.json")
	for _, token := range []string{
		"host.docker.internal:3000",
		`"process": "app"`,
		`"service": "Observability Test App"`,
		`"environment": "local"`,
	} {
		if !strings.Contains(renderedDefaultTargets, token) {
			t.Fatalf("rendered default metrics targets missing %q\n%s", token, renderedDefaultTargets)
		}
	}

	buildRenderedDefaultApp(t, projectDir, nil, "build rendered app")

	t.Setenv("APP_NAME", "Observability Test App")
	t.Setenv("APP_ENV", "local")
	t.Setenv("OBSERVABILITY_METRICS_TARGET_HOST", "host.docker.internal")
	t.Setenv("API_HTTP_PORT", "3000")
	t.Setenv("METRICS_API_PORT", "10000")
	t.Setenv("METRICS_JOBS_PORT", "10002")
	t.Setenv("METRICS_SCHEDULER_PORT", "10001")
	if _, err := generate.GenerateObservabilityFiles(projectDir); err != nil {
		t.Fatalf("generate observability files: %v", err)
	}

	composeText := readRenderedFile(t, projectDir, "docker-compose.yml")
	for _, token := range []string{
		"victoriametrics:",
		"vmagent:",
		"grafana:",
		"grafana-seed:",
		"stop_grace_period: 1s",
		"build:\n      context: ./containers/observability/vmagent",
		"victoriametrics:/victoria-metrics-data",
		"build:\n      context: ./containers/observability/grafana",
		"grafana:/var/lib/grafana",
		"dockerfile: Dockerfile.seed",
		`entrypoint: ["sh"]`,
		`command: ["/seed-dashboards.sh"]`,
		"mariadb:/var/lib/mysql",
	} {
		if !strings.Contains(composeText, token) {
			t.Fatalf("docker-compose.yml missing %q\n%s", token, composeText)
		}
	}
	if strings.Contains(composeText, "/var/lib/grafana/dashboards") {
		t.Fatalf("docker-compose.yml should not mount dashboards under Grafana data path\n%s", composeText)
	}
	for _, token := range []string{
		"grafana-data-init:",
		`condition: service_completed_successfully`,
		`chown -R "$${uid}:$${gid}" /var/lib/grafana`,
		"./_data/victoriametrics:/victoria-metrics-data",
		"./_data/grafana:/var/lib/grafana",
		"./_data/mariadb:/var/lib/mysql",
		"image: victoriametrics/vmagent:v1.120.0",
		"image: grafana/grafana:12.0.2",
		"image: curlimages/curl:8.10.1",
		"./containers/observability/vmagent:/etc/vmagent:ro",
		"./containers/observability/grafana/provisioning:/etc/grafana/provisioning:ro",
		"./containers/observability/grafana/dashboards:/etc/grafana/dashboards:ro",
		"./containers/observability/grafana/seed-dashboards.sh:/seed-dashboards.sh:ro",
		"source: ./containers/observability/vmagent/prometheus.yml",
		"target: /etc/vmagent/prometheus.yml",
		"source: ./containers/observability/vmagent/metrics-targets.json",
		"target: /etc/vmagent/metrics-targets.json",
		"source: ./containers/observability/grafana/provisioning",
		"source: ./containers/observability/grafana/dashboards",
		"source: ./containers/observability/grafana/seed-dashboards.sh",
		"create_host_path: false",
		`command: ["sh", "/seed-dashboards.sh"]`,
	} {
		if strings.Contains(composeText, token) {
			t.Fatalf("docker-compose.yml should not contain %q\n%s", token, composeText)
		}
	}

	vmagentDockerfile := readRenderedFile(t, projectDir, "containers/observability/vmagent/Dockerfile")
	for _, token := range []string{
		"FROM victoriametrics/vmagent:v1.120.0",
		"COPY prometheus.yml /etc/vmagent/prometheus.yml",
		"COPY metrics-targets.json /etc/vmagent/metrics-targets.json",
	} {
		if !strings.Contains(vmagentDockerfile, token) {
			t.Fatalf("vmagent Dockerfile missing %q\n%s", token, vmagentDockerfile)
		}
	}

	grafanaDockerfile := readRenderedFile(t, projectDir, "containers/observability/grafana/Dockerfile")
	for _, token := range []string{
		"FROM grafana/grafana:12.0.2",
		"COPY provisioning /etc/grafana/provisioning",
		"COPY dashboards /etc/grafana/dashboards",
	} {
		if !strings.Contains(grafanaDockerfile, token) {
			t.Fatalf("grafana Dockerfile missing %q\n%s", token, grafanaDockerfile)
		}
	}

	grafanaSeedDockerfile := readRenderedFile(t, projectDir, "containers/observability/grafana/Dockerfile.seed")
	for _, token := range []string{
		"FROM curlimages/curl:8.10.1",
		"COPY seed-dashboards.sh /seed-dashboards.sh",
	} {
		if !strings.Contains(grafanaSeedDockerfile, token) {
			t.Fatalf("grafana seed Dockerfile missing %q\n%s", token, grafanaSeedDockerfile)
		}
	}
	if strings.Contains(grafanaSeedDockerfile, "COPY dashboards") {
		t.Fatalf("grafana seed Dockerfile should rely on Grafana provisioning for dashboards\n%s", grafanaSeedDockerfile)
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
		"host.docker.internal:3000",
		`"process": "app"`,
		`"service": "Observability Test App"`,
		`"environment": "local"`,
	} {
		if !strings.Contains(metricsTargetsJSON, token) {
			t.Fatalf("metrics targets missing %q\n%s", token, metricsTargetsJSON)
		}
	}

	envText := readRenderedFile(t, projectDir, ".env")
	for _, token := range []string{
		"METRICS_PORT=10000",
		"METRICS_API_PORT=10000",
		"METRICS_JOBS_PORT=10002",
		"METRICS_SCHEDULER_PORT=10001",
		"OBSERVABILITY_METRICS_TARGET_MODE=auto",
		"OBSERVABILITY_METRICS_TARGET_HOST=host.docker.internal",
		"GRAFANA_PORT=13001",
	} {
		if !strings.Contains(envText, token) {
			t.Fatalf(".env missing %q\n%s", token, envText)
		}
	}

	datasourceYAML := readRenderedFile(t, projectDir, "containers/observability/grafana/provisioning/datasources/datasource.yml")
	if !strings.Contains(datasourceYAML, "http://victoriametrics:8428") {
		t.Fatalf("grafana datasource missing victoria url\n%s", datasourceYAML)
	}

	dashboardsYAML := readRenderedFile(t, projectDir, "containers/observability/grafana/provisioning/dashboards/dashboards.yml")
	if !strings.Contains(dashboardsYAML, "path: /etc/grafana/dashboards") {
		t.Fatalf("grafana dashboard provisioning path should use read-only config mount\n%s", dashboardsYAML)
	}

	grafanaSeedScript := readRenderedFile(t, projectDir, "containers/observability/grafana/seed-dashboards.sh")
	for _, token := range []string{
		`curl -sS -u "${auth}" -o /dev/null -w "%{http_code}" "${grafana_url}/api/user"`,
		`[ "${status}" = "200" ]`,
		"dashboard_is_starred",
		"-X PUT",
		"-X POST",
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
	for _, token := range []string{
		"/api/health",
		"/api/datasources/uid/goforj-victoriametrics",
		"/api/dashboards/db",
		"/dashboards/*.json",
		`"overwrite":true`,
		"|| true",
	} {
		if strings.Contains(grafanaSeedScript, token) {
			t.Fatalf("grafana seed script should not contain %q\n%s", token, grafanaSeedScript)
		}
	}

	for _, dashboard := range []string{
		"platform-overview.json",
		"lighthouse-inspects-overview.json",
		"cache-overview.json",
		"storage-overview.json",
		"events-overview.json",
		"http-overview.json",
		"auth-overview.json",
		"mail-overview.json",
		"database-overview.json",
		"queue-overview.json",
		"scheduler-overview.json",
	} {
		body := readRenderedFile(t, projectDir, filepath.Join("containers/observability/grafana/dashboards", dashboard))
		var decoded any
		if err := json.Unmarshal([]byte(body), &decoded); err != nil {
			t.Fatalf("rendered grafana dashboard %s is invalid JSON: %v\n%s", dashboard, err, body)
		}
	}

	httpDashboardJSON := readRenderedFile(t, projectDir, "containers/observability/grafana/dashboards/http-overview.json")
	for _, token := range []string{
		"HTTP Overview",
		"http_requests_by_route_total",
		"http_request_duration_by_route_seconds_bucket",
		`label_values(http_requests_by_route_total{app=~\"$app\"}, route)`,
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
		`label_values(database_queries_by_fingerprint_total{app=~\"$app\"}, fingerprint)`,
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

	schedulerRegistryGo := readRenderedFile(t, projectDir, "internal/schedules/registration.go")
	if !strings.Contains(schedulerRegistryGo, `s.metrics.RecordSchedulerJob(ctx, event)`) {
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
		!strings.Contains(observabilityReadme, "http://localhost:13001") ||
		!strings.Contains(observabilityReadme, "grafana-seed") {
		t.Fatalf("observability readme missing local URLs\n%s", observabilityReadme)
	}
}

func TestRenderedObservabilityComposeStartsAndSeedsGrafana(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is required for rendered observability compose smoke")
	}
	if out, err := exec.Command("docker", "compose", "version").CombinedOutput(); err != nil {
		t.Skipf("docker compose is required for rendered observability compose smoke: %v\n%s", err, string(out))
	}

	projectDir := t.TempDir()
	testkit.RenderProjectWithForj(t, projectDir, testkit.RenderProjectRequest{
		Config: project.Config{
			ProjectName:  "Observability Compose Smoke",
			GoModuleName: "example.com/observabilitycomposesmoke",
			UpdatedAt:    "2026-06-19 00:00:00 UTC",
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
			},
		},
	})

	t.Setenv("APP_NAME", "Observability Compose Smoke")
	t.Setenv("APP_ENV", "local")
	t.Setenv("OBSERVABILITY_METRICS_TARGET_HOST", "host.docker.internal")
	if _, err := generate.GenerateObservabilityFiles(projectDir); err != nil {
		t.Fatalf("generate observability files: %v", err)
	}

	vmPort := freeTCPPort(t)
	grafanaPort := freeTCPPort(t)
	mailpitSMTPPort := freeTCPPort(t)
	mailpitHTTPPort := freeTCPPort(t)
	databasePort := freeTCPPort(t)
	redisPort := freeTCPPort(t)
	portOverrides := map[string]string{
		"OBSERVABILITY_VM_PORT": vmPort,
		"GRAFANA_PORT":          grafanaPort,
		"MAILPIT_SMTP_PORT":     mailpitSMTPPort,
		"MAILPIT_HTTP_PORT":     mailpitHTTPPort,
		"DB_MYSQL_PORT":         databasePort,
		"REDIS_PORT":            redisPort,
	}
	if err := testkit.ReplaceOrAppendEnvValues([]string{filepath.Join(projectDir, ".env")}, portOverrides); err != nil {
		t.Fatalf("set observability smoke ports: %v", err)
	}
	for key, value := range portOverrides {
		t.Setenv(key, value)
	}

	projectName := fmt.Sprintf("goforj-observability-%d", time.Now().UnixNano())
	defer runDockerComposeBestEffort(t, projectDir, projectName, "down", "-v", "--remove-orphans")

	runDockerCompose(t, projectDir, projectName, 3*time.Minute, "up", "-d")

	assertGrafanaComposeAPIStatus(t, projectDir, projectName, "/api/datasources/uid/goforj-victoriametrics", http.StatusOK)
	dashboardUIDs := []string{
		"goforj-platform-overview",
		"goforj-http-overview",
		"goforj-database-overview",
		"goforj-queue-overview",
		"goforj-scheduler-overview",
	}
	for _, uid := range dashboardUIDs {
		assertGrafanaComposeAPIStatus(t, projectDir, projectName, "/api/dashboards/uid/"+uid, http.StatusOK)
		assertGrafanaDashboardStarred(t, projectDir, projectName, uid)
	}
	assertGrafanaHomeDashboard(t, projectDir, projectName, "goforj-platform-overview")
}

func TestRenderedObservabilityTargetsIncludeConventionalApps(t *testing.T) {
	projectDir := t.TempDir()
	for _, target := range []string{"billing", "customer-portal"} {
		if err := writeConventionalAppMarker(projectDir, target); err != nil {
			t.Fatalf("write %s target marker: %v", target, err)
		}
	}

	testkit.RenderProjectWithForj(t, projectDir, testkit.RenderProjectRequest{
		Config: project.Config{
			ProjectName:  "Observability Target Test",
			GoModuleName: "example.com/observabilitytargettest",
			UpdatedAt:    "2026-06-07 00:00:00 UTC",
			Render: project.RenderConfig{
				Components: project.Components{
					CLI:           true,
					WebAPI:        true,
					Metrics:       true,
					Docker:        true,
					Scheduler:     true,
					Jobs:          true,
					Observability: true,
				},
			},
		},
	})
	if err := testkit.ReplaceOrAppendEnvValues([]string{filepath.Join(projectDir, ".env")}, map[string]string{
		"QUEUE_DRIVER":            "workerpool",
		"QUEUE_SUPPORTED_DRIVERS": "workerpool",
	}); err != nil {
		t.Fatalf("set observability target queue driver: %v", err)
	}

	targets := readRenderedMetricsTargets(t, projectDir)
	want := []renderedMetricsTarget{
		{App: "app", Process: "app", Target: "host.docker.internal:3000"},
		{App: "billing", Process: "app", Target: "host.docker.internal:3001"},
		{App: "customer-portal", Process: "app", Target: "host.docker.internal:3002"},
	}
	assertRenderedMetricsTargets(t, targets, "Observability Target Test", "local", want)

	t.Setenv("APP_NAME", "Observability Target Test")
	t.Setenv("APP_ENV", "local")
	t.Setenv("OBSERVABILITY_METRICS_TARGET_MODE", "local-multi")
	t.Setenv("OBSERVABILITY_METRICS_TARGET_HOST", "host.docker.internal")
	if _, err := generate.GenerateObservabilityFiles(projectDir); err != nil {
		t.Fatalf("generate local-multi observability targets: %v", err)
	}

	targets = readRenderedMetricsTargets(t, projectDir)
	want = []renderedMetricsTarget{
		{App: "app", Process: "api", Target: "host.docker.internal:10000"},
		{App: "app", Process: "jobs", Target: "host.docker.internal:10002"},
		{App: "app", Process: "scheduler", Target: "host.docker.internal:10001"},
		{App: "billing", Process: "api", Target: "host.docker.internal:10010"},
		{App: "billing", Process: "jobs", Target: "host.docker.internal:10012"},
		{App: "billing", Process: "scheduler", Target: "host.docker.internal:10011"},
		{App: "customer-portal", Process: "api", Target: "host.docker.internal:10020"},
		{App: "customer-portal", Process: "jobs", Target: "host.docker.internal:10022"},
		{App: "customer-portal", Process: "scheduler", Target: "host.docker.internal:10021"},
	}
	assertRenderedMetricsTargets(t, targets, "Observability Target Test", "local", want)
}

func freeTCPPort(t *testing.T) string {
	t.Helper()
	addr := findFreeAddr(t)
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split free addr %q: %v", addr, err)
	}
	return port
}

func runDockerCompose(t *testing.T, projectDir string, projectName string, timeout time.Duration, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	fullArgs := append([]string{"compose", "-p", projectName}, args...)
	cmd := exec.CommandContext(ctx, "docker", fullArgs...)
	cmd.Dir = projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker %s failed: %v\n%s", strings.Join(fullArgs, " "), err, string(output))
	}
}

func runDockerComposeBestEffort(t *testing.T, projectDir string, projectName string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fullArgs := append([]string{"compose", "-p", projectName}, args...)
	cmd := exec.CommandContext(ctx, "docker", fullArgs...)
	cmd.Dir = projectDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Logf("docker %s cleanup failed: %v\n%s", strings.Join(fullArgs, " "), err, string(output))
	}
}

func dockerComposeOutput(t *testing.T, projectDir string, projectName string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fullArgs := append([]string{"compose", "-p", projectName}, args...)
	cmd := exec.CommandContext(ctx, "docker", fullArgs...)
	cmd.Dir = projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("docker %s failed: %v\n%s", strings.Join(fullArgs, " "), err, string(output))
	}
	return string(output)
}

func assertGrafanaComposeAPIStatus(t *testing.T, projectDir string, projectName string, path string, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	var output string
	for time.Now().Before(deadline) {
		output = dockerComposeOutput(t, projectDir, projectName,
			"run", "--rm", "--no-deps",
			"--entrypoint", "curl",
			"grafana-seed",
			"-sS",
			"-u", "admin:admin",
			"-o", "/dev/null",
			"-w", "%{http_code}",
			"http://grafana:3000"+path,
		)
		got, err := strconv.Atoi(strings.TrimSpace(output))
		if err == nil && got == want {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("GET grafana %s did not reach status %d\nlast response: %s\nlogs:\n%s", path, want, output, dockerComposeOutput(t, projectDir, projectName, "logs", "grafana", "grafana-seed"))
}

// assertGrafanaDashboardStarred waits because detached Compose startup does not wait for the one-shot seeder to finish.
func assertGrafanaDashboardStarred(t *testing.T, projectDir string, projectName string, uid string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	var output string
	for time.Now().Before(deadline) {
		output = dockerComposeOutput(t, projectDir, projectName,
			"run", "--rm", "--no-deps",
			"--entrypoint", "curl",
			"grafana-seed",
			"-fsS",
			"-u", "admin:admin",
			"http://grafana:3000/api/dashboards/uid/"+uid,
		)
		var dashboard struct {
			Meta struct {
				IsStarred bool `json:"isStarred"`
			} `json:"meta"`
		}
		if err := json.Unmarshal([]byte(output), &dashboard); err == nil && dashboard.Meta.IsStarred {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("Grafana dashboard %s was not starred by the seed container\nlast response: %s\nlogs:\n%s", uid, output, dockerComposeOutput(t, projectDir, projectName, "logs", "grafana-seed"))
}

// assertGrafanaHomeDashboard waits because the organization preference is written after every dashboard is starred.
func assertGrafanaHomeDashboard(t *testing.T, projectDir string, projectName string, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	var output string
	for time.Now().Before(deadline) {
		output = dockerComposeOutput(t, projectDir, projectName,
			"run", "--rm", "--no-deps",
			"--entrypoint", "curl",
			"grafana-seed",
			"-fsS",
			"-u", "admin:admin",
			"http://grafana:3000/api/org/preferences",
		)
		var preferences struct {
			HomeDashboardUID string `json:"homeDashboardUID"`
		}
		if err := json.Unmarshal([]byte(output), &preferences); err == nil && preferences.HomeDashboardUID == want {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("Grafana home dashboard UID did not become %q\nlast response: %s\nlogs:\n%s", want, output, dockerComposeOutput(t, projectDir, projectName, "logs", "grafana-seed"))
}

func readRenderedFile(t *testing.T, root string, rel string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read rendered file %s: %v", rel, err)
	}
	return string(body)
}

type renderedMetricsTarget struct {
	App     string
	Process string
	Target  string
}

type renderedMetricsTargetEntry struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels"`
}

func readRenderedMetricsTargets(t *testing.T, root string) []renderedMetricsTargetEntry {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, "containers", "observability", "vmagent", "metrics-targets.json"))
	if err != nil {
		t.Fatalf("read rendered metrics targets: %v", err)
	}
	var entries []renderedMetricsTargetEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		t.Fatalf("unmarshal rendered metrics targets: %v\n%s", err, body)
	}
	return entries
}

func assertRenderedMetricsTargets(t *testing.T, entries []renderedMetricsTargetEntry, service string, environment string, want []renderedMetricsTarget) {
	t.Helper()
	if len(entries) != len(want) {
		t.Fatalf("metrics target count = %d, want %d\n%#v", len(entries), len(want), entries)
	}
	for i, entry := range entries {
		if len(entry.Targets) != 1 || entry.Targets[0] != want[i].Target {
			t.Fatalf("metrics target[%d] = %#v, want %q", i, entry.Targets, want[i].Target)
		}
		if entry.Labels["app"] != want[i].App {
			t.Fatalf("metrics target[%d] app = %q, want %q", i, entry.Labels["app"], want[i].App)
		}
		if entry.Labels["process"] != want[i].Process {
			t.Fatalf("metrics target[%d] process = %q, want %q", i, entry.Labels["process"], want[i].Process)
		}
		if entry.Labels["service"] != service {
			t.Fatalf("metrics target[%d] service = %q, want %q", i, entry.Labels["service"], service)
		}
		if entry.Labels["environment"] != environment {
			t.Fatalf("metrics target[%d] environment = %q, want %q", i, entry.Labels["environment"], environment)
		}
	}
}
