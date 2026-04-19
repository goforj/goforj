package forj

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/goforj/crypt"
	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/coredeps"
	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
	"github.com/goforj/goforj/templates"
	"go/format"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"
)

var (
	markStep     = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Render("▸")
	markCreate   = lipgloss.NewStyle().Foreground(lipgloss.Color("84")).Render("✔")
	markSkip     = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("·")
	markAction   = lipgloss.NewStyle().Foreground(lipgloss.Color("45")).Render("›")
	headerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
	summaryStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("84")).Bold(true)
	nextStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	bulletStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Render("·")
	commandStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
	boxBorder    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	wireInstallOnce sync.Once
	wireInstallErr  error
	wireBinaryPath  string
)

var templatesFS = templates.FS

// Components represents the components of the project
type ComponentRenderInput struct {
	components project.Components
	renderAll  bool
}

// ProjectRenderer is well, a project renderer :)
type ProjectRenderer struct {
	logger *logger.AppLogger
	config *project.Config
	stats  *renderStats
	lines  []string
}

type renderStats struct {
	created []string
	skipped []string
}

func (s *renderStats) recordCreated(path string) {
	if path == "" {
		return
	}
	s.created = append(s.created, path)
}

func (s *renderStats) recordSkipped(path string) {
	if path == "" {
		return
	}
	s.skipped = append(s.skipped, path)
}

type renderCounts struct {
	created int
	skipped int
}

type templateRenderConfig struct {
	*project.Config
	Components project.Components
}

type templateMapping struct {
	tmpl string
	dest string
}

func mapTemplate(tmpl string) templateMapping {
	return templateMapping{
		tmpl: tmpl,
		dest: strings.TrimSuffix(tmpl, ".tmpl"),
	}
}

func mapTemplateTo(tmpl, dest string) templateMapping {
	return templateMapping{
		tmpl: tmpl,
		dest: dest,
	}
}

func (s *renderStats) counts() renderCounts {
	return renderCounts{
		created: len(s.created),
		skipped: len(s.skipped),
	}
}

func renderCountsLine(title string, created, skipped int, unit string) string {
	label := fmt.Sprintf("%-32s", title)
	line := fmt.Sprintf("%s %s %s %d", markStep, label, markCreate, created)
	if unit != "" {
		line += " " + unit
	}
	if skipped > 0 {
		line += fmt.Sprintf(" %s %d skipped", markSkip, skipped)
	}
	return line
}

func maybeFormatGoSource(destPath string, content []byte) ([]byte, error) {
	if !strings.HasSuffix(destPath, ".go") {
		return content, nil
	}
	formatted, err := format.Source(content)
	if err != nil {
		return nil, fmt.Errorf("gofmt %s: %w", destPath, err)
	}
	return formatted, nil
}

func generateLighthouseToken() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	return generateRandomToken(charset, 20)
}

func generateJWTSecretKey() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	return generateRandomToken(charset, 48)
}

func generateAppDiagToken() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	return generateRandomToken(charset, 32)
}

func generateRandomToken(charset string, length int) (string, error) {
	if length <= 0 {
		return "", nil
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", err
	}
	for i := 0; i < length; i++ {
		buf[i] = charset[int(buf[i])%len(charset)]
	}
	return string(buf), nil
}

// NewProjectRenderer creates a new ProjectRenderer instance
func NewProjectRenderer(logger *logger.AppLogger) *ProjectRenderer {
	return &ProjectRenderer{logger: logger, stats: &renderStats{}}
}

