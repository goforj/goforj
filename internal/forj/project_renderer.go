package forj

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/goforj/crypt"
	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/coredeps"
	"github.com/goforj/goforj/internal/forj/makeapp"
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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	markStep     = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Render("▸")
	markCreate   = lipgloss.NewStyle().Foreground(lipgloss.Color("84")).Render("✔")
	markSkip     = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("·")
	markAction   = lipgloss.NewStyle().Foreground(lipgloss.Color("45")).Render("›")
	headerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
	summaryStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("84")).Bold(true)
	nextStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	timingStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	bulletStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Render("·")
	commandStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
	boxBorder    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	wireInstallOnce sync.Once
	wireInstallErr  error
	wireBinaryPath  string
)

var templatesFS = templates.FS

type wireGenerateError struct {
	dir    string
	output string
	err    error
	stale  bool
}

func (e *wireGenerateError) Error() string {
	return fmt.Sprintf("wire generate %s: %v (%s)", e.dir, e.err, e.output)
}

func (e *wireGenerateError) Unwrap() error {
	return e.err
}

// ComponentRenderInput controls whether rendering uses explicit components or the stored project config.
type ComponentRenderInput struct {
	components project.Components
	renderAll  bool
}

// ProjectRenderer renders project files from the current config and template set.
type ProjectRenderer struct {
	logger  *logger.AppLogger
	config  *project.Config
	stats   *renderStats
	lines   []string
	timings bool
}

type renderStats struct {
	mu      sync.Mutex
	created []string
	skipped []string
}

func (s *renderStats) recordCreated(path string) {
	if path == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.created = append(s.created, path)
}

func (s *renderStats) recordSkipped(path string) {
	if path == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.skipped = append(s.skipped, path)
}

type renderCounts struct {
	created int
	skipped int
}

type templateRenderConfig struct {
	*project.Config
	Components           project.Components
	ProjectComponents    project.Components
	Target               project.AppTarget
	TargetPackageName    string
	TargetAppImportPath  string
	TargetWireImportPath string
	TargetIsDefault      bool
	HasNamedTargets      bool
	RuntimeTargets       []runtimeTargetMetadata
}

