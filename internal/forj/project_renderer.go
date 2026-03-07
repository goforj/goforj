package forj

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/goforj/crypt"
	"github.com/goforj/goforj/internal/console"
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
			Render:     project.RenderConfig{Components: input.components},
			Components: input.components,
		}
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
					needsToken := path == ".env" && !strings.Contains(text, "LIGHTHOUSE_TOKEN=")
					needsEnabled := path == ".env" && !strings.Contains(text, "LIGHTHOUSE_ENABLED=")
					needsSwagger := path == ".env" && !strings.Contains(text, "SWAGGER_ENABLED=")
					needsKey := allowAppKey && !strings.Contains(text, "APP_KEY=")
					needsJWTSecret := false

					appKey := ""
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
						if strings.HasPrefix(trimmed, "JWT_SECRET_KEY=") {
							jwtSecret = strings.TrimSpace(strings.TrimPrefix(trimmed, "JWT_SECRET_KEY="))
							jwtLineIdx = idx
							continue
						}
					}
					if path == ".env" && (jwtSecret == "" || jwtSecret == "xxx") {
						needsJWTSecret = true
					}
					if !(needsURL || needsToken || needsEnabled || needsSwagger || needsKey || needsJWTSecret) {
						return nil
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
							lines[jwtLineIdx] = fmt.Sprintf("JWT_SECRET_KEY=%s", jwtSecret)
						} else {
							writeLines = append(writeLines, fmt.Sprintf("JWT_SECRET_KEY=%s", jwtSecret))
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
					jwtSecret, err := generateJWTSecretKey()
					if err != nil {
						return fmt.Errorf("failed to generate JWT secret: %w", err)
					}
					p.config.AppKey = key
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
				"internal/events/driver.go.tmpl",
				"internal/events/event.go.tmpl",
				"internal/events/factory.go.tmpl",
				"internal/events/topics.go.tmpl",
				"internal/events/bus_inproc.go.tmpl",
				"internal/events/bus_redis.go.tmpl",
				"internal/events/helpers.go.tmpl",
				"internal/events/make_event_cmd.go.tmpl",
				"internal/events/driver_test.go.tmpl",
				"internal/events/factory_test.go.tmpl",
				"internal/events/bus_inproc_test.go.tmpl",
				"internal/events/bus_redis_test.go.tmpl",
				"internal/events/helpers_test.go.tmpl",
				"internal/events/make_event_cmd_test.go.tmpl",
				"internal/events/bus_integration_test.go.tmpl",
				"internal/events/README.md.tmpl",
				"internal/lifecycle/manager.go.tmpl",
				"internal/lifecycle/manager_test.go.tmpl",
				"internal/lifecycle/README.md.tmpl",
				"internal/console/console.go.tmpl",
				"internal/cmd/hello_world_cmd.go.tmpl",
				"internal/cmd/test_event_pipeline_cmd.go.tmpl",
				"internal/cmd/monitor_seed_cmd.go.tmpl",
				"internal/cmd/monitor_reset_cmd.go.tmpl",
				"internal/cmd/monitor_retention_cmd.go.tmpl",
				"internal/cmd/monitor_poll_cmd.go.tmpl",
				"internal/cmd/push_monitor_trigger_cmd.go.tmpl",
				"internal/cmd/test_monitor_poll_loop_cmd.go.tmpl",
				"internal/cmd/kong_help_formatter.go.tmpl",
				"internal/cmd/run_cmd.go.tmpl",
				"internal/cmd/root_cmd.go.tmpl",
				"internal/logger/app.go.tmpl",
				"internal/logger/app_test.go.tmpl",
				"internal/logger/dedupe.go.tmpl",
				"internal/logger/dedupe_test.go.tmpl",
				"internal/logger/wire.go.tmpl",
				"project/config.go.tmpl",
				"wire/app.go.tmpl",
				"wire/app_test.go.tmpl",
				"wire/inject_cache.go.tmpl",
				"wire/inject_app_services.go.tmpl",
				"wire/inject_cmd.go.tmpl",
				"wire/wire.go.tmpl",
			},
			renderOnceTemplates: []string{
				".gitignore.tmpl",
				".db-relationships.yaml.tmpl",
				"internal/cmd/app_commands.go.tmpl",
				"internal/cmd/wire.go.tmpl",
				"internal/lifecycle/lifecycle_registry.go.tmpl",
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
			enabled: p.config.Components.Docker,
			templates: append([]string{"docker-compose.yml.tmpl"},
				func() []string {
					if p.config.Components.DatabaseMySQL {
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
			enabled: p.config.Components.WebAPI || p.config.Components.WebUI || p.config.Components.Scheduler || p.config.Components.Jobs,
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
			enabled: p.config.Components.WebAPI || p.config.Components.WebUI,
			templates: []string{
				"wire/inject_http.go.tmpl",
				"internal/http/lighthouse.go.tmpl",
				"internal/http/README.md.tmpl",
				"internal/http/cors.go.tmpl",
				"internal/http/route.go.tmpl",
				"internal/http/routes_list.go.tmpl",
				"internal/http/routes_list_cmd.go.tmpl",
				"internal/http/routes_list_test.go.tmpl",
				"internal/http/health.go.tmpl",
				"internal/http/health_test.go.tmpl",
				"internal/http/swagger.go.tmpl",
				"internal/http/swagger_test.go.tmpl",
				"internal/http/readiness_checks.go.tmpl",
				"internal/http/middleware_non_200.go.tmpl",
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
			enabled:   p.config.Components.WebUI,
			templates: []string{},
			renderOnceTemplates: []string{
				"frontend/dist/index.html.tmpl",
			},
		},
		{
			title:   "Demo App Components Rendering",
			enabled: input.renderAll && p.config.Components.DemoApp,
			action: func() error {
				if !p.config.Components.HasDatabase() {
					return nil
				}

				includeDemoInternal := func(tmpl string) bool {
					if p.config.Components.Jobs {
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
				if err := p.writeTemplateMappings(map[string]string{
					"internal/router/routes_registry.go.tmpl": "internal/router/routes_registry.go",
					"wire/inject_http_controllers.go.tmpl":    "wire/inject_http_controllers.go",
					"internal/cmd/app_commands.go.tmpl":       "internal/cmd/app_commands.go",
					"internal/cmd/wire.go.tmpl":               "internal/cmd/wire.go",
				}); err != nil {
					return err
				}
				return nil
			},
		},
		{
			title:   "Demo App Frontend Scaffolding",
			enabled: input.renderAll && p.config.Components.DemoApp && p.config.Components.WebUI,
			action:  p.scaffoldDemoFrontend,
		},
		{
			title:   "Database Components Rendering",
			enabled: p.config.Components.HasDatabase(),
			templates: []string{
				"wire/inject_db.go.tmpl",
				"wire/inject_repositories.go.tmpl",
				"internal/dbconns/connections.go.tmpl",
				"internal/dbconns/gorm_log_writer.go.tmpl",
				"internal/dbconns/connections_test.go.tmpl",
				"internal/dbconns/generate_cmd.go.tmpl",
				"internal/dbconns/generate_cmd_test.go.tmpl",
				"internal/cmd/generate_all_cmd.go.tmpl",
				"internal/modelgen/make_model_cmd.go.tmpl",
				"internal/modelgen/make_model_mysql_integration_test.go.tmpl",
				"internal/modelgen/make_model_postgres_integration_test.go.tmpl",
				"internal/modelgen/make_model_sqlite_integration_test.go.tmpl",
				"internal/modelgen/repository_wire_test.go.tmpl",
			},
			raw: []string{"internal/modelgen/model.tmpl"},
			action: func() error {
				if err := p.writeTemplateMappings(map[string]string{
					"migrations/migrations.go.tmpl":                "migrations/migrations.go",
					"migrations/migrations_test.go.tmpl":           "migrations/migrations_test.go",
					"migrations/migration_connection_test.go.tmpl": "migrations/migration_connection_test.go",
					"migrations/migration_commands_test.go.tmpl":   "migrations/migration_commands_test.go",
					"migrations/migrate_cmd.go.tmpl":               "migrations/migrate_cmd.go",
					"migrations/migrate_rollback_cmd.go.tmpl":      "migrations/migrate_rollback_cmd.go",
					"migrations/.goforj/placeholder.txt.tmpl":      "migrations/.goforj/placeholder.txt",
				}); err != nil {
					return err
				}
				if err := p.writeTemplateMappingsOnce(map[string]string{
					"migrations/2025_04_25_235625_new_user_table.up.sql.tmpl":   "migrations/2025_04_25_235625_new_user_table.up.sql",
					"migrations/2025_04_25_235625_new_user_table.down.sql.tmpl": "migrations/2025_04_25_235625_new_user_table.down.sql",
				}); err != nil {
					return err
				}
				return nil
			},
		},
		{
			title:   "Scheduler Components Rendering",
			enabled: p.config.Components.Scheduler,
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
			enabled: p.config.Components.Jobs,
			templates: append([]string{
				"internal/jobs/example_hello_job.go.tmpl",
				"internal/jobs/example_hello_job_cmd.go.tmpl",
				"internal/jobs/benchmark_run_cmd.go.tmpl",
				"internal/jobs/benchmark_system.go.tmpl",
				"internal/jobs/make_job_cmd.go.tmpl",
				"internal/jobs/lighthouse.go.tmpl",
				"internal/jobs/worker.go.tmpl",
				"internal/jobs/worker_logger.go.tmpl",
				"internal/jobs/worker_cmd.go.tmpl",
				"wire/inject_queue.go.tmpl",
				"wire/inject_jobs.go.tmpl",
				"wire/inject_jobs_app.go.tmpl",
			}, func() []string {
				if !p.config.Components.StressTest {
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
	if err := p.timeRenderStage("syncCoreLibraries", p.syncCoreLibraries); err != nil {
		return fmt.Errorf("sync core libraries: %w", err)
	}

	// Run go mod tidy to ensure all dependencies are downloaded
	if err := p.timeRenderStage("goModTidy", p.goModTidy); err != nil {
		return fmt.Errorf("go mod tidy: %w", err)
	}

	// Run wire install + generate to make main runnable immediately.
	if err := p.timeRenderStage("runWireGenerate", p.runWireGenerate); err != nil {
		return fmt.Errorf("wire generate: %w", err)
	}

	if input.renderAll && p.config.Components.HasDatabase() {
		if err := p.timeRenderStage("runGenerateDbConns", p.runGenerateDbConns); err != nil {
			return fmt.Errorf("generate dbconns: %w", err)
		}
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
		filepath.Join("internal", "cmd", "demo_push_monitor_trigger_cmd.go"),
		filepath.Join("internal", "cmd", "lifecycle_hooks.go"),
		filepath.Join("internal", "http", "devconsole.go"),
		filepath.Join("internal", "jobs", "devconsole.go"),
		filepath.Join("internal", "scheduler", "devconsole.go"),
	}
	for _, path := range legacyPaths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.RemoveAll(filepath.Join("internal", "devconsole")); err != nil {
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
	stressEnabled := p.config.Components.Jobs && p.config.Components.StressTest
	appCommandsPath := filepath.Join("internal", "cmd", "app_commands.go")
	if data, err := os.ReadFile(appCommandsPath); err == nil {
		updated := syncStressAppCommands(string(data), stressEnabled)
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
		updated := syncStressCommandWire(string(data), stressEnabled)
		if updated != string(data) {
			if err := os.WriteFile(cmdWirePath, []byte(updated), 0o644); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
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
		p.logger.Error().
			Str("stdout", stdout.String()).
			Str("stderr", stderr.String()).
			Msg("🔴 go mod tidy failed")
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
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
	modules := []string{
		"github.com/goforj/cache@v0.1.5",
		"github.com/goforj/cache/cachecore@v0.1.5",
		"github.com/goforj/cache/driver/rediscache@v0.1.5",
		"github.com/goforj/queue@v0.1.5",
		"github.com/goforj/scheduler@v1.4.0",
		"github.com/goforj/env/v2@latest",
	}
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
		if commandExists("wire") {
			return
		}
		wireInstallErr = installWire()
	})
	if wireInstallErr != nil {
		return wireInstallErr
	}

	if out, err := runWireCommand(); err != nil {
		trimmed := strings.TrimSpace(string(out))
		// If a stale wire binary was built with an older Go toolchain, reinstall
		// wire with the current toolchain and retry once.
		if strings.Contains(trimmed, "package requires newer Go version") {
			if installErr := installWire(); installErr != nil {
				return installErr
			}
			if retryOut, retryErr := runWireCommand(); retryErr != nil {
				return fmt.Errorf("wire generate: %w (%s)", retryErr, strings.TrimSpace(string(retryOut)))
			}
		} else {
			return fmt.Errorf("wire generate: %w (%s)", err, trimmed)
		}
	}

	p.lines = append(p.lines, renderCountsLine("wire generate", 1, 0, "command"))
	return nil
}

func runWireCommand() ([]byte, error) {
	cmd := exec.Command("wire")
	cmd.Dir = "wire"
	cmd.Env = os.Environ()
	return cmd.CombinedOutput()
}

func installWire() error {
	install := exec.Command("go", "install", "github.com/goforj/wire/cmd/wire@latest")
	install.Env = os.Environ()
	if out, err := install.CombinedOutput(); err != nil {
		return fmt.Errorf("wire install: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// runGenerateDbConns executes the generated app CLI to build db accessors.
func (p *ProjectRenderer) runGenerateDbConns() error {
	cmd := exec.Command("go", "run", ".", "generate:dbconns")
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("generate:dbconns failed: %w (%s)", err, strings.TrimSpace(string(out)))
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
	if err := t.Execute(&buf, data); err != nil {
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

// writeTemplateMappings writes templates using explicit source->destination pairs.
func (p *ProjectRenderer) writeTemplateMappings(mapping map[string]string) error {
	for tmpl, dest := range mapping {
		if err := p.renderTemplateFile(dest, tmpl, p.config); err != nil {
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
func (p *ProjectRenderer) writeTemplateMappingsOnce(mapping map[string]string) error {
	for tmpl, dest := range mapping {
		if _, err := os.Stat(dest); err == nil {
			p.stats.recordSkipped(dest)
			continue
		}
		if err := p.renderTemplateFile(dest, tmpl, p.config); err != nil {
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
		if p.config.Components.WebUI {
			steps = append(steps, fmt.Sprintf("Install frontend deps if you plan to edit the UI: %s", commandStyle.Render("cd frontend && npm install")))
		}
		if p.config.Components.HasDatabase() {
			steps = append(steps, fmt.Sprintf("Review initial migrations under %s before first run", commandStyle.Render("migrations")))
		}
	}

	return steps
}