// Render is the main rendering function
func (p *ProjectRenderer) Render(input ComponentRenderInput) error {
	p.stats = &renderStats{}
	p.lines = nil

	if input.renderAll {
		cfg, err := project.LoadProjectConfig()
		if err != nil {
			return err
		}
		p.config = cfg
	} else {
		p.config = &project.Config{
			Render: project.RenderConfig{Components: input.components},
		}
	}
	if p.config.Render.Components.DemoApp {
		p.config.Render.Components.Auth = true
	}
	if err := p.config.Render.Components.ValidateRenderContract(); err != nil {
		return err
	}

	steps := []struct {
		title               string
		enabled             bool
		templates           []string
		renderOnceTemplates []string
		raw                 []string
		action              func() error
	}{
		{
			title:   "Go Module Initialization",
			enabled: input.renderAll,
			action:  p.createGoMod,
		},
		{
			title:     "Main File Rendering",
			enabled:   input.renderAll,
			templates: []string{"main.go.tmpl"},
		},
		{
			title:   "Environment Files Initialization",
			enabled: input.renderAll,
			action: func() error {
				envTemplates := []string{
					".env.tmpl",
					".env.host.tmpl",
				}
				ensureEnvDefaults := func(path string, allowAppKey bool) error {
					content, err := os.ReadFile(path)
					if err != nil {
						return err
					}
					text := string(content)
					needsURL := path == ".env" && !strings.Contains(text, "LIGHTHOUSE_URL=")
					needsAppDiagToken := path == ".env" && !strings.Contains(text, "APP_DIAG_TOKEN=")
					needsToken := path == ".env" && !strings.Contains(text, "LIGHTHOUSE_TOKEN=")
					needsEnabled := path == ".env" && !strings.Contains(text, "LIGHTHOUSE_ENABLED=")
					needsSwagger := path == ".env" && !strings.Contains(text, "SWAGGER_ENABLED=")
					needsKey := allowAppKey && !strings.Contains(text, "APP_KEY=")
					needsJWTSecret := false

					appKey := ""
					appDiagToken := ""
					tokenValue := ""
					jwtSecret := ""
					jwtLineIdx := -1
					lines := strings.Split(text, "\n")
					for idx, line := range lines {
						trimmed := strings.TrimSpace(line)
						if trimmed == "" || strings.HasPrefix(trimmed, "#") {
							continue
						}
						if strings.HasPrefix(trimmed, "APP_KEY=") {
							appKey = strings.TrimSpace(strings.TrimPrefix(trimmed, "APP_KEY="))
							continue
						}
						if strings.HasPrefix(trimmed, "LIGHTHOUSE_TOKEN=") {
							tokenValue = strings.TrimSpace(strings.TrimPrefix(trimmed, "LIGHTHOUSE_TOKEN="))
							continue
						}
						if strings.HasPrefix(trimmed, "APP_DIAG_TOKEN=") {
							appDiagToken = strings.TrimSpace(strings.TrimPrefix(trimmed, "APP_DIAG_TOKEN="))
							continue
						}
						if strings.HasPrefix(trimmed, "API_JWT_SECRET_KEY=") {
							jwtSecret = strings.TrimSpace(strings.TrimPrefix(trimmed, "API_JWT_SECRET_KEY="))
							jwtLineIdx = idx
							continue
						}
					}
					if path == ".env" && (jwtSecret == "" || jwtSecret == "xxx") {
						needsJWTSecret = true
					}
					if !(needsURL || needsAppDiagToken || needsToken || needsEnabled || needsSwagger || needsKey || needsJWTSecret) {
						return nil
					}
					if needsAppDiagToken && appDiagToken == "" {
						value, err := generateAppDiagToken()
						if err != nil {
							return fmt.Errorf("failed to generate app diagnostics token: %w", err)
						}
						appDiagToken = value
					}
					if needsToken && tokenValue == "" {
						value, err := generateLighthouseToken()
						if err != nil {
							return fmt.Errorf("failed to generate lighthouse token: %w", err)
						}
						tokenValue = value
					}
					if needsJWTSecret {
						value, err := generateJWTSecretKey()
						if err != nil {
							return fmt.Errorf("failed to generate JWT secret: %w", err)
						}
						jwtSecret = value
					}
					if needsKey && appKey == "" {
						key, err := crypt.GenerateAppKey()
						if err != nil {
							return fmt.Errorf("failed to generate app key: %w", err)
						}
						appKey = key
					}
					writeLines := make([]string, 0)
					if needsKey && appKey != "" {
						writeLines = append(writeLines, fmt.Sprintf("APP_KEY=%s", appKey))
					}
					if needsAppDiagToken && appDiagToken != "" {
						writeLines = append(writeLines, fmt.Sprintf("APP_DIAG_TOKEN=%s", appDiagToken))
					}
					if needsURL {
						writeLines = append(writeLines, "LIGHTHOUSE_URL=ws://localhost:3000/lighthouse/ws/agent")
					}
					if needsToken {
						writeLines = append(writeLines, fmt.Sprintf("LIGHTHOUSE_TOKEN=%s", tokenValue))
					}
					if needsEnabled {
						writeLines = append(writeLines, "LIGHTHOUSE_ENABLED=true")
					}
					if needsSwagger {
						writeLines = append(writeLines, "SWAGGER_ENABLED=true")
					}
					if needsJWTSecret && jwtSecret != "" {
						if jwtLineIdx >= 0 && jwtLineIdx < len(lines) {
							lines[jwtLineIdx] = fmt.Sprintf("API_JWT_SECRET_KEY=%s", jwtSecret)
						} else {
							writeLines = append(writeLines, fmt.Sprintf("API_JWT_SECRET_KEY=%s", jwtSecret))
						}
					}
					if len(writeLines) > 0 {
						lines = append(lines, writeLines...)
					}
					updated := strings.Join(lines, "\n")
					if !strings.HasSuffix(updated, "\n") {
						updated += "\n"
					}
					if updated == text {
						return nil
					}
					if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
						return err
					}
					return nil
				}
				for _, tmpl := range envTemplates {
					name := strings.TrimSuffix(strings.TrimPrefix(tmpl, ""), ".tmpl")
					if _, err := os.Stat(name); err == nil {
						allowAppKey := name == ".env"
						if err := ensureEnvDefaults(name, allowAppKey); err != nil {
							return err
						}
						//fmt.Printf("  %s already exists [%v]\n", markSkip, name)
						continue
					}
					key, err := crypt.GenerateAppKey()
					if err != nil {
						return fmt.Errorf("failed to generate app key: %w", err)
					}
					token, err := generateLighthouseToken()
					if err != nil {
						return fmt.Errorf("failed to generate lighthouse token: %w", err)
					}
					appDiagToken, err := generateAppDiagToken()
					if err != nil {
						return fmt.Errorf("failed to generate app diagnostics token: %w", err)
					}
					jwtSecret, err := generateJWTSecretKey()
					if err != nil {
						return fmt.Errorf("failed to generate JWT secret: %w", err)
					}
					p.config.AppKey = key
					p.config.AppDiagToken = appDiagToken
					p.config.LighthouseToken = token
					p.config.JWTSecretKey = jwtSecret
					return p.writeTemplates(envTemplates)
				}
				return nil
			},
		},
		{
			title:   "Bin Directory Initialization",
			enabled: input.renderAll,
			action: func() error {
				if err := os.MkdirAll("bin", 0755); err != nil {
					return err
				}
				p.stats.recordCreated("bin/")
				return nil
			},
		},
		{
			title:   "Core Components Rendering",
			enabled: input.renderAll,
			templates: []string{
				"internal/events/event.go.tmpl",
				"internal/events/topics.go.tmpl",
				"internal/events/bus_transport.go.tmpl",
				"internal/events/make_event_cmd.go.tmpl",
				"internal/events/make_event_cmd_test.go.tmpl",
				"internal/events/bus_integration_test.go.tmpl",
				"internal/events/README.md.tmpl",
				"internal/app/lifecycle.go.tmpl",
				"internal/app/lifecycle_test.go.tmpl",
				"internal/app/timeouts.go.tmpl",
				"internal/app/README.md.tmpl",
				"internal/caches/README.md.tmpl",
				"internal/storages/README.md.tmpl",
				"internal/console/console.go.tmpl",
				"internal/app/about.go.tmpl",
				"internal/app/discovery.go.tmpl",
				"internal/cmd/about_cmd.go.tmpl",
				"internal/cmd/about_grid.go.tmpl",
				"internal/cmd/hello_world_cmd.go.tmpl",
				"internal/cmd/test_event_pipeline_cmd.go.tmpl",
				"internal/cmd/monitor_seed_cmd.go.tmpl",
				"internal/cmd/monitor_reset_cmd.go.tmpl",
				"internal/cmd/monitor_retention_cmd.go.tmpl",
				"internal/cmd/monitor_poll_cmd.go.tmpl",
				"internal/cmd/push_monitor_trigger_cmd.go.tmpl",
				"internal/cmd/test_monitor_poll_loop_cmd.go.tmpl",
				"internal/cmd/kong_help_formatter.go.tmpl",
				"internal/cmd/skip_boot.go.tmpl",
				"internal/cmd/run_cmd.go.tmpl",
				"internal/cmd/root_cmd.go.tmpl",
				"internal/logger/app.go.tmpl",
				"internal/logger/app_test.go.tmpl",
				"internal/logger/dedupe.go.tmpl",
				"internal/logger/dedupe_test.go.tmpl",
				"internal/logger/wire.go.tmpl",
				"internal/lighthouse/project_config.go.tmpl",
				"wire/app.go.tmpl",
				"wire/app_test.go.tmpl",
				"wire/inject_cache.go.tmpl",
				"wire/inject_app_services.go.tmpl",
				"wire/inject_cmd.go.tmpl",
				"wire/inject_storage.go.tmpl",
				"wire/wire.go.tmpl",
			},
			renderOnceTemplates: []string{
				".gitignore.tmpl",
				".db-relationships.yaml.tmpl",
				"internal/cmd/app_commands.go.tmpl",
				"internal/cmd/wire.go.tmpl",
				"internal/app/lifecycle_registry.go.tmpl",
			},
			raw: []string{
				"internal/events/event.tmpl",
			},
		},
		{
			title:   "Legacy File Cleanup",
			enabled: input.renderAll,
			action:  p.cleanupLegacyGeneratedFiles,
		},
		{
			title:   "Docker Components Rendering",
			enabled: p.config.Render.Components.Docker,
			templates: append([]string{"docker-compose.yml.tmpl"},
				func() []string {
					if p.config.Render.Components.DatabaseMySQL {
						return []string{
							"containers/mariadb/Dockerfile",
							"containers/mariadb/my.cnf",
						}
					}
					return nil
				}()...,
			),
		},
		{
			title:   "Dev Console Components Rendering",
			enabled: p.config.Render.Components.WebAPI || p.config.Render.Components.WebUI || p.config.Render.Components.Scheduler || p.config.Render.Components.Jobs,
			templates: []string{
				"internal/lighthouse/agent.go.tmpl",
				"internal/lighthouse/cli.go.tmpl",
				"internal/lighthouse/conn.go.tmpl",
				"internal/lighthouse/enable.go.tmpl",
				"internal/lighthouse/hub.go.tmpl",
				"internal/lighthouse/log_hook.go.tmpl",
				"internal/lighthouse/protocol.go.tmpl",
				"internal/lighthouse/server.go.tmpl",
				"internal/lighthouse/editor.go.tmpl",
				"internal/lighthouse/ui.go.tmpl",
			},
			raw: []string{
				"internal/lighthouse/ui/dist",
			},
		},
		{
			title:   "Web API Components Rendering",
			enabled: p.config.Render.Components.WebAPI || p.config.Render.Components.WebUI,
			templates: []string{
				"wire/inject_http.go.tmpl",
				"internal/http/lighthouse.go.tmpl",
				"internal/http/README.md.tmpl",
				"internal/http/cors.go.tmpl",
				"internal/http/routes_list_cmd.go.tmpl",
				"internal/http/health.go.tmpl",
				"internal/http/health_test.go.tmpl",
				"internal/http/swagger.go.tmpl",
				"internal/http/swagger_test.go.tmpl",
				"internal/http/readiness_checks.go.tmpl",
				"internal/http/serve_cmd.go.tmpl",
				"internal/http/server.go.tmpl",
				"internal/http/spa.go.tmpl",
				"internal/http/types.go.tmpl",
				"internal/hello/controller.go.tmpl",
			},
			renderOnceTemplates: []string{
				"internal/router/routes_registry.go.tmpl",
				"wire/inject_http_controllers.go.tmpl",
			},
		},
		{
			title:     "Web UI Components Rendering",
			enabled:   p.config.Render.Components.WebUI,
			templates: []string{},
			renderOnceTemplates: []string{
				"frontend/dist/index.html.tmpl",
			},
		},
		{
			title:   "Auth Components Rendering",
			enabled: p.config.Render.Components.Auth && p.config.Render.Components.HasDatabase(),
			templates: []string{
				"internal/auth/controller.go.tmpl",
				"internal/auth/delivery.go.tmpl",
				"internal/auth/email_verification.go.tmpl",
				"internal/auth/login_attempt.go.tmpl",
				"internal/auth/password_reset.go.tmpl",
				"internal/auth/service.go.tmpl",
				"internal/auth/session.go.tmpl",
				"internal/auth/user.go.tmpl",
				"wire/inject_auth.go.tmpl",
			},
			action: func() error {
				if err := p.writeTemplateMappings([]templateMapping{
					mapTemplate("internal/router/routes_registry.go.tmpl"),
					mapTemplate("wire/inject_http_controllers.go.tmpl"),
				}); err != nil {
					return err
				}
				if err := p.writeTemplateMappingsOnce([]templateMapping{
					mapTemplate("migrations/2026_04_09_000001_auth_users.mysql.up.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000001_auth_users.mysql.down.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000001_auth_users.postgres.up.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000001_auth_users.postgres.down.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000001_auth_users.sqlite.up.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000001_auth_users.sqlite.down.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000002_auth_sessions.mysql.up.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000002_auth_sessions.mysql.down.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000002_auth_sessions.postgres.up.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000002_auth_sessions.postgres.down.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000002_auth_sessions.sqlite.up.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000002_auth_sessions.sqlite.down.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000003_auth_password_resets.mysql.up.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000003_auth_password_resets.mysql.down.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000003_auth_password_resets.postgres.up.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000003_auth_password_resets.postgres.down.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000003_auth_password_resets.sqlite.up.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000003_auth_password_resets.sqlite.down.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000004_auth_login_attempts.mysql.up.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000004_auth_login_attempts.mysql.down.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000004_auth_login_attempts.postgres.up.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000004_auth_login_attempts.postgres.down.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000004_auth_login_attempts.sqlite.up.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000004_auth_login_attempts.sqlite.down.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000005_auth_email_verifications.mysql.up.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000005_auth_email_verifications.mysql.down.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000005_auth_email_verifications.postgres.up.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000005_auth_email_verifications.postgres.down.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000005_auth_email_verifications.sqlite.up.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000005_auth_email_verifications.sqlite.down.sql.tmpl"),
				}); err != nil {
					return err
				}
				return nil
			},
		},
		{
			title:   "Demo App Components Rendering",
			enabled: input.renderAll && p.config.Render.Components.DemoApp,
			action: func() error {
				if !p.config.Render.Components.HasDatabase() {
					return nil
				}

				includeDemoInternal := func(tmpl string) bool {
					if p.config.Render.Components.Jobs {
						return !strings.HasPrefix(filepath.ToSlash(tmpl), "demo/internal/migrations/")
					}
					s := filepath.ToSlash(tmpl)
					if strings.HasPrefix(s, "demo/internal/migrations/") {
						return false
					}
					if strings.HasPrefix(s, "demo/internal/alerts/") {
						return false
					}
					if strings.HasSuffix(s, "demo/internal/monitoring/check_service.go.tmpl") {
						return false
					}
					if strings.HasSuffix(s, "demo/internal/monitoring/monitor_check_job.go.tmpl") {
						return false
					}
					return true
				}
				if err := p.writeTemplatesUnder("demo/internal", "internal", includeDemoInternal); err != nil {
					return err
				}
				if err := p.writeTemplatesUnder("demo/internal/migrations", "migrations", nil); err != nil {
					return err
				}

				// Demo app evolves routing/controller wiring; force refresh on render.
				if err := p.writeTemplateMappings([]templateMapping{
					mapTemplate("internal/router/routes_registry.go.tmpl"),
					mapTemplate("wire/inject_http_controllers.go.tmpl"),
					mapTemplate("internal/cmd/app_commands.go.tmpl"),
					mapTemplate("internal/cmd/wire.go.tmpl"),
				}); err != nil {
					return err
				}
				return nil
			},
		},
		{
			title:   "Demo App Frontend Scaffolding",
			enabled: input.renderAll && p.config.Render.Components.DemoApp && p.config.Render.Components.WebUI,
			action:  p.scaffoldDemoFrontend,
		},
		{
			title:   "Database Components Rendering",
			enabled: p.config.Render.Components.HasDatabase(),
			templates: []string{
				"wire/inject_db.go.tmpl",
				"wire/inject_repositories.go.tmpl",
				"internal/database/connections.go.tmpl",
				"internal/database/gorm_log_writer.go.tmpl",
				"internal/database/connections_test.go.tmpl",
				"internal/modelgen/make_model_cmd.go.tmpl",
				"internal/modelgen/make_model_mysql_integration_test.go.tmpl",
				"internal/modelgen/make_model_postgres_integration_test.go.tmpl",
				"internal/modelgen/make_model_sqlite_integration_test.go.tmpl",
				"internal/modelgen/repository_wire_test.go.tmpl",
			},
			raw: []string{"internal/modelgen/model.tmpl"},
			action: func() error {
				if err := p.writeTemplateMappings([]templateMapping{
					mapTemplate("migrations/migrations.go.tmpl"),
					mapTemplate("migrations/migrations_test.go.tmpl"),
					mapTemplate("migrations/migration_connection_test.go.tmpl"),
					mapTemplate("migrations/migration_commands_test.go.tmpl"),
					mapTemplate("migrations/migrate_cmd.go.tmpl"),
					mapTemplate("migrations/migrate_rollback_cmd.go.tmpl"),
					mapTemplate("migrations/.goforj/placeholder.txt.tmpl"),
				}); err != nil {
					return err
				}
				return nil
			},
		},
		{
			title:   "Scheduler Components Rendering",
			enabled: p.config.Render.Components.Scheduler,
			templates: []string{
				"internal/scheduler/lighthouse.go.tmpl",
				"internal/scheduler/scheduler.go.tmpl",
				"internal/scheduler/cmd.go.tmpl",
				"wire/inject_scheduler.go.tmpl",
			},
			renderOnceTemplates: []string{
				"internal/scheduler/scheduler_registry.go.tmpl",
			},
		},
		{
			title:   "Job Components Rendering",
			enabled: p.config.Render.Components.Jobs,
			templates: append([]string{
				"internal/queues/README.md.tmpl",
				"internal/jobs/example_hello_job.go.tmpl",
				"internal/jobs/example_hello_job_cmd.go.tmpl",
				"internal/jobs/benchmark_run_cmd.go.tmpl",
				"internal/jobs/benchmark_system.go.tmpl",
				"internal/jobs/make_job_cmd.go.tmpl",
				"internal/jobs/lighthouse.go.tmpl",
				"internal/jobs/lighthouse_benchmark.go.tmpl",
				"internal/jobs/lighthouse_queue.go.tmpl",
				"internal/jobs/worker.go.tmpl",
				"internal/jobs/worker_logger.go.tmpl",
				"internal/jobs/worker_cmd.go.tmpl",
				"wire/inject_queue.go.tmpl",
				"wire/inject_jobs.go.tmpl",
				"wire/inject_jobs_app.go.tmpl",
			}, func() []string {
				if !p.config.Render.Components.StressTest {
					return nil
				}
				return []string{
					"internal/jobs/stress_job.go.tmpl",
					"internal/cmd/queue_stress_tick_cmd.go.tmpl",
				}
			}()...),
			raw: []string{"internal/jobs/job.tmpl"},
		},
	}

	for _, step := range steps {
		if !step.enabled {
			continue
		}

		before := p.stats.counts()

		if len(step.templates) > 0 {
			if err := p.writeTemplates(step.templates); err != nil {
				return err
			}
		}

		if len(step.renderOnceTemplates) > 0 {
			if err := p.writeTemplatesOnce(step.renderOnceTemplates); err != nil {
				return err
			}
		}

		if len(step.raw) > 0 {
			if err := p.writeRawFiles(step.raw); err != nil {
				return err
			}
		}
		if step.action != nil {
			if err := step.action(); err != nil {
				return err
			}
		}

		p.printStepSummary(step.title, before)
	}

	// Run go mod tidy to ensure all dependencies are downloaded
	if err := p.timeRenderStage("applyModuleReplaces", p.applyModuleReplaces); err != nil {
		return fmt.Errorf("apply module replaces: %w", err)
	}

	// Sync core libraries so generated templates and module APIs stay aligned.
	if err := p.timeRenderStage("syncCoreLibraries", p.syncCoreLibraries); err != nil {
		return fmt.Errorf("sync core libraries: %w", err)
	}

	if input.renderAll {
		if err := p.timeRenderStage("generateProjectFiles", p.runGenerateAll); err != nil {
			return fmt.Errorf("generate: %w", err)
		}
	}

	// Run go mod tidy to ensure all dependencies are downloaded
	if err := p.timeRenderStage("goModTidy", p.goModTidy); err != nil {
		return fmt.Errorf("go mod tidy: %w", err)
	}

	// Run wire install + generate to make main runnable immediately.
	if err := p.timeRenderStage("runWireGenerate", p.runWireGenerate); err != nil {
		return fmt.Errorf("wire generate: %w", err)
	}

	p.printRenderDetails()
	p.printOverallSummary()

	return nil
}