type runtimeTargetMetadata struct {
	Name        string
	Index       int
	EnvPrefix   string
	HTTPPort    int
	RuntimeBase int
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
	s.mu.Lock()
	defer s.mu.Unlock()
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

func renderCountsLineWithTiming(title string, created, skipped int, unit string, elapsed time.Duration) string {
	line := renderCountsLine(title, created, skipped, unit)
	return appendRenderTiming(line, elapsed)
}

func formatRenderElapsed(elapsed time.Duration) string {
	return formatDevElapsed(elapsed)
}

func appendRenderTiming(line string, elapsed time.Duration) string {
	if elapsed <= 0 {
		return line
	}
	return line + " " + markSkip + " " + timingStyle.Render(formatRenderElapsed(elapsed))
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

func generateLighthouseSecret() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	return generateRandomToken(charset, 32)
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
		p.config.Render.StarterKit = project.StarterKitNone
	}
	p.config.Render.Components.ResolveDependencies()
	if err := p.config.Render.Components.ValidateRenderContract(); err != nil {
		return err
	}
	p.config.Render.StarterKit = project.NormalizeStarterKit(p.config.Render.StarterKit)
	if !p.config.Render.Components.WebUI {
		p.config.Render.StarterKit = project.StarterKitNone
	}
	if err := project.ValidateStarterKitContract(p.config.Render.StarterKit, p.config.Render.Components); err != nil {
		return err
	}
	if input.renderAll {
		if err := p.syncProjectConfigForRender(); err != nil {
			return err
		}
	}
	if err := p.migrateAppOwnedWireFilenames(); err != nil {
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
			title:   "Default App Target Rendering",
			enabled: input.renderAll,
			action: func() error {
				return p.renderAppTarget(project.DefaultAppTarget())
			},
		},
		{
			title:   "Environment Files Initialization",
			enabled: input.renderAll,
			action: func() error {
				envTemplates := []string{
					".env.tmpl",
					".env.host.tmpl",
				}
				localEnvTemplate := ".env.local.tmpl"
				ensureEnvDefaults := func(path string, allowAppKey bool) error {
					content, err := os.ReadFile(path)
					if err != nil {
						return err
					}
					text := string(content)
					needsURL := path == ".env" && !strings.Contains(text, "LIGHTHOUSE_URL=")
					needsAppDiagToken := path == ".env" && !strings.Contains(text, "APP_DIAG_TOKEN=")
					needsSecret := path == ".env" && !strings.Contains(text, "LIGHTHOUSE_SECRET=")
					needsEnabled := path == ".env" && !strings.Contains(text, "LIGHTHOUSE_ENABLED=")
					needsTraceCache := path == ".env" && !strings.Contains(text, "CACHE_INSPECTS_DRIVER=")
					needsLighthouseCache := path == ".env" && !strings.Contains(text, "CACHE_LIGHTHOUSE_DRIVER=")
					needsSwagger := path == ".env" && !strings.Contains(text, "SWAGGER_ENABLED=")
					needsForjMakeOpen := path == ".env" && !strings.Contains(text, "FORJ_MAKE_OPEN=")
					needsForjEditor := path == ".env" && !strings.Contains(text, "FORJ_EDITOR=")
					needsGrafanaPortDefault := false
					needsKey := allowAppKey && !strings.Contains(text, "APP_KEY=")
					needsJWTSecret := false

					appKey := ""
					appDiagToken := ""
					secretValue := ""
					jwtSecret := ""
					jwtLineIdx := -1
					lines := strings.Split(text, "\n")
					filteredLines := make([]string, 0, len(lines))
					seenLighthouseSecret := false
					for _, line := range lines {
						trimmed := strings.TrimSpace(line)
						if trimmed == "" || strings.HasPrefix(trimmed, "#") {
							filteredLines = append(filteredLines, line)
							continue
						}
						if strings.HasPrefix(trimmed, "APP_KEY=") {
							appKey = strings.TrimSpace(strings.TrimPrefix(trimmed, "APP_KEY="))
							filteredLines = append(filteredLines, line)
							continue
						}
						if strings.HasPrefix(trimmed, "LIGHTHOUSE_SECRET=") {
							if !seenLighthouseSecret {
								secretValue = strings.TrimSpace(strings.TrimPrefix(trimmed, "LIGHTHOUSE_SECRET="))
								filteredLines = append(filteredLines, line)
								seenLighthouseSecret = true
							}
							continue
						}
						if strings.HasPrefix(trimmed, "APP_DIAG_TOKEN=") {
							appDiagToken = strings.TrimSpace(strings.TrimPrefix(trimmed, "APP_DIAG_TOKEN="))
							filteredLines = append(filteredLines, line)
							continue
						}
						if strings.HasPrefix(trimmed, "API_JWT_SECRET_KEY=") {
							jwtSecret = strings.TrimSpace(strings.TrimPrefix(trimmed, "API_JWT_SECRET_KEY="))
							filteredLines = append(filteredLines, line)
							jwtLineIdx = len(filteredLines) - 1
							continue
						}
						filteredLines = append(filteredLines, line)
					}
					lines = filteredLines
					if path == ".env" && (jwtSecret == "" || jwtSecret == "xxx") {
						needsJWTSecret = true
					}
					if path == ".env" && p.config.Render.Components.Grafana {
						lines, needsGrafanaPortDefault = migrateGeneratedEnvDefault(lines, "GRAFANA_PORT", "3001", "13001")
					}
					if !(needsURL || needsAppDiagToken || needsSecret || needsEnabled || needsTraceCache || needsLighthouseCache || needsSwagger || needsForjMakeOpen || needsForjEditor || needsGrafanaPortDefault || needsKey || needsJWTSecret) {
						return nil
					}
					if needsAppDiagToken && appDiagToken == "" {
						value, err := generateAppDiagToken()
						if err != nil {
							return fmt.Errorf("failed to generate app diagnostics token: %w", err)
						}
						appDiagToken = value
					}
					if needsSecret && secretValue == "" {
						value, err := generateLighthouseSecret()
						if err != nil {
							return fmt.Errorf("failed to generate lighthouse secret: %w", err)
						}
						secretValue = value
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
					if needsSecret {
						writeLines = append(writeLines, fmt.Sprintf("LIGHTHOUSE_SECRET=%s", secretValue))
					}
					if needsEnabled {
						writeLines = append(writeLines, "LIGHTHOUSE_ENABLED=true")
					}
					if needsTraceCache {
						writeLines = append(writeLines, "CACHE_INSPECTS_DRIVER=memory")
					}
					if needsLighthouseCache {
						writeLines = append(writeLines, "CACHE_LIGHTHOUSE_DRIVER=memory")
					}
					if needsSwagger {
						writeLines = append(writeLines, "SWAGGER_ENABLED=true")
					}
					if needsForjMakeOpen || needsForjEditor {
						if len(writeLines) > 0 {
							writeLines = append(writeLines, "")
						}
						if !strings.Contains(text, "# Forj") {
							writeLines = append(writeLines, "# Forj")
						}
						if needsForjMakeOpen {
							writeLines = append(writeLines, "FORJ_MAKE_OPEN=auto # options: auto, always, never")
						}
						if needsForjEditor {
							writeLines = append(writeLines, "# Optional editor command for make commands; falls back to common GUI editors.")
							writeLines = append(writeLines, "FORJ_EDITOR=")
						}
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
				missingEnvTemplates := make([]string, 0, len(envTemplates))
				for _, tmpl := range envTemplates {
					name := strings.TrimSuffix(strings.TrimPrefix(tmpl, ""), ".tmpl")
					if _, err := os.Stat(name); err == nil {
						allowAppKey := name == ".env"
						if err := ensureEnvDefaults(name, allowAppKey); err != nil {
							return err
						}
						continue
					}
					missingEnvTemplates = append(missingEnvTemplates, tmpl)
				}
				if len(missingEnvTemplates) > 0 {
					key, err := crypt.GenerateAppKey()
					if err != nil {
						return fmt.Errorf("failed to generate app key: %w", err)
					}
					secret, err := generateLighthouseSecret()
					if err != nil {
						return fmt.Errorf("failed to generate lighthouse secret: %w", err)
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
					p.config.LighthouseSecret = secret
					p.config.JWTSecretKey = jwtSecret
					if err := p.writeTemplates(missingEnvTemplates); err != nil {
						return err
					}
				}
				return p.writeTemplates([]string{localEnvTemplate})
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
				"internal/makecmd/editor.go.tmpl",
				"internal/makecmd/env_section_editor.go.tmpl",
				"internal/makecmd/generator_helpers.go.tmpl",
				"internal/makecmd/generator_helpers_test.go.tmpl",
				"internal/makecmd/help.go.tmpl",
				"internal/makecmd/resource_maker.go.tmpl",
				"internal/makecmd/command_names.go.tmpl",
				"internal/makecmd/make_command_cmd.go.tmpl",
				"internal/makecmd/make_command_cmd_test.go.tmpl",
				"internal/makecmd/make_event_cmd.go.tmpl",
				"internal/makecmd/make_event_cmd_test.go.tmpl",
				"internal/makecmd/make_subscriber_cmd.go.tmpl",
				"internal/makecmd/make_subscriber_cmd_test.go.tmpl",
				"internal/makecmd/make_migration_cmd.go.tmpl",
				"internal/makecmd/make_migration_cmd_test.go.tmpl",
				"internal/events/bus_integration_test.go.tmpl",
				"internal/events/README.md.tmpl",
				"internal/runtime/lifecycle.go.tmpl",
				"internal/runtime/lifecycle_test.go.tmpl",
				"internal/runtime/runtime.go.tmpl",
				"internal/runtime/source.go.tmpl",
				"internal/runtime/targets.go.tmpl",
				"internal/runtime/targets_test.go.tmpl",
				"internal/runtime/runtime_host.go.tmpl",
				"internal/runtime/runtime_host_test.go.tmpl",
				"internal/runtime/timeouts.go.tmpl",
				"internal/runtime/README.md.tmpl",
				"internal/caches/README.md.tmpl",
				"internal/storages/README.md.tmpl",
				"internal/observability/cache_observer.go.tmpl",
				"internal/observability/event_observer.go.tmpl",
				"internal/observability/mail_observer.go.tmpl",
				"internal/observability/queue_observer.go.tmpl",
				"internal/observability/queue_observer_test.go.tmpl",
				"internal/observability/storage_observer.go.tmpl",
				"internal/console/console.go.tmpl",
				"internal/runtime/about.go.tmpl",
				"internal/runtime/discovery.go.tmpl",
				"internal/cmd/about_cmd.go.tmpl",
				"internal/cmd/about_cmd_test.go.tmpl",
				"internal/cmd/about_grid.go.tmpl",
				"internal/cmd/cache_shell_cmd.go.tmpl",
				"internal/cmd/hello_world_cmd.go.tmpl",
				"internal/cmd/json_helpers.go.tmpl",
				"internal/cmd/test_event_pipeline_cmd.go.tmpl",
				"internal/monitoring/seed_cmd.go.tmpl",
				"internal/monitoring/reset_cmd.go.tmpl",
				"internal/monitoring/retention_cmd.go.tmpl",
				"internal/monitoring/json_helpers.go.tmpl",
				"internal/monitoring/poll_cmd.go.tmpl",
				"internal/monitoring/push_trigger_cmd.go.tmpl",
				"internal/monitoring/test_poll_loop_cmd.go.tmpl",
				"internal/cmd/kong_help_formatter.go.tmpl",
				"internal/cmd/default_launch.go.tmpl",
				"internal/cmd/default_launch_test.go.tmpl",
				"internal/cmd/env_defaults.go.tmpl",
				"internal/cmd/env_defaults_test.go.tmpl",
				"internal/cmd/preboot.go.tmpl",
				"internal/cmd/preboot_test.go.tmpl",
				"internal/cmd/target_identity.go.tmpl",
				"internal/cmd/run_cmd.go.tmpl",
				"internal/logger/app.go.tmpl",
				"internal/logger/bench_test.go.tmpl",
				"internal/logger/app_test.go.tmpl",
				"internal/logger/dedupe.go.tmpl",
				"internal/logger/dedupe_test.go.tmpl",
				"internal/logger/event.go.tmpl",
				"internal/logger/wire.go.tmpl",
				"internal/inspects/README.md.tmpl",
				"internal/inspects/manager.go.tmpl",
				"internal/inspects/manager_test.go.tmpl",
				"internal/inspects/manager_bench_test.go.tmpl",
				"internal/lighthouse/project_config.go.tmpl",
			},
			renderOnceTemplates: []string{
				".gitignore.tmpl",
				".db-relationships.yaml.tmpl",
			},
			raw: []string{
				"internal/makecmd/event.tmpl",
				"internal/makecmd/make_command.tmpl",
				"internal/makecmd/README.md",
				"internal/makecmd/subscriber.tmpl",
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
				"internal/lighthouse/conn_test.go.tmpl",
				"internal/lighthouse/enable.go.tmpl",
				"internal/lighthouse/hub.go.tmpl",
				"internal/lighthouse/hub_test.go.tmpl",
				"internal/lighthouse/inspects.go.tmpl",
				"internal/lighthouse/inspects_test.go.tmpl",
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
				"internal/http/lighthouse.go.tmpl",
				"internal/http/README.md.tmpl",
				"internal/http/cors.go.tmpl",
				"internal/http/routes_list_cmd.go.tmpl",
				"internal/http/health.go.tmpl",
				"internal/http/health_test.go.tmpl",
				"internal/http/inspect_child_event_test.go.tmpl",
				"internal/http/inspects_bench_test.go.tmpl",
				"internal/http/runtime_bench_test.go.tmpl",
				"internal/http/server_test.go.tmpl",
				"internal/http/swagger.go.tmpl",
				"internal/http/swagger_test.go.tmpl",
				"internal/http/readiness_checks.go.tmpl",
				"internal/http/runtime.go.tmpl",
				"internal/http/serve_cmd.go.tmpl",
				"internal/http/server.go.tmpl",
				"internal/http/spa.go.tmpl",
				"internal/http/types.go.tmpl",
				"internal/hello/controller.go.tmpl",
				"internal/makecmd/make_controller_cmd.go.tmpl",
				"internal/makecmd/make_controller_cmd_test.go.tmpl",
			},
			action: func() error {
				if input.renderAll {
					return nil
				}
				if err := p.writeTemplateMappings([]templateMapping{
					mapTemplateTo("wire/inject_http.go.tmpl", "app/wire/inject_http.go"),
				}); err != nil {
					return err
				}
				return p.writeTemplateMappingsOnce([]templateMapping{
					mapTemplateTo("wire/inject_http_controllers_app.go.tmpl", "app/wire/inject_http_controllers_app.go"),
				})
			},
		},
		{
			title:     "Web UI Components Rendering",
			enabled:   p.config.Render.Components.WebUI,
			templates: []string{},
			renderOnceTemplates: []string{
				"frontend/dist/index.html.tmpl",
			},
			action: func() error {
				if input.renderAll {
					return nil
				}
				if err := p.writeTemplateMappingsOnce([]templateMapping{
					mapTemplateTo("frontend/dist/index.html.tmpl", appTargetFrontendDistIndex(project.DefaultAppTarget())),
				}); err != nil {
					return err
				}
				return p.ensureFrontendPlaceholderAssets(project.DefaultAppTarget())
			},
		},
		{
			title:   "Starter Kit Rendering",
			enabled: p.config.Render.Components.WebUI && p.config.Render.StarterKit == project.StarterKitVue && !p.config.Render.Components.DemoApp,
			action:  p.scaffoldVueStarterKit,
		},
		{
			title:   "Metrics Components Rendering",
			enabled: p.config.Render.Components.Metrics,
			templates: []string{
				"internal/metrics/README.md.tmpl",
				"internal/metrics/endpoint.go.tmpl",
				"internal/metrics/manager.go.tmpl",
				"internal/metrics/manager_test.go.tmpl",
			},
			action: func() error {
				if !p.config.Render.Components.WebAPI {
					return nil
				}
				return p.writeTemplates([]string{
					"internal/http/metrics.go.tmpl",
					"internal/http/metrics_test.go.tmpl",
				})
			},
		},
		{
			title:   "Observability Components Rendering",
			enabled: p.config.Render.Components.Observability,
			templates: []string{
				"internal/observability/README.md.tmpl",
				"containers/observability/vmagent/prometheus.yml.tmpl",
			},
			action: func() error {
				templates := []string{}
				if p.config.Render.Components.Grafana {
					templates = append(templates,
						"containers/observability/grafana/provisioning/datasources/datasource.yml.tmpl",
						"containers/observability/grafana/provisioning/dashboards/dashboards.yml.tmpl",
						"containers/observability/grafana/seed-dashboards.sh.tmpl",
						"containers/observability/grafana/dashboards/platform-overview.json.tmpl",
						"containers/observability/grafana/dashboards/lighthouse-inspects-overview.json.tmpl",
						"containers/observability/grafana/dashboards/cache-overview.json.tmpl",
						"containers/observability/grafana/dashboards/storage-overview.json.tmpl",
						"containers/observability/grafana/dashboards/events-overview.json.tmpl",
						"containers/observability/grafana/dashboards/http-overview.json.tmpl",
						"containers/observability/grafana/dashboards/auth-overview.json.tmpl",
						"containers/observability/grafana/dashboards/queue-overview.json.tmpl",
						"containers/observability/grafana/dashboards/scheduler-overview.json.tmpl",
					)
					if p.config.Render.Components.Mail {
						templates = append(templates, "containers/observability/grafana/dashboards/mail-overview.json.tmpl")
					}
					if p.config.Render.Components.HasDatabase() {
						templates = append(templates, "containers/observability/grafana/dashboards/database-overview.json.tmpl")
					}
				}
				if len(templates) == 0 {
					return nil
				}
				return p.writeTemplates(templates)
			},
		},
		{
			title:   "Mail Components Rendering",
			enabled: p.config.Render.Components.Mail,
			templates: []string{
				"internal/mail/README.md.tmpl",
			},
		},
		{
			title:   "Auth Components Rendering",
			enabled: p.config.Render.Components.Auth && p.config.Render.Components.HasDatabase(),
			templates: []string{
				"internal/mail/auth_delivery.go.tmpl",
				"internal/auth/controller.go.tmpl",
				"internal/auth/delivery.go.tmpl",
				"internal/auth/bootstrap_cmd.go.tmpl",
				"internal/auth/create_user_cmd.go.tmpl",
				"internal/auth/email_verification.go.tmpl",
				"internal/auth/login_attempt.go.tmpl",
				"internal/auth/service_integration_test.go.tmpl",
				"internal/auth/password_reset.go.tmpl",
				"internal/auth/set_password_cmd.go.tmpl",
				"internal/auth/service.go.tmpl",
				"internal/auth/session.go.tmpl",
				"internal/auth/user.go.tmpl",
			},
			action: func() error {
				if !input.renderAll {
					if err := p.writeTemplateMappings([]templateMapping{
						mapTemplate("app/routes.go.tmpl"),
						mapTemplateTo("wire/inject_auth.go.tmpl", "app/wire/inject_auth.go"),
					}); err != nil {
						return err
					}
					if err := p.writeTemplateMappingsOnce([]templateMapping{
						mapTemplateTo("wire/inject_http_controllers_app.go.tmpl", "app/wire/inject_http_controllers_app.go"),
					}); err != nil {
						return err
					}
				}
				return p.writeTemplateMappingsOnce([]templateMapping{
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
				})
			},
		},
		{
			title:   "OAuth Components Rendering",
			enabled: p.config.Render.Components.Auth && p.config.Render.Components.OAuth && p.config.Render.Components.HasDatabase(),
			templates: []string{
				"internal/auth/identity.go.tmpl",
				"internal/auth/oauth_provider.go.tmpl",
				"internal/auth/oauth_provider_apple.go.tmpl",
				"internal/auth/oauth_provider_github.go.tmpl",
				"internal/auth/oauth_provider_google.go.tmpl",
				"internal/auth/oauth_provider_microsoft.go.tmpl",
				"internal/auth/oauth_integration_test.go.tmpl",
				"internal/auth/oauth_state.go.tmpl",
			},
			action: func() error {
				return p.writeTemplateMappingsOnce([]templateMapping{
					mapTemplate("migrations/2026_04_09_000006_auth_identities.mysql.up.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000006_auth_identities.mysql.down.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000006_auth_identities.postgres.up.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000006_auth_identities.postgres.down.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000006_auth_identities.sqlite.up.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000006_auth_identities.sqlite.down.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000007_auth_oauth_states.mysql.up.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000007_auth_oauth_states.mysql.down.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000007_auth_oauth_states.postgres.up.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000007_auth_oauth_states.postgres.down.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000007_auth_oauth_states.sqlite.up.sql.tmpl"),
					mapTemplate("migrations/2026_04_09_000007_auth_oauth_states.sqlite.down.sql.tmpl"),
				})
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
			templates: append([]string{
				"internal/database/connections.go.tmpl",
				"internal/database/fingerprinting.go.tmpl",
				"internal/database/gorm_log_writer.go.tmpl",
				"internal/database/connections_test.go.tmpl",
				"internal/database/fingerprinting_test.go.tmpl",
				"internal/cmd/db_shell_cmd.go.tmpl",
				"internal/makecmd/make_model_cmd.go.tmpl",
				"internal/makecmd/make_model_mysql_integration_test.go.tmpl",
				"internal/makecmd/make_model_postgres_integration_test.go.tmpl",
				"internal/makecmd/make_model_sqlite_integration_test.go.tmpl",
				"internal/makecmd/repository_wire_test.go.tmpl",
			}, func() []string {
				if p.config.Render.Components.Metrics {
					return []string{"internal/database/metrics_logger.go.tmpl"}
				}
				return nil
			}()...),
			raw: []string{"internal/makecmd/model.tmpl"},
			action: func() error {
				if !input.renderAll {
					if err := p.writeTemplateMappings([]templateMapping{
						mapTemplateTo("wire/inject_db.go.tmpl", "app/wire/inject_db.go"),
					}); err != nil {
						return err
					}
					if err := p.writeTemplateMappingsOnce([]templateMapping{
						mapTemplateTo("wire/inject_repositories_app.go.tmpl", "app/wire/inject_repositories_app.go"),
					}); err != nil {
						return err
					}
				}
				return p.writeTemplateMappings([]templateMapping{
					mapTemplate("migrations/migrations.go.tmpl"),
					mapTemplate("migrations/migrations_test.go.tmpl"),
					mapTemplate("migrations/migration_connection_test.go.tmpl"),
					mapTemplate("migrations/migration_commands_test.go.tmpl"),
					mapTemplate("migrations/migrate_cmd.go.tmpl"),
					mapTemplate("migrations/migrate_rollback_cmd.go.tmpl"),
					mapTemplate("migrations/.goforj/placeholder.txt.tmpl"),
				})
			},
		},
		{
			title:   "Scheduler Components Rendering",
			enabled: p.config.Render.Components.Scheduler,
			templates: []string{
				"internal/schedules/lighthouse.go.tmpl",
				"internal/schedules/runtime.go.tmpl",
				"internal/schedules/scheduler.go.tmpl",
				"internal/schedules/app_schedules.go.tmpl",
				"internal/schedules/cmd.go.tmpl",
				"internal/schedules/registration.go.tmpl",
				"internal/makecmd/make_schedule_cmd.go.tmpl",
				"internal/makecmd/make_schedule_cmd_test.go.tmpl",
			},
			action: func() error {
				if input.renderAll {
					return nil
				}
				if err := p.writeTemplateMappings([]templateMapping{
					mapTemplateTo("wire/inject_scheduler.go.tmpl", "app/wire/inject_scheduler.go"),
				}); err != nil {
					return err
				}
				return p.writeTemplateMappingsOnce([]templateMapping{
					mapTemplateTo("wire/inject_schedules_app.go.tmpl", "app/wire/inject_schedules_app.go"),
				})
			},
			raw: []string{"internal/makecmd/schedule.tmpl"},
		},
		{
			title:   "Job Components Rendering",
			enabled: p.config.Render.Components.Jobs,
			templates: []string{
				"internal/queues/README.md.tmpl",
				"internal/jobs/example_hello_job.go.tmpl",
				"internal/jobs/example_hello_job_cmd.go.tmpl",
				"internal/jobs/benchmark_run_cmd.go.tmpl",
				"internal/jobs/benchmark_system.go.tmpl",
				"internal/makecmd/make_job_cmd.go.tmpl",
				"internal/makecmd/make_job_cmd_test.go.tmpl",
				"internal/makecmd/make_queue_cmd.go.tmpl",
				"internal/makecmd/make_queue_cmd_test.go.tmpl",
				"internal/jobs/lighthouse.go.tmpl",
				"internal/jobs/lighthouse_benchmark.go.tmpl",
				"internal/jobs/lighthouse_queue.go.tmpl",
				"internal/jobs/runtime.go.tmpl",
				"internal/jobs/worker.go.tmpl",
				"internal/jobs/worker_logger.go.tmpl",
				"internal/jobs/worker_cmd.go.tmpl",
			},
			raw: []string{"internal/makecmd/job.tmpl"},
			action: func() error {
				if input.renderAll {
					return nil
				}
				if err := p.writeTemplateMappings([]templateMapping{
					mapTemplateTo("wire/inject_jobs.go.tmpl", "app/wire/inject_jobs.go"),
				}); err != nil {
					return err
				}
				return p.writeTemplateMappingsOnce([]templateMapping{
					mapTemplateTo("wire/inject_jobs_app.go.tmpl", "app/wire/inject_jobs_app.go"),
				})
			},
		},
		{
			title:   "Named App Target Rendering",
			enabled: input.renderAll,
			action:  p.renderNamedAppTargets,
		},
	}

	for _, step := range steps {
		if !step.enabled {
			continue
		}

		before := p.stats.counts()
		started := time.Now()

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

		p.printStepSummary(step.title, before, time.Since(started))
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

// RenderAppTargetOnly renders one named app target without replaying the full project scaffold.
func (p *ProjectRenderer) RenderAppTargetOnly(target project.AppTarget, opts makeapp.RenderOptions) error {
	p.stats = &renderStats{}
	p.lines = nil

	cfg, err := project.LoadProjectConfig()
	if err != nil {
		return err
	}
	p.config = cfg
	if p.config.Render.Components.DemoApp {
		p.config.Render.Components.Auth = true
		p.config.Render.StarterKit = project.StarterKitNone
	}
	p.config.Render.Components.ResolveDependencies()
	if err := p.config.Render.Components.ValidateRenderContract(); err != nil {
		return err
	}
	p.config.Render.StarterKit = project.NormalizeStarterKit(p.config.Render.StarterKit)
	if !p.config.Render.Components.WebUI {
		p.config.Render.StarterKit = project.StarterKitNone
	}
	if err := project.ValidateStarterKitContract(p.config.Render.StarterKit, p.config.Render.Components); err != nil {
		return err
	}
	if err := p.migrateAppOwnedWireFilenames(); err != nil {
		return err
	}
	if err := p.syncLegacyGeneratedTemplates(); err != nil {
		return err
	}
	promotedProjectComponents := false
	if target.Name != "" && target.Name != project.DefaultAppTargetName {
		promoted, err := p.setAppTargetConfig(target.Name, opts.Components, opts.StarterKit)
		if err != nil {
			return err
		}
		promotedProjectComponents = promoted
		if err := p.writeTargetEnvDefaults(target, targetRenderComponents(p.config, target)); err != nil {
			return err
		}
		if err := writeProjectConfig(".goforj.yml", p.config); err != nil {
			return err
		}
	}

	if err := p.renderAppTarget(target); err != nil {
		return err
	}
	if promotedProjectComponents {
		return p.Render(ComponentRenderInput{renderAll: true})
	}
	if err := p.writeTemplates([]string{
		"internal/runtime/targets.go.tmpl",
		"internal/runtime/targets_test.go.tmpl",
	}); err != nil {
		return err
	}
	if p.config.Render.Components.HasDatabase() {
		if err := p.expandDefaultMigrationsForNamedTargets(); err != nil {
			return err
		}
	}
	if !opts.SkipWire {
		if err := p.runWireGenerate(); err != nil {
			return err
		}
	}
	return nil
}

// RemoveAppTarget removes conventional target-owned files and refreshes generated target metadata.
func (p *ProjectRenderer) RemoveAppTarget(target project.AppTarget) (makeapp.RemoveResult, error) {
	p.stats = &renderStats{}
	p.lines = nil

	target = normalizeRenderAppTarget(target)
	if target.Name == "" || target.Name == project.DefaultAppTargetName {
		return makeapp.RemoveResult{}, fmt.Errorf("default app target cannot be removed")
	}

	cfg, err := project.LoadProjectConfig()
	if err != nil {
		return makeapp.RemoveResult{}, err
	}
	p.config = cfg

	var result makeapp.RemoveResult
	for _, path := range []string{
		target.AppDir,
		appTargetFrontendDir(target),
	} {
		removed, err := removeDirIfExists(path)
		if err != nil {
			return result, err
		}
		if removed {
			result.Removed = append(result.Removed, path)
		}
	}
	for _, path := range []string{
		target.Entrypoint,
		filepath.Join("bin", target.Name),
	} {
		removed, err := removeFileIfExists(path)
		if err != nil {
			return result, err
		}
		if removed {
			result.Removed = append(result.Removed, path)
		}
	}

	cmdDir := filepath.Dir(target.Entrypoint)
	removed, err := removeEmptyDirIfEmpty(cmdDir)
	if err != nil {
		return result, err
	}
	if removed {
		result.Removed = append(result.Removed, cmdDir)
	}
	for _, path := range []string{".env", ".env.host"} {
		updated, err := removeTargetEnvDefaults(path, target.Name)
		if err != nil {
			return result, err
		}
		if updated {
			result.Updated = append(result.Updated, path)
		}
	}
	if p.removeAppTargetConfig(target.Name) {
		if err := writeProjectConfig(".goforj.yml", p.config); err != nil {
			return result, err
		}
		result.Updated = append(result.Updated, ".goforj.yml")
	}
	if !result.Changed() {
		return result, nil
	}
	if err := p.writeTemplates([]string{
		"internal/runtime/targets.go.tmpl",
		"internal/runtime/targets_test.go.tmpl",
	}); err != nil {
		return result, err
	}
	result.Updated = append(result.Updated,
		filepath.Join("internal", "runtime", "targets.go"),
		filepath.Join("internal", "runtime", "targets_test.go"),
	)
	return result, nil
}

// removeAppTargetConfig forgets target-local render choices without downgrading project capabilities.
func (p *ProjectRenderer) removeAppTargetConfig(name string) bool {
	if p.config == nil || p.config.AppTargets == nil {
		return false
	}
	if _, ok := p.config.AppTargets[name]; !ok {
		return false
	}
	delete(p.config.AppTargets, name)
	if len(p.config.AppTargets) == 0 {
		p.config.AppTargets = nil
	}
	return true
}

// setAppTargetConfig persists target participation so future full renders keep the same target shape.
func (p *ProjectRenderer) setAppTargetConfig(name string, components project.Components, starterKit project.StarterKit) (bool, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == project.DefaultAppTargetName {
		return false, nil
	}
	if p.config.AppTargets == nil {
		p.config.AppTargets = map[string]project.AppTargetConfig{}
	}
	if components == (project.Components{}) {
		components = project.TargetDefaultComponents(p.config.Render.Components)
	}
	components = project.NormalizeTargetComponents(p.config.Render.Components, components)
	if err := components.ValidateRenderContract(); err != nil {
		return false, err
	}
	starterKit = project.NormalizeStarterKit(starterKit)
	if !components.WebUI {
		starterKit = project.StarterKitNone
	}
	if err := project.ValidateStarterKitContract(starterKit, components); err != nil {
		return false, err
	}
	before := p.config.Render.Components
	p.config.Render.Components = project.PromoteTargetComponents(p.config.Render.Components, components)
	if err := p.config.Render.Components.ValidateRenderContract(); err != nil {
		return false, err
	}
	p.config.AppTargets[name] = project.AppTargetConfig{
		Components: components,
		StarterKit: starterKit,
	}
	return p.config.Render.Components != before, nil
}

// writeTargetEnvDefaults appends target-scoped env defaults without changing the default App values.
func (p *ProjectRenderer) writeTargetEnvDefaults(target project.AppTarget, components project.Components) error {
	if target.Name == "" || target.Name == project.DefaultAppTargetName {
		return nil
	}
	prefix := strEnvPrefix(target.Name)
	if prefix == "" {
		return nil
	}

	metadata := runtimeTargetMetadataForTarget(target)
	envDefaults := targetRuntimeEnvDefaults(prefix, metadata, components)
	if driver := components.DatabaseDriver(); driver != "" {
		baseDriver := ""
		if p.config != nil {
			baseDriver = p.config.Render.Components.DatabaseDriver()
		}
		envDefaults = mergeEnvDefaults(envDefaults, targetDatabaseEnvDefaults(prefix, driver, baseDriver, false))
	}
	envGlobals, envTargetDefaults := splitEnvDefaultsByPrefix(envDefaults, prefix)
	if err := upsertEnvDefaults(".env", envGlobals); err != nil {
		return err
	}
	if err := upsertTargetEnvDefaults(".env", target.Name, prefix, envTargetDefaults); err != nil {
		return err
	}

	hostDefaults := map[string]string{}
	if driver := components.DatabaseDriver(); driver != "" {
		baseDriver := ""
		if p.config != nil {
			baseDriver = p.config.Render.Components.DatabaseDriver()
		}
		hostDefaults = targetDatabaseEnvDefaults(prefix, driver, baseDriver, true)
	}
	delete(hostDefaults, "DB_SUPPORTED_DRIVERS")
	hostGlobals, hostTargetDefaults := splitEnvDefaultsByPrefix(hostDefaults, prefix)
	if err := upsertEnvDefaults(".env.host", hostGlobals); err != nil {
		return err
	}
	if err := upsertTargetEnvDefaults(".env.host", target.Name, prefix, hostTargetDefaults); err != nil {
		return err
	}
	return nil
}

// mergeEnvDefaults combines generated env defaults while letting later defaults win.
func mergeEnvDefaults(base map[string]string, overlays ...map[string]string) map[string]string {
	merged := make(map[string]string, len(base))
	for key, value := range base {
		merged[key] = value
	}
	for _, overlay := range overlays {
		for key, value := range overlay {
			merged[key] = value
		}
	}
	return merged
}

// splitEnvDefaultsByPrefix keeps global defaults out of target-specific env sections.
func splitEnvDefaultsByPrefix(defaults map[string]string, prefix string) (map[string]string, map[string]string) {
	globals := map[string]string{}
	targetDefaults := map[string]string{}
	targetPrefix := prefix + "_"
	for key, value := range defaults {
		if strings.HasPrefix(key, targetPrefix) {
			targetDefaults[key] = value
			continue
		}
		globals[key] = value
	}
	return globals, targetDefaults
}

// targetRuntimeEnvDefaults writes only target values that app owners commonly edit.
func targetRuntimeEnvDefaults(prefix string, metadata runtimeTargetMetadata, components project.Components) map[string]string {
	values := map[string]string{}
	if components.WebAPI || components.WebUI {
		values[prefix+"_APP_URL"] = fmt.Sprintf("http://localhost:%d", metadata.HTTPPort)
		values[prefix+"_API_HTTP_PORT"] = strconv.Itoa(metadata.HTTPPort)
	}
	return values
}

// targetDatabaseEnvDefaults creates conventional env keys for one target database driver.
func targetDatabaseEnvDefaults(prefix string, driver string, baseDriver string, host bool) map[string]string {
	values := map[string]string{}
	if !host {
		values["DB_SUPPORTED_DRIVERS"] = driver
		values[prefix+"_DB_DATABASE"] = targetDatabaseName(prefix, driver)
		if driver != "sqlite" {
			values[prefix+"_DB_SQLITE_DATABASE"] = targetDatabaseName(prefix, "sqlite")
		}
	}
	if baseDriver != "" && driver == baseDriver {
		return values
	}
	if !host {
		values[prefix+"_DB_DRIVER"] = driver
	}
	switch driver {
	case "mysql":
		values[prefix+"_DB_HOST"] = targetDatabaseHost("mysql", host)
		if !host {
			values[prefix+"_DB_USERNAME"] = "user"
			values[prefix+"_DB_PASSWORD"] = "password"
			values[prefix+"_DB_PORT"] = "3306"
		}
	case "postgres":
		values[prefix+"_DB_HOST"] = targetDatabaseHost("postgres", host)
		if !host {
			values[prefix+"_DB_USERNAME"] = "postgres"
			values[prefix+"_DB_PASSWORD"] = "postgres"
			values[prefix+"_DB_PORT"] = "5432"
		}
	case "sqlite":
	}
	return values
}

// targetDatabaseName keeps target database names compact while preserving SQLite paths.
func targetDatabaseName(prefix string, driver string) string {
	if driver == "sqlite" {
		return "./_data/sqlite/" + strings.ToLower(prefix) + ".db"
	}
	return prefixDatabaseName(prefix, "db")
}

// targetDatabaseHost maps container defaults to localhost when writing host override env.
func targetDatabaseHost(service string, host bool) string {
	if host {
		return "localhost"
	}
	return service
}

// prefixDatabaseName keeps generated target database names compact and deterministic.
func prefixDatabaseName(prefix string, fallback string) string {
	name := strings.ToLower(strings.ReplaceAll(prefix, "_", ""))
	if name == "" {
		return fallback
	}
	return name
}

// strEnvPrefix converts target slugs into env-safe prefixes without relying on config.
func strEnvPrefix(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '-' || r == '_' || r == ' ' || r == '.'
	})
	for i, part := range parts {
		parts[i] = strings.ToUpper(part)
	}
	return strings.Join(parts, "_")
}

// upsertEnvDefaults preserves existing env files while filling missing target defaults.
func upsertEnvDefaults(path string, defaults map[string]string) error {
	if len(defaults) == 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	text := string(data)
	lines := strings.Split(text, "\n")
	for key, value := range defaults {
		if key == "DB_SUPPORTED_DRIVERS" {
			lines = upsertSupportedDriver(lines, value)
			continue
		}
		lines = upsertEnvLine(lines, key, value)
	}
	content := strings.Join(lines, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// upsertTargetEnvDefaults groups target overrides so multi-app env files remain readable.
func upsertTargetEnvDefaults(path string, targetName string, prefix string, defaults map[string]string) error {
	if len(defaults) == 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	lines := strings.Split(string(data), "\n")
	lines = removeEnvKeys(lines, defaults)
	lines = removeEnvSectionHeader(lines, targetName)
	lines = trimTrailingBlankLines(lines)
	if len(lines) > 0 {
		lines = append(lines, "")
	}
	lines = append(lines, "# "+envSectionTitle(targetName))
	for _, key := range orderedEnvDefaultKeys(defaults, prefix) {
		lines = append(lines, key+"="+defaults[key])
	}
	content := strings.Join(lines, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// removeEnvKeys prevents old loose target entries from surviving after sectioned rendering.
func removeEnvKeys(lines []string, defaults map[string]string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		key, _, ok := parseEnvLine(line)
		if ok {
			if _, exists := defaults[key]; exists {
				continue
			}
		}
		out = append(out, line)
	}
	return out
}

// removeEnvSectionHeader avoids duplicating the generated heading on repeated renders.
func removeEnvSectionHeader(lines []string, targetName string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if isEnvSectionHeader(line, targetName) {
			continue
		}
		out = append(out, line)
	}
	return out
}

// removeTargetEnvDefaults deletes target-prefixed overrides when an app target is removed.
func removeTargetEnvDefaults(path string, targetName string) (bool, error) {
	prefix := strEnvPrefix(targetName)
	if prefix == "" {
		return false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	original := string(data)
	lines := strings.Split(original, "\n")
	out := make([]string, 0, len(lines))
	targetPrefix := prefix + "_"
	for _, line := range lines {
		key, _, ok := parseEnvLine(line)
		if ok && strings.HasPrefix(key, targetPrefix) {
			continue
		}
		if isEnvSectionHeader(line, targetName) {
			continue
		}
		out = append(out, line)
	}
	out = collapseBlankLines(trimTrailingBlankLines(out))
	content := strings.Join(out, "\n")
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if content == original {
		return false, nil
	}
	return true, os.WriteFile(path, []byte(content), 0o644)
}

// isEnvSectionHeader recognizes generated target headings case-insensitively for cleanup.
func isEnvSectionHeader(line string, targetName string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "#") {
		return false
	}
	title := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
	return strings.EqualFold(title, envSectionTitle(targetName)) || strings.EqualFold(title, targetName)
}

// collapseBlankLines removes empty gaps left after deleting target env sections.
func collapseBlankLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	previousBlank := false
	for _, line := range lines {
		blank := strings.TrimSpace(line) == ""
		if blank && previousBlank {
			continue
		}
		out = append(out, line)
		previousBlank = blank
	}
	return out
}

// trimTrailingBlankLines keeps generated sections separated by one predictable blank line.
func trimTrailingBlankLines(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// orderedEnvDefaultKeys writes generated target env keys in the same order every render.
func orderedEnvDefaultKeys(defaults map[string]string, prefix string) []string {
	keys := make([]string, 0, len(defaults))
	for key := range defaults {
		keys = append(keys, key)
	}
	rank := map[string]int{
		prefix + "_APP_URL":                10,
		prefix + "_API_HTTP_PORT":          20,
		prefix + "_METRICS_API_PORT":       30,
		prefix + "_METRICS_SCHEDULER_PORT": 40,
		prefix + "_METRICS_JOBS_PORT":      50,
		prefix + "_DB_DRIVER":              60,
		prefix + "_DB_DATABASE":            70,
		prefix + "_DB_SQLITE_DATABASE":     75,
		prefix + "_DB_HOST":                80,
		prefix + "_DB_PORT":                90,
		prefix + "_DB_USERNAME":            100,
		prefix + "_DB_PASSWORD":            110,
	}
	sort.Slice(keys, func(left, right int) bool {
		leftRank, leftKnown := rank[keys[left]]
		rightRank, rightKnown := rank[keys[right]]
		if leftKnown != rightKnown {
			return leftKnown
		}
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return keys[left] < keys[right]
	})
	return keys
}

// envSectionTitle formats target slugs as readable env section headings.
func envSectionTitle(targetName string) string {
	parts := strings.FieldsFunc(targetName, func(r rune) bool {
		return r == '-' || r == '_' || r == ' ' || r == '.'
	})
	for idx, part := range parts {
		part = strings.ToLower(part)
		if part == "" {
			continue
		}
		parts[idx] = strings.ToUpper(part[:1]) + part[1:]
	}
	title := strings.Join(parts, " ")
	if title == "" {
		return targetName
	}
	return title
}

// upsertSupportedDriver appends a database driver while preserving existing driver order.
func upsertSupportedDriver(lines []string, driver string) []string {
	for idx, line := range lines {
		key, value, ok := parseEnvLine(line)
		if !ok || key != "DB_SUPPORTED_DRIVERS" {
			continue
		}
		drivers := appendDriver(value, driver)
		lines[idx] = key + "=" + strings.Join(drivers, ",")
		return lines
	}
	return append(lines, "DB_SUPPORTED_DRIVERS="+driver)
}

// appendDriver normalizes a comma-separated driver list before adding a new driver.
func appendDriver(value string, driver string) []string {
	seen := map[string]bool{}
	var out []string
	for _, part := range strings.Split(value, ",") {
		normalized := strings.ToLower(strings.TrimSpace(part))
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
	}
	driver = strings.ToLower(strings.TrimSpace(driver))
	if driver != "" && !seen[driver] {
		out = append(out, driver)
	}
	return out
}

// upsertEnvLine replaces an existing env key or appends it when missing.
func upsertEnvLine(lines []string, key string, value string) []string {
	for idx, line := range lines {
		currentKey, _, ok := parseEnvLine(line)
		if !ok || currentKey != key {
			continue
		}
		lines[idx] = key + "=" + value
		return lines
	}
	return append(lines, key+"="+value)
}

// parseEnvLine reads simple env assignments while ignoring comments and blank lines.
func parseEnvLine(line string) (string, string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", "", false
	}
	key, value, ok := strings.Cut(trimmed, "=")
	if !ok {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if comment := strings.Index(value, "#"); comment >= 0 {
		value = strings.TrimSpace(value[:comment])
	}
	return key, value, key != ""
}

// SetTimings controls whether render summaries include elapsed phase timings.
func (p *ProjectRenderer) SetTimings(enabled bool) {
	p.timings = enabled
}

func (p *ProjectRenderer) timeRenderStage(name string, fn func() error) error {
	timingEnabled := p.renderTimingEnabled()
	if !timingEnabled {
		return fn()
	}
	lineStart := len(p.lines)
	started := time.Now()
	err := fn()
	elapsed := time.Since(started)
	if err == nil {
		for i := lineStart; i < len(p.lines); i++ {
			p.lines[i] = appendRenderTiming(p.lines[i], elapsed)
		}
		p.flushRenderLines(lineStart)
		if renderDebugEnabled() {
			console.Debugf("render stage %s: %s", name, elapsed)
		}
	}
	return err
}

func (p *ProjectRenderer) renderTimingEnabled() bool {
	if p != nil && p.timings {
		return true
	}
	for _, key := range []string{"FORJ_RENDER_TIMINGS", "FORJ_RENDER_DEBUG_TIMINGS"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" && value != "0" {
			return true
		}
	}
	return false
}

func (p *ProjectRenderer) streamRenderTimings() bool {
	return runningInsideDevCommand() && p.renderTimingEnabled()
}

func (p *ProjectRenderer) appendRenderLine(line string) {
	p.lines = append(p.lines, line)
	p.flushRenderLines(len(p.lines) - 1)
}

func (p *ProjectRenderer) flushRenderLines(start int) {
	if !p.streamRenderTimings() || start >= len(p.lines) {
		return
	}
	for _, line := range p.lines[start:] {
		fmt.Println(line)
	}
	p.lines = p.lines[:start]
}

// migrateGeneratedEnvDefault updates old generated defaults without overriding custom app owner values.
func migrateGeneratedEnvDefault(lines []string, key string, oldValue string, newValue string) ([]string, bool) {
	prefix := key + "="
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || !strings.HasPrefix(trimmed, prefix) {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		if value != oldValue {
			return lines, false
		}
		updated := make([]string, len(lines))
		copy(updated, lines)
		updated[i] = key + "=" + newValue
		return updated, true
	}
	return lines, false
}

func renderDebugEnabled() bool {
	for _, key := range []string{"FORJ_DEBUG", "DEBUG"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" && value != "0" {
			return true
		}
	}
	return false
}

func (p *ProjectRenderer) syncProjectConfigForRender() error {
	if p.config == nil {
		return nil
	}
	changed := false
	defaultTarget := project.DefaultAppTarget()
	if len(p.config.Dev.WirePaths) == 0 || len(p.config.Dev.WirePaths) == 1 && p.config.Dev.WirePaths[0] == "wire" {
		p.config.Dev.WirePaths = []string{defaultTarget.WireDir}
		changed = true
	}
	if removeLegacyInitialBuildTask(&p.config.Dev.Pre) {
		changed = true
	}
	for i := range p.config.Dev.Watches {
		normalized := normalizeDevWatchWireGenExclusion(p.config.Dev.Watches[i].Watch)
		if normalized != p.config.Dev.Watches[i].Watch {
			p.config.Dev.Watches[i].Watch = normalized
			changed = true
		}
	}
	if p.config.Render.Components.WebUI && p.config.Render.StarterKit == project.StarterKitVue && !p.config.Render.Components.DemoApp {
		task := project.DevTask{
			Name: "Install Frontend Dependencies",
			Cmd:  "cd " + filepath.ToSlash(defaultFrontendDir()) + " && npm install",
		}
		if !hasDevTask(p.config.Dev.Pre, task) {
			p.config.Dev.Pre = append(p.config.Dev.Pre, task)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return writeProjectConfig(".goforj.yml", p.config)
}

// normalizeDevWatchWireGenExclusion keeps the generated wire exclusion stable across repeated renders.
func normalizeDevWatchWireGenExclusion(watch string) string {
	normalized := strings.ReplaceAll(watch, "-xfile wire/wire_gen\\.go$", "-xfile app/wire/wire_gen\\.go$")
	for strings.Contains(normalized, "app/app/") {
		normalized = strings.ReplaceAll(normalized, "app/app/", "app/")
	}
	return normalized
}

// removeLegacyInitialBuildTask removes the old single-app bootstrap build now owned by forj dev.
func removeLegacyInitialBuildTask(tasks *[]project.DevTask) bool {
	if tasks == nil {
		return false
	}
	filtered := (*tasks)[:0]
	removed := false
	for _, task := range *tasks {
		if strings.TrimSpace(task.Name) == "Initial build" && strings.TrimSpace(task.Cmd) == "forj build -o ./bin/app" {
			removed = true
			continue
		}
		filtered = append(filtered, task)
	}
	*tasks = filtered
	return removed
}

func hasDevTask(tasks []project.DevTask, target project.DevTask) bool {
	for _, task := range tasks {
		if strings.TrimSpace(task.Name) == target.Name && strings.TrimSpace(task.Cmd) == target.Cmd {
			return true
		}
	}
	return false
}

// renderNamedAppTargets renders every non-default target discovered from conventional project layout.
func (p *ProjectRenderer) renderNamedAppTargets() error {
	targets, err := p.namedAppRenderTargets()
	if err != nil {
		return err
	}
	if len(targets) == 1 {
		target := targets[0]
		if err := p.renderAppTarget(target); err != nil {
			return fmt.Errorf("render app target %s: %w", target.Name, err)
		}
	} else if len(targets) > 1 {
		errs := make([]error, len(targets))
		var wg sync.WaitGroup
		for i, target := range targets {
			wg.Add(1)
			go func(i int, target project.AppTarget) {
				defer wg.Done()
				if err := p.renderAppTarget(target); err != nil {
					errs[i] = fmt.Errorf("render app target %s: %w", target.Name, err)
				}
			}(i, target)
		}
		wg.Wait()
		for _, err := range errs {
			if err != nil {
				return err
			}
		}
	}
	if len(targets) > 0 && p.config.Render.Components.HasDatabase() {
		if err := p.expandDefaultMigrationsForNamedTargets(); err != nil {
			return err
		}
	}
	return nil
}

// expandDefaultMigrationsForNamedTargets moves single-target migration streams into the explicit default target layout.
func (p *ProjectRenderer) expandDefaultMigrationsForNamedTargets() error {
	root := "migrations"
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		name := entry.Name()
		source := filepath.Join(root, name)
		if entry.IsDir() {
			if shouldSkipMigrationExpansionDir(name) {
				continue
			}
			if err := moveDirectMigrationSQLFiles(source, filepath.Join(root, "app", name)); err != nil {
				return err
			}
			continue
		}
		if !isMigrationSQLFile(name) {
			continue
		}
		if err := moveMigrationFile(source, filepath.Join(root, "app", "default", name)); err != nil {
			return err
		}
	}
	return nil
}

// shouldSkipMigrationExpansionDir preserves metadata and already-expanded target directories.
func shouldSkipMigrationExpansionDir(name string) bool {
	return name == ".goforj" || name == "app" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

// moveDirectMigrationSQLFiles moves only legacy direct SQL files and leaves nested target layouts alone.
func moveDirectMigrationSQLFiles(sourceDir, destDir string) error {
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}
	moved := false
	for _, entry := range entries {
		if entry.IsDir() || !isMigrationSQLFile(entry.Name()) {
			continue
		}
		if err := moveMigrationFile(filepath.Join(sourceDir, entry.Name()), filepath.Join(destDir, entry.Name())); err != nil {
			return err
		}
		moved = true
	}
	if !moved {
		return nil
	}

	remaining, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}
	if len(remaining) == 0 {
		return os.Remove(sourceDir)
	}
	return nil
}

// moveMigrationFile moves one migration file unless a different destination already exists.
func moveMigrationFile(source, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(dest); err == nil {
		same, err := filesHaveSameContent(source, dest)
		if err != nil {
			return err
		}
		if same {
			return os.Remove(source)
		}
		return fmt.Errorf("migration expansion destination already exists with different content: %s", dest)
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(source, dest)
}

// filesHaveSameContent prevents migration expansion from overwriting user-edited migration files.
func filesHaveSameContent(left, right string) (bool, error) {
	leftBytes, err := os.ReadFile(left)
	if err != nil {
		return false, err
	}
	rightBytes, err := os.ReadFile(right)
	if err != nil {
		return false, err
	}
	return bytes.Equal(leftBytes, rightBytes), nil
}

// isMigrationSQLFile reports whether a file belongs to a generated SQL migration pair.
func isMigrationSQLFile(name string) bool {
	return strings.HasSuffix(name, ".up.sql") || strings.HasSuffix(name, ".down.sql")
}

// namedAppRenderTargets discovers named targets from conventional project layout only.
func (p *ProjectRenderer) namedAppRenderTargets() ([]project.AppTarget, error) {
	seen := map[string]bool{project.DefaultAppTargetName: true}
	targets := make([]project.AppTarget, 0)
	add := func(target project.AppTarget) {
		target = normalizeRenderAppTarget(target)
		if target.Name == "" || seen[target.Name] || !project.IsSafeAppTargetName(target.Name) || project.IsReservedAppTargetName(target.Name) {
			return
		}
		seen[target.Name] = true
		targets = append(targets, target)
	}

	for _, target := range discoverConventionalAppTargets() {
		add(target)
	}

	return targets, nil
}

// discoverConventionalAppTargets treats existing cmd/<target> and app/<target> layouts as source of truth.
func discoverConventionalAppTargets() []project.AppTarget {
	names := make(map[string]bool)
	if entries, err := os.ReadDir("cmd"); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if name == project.DefaultAppTargetName || !project.IsSafeAppTargetName(name) {
				continue
			}
			if _, err := os.Stat(filepath.Join("cmd", name, "main.go")); err == nil {
				names[name] = true
			}
		}
	}
	if entries, err := os.ReadDir("app"); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if name == project.DefaultAppTargetName || !project.IsSafeAppTargetName(name) || project.IsReservedAppTargetName(name) {
				continue
			}
			if hasConventionalAppTargetFiles(filepath.Join("app", name)) {
				names[name] = true
			}
		}
	}

	targets := make([]project.AppTarget, 0, len(names))
	for name := range names {
		targets = append(targets, project.DefaultNamedAppTarget(name))
	}
	sort.Slice(targets, func(i, j int) bool {
		return targets[i].Name < targets[j].Name
	})
	return targets
}

// hasConventionalAppTargetFiles avoids treating arbitrary app subpackages as targets unless they look target-owned.
func hasConventionalAppTargetFiles(appDir string) bool {
	for _, path := range []string{
		filepath.Join(appDir, "wire"),
		filepath.Join(appDir, "commands.go"),
		filepath.Join(appDir, "root_cmd.go"),
		filepath.Join(appDir, "routes.go"),
		filepath.Join(appDir, "schedules.go"),
		filepath.Join(appDir, "lifecycle.go"),
	} {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

// normalizeRenderAppTarget fills missing paths from the framework target conventions.
func normalizeRenderAppTarget(target project.AppTarget) project.AppTarget {
	if strings.TrimSpace(target.Name) == "" {
		return project.DefaultAppTarget()
	}
	namedDefault := project.DefaultNamedAppTarget(target.Name)
	if target.Entrypoint == "" {
		target.Entrypoint = namedDefault.Entrypoint
	}
	if target.AppDir == "" {
		target.AppDir = namedDefault.AppDir
	}
	if target.WireDir == "" {
		target.WireDir = namedDefault.WireDir
	}
	return target
}

// defaultFrontendDir returns the editable frontend root for the conventional default target.
func defaultFrontendDir() string {
	return appTargetFrontendDir(project.DefaultAppTarget())
}

// appTargetFrontendDir keeps frontend source next to the command package that embeds its build output.
func appTargetFrontendDir(target project.AppTarget) string {
	target = normalizeRenderAppTarget(target)
	return filepath.Join(filepath.Dir(target.Entrypoint), "frontend")
}

// appTargetFrontendDistIndex returns the target-local placeholder path required by go:embed.
func appTargetFrontendDistIndex(target project.AppTarget) string {
	return filepath.Join(appTargetFrontendDir(target), "dist", "index.html")
}

// renderAppTarget writes the target entrypoint, composition files, and target-local Wire graph.
func (p *ProjectRenderer) renderAppTarget(target project.AppTarget) error {
	target = normalizeRenderAppTarget(target)
	if err := p.writeTemplateMappingsForTarget(target, p.appTargetFrameworkMappings(target)); err != nil {
		return err
	}
	if err := p.migrateFrontendDistPlaceholder(target); err != nil {
		return err
	}
	if err := p.writeTemplateMappingsOnceForTarget(target, p.appTargetAppOwnedMappings(target)); err != nil {
		return err
	}
	if err := p.ensureFrontendPlaceholderAssets(target); err != nil {
		return err
	}
	if target.Name != project.DefaultAppTargetName {
		return p.scaffoldTargetStarterKit(target)
	}
	return nil
}

// migrateFrontendDistPlaceholder updates the old generated "no frontend deployed" page without touching real SPA builds.
func (p *ProjectRenderer) migrateFrontendDistPlaceholder(target project.AppTarget) error {
	if p.config == nil || !targetRenderComponents(p.config, target).WebUI {
		return nil
	}
	target = normalizeRenderAppTarget(target)
	path := appTargetFrontendDistIndex(target)
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !isGeneratedFrontendDistPlaceholderNeedingRefresh(string(content), p.config.ProjectName) {
		return nil
	}
	if err := p.renderTemplateFile(path, "frontend/dist/index.html.tmpl", templateDataForTarget(p.config, target)); err != nil {
		return err
	}
	return p.ensureFrontendPlaceholderAssets(target)
}

// ensureFrontendPlaceholderAssets copies static assets only when the generated fallback page references them.
func (p *ProjectRenderer) ensureFrontendPlaceholderAssets(target project.AppTarget) error {
	if p.config == nil || !targetRenderComponents(p.config, target).WebUI {
		return nil
	}
	target = normalizeRenderAppTarget(target)
	index := appTargetFrontendDistIndex(target)
	content, err := os.ReadFile(index)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	contentString := string(content)
	if strings.Contains(contentString, frontendPlaceholderLogoName) {
		if err := p.copyFrontendPlaceholderAsset(filepath.Join(filepath.Dir(index), frontendPlaceholderLogoName), frontendPlaceholderLogoTemplate); err != nil {
			return err
		}
	}
	return nil
}

// isGeneratedFrontendDistPlaceholderNeedingRefresh recognizes generated fallback pages that predate the current placeholder.
func isGeneratedFrontendDistPlaceholderNeedingRefresh(content string, projectName string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == strings.TrimSpace(oldFrontendDistPlaceholder(projectName)) {
		return true
	}
	oldPlaceholderCopy := "This app " + "target is running, but no frontend " + "build has been deployed yet."
	oldSubtitle := "Application " + "target"
	previousSubtitle := "Ready for your " + "frontend"
	previousCopy := "Your app " + "is running. Add your " + "frontend to make this page yours."
	previousPolishedCopy := "Your application is accepting requests and ready to go."
	hasGeneratedPlaceholderCopy := strings.Contains(trimmed, oldPlaceholderCopy) ||
		strings.Contains(trimmed, previousCopy) ||
		strings.Contains(trimmed, "Your app is live. Create the experience that belongs here.") ||
		strings.Contains(trimmed, "Your app is live. Build the interface that belongs here.") ||
		strings.Contains(trimmed, previousPolishedCopy)
	if !hasGeneratedPlaceholderCopy || !strings.Contains(trimmed, "GoForj") {
		return false
	}
	legacyLogoName := "goforj-" + "v7.png"
	return strings.Contains(trimmed, `<span class="mark">G</span>`) ||
		strings.Contains(trimmed, legacyLogoName) ||
		!strings.Contains(trimmed, "brand-tagline") ||
		strings.Contains(trimmed, oldSubtitle) ||
		strings.Contains(trimmed, previousSubtitle) ||
		!strings.Contains(trimmed, "Composable apps for Go") ||
		!strings.Contains(trimmed, `class="app-meta"`) ||
		!strings.Contains(trimmed, `class="core"`) ||
		!strings.Contains(trimmed, `class="cube"`) ||
		!strings.Contains(trimmed, "Read the docs") ||
		!strings.Contains(trimmed, `class="visual"`) ||
		!strings.Contains(trimmed, `class="status"`) ||
		!strings.Contains(trimmed, `rel="icon"`)
}

// oldFrontendDistPlaceholder matches the previous generated placeholder so migrations do not overwrite custom HTML.
func oldFrontendDistPlaceholder(projectName string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Title</title>

    You've not deployed anything for %s yet.
</head>
<body>

</body>
</html>`, projectName)
}

// appTargetFrameworkMappings returns files the CLI can safely refresh on every render.
func (p *ProjectRenderer) appTargetFrameworkMappings(target project.AppTarget) []templateMapping {
	components := targetRenderComponents(p.config, target)
	mappings := []templateMapping{
		mapTemplateTo("cmd/app/main.go.tmpl", target.Entrypoint),
		mapTemplateTo("app/root_cmd.go.tmpl", filepath.Join(target.AppDir, "root_cmd.go")),
		mapTemplateTo("wire/app.go.tmpl", filepath.Join(target.WireDir, "app.go")),
		mapTemplateTo("wire/app_test.go.tmpl", filepath.Join(target.WireDir, "app_test.go")),
		mapTemplateTo("wire/inject_cmd.go.tmpl", filepath.Join(target.WireDir, "inject_cmd.go")),
		mapTemplateTo("wire/inject_managers.go.tmpl", filepath.Join(target.WireDir, "inject_managers.go")),
		mapTemplateTo("wire/wire.go.tmpl", filepath.Join(target.WireDir, "wire.go")),
	}
	if components.HasDatabase() {
		mappings = append(mappings, mapTemplateTo("wire/inject_db.go.tmpl", filepath.Join(target.WireDir, "inject_db.go")))
	}
	if components.Auth && components.HasDatabase() {
		mappings = append(mappings, mapTemplateTo("wire/inject_auth.go.tmpl", filepath.Join(target.WireDir, "inject_auth.go")))
	}
	if components.WebAPI || components.WebUI {
		mappings = append(mappings, mapTemplateTo("wire/inject_http.go.tmpl", filepath.Join(target.WireDir, "inject_http.go")))
	}
	if components.Scheduler {
		mappings = append(mappings, mapTemplateTo("wire/inject_scheduler.go.tmpl", filepath.Join(target.WireDir, "inject_scheduler.go")))
	}
	if components.Jobs {
		mappings = append(mappings, mapTemplateTo("wire/inject_jobs.go.tmpl", filepath.Join(target.WireDir, "inject_jobs.go")))
	}
	return mappings
}

// appTargetAppOwnedMappings returns customization files that should be created once and then preserved.
func (p *ProjectRenderer) appTargetAppOwnedMappings(target project.AppTarget) []templateMapping {
	components := targetRenderComponents(p.config, target)
	mappings := []templateMapping{
		mapTemplateTo("app/lifecycle.go.tmpl", filepath.Join(target.AppDir, "lifecycle.go")),
		mapTemplateTo("app/commands.go.tmpl", filepath.Join(target.AppDir, "commands.go")),
		mapTemplateTo("wire/inject_services_app.go.tmpl", filepath.Join(target.WireDir, "inject_services_app.go")),
		mapTemplateTo("wire/inject_subscribers_app.go.tmpl", filepath.Join(target.WireDir, "inject_subscribers_app.go")),
		mapTemplateTo("wire/inject_cmd_app.go.tmpl", filepath.Join(target.WireDir, "inject_cmd_app.go")),
	}
	if components.WebUI {
		mappings = append(mappings, mapTemplateTo("frontend/dist/index.html.tmpl", appTargetFrontendDistIndex(target)))
	}
	if components.WebAPI || components.WebUI {
		mappings = append(mappings,
			mapTemplateTo("app/routes.go.tmpl", filepath.Join(target.AppDir, "routes.go")),
			mapTemplateTo("wire/inject_http_controllers_app.go.tmpl", filepath.Join(target.WireDir, "inject_http_controllers_app.go")),
		)
	}
	if components.HasDatabase() {
		mappings = append(mappings, mapTemplateTo("wire/inject_repositories_app.go.tmpl", filepath.Join(target.WireDir, "inject_repositories_app.go")))
	}
	if components.Scheduler {
		mappings = append(mappings,
			mapTemplateTo("app/schedules.go.tmpl", filepath.Join(target.AppDir, "schedules.go")),
			mapTemplateTo("wire/inject_schedules_app.go.tmpl", filepath.Join(target.WireDir, "inject_schedules_app.go")),
		)
	}
	if components.Jobs {
		mappings = append(mappings, mapTemplateTo("wire/inject_jobs_app.go.tmpl", filepath.Join(target.WireDir, "inject_jobs_app.go")))
	}
	return mappings
}

func writeProjectConfig(path string, cfg *project.Config) error {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(cfg); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func (p *ProjectRenderer) cleanupLegacyGeneratedFiles() error {
	legacyPaths := []string{
		filepath.Join("internal", "cmd", "generate_all_cmd.go"),
		filepath.Join("internal", "cmd", "generate_cmd.go"),
		"main.go",
		filepath.Join("app", "providers.go"),
		filepath.Join("internal", "storage", "generate_cmd.go"),
		filepath.Join("internal", "database", "generate_cmd.go"),
		filepath.Join("internal", "database", "generate_cmd_test.go"),
		filepath.Join("project", "config.go"),
		filepath.Join("internal", "cmd", "demo_push_monitor_trigger_cmd.go"),
		filepath.Join("internal", "cmd", "monitor_seed_cmd.go"),
		filepath.Join("internal", "cmd", "monitor_reset_cmd.go"),
		filepath.Join("internal", "cmd", "monitor_retention_cmd.go"),
		filepath.Join("internal", "cmd", "monitor_poll_cmd.go"),
		filepath.Join("internal", "cmd", "push_monitor_trigger_cmd.go"),
		filepath.Join("internal", "cmd", "test_monitor_poll_loop_cmd.go"),
		filepath.Join("internal", "cmd", "lifecycle_hooks.go"),
		filepath.Join("internal", "cmd", "about_service.go"),
		filepath.Join("internal", "cmd", "standalone.go"),
		filepath.Join("internal", "cmd", "signatures.go"),
		filepath.Join("internal", "cmd", "app_commands.go"),
		filepath.Join("internal", "cmd", "root_cmd.go"),
		filepath.Join("internal", "cmd", "wire.go"),
		filepath.Join("internal", "cmd", "skip_boot.go"),
		filepath.Join("internal", "cmd", "skip_boot_test.go"),
		filepath.Join("internal", "observability", "observers.go"),
		filepath.Join("internal", "observability", "observers_test.go"),
		filepath.Join("internal", "router", "routes_registry.go"),
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
		filepath.Join("internal", "app", "README.md"),
		filepath.Join("internal", "app", "about.go"),
		filepath.Join("internal", "app", "discovery.go"),
		filepath.Join("internal", "app", "lifecycle.go"),
		filepath.Join("internal", "app", "lifecycle_registry.go"),
		filepath.Join("internal", "app", "lifecycle_test.go"),
		filepath.Join("internal", "app", "manager.go"),
		filepath.Join("internal", "app", "manager_test.go"),
		filepath.Join("internal", "app", "registry.go"),
		filepath.Join("internal", "app", "runtime.go"),
		filepath.Join("internal", "app", "runtime_host.go"),
		filepath.Join("internal", "app", "runtime_host_test.go"),
		filepath.Join("internal", "app", "source.go"),
		filepath.Join("internal", "app", "timeouts.go"),
		filepath.Join("internal", "jobs", "devconsole.go"),
		filepath.Join("internal", "jobs", "queue_registration.go"),
		filepath.Join("internal", "scheduler", "devconsole.go"),
		filepath.Join("internal", "scheduler", "app_schedules.go"),
		filepath.Join("internal", "scheduler", "cmd.go"),
		filepath.Join("internal", "scheduler", "lighthouse.go"),
		filepath.Join("internal", "scheduler", "runtime.go"),
		filepath.Join("internal", "scheduler", "scheduler.go"),
		filepath.Join("internal", "scheduler", "scheduler_registry.go"),
		filepath.Join("internal", "schedules", "scheduler_registry.go"),
		filepath.Join("migrations", "2025_04_25_235625_new_user_table.up.sql"),
		filepath.Join("migrations", "2025_04_25_235625_new_user_table.down.sql"),
		filepath.Join("wire", "app.go"),
		filepath.Join("wire", "app_test.go"),
		filepath.Join("wire", "inject_app_services.go"),
		filepath.Join("wire", "inject_services_app.go"),
		filepath.Join("wire", "inject_auth.go"),
		filepath.Join("wire", "inject_cache.go"),
		filepath.Join("wire", "inject_cmd.go"),
		filepath.Join("wire", "inject_db.go"),
		filepath.Join("wire", "inject_event_subscribers.go"),
		filepath.Join("wire", "inject_subscribers_app.go"),
		filepath.Join("wire", "inject_http.go"),
		filepath.Join("wire", "inject_http_controllers.go"),
		filepath.Join("wire", "inject_controllers_app.go"),
		filepath.Join("wire", "inject_http_controllers_app.go"),
		filepath.Join("wire", "inject_jobs.go"),
		filepath.Join("wire", "inject_jobs_app.go"),
		filepath.Join("wire", "inject_mail.go"),
		filepath.Join("wire", "inject_queue.go"),
		filepath.Join("wire", "inject_repositories.go"),
		filepath.Join("wire", "inject_repositories_app.go"),
		filepath.Join("wire", "inject_scheduler.go"),
		filepath.Join("wire", "inject_scheduler_schedules.go"),
		filepath.Join("wire", "inject_schedules_app.go"),
		filepath.Join("wire", "inject_storage.go"),
		filepath.Join("wire", "wire.go"),
		filepath.Join("wire", "wire_gen.go"),
		filepath.Join("app", "wire", "inject_app_services.go"),
		filepath.Join("app", "wire", "inject_cache.go"),
		filepath.Join("app", "wire", "inject_events.go"),
		filepath.Join("app", "wire", "inject_event_subscribers.go"),
		filepath.Join("app", "wire", "inject_http_controllers.go"),
		filepath.Join("app", "wire", "inject_inspects.go"),
		filepath.Join("app", "wire", "inject_mail.go"),
		filepath.Join("app", "wire", "inject_queue.go"),
		filepath.Join("app", "wire", "inject_repositories.go"),
		filepath.Join("app", "wire", "inject_scheduler_schedules.go"),
		filepath.Join("app", "wire", "inject_storage.go"),
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
	if err := os.Remove(filepath.Join("wire")); err != nil && !os.IsNotExist(err) && !errors.Is(err, syscall.ENOTEMPTY) {
		return err
	}
	if err := p.syncLegacyGeneratedTemplates(); err != nil {
		return err
	}

	for _, target := range renderAppTargets() {
		components := targetRenderComponents(p.config, target)
		for _, path := range appOwnedWirePathsForTarget(target) {
			if data, err := os.ReadFile(path); err == nil {
				updated := syncAppOwnedWireSetNames(string(data))
				switch filepath.Base(path) {
				case "inject_jobs_app.go":
					updated = syncDemoAppJobInjector(updated, p.config.GoModuleName, components)
				case "inject_repositories_app.go":
					updated = syncDemoAppRepositoryInjector(updated, p.config.GoModuleName, components)
				case "inject_services_app.go":
					updated = syncDemoAppServiceInjector(updated, p.config.GoModuleName, components)
				}
				if updated != string(data) {
					formatted, err := format.Source([]byte(updated))
					if err != nil {
						return fmt.Errorf("gofmt %s: %w", path, err)
					}
					if err := os.WriteFile(path, formatted, 0o644); err != nil {
						return err
					}
				}
			} else if !os.IsNotExist(err) {
				return err
			}
		}
	}

	appServiceInjectorPath := filepath.Join("app", "wire", "inject_services_app.go")
	if data, err := os.ReadFile(appServiceInjectorPath); err == nil {
		updated := syncLegacyAppServiceInjector(string(data), p.config.GoModuleName)
		if updated != string(data) {
			formatted, err := format.Source([]byte(updated))
			if err != nil {
				return fmt.Errorf("gofmt %s: %w", appServiceInjectorPath, err)
			}
			if err := os.WriteFile(appServiceInjectorPath, formatted, 0o644); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	appLifecyclePath := filepath.Join("app", "lifecycle.go")
	if data, err := os.ReadFile(appLifecyclePath); err == nil {
		updated := syncLegacyAppLifecycleRegistry(string(data), p.config.GoModuleName)
		if updated != string(data) {
			formatted, err := format.Source([]byte(updated))
			if err != nil {
				return fmt.Errorf("gofmt %s: %w", appLifecyclePath, err)
			}
			if err := os.WriteFile(appLifecyclePath, formatted, 0o644); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	scheduleInjectorPath := filepath.Join("app", "wire", "inject_schedules_app.go")
	if data, err := os.ReadFile(scheduleInjectorPath); err == nil {
		updated := syncLegacyScheduleInjector(string(data), p.config.GoModuleName)
		if updated != string(data) {
			formatted, err := format.Source([]byte(updated))
			if err != nil {
				return fmt.Errorf("gofmt %s: %w", scheduleInjectorPath, err)
			}
			if err := os.WriteFile(scheduleInjectorPath, formatted, 0o644); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	// Migrate legacy scheduler command names in preserved app-owned schedules.
	appSchedulesPath := filepath.Join("app", "schedules.go")
	if data, err := os.ReadFile(appSchedulesPath); err == nil {
		updated := syncLegacyScheduleInjectorPackage(string(data))
		updated = strings.ReplaceAll(updated, "demo:push-monitor-trigger", "monitor:push-test-trigger")
		updated = strings.ReplaceAll(updated, "push-monitor-trigger", "monitor:push-test-trigger")
		if updated != string(data) {
			formatted, err := format.Source([]byte(updated))
			if err != nil {
				return fmt.Errorf("gofmt %s: %w", appSchedulesPath, err)
			}
			if err := os.WriteFile(appSchedulesPath, formatted, 0o644); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	healthEnabled := p.config.Render.Components.WebAPI || p.config.Render.Components.WebUI
	appCommandsPath := filepath.Join("app", "commands.go")
	if data, err := os.ReadFile(appCommandsPath); err == nil {
		updated := syncCommandsName(string(data))
		updated = syncHealthCommands(updated, healthEnabled)
		if updated != string(data) {
			formatted, err := format.Source([]byte(updated))
			if err != nil {
				return fmt.Errorf("gofmt %s: %w", appCommandsPath, err)
			}
			if err := os.WriteFile(appCommandsPath, formatted, 0o644); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	cmdWirePath := filepath.Join("app", "wire", "inject_cmd.go")
	if data, err := os.ReadFile(cmdWirePath); err == nil {
		updated := syncHealthCommandWire(string(data), healthEnabled)
		if updated != string(data) {
			if err := os.WriteFile(cmdWirePath, []byte(updated), 0o644); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	prebootPath := filepath.Join("internal", "cmd", "preboot.go")
	if data, err := os.ReadFile(prebootPath); err == nil {
		updated := syncHealthPreboot(string(data), healthEnabled)
		if updated != string(data) {
			if err := os.WriteFile(prebootPath, []byte(updated), 0o644); err != nil {
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
			dest: filepath.Join("app", "wire", "inject_services_app.go"),
			tmpl: "wire/inject_services_app.go.tmpl",
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
		{
			dest: filepath.Join("internal", "jobs", "worker.go"),
			tmpl: "internal/jobs/worker.go.tmpl",
			matches: []string{
				"internal/alerts",
				"internal/monitoring",
				"hello *ExampleHelloJob,",
				"monitorCheck *monitoring.MonitorCheckJob,",
				"alertDispatch *alerts.DispatchJob,",
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

// migrateAppOwnedWireFilenames preserves user-owned injector contents while adopting clearer app/wire names.
func (p *ProjectRenderer) migrateAppOwnedWireFilenames() error {
	return migratePreservedFile(
		filepath.Join("app", "wire", "inject_controllers_app.go"),
		filepath.Join("app", "wire", "inject_http_controllers_app.go"),
	)
}

// appOwnedWirePathsForTarget lists render-once injectors that may need compatibility repairs.
func appOwnedWirePathsForTarget(target project.AppTarget) []string {
	wireDir := target.WireDir
	if wireDir == "" {
		wireDir = project.DefaultAppTarget().WireDir
	}
	return []string{
		filepath.Join(wireDir, "inject_cmd_app.go"),
		filepath.Join(wireDir, "inject_http_controllers_app.go"),
		filepath.Join(wireDir, "inject_jobs_app.go"),
		filepath.Join(wireDir, "inject_repositories_app.go"),
		filepath.Join(wireDir, "inject_schedules_app.go"),
		filepath.Join(wireDir, "inject_services_app.go"),
		filepath.Join(wireDir, "inject_subscribers_app.go"),
	}
}

// migratePreservedFile renames a render-once file only when the replacement does not already exist.
func migratePreservedFile(oldPath string, newPath string) error {
	if _, err := os.Stat(oldPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if _, err := os.Stat(newPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(oldPath, newPath)
}

// syncAppOwnedWireSetNames updates preserved app-owned injectors to the current set naming contract.
func syncAppOwnedWireSetNames(content string) string {
	updated := strings.ReplaceAll(content, "cmdAppSet", "appCommandSet")
	updated = strings.ReplaceAll(updated, "httpAppControllerSet", "appHttpControllerSet")
	updated = strings.ReplaceAll(updated, "appControllerSet", "appHttpControllerSet")
	updated = strings.ReplaceAll(updated, "httpControllerSet provides all HTTP route controllers.", "appHttpControllerSet provides all HTTP route controllers.")
	updated = strings.ReplaceAll(updated, "jobAppSet", "appJobSet")
	updated = strings.ReplaceAll(updated, "schedulerScheduleSet", "appScheduleSet")
	updated = strings.ReplaceAll(updated, "eventSubscriberSet", "appSubscriberSet")
	return updated
}

// syncDemoAppJobInjector repairs early target job injectors that omitted demo job providers.
func syncDemoAppJobInjector(content string, moduleName string, components project.Components) string {
	if !components.DemoApp || !components.Jobs || strings.TrimSpace(moduleName) == "" {
		return content
	}
	updated := content
	updated = ensureGoImport(updated, moduleName+"/internal/alerts", "")
	updated = ensureGoImport(updated, moduleName+"/internal/monitoring", "")
	for _, provider := range []string{
		"\talerts.NewDispatchJob,",
		"\tmonitoring.NewCheckService,",
		"\tmonitoring.NewMonitorCheckJob,",
	} {
		if !strings.Contains(updated, strings.TrimSpace(provider)) {
			updated = insertIntoWireSet(updated, "appJobSet", provider)
		}
	}
	return updated
}

// syncDemoAppRepositoryInjector repairs early target repository injectors that omitted demo repositories.
func syncDemoAppRepositoryInjector(content string, moduleName string, components project.Components) string {
	if !components.DemoApp || !components.HasDatabase() || strings.TrimSpace(moduleName) == "" {
		return content
	}
	updated := content
	for _, importPath := range []string{
		moduleName + "/internal/appsettings",
		moduleName + "/internal/models",
		moduleName + "/internal/notification",
	} {
		updated = ensureGoImport(updated, importPath, "")
	}
	if strings.Contains(updated, "wire.Value(repositorySetPlaceholder{})") {
		updated = strings.Replace(updated, "\twire.Value(repositorySetPlaceholder{}),\n", "", 1)
	}
	for _, provider := range []string{
		"\tappsettings.NewAppSettingRepo,",
		"\tmodels.NewAlertDispatchEventRepo,",
		"\tmodels.NewIncidentRepo,",
		"\tmodels.NewMonitorCheckRepo,",
		"\tmodels.NewMonitorCheckRollupRepo,",
		"\tmodels.NewMonitorRepo,",
		"\tnotification.NewChannelRepo,",
	} {
		if !strings.Contains(updated, strings.TrimSpace(provider)) {
			updated = insertIntoWireSet(updated, "repositorySet", provider)
		}
	}
	return updated
}

// syncDemoAppServiceInjector repairs early target service injectors that omitted demo providers.
func syncDemoAppServiceInjector(content string, moduleName string, components project.Components) string {
	if !components.DemoApp || !components.HasDatabase() || strings.TrimSpace(moduleName) == "" {
		return content
	}
	updated := content
	for _, importPath := range []string{
		moduleName + "/internal/appsettings",
		moduleName + "/internal/logger",
		moduleName + "/internal/monitoring",
		moduleName + "/internal/notification",
	} {
		updated = ensureGoImport(updated, importPath, "")
	}
	for _, provider := range []string{
		"\tmonitoring.NewIncidentTransitionService,",
		"\tmonitoring.NewRetentionService,",
		"\tnotification.NewManager,",
		"\tpreseedDemoDefaults,",
	} {
		if !strings.Contains(updated, strings.TrimSpace(provider)) {
			updated = insertIntoWireSet(updated, "appSet", provider)
		}
	}
	if !strings.Contains(updated, "type demoPreseedReady struct{}") {
		updated = strings.TrimRight(updated, "\n") + demoPreseedReadyBlock() + "\n"
	}
	return updated
}

// demoPreseedReadyBlock restores demo preseed wiring for targets rendered before the provider split.
func demoPreseedReadyBlock() string {
	return `

type demoPreseedReady struct{}

func preseedDemoDefaults(
	logger *logger.AppLogger,
	settingRepo *appsettings.AppSettingRepo,
	channelRepo *notification.ChannelRepo,
) (*demoPreseedReady, error) {
	setupCtx := runtime.BackgroundSourceContext(runtime.SourceStartup)
	if inserted, err := settingRepo.PreseedDefaults(setupCtx); err != nil {
		return nil, err
	} else if inserted > 0 {
		logger.Info().Int("inserted", inserted).Msg("app settings preseeded")
	}
	if inserted, err := channelRepo.PreseedDefaults(setupCtx); err != nil {
		return nil, err
	} else if inserted > 0 {
		logger.Info().Int("inserted", inserted).Msg("notification channels preseeded")
	}
	return &demoPreseedReady{}, nil
}
`
}

// syncLegacyScheduleInjectorPackage updates preserved schedule wiring after the scheduler package rename.
func syncLegacyScheduleInjectorPackage(content string) string {
	updated := strings.ReplaceAll(content, "/internal/scheduler", "/internal/schedules")
	updated = strings.ReplaceAll(updated, "scheduler.AppSchedules", "schedules.AppSchedules")
	updated = strings.ReplaceAll(updated, "scheduler.NewAppSchedules", "schedules.NewAppSchedules")
	updated = strings.ReplaceAll(updated, "scheduler.ScheduleRegistry", "schedules.ScheduleRegistry")
	updated = strings.ReplaceAll(updated, "scheduler.RegisterRecurring", "schedules.RegisterRecurring")
	updated = strings.ReplaceAll(updated, "*scheduler.Scheduler", "*schedules.Scheduler")
	return updated
}

// syncLegacyScheduleInjector updates preserved app schedule wiring after schedule registration moved to app/.
func syncLegacyScheduleInjector(content string, moduleName string) string {
	moduleName = strings.TrimSpace(moduleName)
	if moduleName == "" {
		return syncLegacyScheduleInjectorPackage(content)
	}

	updated := syncLegacyScheduleInjectorPackage(content)
	schedulesPath := moduleName + "/internal/schedules"
	targetAppPath := moduleName + "/app"
	updated = ensureGoImport(updated, schedulesPath, "")
	updated = ensureGoImport(updated, targetAppPath, "targetapp")
	updated = strings.ReplaceAll(updated, "\tschedules.NewAppSchedules,", "\tProvideAppSchedules,")

	if !strings.Contains(updated, "func ProvideAppSchedules(") {
		updated = appendProvideAppSchedules(updated)
	}
	if !strings.Contains(updated, "targetapp.NewScheduleRegistry") {
		updated = insertIntoWireSet(updated, "appScheduleSet", "\ttargetapp.NewScheduleRegistry,")
	}
	if !strings.Contains(updated, "wire.Bind(new(schedules.ScheduleRegistry), new(*targetapp.ScheduleRegistry))") {
		updated = insertIntoWireSet(updated, "appScheduleSet", "\twire.Bind(new(schedules.ScheduleRegistry), new(*targetapp.ScheduleRegistry)),")
	}
	return updated
}

// appendProvideAppSchedules adds the explicit zero-arg provider Wire needs for an empty legacy container.
func appendProvideAppSchedules(content string) string {
	content = strings.TrimRight(content, "\n")
	return content + `

// ProvideAppSchedules creates the legacy AppSchedules container.
func ProvideAppSchedules() *schedules.AppSchedules {
	return schedules.NewAppSchedules()
}
`
}

// syncLegacyAppServiceInjector updates preserved app service wiring after lifecycle moved to app/.
func syncLegacyAppServiceInjector(content string, moduleName string) string {
	moduleName = strings.TrimSpace(moduleName)
	if moduleName == "" {
		return content
	}

	updated := content
	updated = replaceQualifiedIdentifier(updated, "app.NewLifecycleRegistry", "targetapp.NewLifecycleRegistry")
	updated = replaceQualifiedIdentifier(updated, "app.NewTimeouts", "runtime.NewTimeouts")
	updated = replaceQualifiedIdentifier(updated, "app.BackgroundSourceContext", "runtime.BackgroundSourceContext")
	updated = replaceQualifiedIdentifier(updated, "app.SourceStartup", "runtime.SourceStartup")
	updated = replaceQualifiedIdentifier(updated, "runtimeapp.NewTimeouts", "runtime.NewTimeouts")
	updated = replaceQualifiedIdentifier(updated, "runtimeapp.BackgroundSourceContext", "runtime.BackgroundSourceContext")
	updated = replaceQualifiedIdentifier(updated, "runtimeapp.SourceStartup", "runtime.SourceStartup")

	legacyRuntimePath := moduleName + "/internal/app"
	runtimePath := moduleName + "/internal/runtime"
	targetAppPath := moduleName + "/app"
	updated = replaceGoImportPath(updated, legacyRuntimePath, runtimePath, "")
	updated = replaceGoImportPath(updated, runtimePath, runtimePath, "")
	updated = ensureGoImport(updated, targetAppPath, "targetapp")
	return updated
}

// syncLegacyAppLifecycleRegistry updates preserved app lifecycle registration imports after the runtime package rename.
func syncLegacyAppLifecycleRegistry(content string, moduleName string) string {
	moduleName = strings.TrimSpace(moduleName)
	if moduleName == "" {
		return content
	}

	updated := content
	for _, name := range []string{
		"Lifecycle",
		"BeforeStartup",
		"Startup",
		"AfterStartup",
		"BeforeShutdown",
		"Shutdown",
		"AfterShutdown",
	} {
		updated = replaceQualifiedIdentifier(updated, "runtimeapp."+name, "runtime."+name)
	}

	legacyRuntimePath := moduleName + "/internal/app"
	runtimePath := moduleName + "/internal/runtime"
	updated = replaceGoImportPath(updated, legacyRuntimePath, runtimePath, "")
	updated = replaceGoImportPath(updated, runtimePath, runtimePath, "")
	return updated
}

// replaceQualifiedIdentifier rewrites an old qualifier without touching longer aliases.
func replaceQualifiedIdentifier(content string, old string, replacement string) string {
	re := regexp.MustCompile(`(^|[^A-Za-z0-9_])` + regexp.QuoteMeta(old))
	return re.ReplaceAllString(content, `${1}`+replacement)
}

// replaceGoImportPath rewrites an existing import to a new path and optional alias.
func replaceGoImportPath(content string, oldPath string, newPath string, alias string) string {
	oldQuotedPath := strconv.Quote(oldPath)
	newQuotedPath := strconv.Quote(newPath)
	replacement := "\t" + newQuotedPath
	if alias != "" {
		replacement = "\t" + alias + " " + newQuotedPath
	}
	replacements := map[string]string{
		"\t" + oldQuotedPath:            replacement,
		"\tapp " + oldQuotedPath:        replacement,
		"\truntimeapp " + oldQuotedPath: replacement,
		"\truntime " + oldQuotedPath:    replacement,
	}
	for from, to := range replacements {
		content = strings.ReplaceAll(content, from, to)
	}
	return content
}

// ensureGoImport inserts a named import into the first import block when missing.
func ensureGoImport(content string, importPath string, alias string) string {
	if strings.Contains(content, strconv.Quote(importPath)) {
		return content
	}
	importStart := strings.Index(content, "import (\n")
	if importStart == -1 {
		return content
	}
	insertAt := importStart + len("import (\n")
	importLine := "\t" + strconv.Quote(importPath) + "\n"
	if alias != "" {
		importLine = "\t" + alias + " " + strconv.Quote(importPath) + "\n"
	}
	return content[:insertAt] + importLine + content[insertAt:]
}

// insertIntoWireSet inserts a provider before the named wire set closes.
func insertIntoWireSet(content string, setName string, provider string) string {
	lines := strings.Split(content, "\n")
	inSet := false
	for i, line := range lines {
		if !inSet && strings.Contains(line, "var "+setName+" = wire.NewSet(") {
			inSet = true
			continue
		}
		if inSet && strings.TrimSpace(line) == ")" {
			lines = append(lines[:i], append([]string{provider}, lines[i:]...)...)
			return strings.Join(lines, "\n")
		}
	}
	return content
}

// syncCommandsName migrates preserved app command registration away from the legacy AppCommands name.
func syncCommandsName(content string) string {
	updated := strings.ReplaceAll(content, "// AppCommands wires application-specific commands into the CLI.", "// Commands wires application-specific commands into the CLI.")
	updated = strings.ReplaceAll(updated, "type AppCommands struct {", "type Commands struct {")
	updated = strings.ReplaceAll(updated, "// NewAppCommands creates a new AppCommands instance with the given commands.", "// NewCommands creates a new Commands instance with the given commands.")
	updated = strings.ReplaceAll(updated, "func NewAppCommands(", "func NewCommands(")
	updated = strings.ReplaceAll(updated, ") *AppCommands {", ") *Commands {")
	updated = strings.ReplaceAll(updated, "return &AppCommands{", "return &Commands{")
	return updated
}

func syncHealthCommandWire(content string, enabled bool) string {
	const healthLine = "\tcmd.NewHealthCmd,\n"
	if !enabled {
		return strings.Replace(content, healthLine, "", 1)
	}
	if strings.Contains(content, healthLine) {
		return content
	}
	anchor := "\tcmd.NewAboutCmd,\n"
	if strings.Contains(content, anchor) {
		return strings.Replace(content, anchor, anchor+healthLine, 1)
	}
	return content
}

// syncHealthPreboot keeps preserved preboot command dispatch aligned with HTTP component availability.
func syncHealthPreboot(content string, enabled bool) string {
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

// syncHealthCommands keeps preserved command registration aligned with HTTP component availability.
func syncHealthCommands(content string, enabled bool) string {
	const fieldLine = "\tHealthCmd cmd.HealthCmd `cmd:\"\"`\n"
	const paramLine = "\thealthCmd *cmd.HealthCmd,\n"
	const assignLine = "\t\tHealthCmd: *healthCmd,\n"
	if !enabled {
		return removeLinesContaining(content, "HealthCmd", "healthCmd")
	}
	updated := content
	if !strings.Contains(updated, "HealthCmd") {
		updated = insertAfterLineContaining(updated, "AboutCmd", fieldLine)
	}
	if !strings.Contains(updated, "healthCmd") {
		updated = insertAfterLineContaining(updated, "aboutCmd", paramLine)
	}
	if !strings.Contains(updated, "HealthCmd:") {
		updated = insertAfterLineContaining(updated, "AboutCmd:", assignLine)
	}
	return updated
}

// insertAfterLineContaining performs narrow render-once migrations without reformatting whole files.
func insertAfterLineContaining(content string, fragment string, insertedLine string) string {
	lines := strings.SplitAfter(content, "\n")
	for i, line := range lines {
		if !strings.Contains(line, fragment) {
			continue
		}
		inserted := []string{insertedLine}
		lines = append(lines[:i+1], append(inserted, lines[i+1:]...)...)
		return strings.Join(lines, "")
	}
	return content
}

// removeLinesContaining removes component-specific lines from preserved files when a component is disabled.
func removeLinesContaining(content string, fragments ...string) string {
	var builder strings.Builder
	for _, line := range strings.SplitAfter(content, "\n") {
		remove := false
		for _, fragment := range fragments {
			if strings.Contains(line, fragment) {
				remove = true
				break
			}
		}
		if !remove {
			builder.WriteString(line)
		}
	}
	return builder.String()
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
	return p.syncCoreLibrariesInDir(".")
}

// syncCoreLibrariesInDir updates core goforj dependencies in a specific module without forcing callers to change process cwd.
func (p *ProjectRenderer) syncCoreLibrariesInDir(dir string) error {
	modules := coredeps.SyncCoreLibraries()
	modules, skipped, err := coreModulesNeedingSync(filepath.Join(dir, "go.mod"), modules)
	if err != nil {
		return err
	}
	if len(modules) == 0 {
		p.lines = append(p.lines, renderCountsLine("sync core libs", 0, skipped, "modules"))
		return nil
	}

	args := []string{"mod", "edit"}
	for _, module := range modules {
		args = append(args, "-require="+module)
	}
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
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
			return fmt.Errorf("go mod edit core libs: %w (%s)", err, detail)
		}
		return fmt.Errorf("go mod edit core libs: %w", err)
	}

	p.lines = append(p.lines, renderCountsLine("sync core libs", len(modules), skipped, "modules"))
	return nil
}

// coreModulesNeedingSync keeps render fast by avoiding go command work when go.mod already has the desired core dependencies.
func coreModulesNeedingSync(path string, desired []string) ([]string, int, error) {
	state, err := readGoModModuleState(path)
	if err != nil {
		if os.IsNotExist(err) {
			return desired, 0, nil
		}
		return nil, 0, err
	}

	pending := make([]string, 0, len(desired))
	skipped := 0
	for _, spec := range desired {
		module, version, ok := strings.Cut(spec, "@")
		if !ok || module == "" || version == "" {
			pending = append(pending, spec)
			continue
		}
		current, required := state.requires[module]
		if state.replaces[module] && required {
			skipped++
			continue
		}
		if required && current == version {
			skipped++
			continue
		}
		pending = append(pending, spec)
	}
	return pending, skipped, nil
}

type goModModuleState struct {
	requires map[string]string
	replaces map[string]bool
}

// readGoModModuleState reads only the directives needed for dependency sync so render does not pay module graph resolution cost.
func readGoModModuleState(path string) (goModModuleState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return goModModuleState{}, err
	}
	state := goModModuleState{
		requires: map[string]string{},
		replaces: map[string]bool{},
	}
	mode := ""
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(stripGoModLineComment(line))
		if trimmed == "" {
			continue
		}
		if mode != "" {
			if trimmed == ")" {
				mode = ""
				continue
			}
			recordGoModDirective(&state, mode, trimmed)
			continue
		}
		switch {
		case trimmed == "require (":
			mode = "require"
		case trimmed == "replace (":
			mode = "replace"
		case strings.HasPrefix(trimmed, "require "):
			recordGoModDirective(&state, "require", strings.TrimSpace(strings.TrimPrefix(trimmed, "require ")))
		case strings.HasPrefix(trimmed, "replace "):
			recordGoModDirective(&state, "replace", strings.TrimSpace(strings.TrimPrefix(trimmed, "replace ")))
		}
	}
	return state, nil
}

