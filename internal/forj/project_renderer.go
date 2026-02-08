package forj

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/goforj/crypt"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
	"github.com/goforj/goforj/templates"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
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

func generateDevconsoleToken() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	length := 20
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
		p.config = &project.Config{Components: input.components}
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
					needsURL := path == ".env" && !strings.Contains(text, "DEVCONSOLE_URL=")
					needsToken := path == ".env" && !strings.Contains(text, "DEVCONSOLE_TOKEN=")
					needsEnabled := path == ".env" && !strings.Contains(text, "DEVCONSOLE_ENABLED=")
					needsKey := allowAppKey && !strings.Contains(text, "APP_KEY=")
					if !(needsURL || needsToken || needsKey) {
						return nil
					}
					appKey := ""
					tokenValue := ""
					for _, line := range strings.Split(text, "\n") {
						trimmed := strings.TrimSpace(line)
						if trimmed == "" || strings.HasPrefix(trimmed, "#") {
							continue
						}
						if strings.HasPrefix(trimmed, "APP_KEY=") {
							appKey = strings.TrimSpace(strings.TrimPrefix(trimmed, "APP_KEY="))
							continue
						}
						if strings.HasPrefix(trimmed, "DEVCONSOLE_TOKEN=") {
							tokenValue = strings.TrimSpace(strings.TrimPrefix(trimmed, "DEVCONSOLE_TOKEN="))
							continue
						}
					}
					if needsToken && tokenValue == "" {
						value, err := generateDevconsoleToken()
						if err != nil {
							return fmt.Errorf("failed to generate devconsole token: %w", err)
						}
						tokenValue = value
					}
					if needsKey && appKey == "" {
						key, err := crypt.GenerateAppKey()
						if err != nil {
							return fmt.Errorf("failed to generate app key: %w", err)
						}
						appKey = key
					}
					appendLines := []string{}
					if needsKey && appKey != "" {
						appendLines = append(appendLines, fmt.Sprintf("APP_KEY=%s", appKey))
					}
					if needsURL {
						appendLines = append(appendLines, "DEVCONSOLE_URL=ws://localhost:3000/__devconsole/ws/agent")
					}
					if needsToken {
						appendLines = append(appendLines, fmt.Sprintf("DEVCONSOLE_TOKEN=%s", tokenValue))
					}
					if needsEnabled {
						appendLines = append(appendLines, "DEVCONSOLE_ENABLED=true")
					}
					if len(appendLines) == 0 {
						return nil
					}
					separator := ""
					if !strings.HasSuffix(text, "\n") {
						separator = "\n"
					}
					appendText := separator + strings.Join(appendLines, "\n") + "\n"
					file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
					if err != nil {
						return err
					}
					defer file.Close()
					if _, err := file.WriteString(appendText); err != nil {
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
						fmt.Printf("  %s already exists [%v]\n", markSkip, name)
						continue
					}
					key, err := crypt.GenerateAppKey()
					if err != nil {
						return fmt.Errorf("failed to generate app key: %w", err)
					}
					token, err := generateDevconsoleToken()
					if err != nil {
						return fmt.Errorf("failed to generate devconsole token: %w", err)
					}
					p.config.AppKey = key
					p.config.DevConsoleToken = token
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
				"internal/console/console.go.tmpl",
				"internal/cmd/hello_world_cmd.go.tmpl",
				"internal/cmd/monitor_seed_cmd.go.tmpl",
				"internal/cmd/monitor_reset_cmd.go.tmpl",
				"internal/cmd/kong_help_formatter.go.tmpl",
				"internal/cmd/root_cmd.go.tmpl",
				"internal/logger/app.go.tmpl",
				"internal/logger/app_test.go.tmpl",
				"internal/logger/wire.go.tmpl",
				"project/config.go.tmpl",
				"wire/app.go.tmpl",
				"wire/inject_app_services.go.tmpl",
				"wire/inject_cmd.go.tmpl",
				"wire/wire.go.tmpl",
			},
			renderOnceTemplates: []string{
				".gitignore.tmpl",
				".db-relationships.yaml.tmpl",
				"internal/cmd/app_commands.go.tmpl",
				"internal/cmd/wire.go.tmpl",
			},
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
				"internal/devconsole/agent.go.tmpl",
				"internal/devconsole/cli.go.tmpl",
				"internal/devconsole/conn.go.tmpl",
				"internal/devconsole/enable.go.tmpl",
				"internal/devconsole/hub.go.tmpl",
				"internal/devconsole/log_hook.go.tmpl",
				"internal/devconsole/protocol.go.tmpl",
				"internal/devconsole/server.go.tmpl",
				"internal/devconsole/editor.go.tmpl",
				"internal/devconsole/ui.go.tmpl",
			},
			raw: []string{
				"internal/devconsole/ui/dist",
			},
		},
		{
			title:   "Web API Components Rendering",
			enabled: p.config.Components.WebAPI || p.config.Components.WebUI,
			templates: []string{
				"wire/inject_http.go.tmpl",
				"internal/http/devconsole.go.tmpl",
				"internal/http/cors.go.tmpl",
				"internal/http/route.go.tmpl",
				"internal/http/routes_list.go.tmpl",
				"internal/http/routes_list_cmd.go.tmpl",
				"internal/http/routes_list_test.go.tmpl",
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
				mappings := map[string]string{}
				if p.config.Components.HasDatabase() {
					mappings["demo/internal/monitoring/controller.go.tmpl"] = "internal/monitoring/controller.go"
					mappings["demo/internal/models/monitor.go.tmpl"] = "internal/models/monitor.go"
					mappings["demo/internal/models/monitor_check.go.tmpl"] = "internal/models/monitor_check.go"
					mappings["demo/internal/models/incident.go.tmpl"] = "internal/models/incident.go"
				}
				if p.config.Components.Jobs {
					mappings["demo/internal/jobs/monitor_check_job.go.tmpl"] = "internal/jobs/monitor_check_job.go"
				}
				if len(mappings) > 0 {
					if err := p.writeTemplateMappings(mappings); err != nil {
						return err
					}
				}
				if p.config.Components.HasDatabase() {
					if err := p.writeTemplateMappingsOnce(map[string]string{
						"demo/internal/migrations/2026_02_06_000001_demo_monitors_table.up.sql.tmpl":   "internal/migrations/2026_02_06_000001_demo_monitors_table.up.sql",
						"demo/internal/migrations/2026_02_06_000001_demo_monitors_table.down.sql.tmpl": "internal/migrations/2026_02_06_000001_demo_monitors_table.down.sql",
					}); err != nil {
						return err
					}
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
				"internal/dbconns/connections_test.go.tmpl",
				"internal/dbconns/generate_cmd.go.tmpl",
				"internal/dbconns/generate_cmd_test.go.tmpl",
				"internal/cmd/generate_all_cmd.go.tmpl",
				"internal/migrations/migrations.go.tmpl",
				"internal/migrations/migrations_test.go.tmpl",
				"internal/migrations/migration_connection_test.go.tmpl",
				"internal/migrations/migration_commands_test.go.tmpl",
				"internal/migrations/migrate_cmd.go.tmpl",
				"internal/migrations/migrate_rollback_cmd.go.tmpl",
				"internal/modelgen/make_model_cmd.go.tmpl",
				"internal/modelgen/make_model_mysql_integration_test.go.tmpl",
				"internal/modelgen/make_model_postgres_integration_test.go.tmpl",
				"internal/modelgen/make_model_sqlite_integration_test.go.tmpl",
				"internal/modelgen/repository_wire_test.go.tmpl",
				"internal/migrations/.goforj/placeholder.txt.tmpl",
			},
			raw: []string{"internal/modelgen/model.tmpl"},
			renderOnceTemplates: []string{
				"internal/migrations/2025_04_25_235625_new_user_table.up.sql.tmpl",
				"internal/migrations/2025_04_25_235625_new_user_table.down.sql.tmpl",
			},
		},
		{
			title:   "Scheduler Components Rendering",
			enabled: p.config.Components.Scheduler,
			templates: []string{
				"internal/scheduler/devconsole.go.tmpl",
				"internal/scheduler/scheduler.go.tmpl",
				"internal/scheduler/job_builder.go.tmpl",
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
			templates: []string{
				"internal/jobs/example_hello_job.go.tmpl",
				"internal/jobs/example_hello_job_cmd.go.tmpl",
				"internal/jobs/make_job_cmd.go.tmpl",
				"internal/jobs/devconsole.go.tmpl",
				"internal/jobs/worker.go.tmpl",
				"internal/jobs/worker_logger.go.tmpl",
				"internal/jobs/worker_cmd.go.tmpl",
				"wire/inject_jobs.go.tmpl",
				"wire/inject_jobs_app.go.tmpl",
			},
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
	if err := p.goModTidy(); err != nil {
		return fmt.Errorf("go mod tidy: %w", err)
	}

	// Run wire install + generate to make main runnable immediately.
	if err := p.runWireGenerate(); err != nil {
		return fmt.Errorf("wire generate: %w", err)
	}

	if input.renderAll && p.config.Components.HasDatabase() {
		if err := p.runGenerateDbConns(); err != nil {
			return fmt.Errorf("generate dbconns: %w", err)
		}
	}

	p.printRenderDetails()
	p.printOverallSummary()

	return nil
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

func (p *ProjectRenderer) runWireGenerate() error {
	install := exec.Command("go", "install", "github.com/goforj/wire/cmd/wire@latest")
	install.Env = os.Environ()
	if out, err := install.CombinedOutput(); err != nil {
		return fmt.Errorf("wire install: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	cmd := exec.Command("wire")
	cmd.Dir = "wire"
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wire generate: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	p.lines = append(p.lines, renderCountsLine("wire generate", 1, 0, "command"))
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
	fmt.Printf("%s\n", renderBox("Rendering project", p.lines))
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
			steps = append(steps, fmt.Sprintf("Review initial migrations under %s before first run", commandStyle.Render("internal/migrations")))
		}
	}

	return steps
}