func (p *ProjectRenderer) timeRenderStage(name string, fn func() error) error {
	if !renderStageTimingEnabled() {
		return fn()
	}
	started := time.Now()
	err := fn()
	console.Debugf("render stage %s: %s", name, time.Since(started))
	return err
}

func renderStageTimingEnabled() bool {
	if !renderDebugEnabled() {
		return false
	}
	for _, key := range []string{"FORJ_RENDER_TIMINGS", "FORJ_RENDER_DEBUG_TIMINGS"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" && value != "0" {
			return true
		}
	}
	return false
}

func renderDebugEnabled() bool {
	for _, key := range []string{"FORJ_DEBUG", "APP_DEBUG", "DEBUG"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" && value != "0" {
			return true
		}
	}
	return false
}

func (p *ProjectRenderer) cleanupLegacyGeneratedFiles() error {
	legacyPaths := []string{
		filepath.Join("internal", "cmd", "generate_all_cmd.go"),
		filepath.Join("internal", "cmd", "generate_cmd.go"),
		filepath.Join("internal", "storage", "generate_cmd.go"),
		filepath.Join("internal", "database", "generate_cmd.go"),
		filepath.Join("internal", "database", "generate_cmd_test.go"),
		filepath.Join("project", "config.go"),
		filepath.Join("internal", "cmd", "demo_push_monitor_trigger_cmd.go"),
		filepath.Join("internal", "cmd", "lifecycle_hooks.go"),
		filepath.Join("internal", "cmd", "about_service.go"),
		filepath.Join("internal", "cmd", "standalone.go"),
		filepath.Join("internal", "http", "devconsole.go"),
		filepath.Join("internal", "http", "route.go"),
		filepath.Join("internal", "http", "middleware_non_200.go"),
		filepath.Join("internal", "http", "routes_list.go"),
		filepath.Join("internal", "http", "routes_list_test.go"),
		filepath.Join("internal", "lifecycle", "README.md"),
		filepath.Join("internal", "lifecycle", "manager.go"),
		filepath.Join("internal", "lifecycle", "manager_test.go"),
		filepath.Join("internal", "lifecycle", "settings.go"),
		filepath.Join("internal", "lifecycle", "lifecycle_registry.go"),
		filepath.Join("internal", "app", "manager.go"),
		filepath.Join("internal", "app", "manager_test.go"),
		filepath.Join("internal", "app", "registry.go"),
		filepath.Join("internal", "jobs", "devconsole.go"),
		filepath.Join("internal", "jobs", "queue_registration.go"),
		filepath.Join("internal", "scheduler", "devconsole.go"),
		filepath.Join("migrations", "2025_04_25_235625_new_user_table.up.sql"),
		filepath.Join("migrations", "2025_04_25_235625_new_user_table.down.sql"),
	}
	for _, path := range legacyPaths {
		if err := removeIfExists(path); err != nil {
			return err
		}
	}
	if err := os.Remove(filepath.Join("project")); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.RemoveAll(filepath.Join("internal", "devconsole")); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join("internal", "lifecycle")); err != nil {
		return err
	}
	if err := p.syncLegacyGeneratedTemplates(); err != nil {
		return err
	}

	// Migrate legacy scheduler command name when scheduler registry is render-once.
	schedulerRegistryPath := filepath.Join("internal", "scheduler", "scheduler_registry.go")
	if data, err := os.ReadFile(schedulerRegistryPath); err == nil {
		updated := strings.ReplaceAll(string(data), "demo:push-monitor-trigger", "monitor:push-test-trigger")
		updated = strings.ReplaceAll(updated, "push-monitor-trigger", "monitor:push-test-trigger")
		if updated != string(data) {
			if err := os.WriteFile(schedulerRegistryPath, []byte(updated), 0o644); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	// Keep stress command wiring in sync for render-once command files.
	stressEnabled := p.config.Render.Components.Jobs && p.config.Render.Components.StressTest
	healthEnabled := p.config.Render.Components.WebAPI || p.config.Render.Components.WebUI
	appCommandsPath := filepath.Join("internal", "cmd", "app_commands.go")
	if data, err := os.ReadFile(appCommandsPath); err == nil {
		updated := syncHealthAppCommands(string(data), healthEnabled)
		updated = syncStressAppCommands(updated, stressEnabled)
		if updated != string(data) {
			if err := os.WriteFile(appCommandsPath, []byte(updated), 0o644); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	cmdWirePath := filepath.Join("internal", "cmd", "wire.go")
	if data, err := os.ReadFile(cmdWirePath); err == nil {
		updated := syncHealthCommandWire(string(data), healthEnabled)
		updated = syncStressCommandWire(updated, stressEnabled)
		if updated != string(data) {
			if err := os.WriteFile(cmdWirePath, []byte(updated), 0o644); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	skipBootPath := filepath.Join("internal", "cmd", "skip_boot.go")
	if data, err := os.ReadFile(skipBootPath); err == nil {
		updated := syncHealthSkipBoot(string(data), healthEnabled)
		if updated != string(data) {
			if err := os.WriteFile(skipBootPath, []byte(updated), 0o644); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if healthEnabled {
		if err := p.renderTemplateFile(filepath.Join("internal", "cmd", "health_cmd.go"), "internal/cmd/health_cmd.go.tmpl", p.config); err != nil {
			return err
		}
		if err := p.renderTemplateFile(filepath.Join("internal", "cmd", "health_cmd_test.go"), "internal/cmd/health_cmd_test.go.tmpl", p.config); err != nil {
			return err
		}
	} else {
		if err := removeIfExists(filepath.Join("internal", "cmd", "health_cmd.go")); err != nil {
			return err
		}
		if err := removeIfExists(filepath.Join("internal", "cmd", "health_cmd_test.go")); err != nil {
			return err
		}
	}
	return nil
}

func (p *ProjectRenderer) syncLegacyGeneratedTemplates() error {
	type templateSync struct {
		dest     string
		tmpl     string
		matches  []string
		requires []string
	}

	syncs := []templateSync{
		{
			dest: "wire/inject_app_services.go",
			tmpl: "wire/inject_app_services.go.tmpl",
			matches: []string{
				"provideSharedRedisClient",
				"events.NewBus(context.Background(), redisClient)",
			},
		},
		{
			dest: "internal/lighthouse/server.go",
			tmpl: "internal/lighthouse/server.go.tmpl",
			matches: []string{
				`"/project"`,
				"project.DevConfig",
				"project.Components",
				"var config project.Config",
				`group.GET("/*"`,
			},
			requires: []string{
				`"/auth/dev-session"`,
				`group.GET("/*"`,
			},
		},
	}

	for _, sync := range syncs {
		data, err := os.ReadFile(sync.dest)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		content := string(data)
		needsRewrite := false
		for _, match := range sync.matches {
			if strings.Contains(content, match) {
				needsRewrite = true
				break
			}
		}
		if !needsRewrite {
			for _, required := range sync.requires {
				if !strings.Contains(content, required) {
					needsRewrite = true
					break
				}
			}
		}
		if !needsRewrite {
			continue
		}
		if err := p.renderTemplateFile(sync.dest, sync.tmpl, p.config); err != nil {
			return err
		}
	}

	if _, err := os.Stat(filepath.Join("internal", "lighthouse", "project_config.go")); os.IsNotExist(err) {
		if err := p.renderTemplateFile(
			filepath.Join("internal", "lighthouse", "project_config.go"),
			"internal/lighthouse/project_config.go.tmpl",
			p.config,
		); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	return nil
}

func syncStressCommandWire(content string, enabled bool) string {
	const stressLine = "\tNewQueueStressTickCmd,\n"
	if enabled {
		if strings.Contains(content, stressLine) {
			return content
		}
		anchor := "\tNewTestMonitorPollLoopCmd,\n"
		if strings.Contains(content, anchor) {
			return strings.Replace(content, anchor, anchor+stressLine, 1)
		}
		return content
	}
	return strings.Replace(content, stressLine, "", 1)
}

func syncStressAppCommands(content string, enabled bool) string {
	const fieldLine = "\tQueueStressTickCmd     QueueStressTickCmd     `cmd:\"\"`\n"
	const paramLine = "\tqueueStressTickCmd *QueueStressTickCmd,\n"
	const assignLine = "\t\tQueueStressTickCmd:     *queueStressTickCmd,\n"
	if enabled {
		updated := content
		fieldAnchor := "\tTestMonitorPollLoopCmd TestMonitorPollLoopCmd `cmd:\"\"`\n"
		paramAnchor := "\ttestMonitorPollLoopCmd *TestMonitorPollLoopCmd,\n"
		assignAnchor := "\t\tTestMonitorPollLoopCmd: *testMonitorPollLoopCmd,\n"
		if !strings.Contains(updated, fieldLine) && strings.Contains(updated, fieldAnchor) {
			updated = strings.Replace(updated, fieldAnchor, fieldAnchor+fieldLine, 1)
		}
		if !strings.Contains(updated, paramLine) && strings.Contains(updated, paramAnchor) {
			updated = strings.Replace(updated, paramAnchor, paramAnchor+paramLine, 1)
		}
		if !strings.Contains(updated, assignLine) && strings.Contains(updated, assignAnchor) {
			updated = strings.Replace(updated, assignAnchor, assignAnchor+assignLine, 1)
		}
		return updated
	}
	updated := strings.Replace(content, fieldLine, "", 1)
	updated = strings.Replace(updated, paramLine, "", 1)
	updated = strings.Replace(updated, assignLine, "", 1)
	return updated
}

func syncHealthCommandWire(content string, enabled bool) string {
	const healthLine = "\tNewHealthCmd,\n"
	if !enabled {
		return strings.Replace(content, healthLine, "", 1)
	}
	if strings.Contains(content, healthLine) {
		return content
	}
	anchor := "\tNewAboutCmd,\n"
	if strings.Contains(content, anchor) {
		return strings.Replace(content, anchor, anchor+healthLine, 1)
	}
	return content
}

func syncHealthSkipBoot(content string, enabled bool) string {
	const healthLine = "\tfunc() interface{} { return NewHealthCmd() },\n"
	if !enabled {
		return strings.Replace(content, healthLine, "", 1)
	}
	if strings.Contains(content, healthLine) {
		return content
	}
	anchor := "\tfunc() interface{} { return NewAboutCmd() },\n"
	if strings.Contains(content, anchor) {
		return strings.Replace(content, anchor, anchor+healthLine, 1)
	}
	return content
}

func syncHealthAppCommands(content string, enabled bool) string {
	const fieldLine = "\tHealthCmd HealthCmd `cmd:\"\"`\n"
	const paramLine = "\thealthCmd *HealthCmd,\n"
	const assignLine = "\t\tHealthCmd: *healthCmd,\n"
	if !enabled {
		updated := strings.Replace(content, fieldLine, "", 1)
		updated = strings.Replace(updated, paramLine, "", 1)
		updated = strings.Replace(updated, assignLine, "", 1)
		return updated
	}
	updated := content
	fieldAnchor := "\tAboutCmd AboutCmd `cmd:\"\"`\n"
	paramAnchor := "\taboutCmd *AboutCmd,\n"
	assignAnchor := "\t\tAboutCmd: *aboutCmd,\n"
	if !strings.Contains(updated, fieldLine) && strings.Contains(updated, fieldAnchor) {
		updated = strings.Replace(updated, fieldAnchor, fieldAnchor+fieldLine, 1)
	}
	if !strings.Contains(updated, paramLine) && strings.Contains(updated, paramAnchor) {
		updated = strings.Replace(updated, paramAnchor, paramAnchor+paramLine, 1)
	}
	if !strings.Contains(updated, assignLine) && strings.Contains(updated, assignAnchor) {
		updated = strings.Replace(updated, assignAnchor, assignAnchor+assignLine, 1)
	}
	return updated
}

// createGoMod initializes the go.mod for the project
func (p *ProjectRenderer) createGoMod() error {
	if err := exec.Command("go", "mod", "init", p.config.GoModuleName).Run(); err != nil {
		p.stats.recordSkipped("go.mod (exists)")
	} else {
		p.stats.recordCreated("go.mod")
	}
	return nil
}

// goModTidy runs `go mod tidy` to ensure dependencies are downloaded.
func (p *ProjectRenderer) goModTidy() error {
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = "." // Or p.config.ProjectRoot if you have it
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		p.logger.Error().
			Str("stdout", stdout.String()).
			Str("stderr", stderr.String()).
			Msg("🔴 go mod tidy failed")
		if detail != "" {
			return fmt.Errorf("go mod tidy: %w (%s)", err, detail)
		}
		return fmt.Errorf("go mod tidy: %w", err)
	}

	modCount := countTidyModules(stdout.String(), stderr.String())
	p.lines = append(p.lines, renderCountsLine("go mod tidy", modCount, 0, "modules"))

	return nil
}

// syncCoreLibraries updates core goforj dependencies so generated templates and
// module APIs stay aligned.
func (p *ProjectRenderer) syncCoreLibraries() error {
	modules := coredeps.SyncCoreLibraries()
	cmd := exec.Command("go", append([]string{"get"}, modules...)...)
	cmd.Dir = "."
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail != "" {
			return fmt.Errorf("go get %w (%s)", err, detail)
		}
		return fmt.Errorf("go get %w", err)
	}

	p.lines = append(p.lines, renderCountsLine("go get core libs", len(modules), 0, "modules"))
	return nil
}

func (p *ProjectRenderer) runWireGenerate() error {
	wireInstallOnce.Do(func() {
		if path, err := exec.LookPath("wire"); err == nil {
			wireBinaryPath = path
			return
		}
		wireBinaryPath, wireInstallErr = installWire()
	})
	if wireInstallErr != nil {
		return wireInstallErr
	}

	if out, err := runWireCommand(wireBinaryPath); err != nil {
		trimmed := strings.TrimSpace(string(out))
		// If a stale wire binary was built with an older Go toolchain, reinstall
		// wire with the current toolchain and retry once.
		if strings.Contains(trimmed, "package requires newer Go version") {
			path, installErr := installWire()
			if installErr != nil {
				return installErr
			}
			wireBinaryPath = path
			if retryOut, retryErr := runWireCommand(wireBinaryPath); retryErr != nil {
				return fmt.Errorf("wire generate: %w (%s)", retryErr, strings.TrimSpace(string(retryOut)))
			}
		} else {
			return fmt.Errorf("wire generate: %w (%s)", err, trimmed)
		}
	}

	p.lines = append(p.lines, renderCountsLine("wire generate", 1, 0, "command"))
	return nil
}

func runWireCommand(wirePath string) ([]byte, error) {
	cmd := exec.Command(wirePath)
	cmd.Dir = "wire"
	cmd.Env = os.Environ()
	return cmd.CombinedOutput()
}

func installWire() (string, error) {
	toolDir, err := os.MkdirTemp("", "forj-wire-*")
	if err != nil {
		return "", fmt.Errorf("create wire tool dir: %w", err)
	}
	wirePath := filepath.Join(toolDir, "wire")
	install := exec.Command("go", "install", wireInstallTarget)
	install.Env = os.Environ()
	install.Env = append(install.Env, "GOBIN="+toolDir)
	if out, err := install.CombinedOutput(); err != nil {
		return "", fmt.Errorf("wire install: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	if _, err := os.Stat(wirePath); err != nil {
		return "", fmt.Errorf("wire install: binary missing after install: %w", err)
	}
	return wirePath, nil
}

func (p *ProjectRenderer) runGenerateAll() error {
	count, _, err := generate.GenerateProjectFiles(
		".",
		true,
		true,
		p.config.Render.Components.Jobs,
		true,
		p.config.Render.Components.HasDatabase(),
	)
	if err != nil {
		return err
	}
	p.lines = append(p.lines, renderCountsLine("forj generate", count, 0, "files"))
	return nil
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func (p *ProjectRenderer) scaffoldDemoFrontend() error {
	if err := p.copyRawPathToDest("demo/frontend", "frontend"); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join("frontend", "dist", "index.html")); err != nil {
		return p.ensureFrontendDistPlaceholder()
	}
	return nil
}

func (p *ProjectRenderer) writeGeneratedFile(path, content string) error {
	newContent := []byte(content)
	if existingContent, err := os.ReadFile(path); err == nil && bytes.Equal(existingContent, newContent) {
		p.stats.recordSkipped(path)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, newContent, 0644); err != nil {
		return err
	}
	p.stats.recordCreated(path)
	return nil
}

func (p *ProjectRenderer) ensureFrontendDistPlaceholder() error {
	distDir := filepath.Join("frontend", "dist")
	if err := os.MkdirAll(distDir, 0755); err != nil {
		return err
	}
	index := filepath.Join(distDir, "index.html")
	if _, err := os.Stat(index); err == nil {
		return nil
	}
	content := "<!doctype html><html><head><meta charset=\"UTF-8\"><title>Build frontend</title></head><body>Run npm run build in frontend to publish SPA assets.</body></html>\n"
	if err := os.WriteFile(index, []byte(content), 0644); err != nil {
		return err
	}
	p.stats.recordCreated(index)
	return nil
}

// renderTemplateFile renders templates based on project configuration settings
func (p *ProjectRenderer) renderTemplateFile(destPath, tmpl string, data any) error {
	tmplBytes, err := templatesFS.ReadFile(tmpl)
	if err != nil {
		return err
	}
	t, err := template.New("").Parse(string(tmplBytes))
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, templateData(data)); err != nil {
		return err
	}

	newContent := buf.Bytes()
	formatted, err := maybeFormatGoSource(destPath, newContent)
	if err != nil {
		return err
	}
	newContent = formatted
	if existingContent, err := os.ReadFile(destPath); err == nil && bytes.Equal(existingContent, newContent) {
		p.stats.recordSkipped(destPath)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(destPath, newContent, 0644); err != nil {
		return err
	}
	p.stats.recordCreated(destPath)
	return nil
}

func templateData(data any) any {
	switch value := data.(type) {
	case *project.Config:
		return templateRenderConfig{
			Config:     value,
			Components: value.Render.Components,
		}
	case project.Config:
		cfg := value
		return templateRenderConfig{
			Config:     &cfg,
			Components: cfg.Render.Components,
		}
	default:
		return data
	}
}

// writeTemplates writes templates to the destination directory of the project
func (p *ProjectRenderer) writeTemplates(tmpls []string) error {
	for _, path := range tmpls {
		dest := strings.TrimSuffix(path, ".tmpl")
		if err := p.renderTemplateFile(dest, path, p.config); err != nil {
			return err
		}
	}
	return nil
}

// writeTemplateMappings writes templates using source->destination mappings.
// mapTemplate(...) derives the destination by removing the trailing .tmpl suffix.
func (p *ProjectRenderer) writeTemplateMappings(mappings []templateMapping) error {
	for _, mapping := range mappings {
		if err := p.renderTemplateFile(mapping.dest, mapping.tmpl, p.config); err != nil {
			return err
		}
	}
	return nil
}

// writeTemplatesUnder renders every .tmpl under srcRoot into destRoot, preserving tree layout.
func (p *ProjectRenderer) writeTemplatesUnder(srcRoot, destRoot string, include func(string) bool) error {
	return fs.WalkDir(templatesFS, srcRoot, func(entry string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(entry, ".tmpl") {
			return nil
		}
		if include != nil && !include(entry) {
			return nil
		}
		rel, err := filepath.Rel(srcRoot, entry)
		if err != nil {
			return err
		}
		dest := filepath.Join(destRoot, strings.TrimSuffix(rel, ".tmpl"))
		return p.renderTemplateFile(dest, entry, p.config)
	})
}

// writeRawFiles writes raw files to the destination directory.
func (p *ProjectRenderer) writeRawFiles(paths []string) error {
	for _, path := range paths {
		if err := p.copyRawPath(path); err != nil {
			return err
		}
	}
	return nil
}

func (p *ProjectRenderer) copyRawPath(path string) error {
	if _, err := fs.ReadDir(templatesFS, path); err == nil {
		return fs.WalkDir(templatesFS, path, func(entry string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			return p.copyRawFile(entry)
		})
	}
	return p.copyRawFile(path)
}

func (p *ProjectRenderer) copyRawPathToDest(path, destRoot string) error {
	if _, err := fs.ReadDir(templatesFS, path); err == nil {
		return fs.WalkDir(templatesFS, path, func(entry string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(path, entry)
			if err != nil {
				return err
			}
			return p.copyRawFileToDest(entry, filepath.Join(destRoot, rel))
		})
	}
	base := filepath.Base(path)
	return p.copyRawFileToDest(path, filepath.Join(destRoot, base))
}

func (p *ProjectRenderer) copyRawFile(path string) error {
	return p.copyRawFileToDest(path, path)
}

func (p *ProjectRenderer) copyRawFileToDest(path, dest string) error {
	content, err := templatesFS.ReadFile(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(dest, content, 0644); err != nil {
		return err
	}
	p.stats.recordCreated(dest)
	return nil
}

// writeTemplatesOnce writes templates to the destination directory only if they do not already exist.
func (p *ProjectRenderer) writeTemplatesOnce(tmpls []string) error {
	for _, path := range tmpls {
		dest := strings.TrimSuffix(path, ".tmpl")

		if _, err := os.Stat(dest); err == nil {
			p.stats.recordSkipped(dest)
			continue
		}

		if err := p.renderTemplateFile(dest, path, p.config); err != nil {
			return err
		}
	}
	return nil
}

// writeTemplateMappingsOnce writes mapped templates only if destination does not yet exist.
func (p *ProjectRenderer) writeTemplateMappingsOnce(mappings []templateMapping) error {
	for _, mapping := range mappings {
		if _, err := os.Stat(mapping.dest); err == nil {
			p.stats.recordSkipped(mapping.dest)
			continue
		}
		if err := p.renderTemplateFile(mapping.dest, mapping.tmpl, p.config); err != nil {
			return err
		}
	}
	return nil
}

func countTidyModules(stdout, stderr string) int {
	combined := strings.Split(strings.TrimSpace(stdout+"\n"+stderr), "\n")
	count := 0
	for _, line := range combined {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(line, "go:") || strings.HasPrefix(line, "downloading") {
			count++
		}
	}
	return count
}

func (p *ProjectRenderer) printStepSummary(title string, before renderCounts) {
	after := p.stats.counts()
	created := after.created - before.created
	skipped := after.skipped - before.skipped
	p.lines = append(p.lines, renderCountsLine(title, created, skipped, "files"))
}

func renderBox(title string, lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	maxLine := 0
	for _, line := range lines {
		if w := lipgloss.Width(line); w > maxLine {
			maxLine = w
		}
	}
	if title != "" && lipgloss.Width(title) > maxLine {
		maxLine = lipgloss.Width(title)
	}
	inner := maxLine + 2
	top := ""
	if title == "" {
		top = boxBorder.Render("┌" + strings.Repeat("─", inner) + "┐")
	} else {
		topTitle := " " + title + " "
		fill := inner - lipgloss.Width(topTitle)
		if fill < 0 {
			fill = 0
		}
		top = boxBorder.Render("┌" + topTitle + strings.Repeat("─", fill) + "┐")
	}
	body := make([]string, 0, len(lines))
	for _, line := range lines {
		w := lipgloss.Width(line)
		if w < maxLine {
			line += strings.Repeat(" ", maxLine-w)
		}
		body = append(body, boxBorder.Render("│")+" "+line+" "+boxBorder.Render("│"))
	}
	bottom := boxBorder.Render("└" + strings.Repeat("─", inner) + "┘")
	box := strings.Join(append([]string{top}, append(body, bottom)...), "\n")
	return lipgloss.NewStyle().MarginLeft(1).Render(box)
}

func (p *ProjectRenderer) printRenderDetails() {
	if len(p.lines) == 0 {
		return
	}
	title := fmt.Sprintf("%s Project rendering complete", markCreate)
	fmt.Printf("%s\n", renderBox(title, p.lines))
}

func (p *ProjectRenderer) printOverallSummary() {
	total := p.stats.counts()
	title := fmt.Sprintf("%s Project render complete (created: %d, skipped: %d)", markCreate, total.created, total.skipped)
	lines := []string{}
	if total.skipped > 0 {
		lines = append(lines, fmt.Sprintf("%s existing files preserved: %d", markSkip, total.skipped))
	}
	next := p.nextSteps()
	if len(next) > 0 {
		lines = append(lines, nextStyle.Render("next steps:"))
		for _, step := range next {
			lines = append(lines, fmt.Sprintf("%s %s", markAction, step))
		}
	}
	fmt.Printf("\n%s\n", renderBox(title, lines))
}

func topNUnique(paths []string, limit int) []string {
	seen := make(map[string]bool, len(paths))
	var out []string
	for _, p := range paths {
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
		if len(out) == limit {
			break
		}
	}
	return out
}

func (p *ProjectRenderer) nextSteps() []string {
	var steps []string

	steps = append(steps, fmt.Sprintf("Set environment defaults in %s and %s", commandStyle.Render(".env"), commandStyle.Render(".env.host")))
	steps = append(steps, fmt.Sprintf("Start the dev loop: %s", commandStyle.Render("forj dev")))

	if p.config != nil {
		if p.config.Render.Components.WebUI {
			steps = append(steps, fmt.Sprintf("Install frontend deps if you plan to edit the UI: %s", commandStyle.Render("cd frontend && npm install")))
		}
		if p.config.Render.Components.HasDatabase() {
			steps = append(steps, fmt.Sprintf("Review initial migrations under %s before first run", commandStyle.Render("migrations")))
		}
	}

	return steps
}