// recordGoModDirective stores minimal require and replace metadata because local replaces intentionally override pinned versions.
func recordGoModDirective(state *goModModuleState, directive string, line string) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return
	}
	switch directive {
	case "require":
		if len(fields) >= 2 {
			state.requires[fields[0]] = fields[1]
		}
	case "replace":
		before, _, ok := strings.Cut(line, "=>")
		if !ok {
			return
		}
		fields = strings.Fields(before)
		if len(fields) > 0 {
			state.replaces[fields[0]] = true
		}
	}
}

// stripGoModLineComment lets the lightweight parser ignore inline comments without pulling in a module-file parser dependency.
func stripGoModLineComment(line string) string {
	before, _, ok := strings.Cut(line, "//")
	if !ok {
		return line
	}
	return before
}

// runWireGenerate refreshes Wire output for the default target and every discovered named target.
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

	wireDirs := p.wireGenerateDirs()
	if err := firstWireGenerateError(runWireGenerateDirs(wireBinaryPath, wireDirs)); err != nil {
		// If a stale wire binary was built with an older Go toolchain, reinstall
		// wire with the current toolchain and retry all target-local graphs once.
		if wireGenerateErr, ok := err.(*wireGenerateError); ok && wireGenerateErr.stale {
			path, installErr := installWire()
			if installErr != nil {
				return installErr
			}
			wireBinaryPath = path
			if retryErr := firstWireGenerateError(runWireGenerateDirs(wireBinaryPath, wireDirs)); retryErr != nil {
				return retryErr
			}
		} else {
			return err
		}
	}

	p.lines = append(p.lines, renderCountsLine("wire generate", len(wireDirs), 0, "commands"))
	return nil
}

func runWireGenerateDirs(wirePath string, wireDirs []string) []error {
	errs := make([]error, len(wireDirs))
	var wg sync.WaitGroup
	for i, wireDir := range wireDirs {
		wg.Add(1)
		go func(i int, wireDir string) {
			defer wg.Done()
			errs[i] = runWireGenerateDir(wirePath, wireDir)
		}(i, wireDir)
	}
	wg.Wait()
	return errs
}

func runWireGenerateDir(wirePath string, wireDir string) error {
	out, err := runWireCommand(wirePath, wireDir)
	if err == nil {
		return nil
	}
	trimmed := strings.TrimSpace(string(out))
	return &wireGenerateError{
		dir:    wireDir,
		output: trimmed,
		err:    err,
		stale:  strings.Contains(trimmed, "package requires newer Go version"),
	}
}

func firstWireGenerateError(errs []error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// wireGenerateDirs returns existing Wire directories in the order they should be generated.
func (p *ProjectRenderer) wireGenerateDirs() []string {
	seen := make(map[string]bool)
	add := func(dir string, dirs *[]string) {
		dir = filepath.Clean(strings.TrimSpace(dir))
		if dir == "." || dir == "" || seen[dir] {
			return
		}
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			seen[dir] = true
			*dirs = append(*dirs, dir)
		}
	}

	dirs := make([]string, 0)
	add(project.DefaultAppTarget().WireDir, &dirs)
	if p.config != nil {
		for _, configured := range p.config.Dev.WirePaths {
			add(configured, &dirs)
		}
	}
	if targets, err := p.namedAppRenderTargets(); err == nil {
		for _, target := range targets {
			add(target.WireDir, &dirs)
		}
	}
	if len(dirs) == 0 {
		add("wire", &dirs)
	}
	return dirs
}

// runWireCommand executes the Wire binary from one target-local Wire package.
func runWireCommand(wirePath string, dir string) ([]byte, error) {
	cmd := exec.Command(wirePath)
	cmd.Dir = dir
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
		p.config.Render.Components.Observability,
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

// removeFileIfExists reports whether a conventional generated file was present.
func removeFileIfExists(path string) (bool, error) {
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// removeDirIfExists reports whether a conventional generated directory was present.
func removeDirIfExists(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if err := os.RemoveAll(path); err != nil {
		return false, err
	}
	if _, err := os.Stat(path); err == nil {
		return false, fmt.Errorf("remove directory %s: still exists after removal", path)
	} else if os.IsNotExist(err) {
		return true, nil
	} else {
		return false, err
	}
}

// removeEmptyDirIfEmpty avoids deleting command-package files that are outside the generated target shape.
func removeEmptyDirIfEmpty(path string) (bool, error) {
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) || errors.Is(err, syscall.ENOTEMPTY) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func (p *ProjectRenderer) scaffoldDemoFrontend() error {
	frontendDir := defaultFrontendDir()
	if err := p.copyRawPathToDest("demo/frontend", frontendDir); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(frontendDir, "dist", "index.html")); err != nil {
		return p.ensureFrontendDistPlaceholder()
	}
	return nil
}

func (p *ProjectRenderer) scaffoldVueStarterKit() error {
	return p.scaffoldVueStarterKitForTarget(project.DefaultAppTarget(), true)
}

// scaffoldTargetStarterKit creates a target-local frontend scaffold only on first target creation.
func (p *ProjectRenderer) scaffoldTargetStarterKit(target project.AppTarget) error {
	starterKit := targetRenderStarterKit(p.config, target)
	if starterKit != project.StarterKitVue {
		return nil
	}
	if _, err := os.Stat(filepath.Join(appTargetFrontendDir(target), "package.json")); err == nil {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return p.scaffoldVueStarterKitForTarget(target, false)
}

// scaffoldVueStarterKitForTarget copies the shared Vue starter kit into a target frontend directory.
func (p *ProjectRenderer) scaffoldVueStarterKitForTarget(target project.AppTarget, overwrite bool) error {
	frontendDir := appTargetFrontendDir(target)
	if overwrite {
		if err := os.RemoveAll(frontendDir); err != nil {
			return err
		}
	}
	if err := p.copyRawPathToDestFiltered("starter-kits/vue/frontend", frontendDir, skipFrontendBuildArtifact); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(frontendDir, "dist", "index.html")); err != nil {
		if p.config != nil {
			if err := p.renderTemplateFile(appTargetFrontendDistIndex(target), "frontend/dist/index.html.tmpl", templateDataForTarget(p.config, target)); err != nil {
				return err
			}
			return p.ensureFrontendPlaceholderAssets(target)
		}
		return p.writeFrontendDistPlaceholder(appTargetFrontendDistIndex(target), defaultFrontendDistPlaceholderContent())
	}
	return nil
}

func skipFrontendBuildArtifact(rel string, d fs.DirEntry) bool {
	name := filepath.Base(rel)
	return d.IsDir() && (name == "node_modules" || name == "dist")
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
	content := defaultFrontendDistPlaceholderContent()
	paths := make([]string, 0)
	for _, target := range renderAppTargets() {
		if !targetRenderComponents(p.config, target).WebUI {
			continue
		}
		paths = append(paths, appTargetFrontendDistIndex(target))
	}
	for _, index := range paths {
		if _, err := os.Stat(index); err == nil {
			continue
		}
		if err := p.writeFrontendDistPlaceholder(index, content); err != nil {
			return err
		}
	}
	return nil
}

const (
	frontendPlaceholderLogoName     = "goforj-logo.png"
	frontendPlaceholderLogoTemplate = "starter-kits/vue/frontend/public/goforj-logo.png"
)

// defaultFrontendDistPlaceholderContent keeps no-SPA fallback pages consistent across targets.
func defaultFrontendDistPlaceholderContent() string {
	return "<!doctype html><html><head><meta charset=\"UTF-8\"><title>Ready to build</title><link rel=\"icon\" href=\"./goforj-logo.png\" type=\"image/png\"><link rel=\"apple-touch-icon\" href=\"./goforj-logo.png\"></head><body><img src=\"./goforj-logo.png\" alt=\"GoForj logo\"></body></html>\n"
}

// writeFrontendDistPlaceholder writes a fallback SPA page and records it in render stats.
func (p *ProjectRenderer) writeFrontendDistPlaceholder(index string, content string) error {
	if err := os.MkdirAll(filepath.Dir(index), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(index, []byte(content), 0644); err != nil {
		return err
	}
	if p.stats != nil {
		p.stats.recordCreated(index)
	}
	if strings.Contains(content, frontendPlaceholderLogoName) {
		if err := p.copyFrontendPlaceholderAsset(filepath.Join(filepath.Dir(index), frontendPlaceholderLogoName), frontendPlaceholderLogoTemplate); err != nil {
			return err
		}
	}
	return nil
}

// copyFrontendPlaceholderAsset keeps fallback pages self-contained without overwriting identical assets.
func (p *ProjectRenderer) copyFrontendPlaceholderAsset(dest string, templatePath string) error {
	content, err := templatesFS.ReadFile(templatePath)
	if err != nil {
		return err
	}
	if existing, err := os.ReadFile(dest); err == nil && bytes.Equal(existing, content) {
		if p.stats != nil {
			p.stats.recordSkipped(dest)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(dest, content, 0644); err != nil {
		return err
	}
	if p.stats != nil {
		p.stats.recordCreated(dest)
	}
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
		return fmt.Errorf("parse template %s: %w", tmpl, err)
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
		return templateDataForTarget(value, project.DefaultAppTarget())
	case project.Config:
		cfg := value
		return templateDataForTarget(&cfg, project.DefaultAppTarget())
	case templateRenderConfig:
		return value
	default:
		return data
	}
}

func templateDataForTarget(config *project.Config, target project.AppTarget) templateRenderConfig {
	if target.Name == "" {
		target = project.DefaultAppTarget()
	}
	appImportPath := filepath.ToSlash(target.AppDir)
	wireImportPath := filepath.ToSlash(target.WireDir)
	components := targetRenderComponents(config, target)
	runtimeTargets := runtimeTargetMetadataForRender()
	return templateRenderConfig{
		Config:               config,
		Components:           components,
		ProjectComponents:    config.Render.Components,
		Target:               target,
		TargetPackageName:    project.AppTargetPackageName(target.Name),
		TargetAppImportPath:  appImportPath,
		TargetWireImportPath: wireImportPath,
		TargetIsDefault:      target.Name == project.DefaultAppTargetName,
		HasNamedTargets:      target.Name != project.DefaultAppTargetName || len(runtimeTargets) > 1,
		RuntimeTargets:       runtimeTargets,
	}
}

// targetRenderComponents resolves the app-slice component participation for a target render.
func targetRenderComponents(config *project.Config, target project.AppTarget) project.Components {
	if config == nil {
		return project.Components{}
	}
	if target.Name == "" || target.Name == project.DefaultAppTargetName {
		return config.Render.Components
	}
	components := config.Render.Components
	targetConfig, ok := config.AppTargets[target.Name]
	if ok {
		components = project.NormalizeTargetComponents(config.Render.Components, targetConfig.Components)
	}
	return components
}

// targetRenderStarterKit resolves the target-specific starter kit selection.
func targetRenderStarterKit(config *project.Config, target project.AppTarget) project.StarterKit {
	if config == nil {
		return project.StarterKitNone
	}
	components := targetRenderComponents(config, target)
	starterKit := config.Render.StarterKit
	if target.Name != "" && target.Name != project.DefaultAppTargetName {
		if targetConfig, ok := config.AppTargets[target.Name]; ok {
			starterKit = targetConfig.StarterKit
		}
	}
	starterKit = project.NormalizeStarterKit(starterKit)
	if !components.WebUI {
		return project.StarterKitNone
	}
	return starterKit
}

// runtimeTargetMetadataForRender creates the compiled target table used by generated runtime defaults.
func runtimeTargetMetadataForRender() []runtimeTargetMetadata {
	targets := renderAppTargets()
	out := make([]runtimeTargetMetadata, 0, len(targets))
	for i, target := range targets {
		out = append(out, runtimeTargetMetadata{
			Name:        target.Name,
			Index:       i,
			EnvPrefix:   targetEnvPrefix(target.Name),
			HTTPPort:    3000 + i,
			RuntimeBase: 10000 + i*10,
		})
	}
	return out
}

// runtimeTargetMetadataForTarget resolves deterministic ports even before a new target exists on disk.
func runtimeTargetMetadataForTarget(target project.AppTarget) runtimeTargetMetadata {
	target = normalizeRenderAppTarget(target)
	targets := append(renderAppTargets(), target)
	seen := map[string]project.AppTarget{}
	for _, candidate := range targets {
		candidate = normalizeRenderAppTarget(candidate)
		if candidate.Name == "" || !project.IsSafeAppTargetName(candidate.Name) || project.IsReservedAppTargetName(candidate.Name) {
			continue
		}
		seen[candidate.Name] = candidate
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		if name != project.DefaultAppTargetName {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	ordered := []string{project.DefaultAppTargetName}
	ordered = append(ordered, names...)
	for index, name := range ordered {
		if name != target.Name {
			continue
		}
		return runtimeTargetMetadata{
			Name:        target.Name,
			Index:       index,
			EnvPrefix:   targetEnvPrefix(target.Name),
			HTTPPort:    3000 + index,
			RuntimeBase: 10000 + index*10,
		}
	}
	return runtimeTargetMetadata{Name: target.Name, EnvPrefix: targetEnvPrefix(target.Name), HTTPPort: 3000, RuntimeBase: 10000}
}

// renderAppTargets returns the default target plus named targets in deterministic runtime order.
func renderAppTargets() []project.AppTarget {
	seen := map[string]bool{}
	targets := make([]project.AppTarget, 0)
	add := func(target project.AppTarget) {
		target = normalizeRenderAppTarget(target)
		if target.Name == "" || seen[target.Name] || !project.IsSafeAppTargetName(target.Name) || project.IsReservedAppTargetName(target.Name) {
			return
		}
		seen[target.Name] = true
		targets = append(targets, target)
	}

	add(project.DefaultAppTarget())
	for _, target := range discoverConventionalAppTargets() {
		add(target)
	}
	if len(targets) <= 1 {
		return targets
	}
	sort.SliceStable(targets[1:], func(i, j int) bool {
		return targets[i+1].Name < targets[j+1].Name
	})
	return targets
}

// targetEnvPrefix converts target slugs into the uppercase env prefix used by generated defaults.
func targetEnvPrefix(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || name == project.DefaultAppTargetName {
		return ""
	}
	var builder strings.Builder
	lastWasSeparator := true
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r - 'a' + 'A')
			lastWasSeparator = false
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
			lastWasSeparator = false
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastWasSeparator = false
		default:
			if !lastWasSeparator {
				builder.WriteByte('_')
				lastWasSeparator = true
			}
		}
	}
	return strings.Trim(builder.String(), "_")
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

// writeTemplateMappingsForTarget renders mapped templates with target-specific package and import data.
func (p *ProjectRenderer) writeTemplateMappingsForTarget(target project.AppTarget, mappings []templateMapping) error {
	data := templateDataForTarget(p.config, normalizeRenderAppTarget(target))
	for _, mapping := range mappings {
		if err := p.renderTemplateFile(mapping.dest, mapping.tmpl, data); err != nil {
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
	return p.copyRawPathToDestFiltered(path, destRoot, nil)
}

func (p *ProjectRenderer) copyRawPathToDestFiltered(path, destRoot string, skip func(rel string, d fs.DirEntry) bool) error {
	if _, err := fs.ReadDir(templatesFS, path); err == nil {
		return fs.WalkDir(templatesFS, path, func(entry string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(path, entry)
			if err != nil {
				return err
			}
			if skip != nil && skip(rel, d) {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
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

// writeTemplateMappingsOnceForTarget preserves target-owned files after their first render.
func (p *ProjectRenderer) writeTemplateMappingsOnceForTarget(target project.AppTarget, mappings []templateMapping) error {
	data := templateDataForTarget(p.config, normalizeRenderAppTarget(target))
	for _, mapping := range mappings {
		if _, err := os.Stat(mapping.dest); err == nil {
			p.stats.recordSkipped(mapping.dest)
			continue
		}
		if err := p.renderTemplateFile(mapping.dest, mapping.tmpl, data); err != nil {
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

func (p *ProjectRenderer) printStepSummary(title string, before renderCounts, elapsed time.Duration) {
	after := p.stats.counts()
	created := after.created - before.created
	skipped := after.skipped - before.skipped
	if p.renderTimingEnabled() {
		p.appendRenderLine(renderCountsLineWithTiming(title, created, skipped, "files", elapsed))
		return
	}
	p.appendRenderLine(renderCountsLine(title, created, skipped, "files"))
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
	if runningInsideDevCommand() {
		for _, line := range p.lines {
			fmt.Println(line)
		}
		return
	}
	title := fmt.Sprintf("%s Project rendering complete", markCreate)
	fmt.Printf("%s\n", renderBox(title, p.lines))
}

func (p *ProjectRenderer) printOverallSummary() {
	total := p.stats.counts()
	if runningInsideDevCommand() {
		fmt.Printf("%s Render complete (created: %d, skipped: %d)\n", markCreate, total.created, total.skipped)
		return
	}
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

	steps = append(steps, fmt.Sprintf("Set environment defaults in %s, %s, and %s", commandStyle.Render(".env"), commandStyle.Render(".env.host"), commandStyle.Render(".env.local")))
	steps = append(steps, fmt.Sprintf("Start the dev loop: %s", commandStyle.Render("forj dev")))

	if p.config != nil {
		if p.config.Render.Components.WebUI {
			cmd := "cd " + filepath.ToSlash(defaultFrontendDir()) + " && npm install"
			steps = append(steps, fmt.Sprintf("Install frontend deps if you plan to edit the UI: %s", commandStyle.Render(cmd)))
		}
		if p.config.Render.Components.Mail && p.config.Render.Components.Docker {
			steps = append(steps, fmt.Sprintf("Open Mailpit inbox at %s", commandStyle.Render("http://localhost:8025")))
		}
		if p.config.Render.Components.HasDatabase() {
			steps = append(steps, fmt.Sprintf("Review initial migrations under %s before first run", commandStyle.Render("migrations")))
		}
		if p.config.Render.Components.Observability {
			observabilityCmd := "docker-compose up -d victoriametrics vmagent"
			if p.config.Render.Components.Grafana {
				observabilityCmd += " grafana grafana-seed"
			}
			steps = append(steps, fmt.Sprintf("Start observability services: %s", commandStyle.Render(observabilityCmd)))
			steps = append(steps, fmt.Sprintf("Inspect VictoriaMetrics at %s", commandStyle.Render("http://localhost:8428")))
		}
		if p.config.Render.Components.Grafana {
			steps = append(steps, fmt.Sprintf("Open Grafana at %s with %s / %s", commandStyle.Render("http://localhost:13001"), commandStyle.Render("admin"), commandStyle.Render("admin")))
		}
	}

	return steps
}

func runningInsideDevCommand() bool {
	return strings.TrimSpace(os.Getenv("FORJ_COMMAND_ORIGIN")) == "dev_command"
}
