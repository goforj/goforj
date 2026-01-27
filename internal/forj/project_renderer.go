package forj

import (
	"bytes"
	"crypto/rand"
	"embed"
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/goforj/crypt"
	"github.com/goforj/goforj/internal/logger"
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
	commandStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
)

//go:embed all:templates
var templates embed.FS

// Components represents the components of the project
type ComponentRenderInput struct {
	components Components
	renderAll  bool
}

// ProjectRenderer is well, a project renderer :)
type ProjectRenderer struct {
	logger *logger.AppLogger
	config *ProjectConfig
	stats  *renderStats
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
	line := fmt.Sprintf(" %s %s %s %d", markStep, label, markCreate, created)
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
	fmt.Printf("\n%s\n\n", headerStyle.Render("GoForj › Rendering project..."))
	p.stats = &renderStats{}

	if input.renderAll {
		cfg, err := LoadProjectConfig()
		if err != nil {
			return err
		}
		p.config = cfg
	} else {
		p.config = &ProjectConfig{Components: input.components}
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
			templates: []string{"templates/main.go.tmpl"},
		},
		{
			title:   "Environment Files Initialization",
			enabled: input.renderAll,
			action: func() error {
				envTemplates := []string{
					"templates/.env.tmpl",
					"templates/.env.host.tmpl",
				}
				ensureEnvDefaults := func(path string, allowAppKey bool) error {
					content, err := os.ReadFile(path)
					if err != nil {
						return err
					}
					text := string(content)
					needsURL := path == ".env" && !strings.Contains(text, "DEVCONSOLE_URL=")
					needsToken := path == ".env" && !strings.Contains(text, "DEVCONSOLE_TOKEN=")
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
					name := strings.TrimSuffix(strings.TrimPrefix(tmpl, "templates/"), ".tmpl")
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
				"templates/internal/console/console.go.tmpl",
				"templates/internal/cmd/hello_world_cmd.go.tmpl",
				"templates/internal/cmd/kong_help_formatter.go.tmpl",
				"templates/internal/cmd/root_cmd.go.tmpl",
				"templates/internal/logger/app.go.tmpl",
				"templates/internal/logger/app_test.go.tmpl",
				"templates/internal/logger/wire.go.tmpl",
				"templates/wire/app.go.tmpl",
				"templates/wire/inject_app_services.go.tmpl",
				"templates/wire/inject_cmd.go.tmpl",
				"templates/wire/wire.go.tmpl",
			},
			renderOnceTemplates: []string{
				"templates/.gitignore.tmpl",
				"templates/.db-relationships.yaml.tmpl",
				"templates/internal/cmd/app_commands.go.tmpl",
				"templates/internal/cmd/wire.go.tmpl",
			},
		},
		{
			title:   "Docker Components Rendering",
			enabled: p.config.Components.Docker,
			templates: append([]string{"templates/docker-compose.yml.tmpl"},
				func() []string {
					if p.config.Components.DatabaseMySQL {
						return []string{
							"templates/containers/mariadb/Dockerfile",
							"templates/containers/mariadb/my.cnf",
						}
					}
					return nil
				}()...,
			),
		},
		{
			title:   "Web API Components Rendering",
			enabled: p.config.Components.WebAPI || p.config.Components.WebUI,
			templates: append(
				[]string{
					"templates/wire/inject_http.go.tmpl",
					"templates/internal/devconsole/agent.go.tmpl",
					"templates/internal/devconsole/cli.go.tmpl",
					"templates/internal/devconsole/conn.go.tmpl",
					"templates/internal/devconsole/hub.go.tmpl",
					"templates/internal/devconsole/log_hook.go.tmpl",
					"templates/internal/devconsole/protocol.go.tmpl",
					"templates/internal/devconsole/server.go.tmpl",
					"templates/internal/devconsole/editor.go.tmpl",
					"templates/internal/devconsole/ui.go.tmpl",
					"templates/internal/http/devconsole.go.tmpl",
					"templates/internal/http/cors.go.tmpl",
					"templates/internal/http/route.go.tmpl",
					"templates/internal/http/routes_list.go.tmpl",
					"templates/internal/http/routes_list_cmd.go.tmpl",
					"templates/internal/http/routes_list_test.go.tmpl",
					"templates/internal/http/middleware_non_200.go.tmpl",
					"templates/internal/http/serve_cmd.go.tmpl",
					"templates/internal/http/server.go.tmpl",
					"templates/internal/http/spa.go.tmpl",
					"templates/internal/http/types.go.tmpl",
					"templates/internal/hello/controller.go.tmpl",
				},
			),
			raw: []string{
				"templates/internal/devconsole/ui/dist",
			},
			renderOnceTemplates: []string{
				"templates/internal/router/routes_registry.go.tmpl",
				"templates/wire/inject_http_controllers.go.tmpl",
			},
		},
		{
			title:     "Web UI Components Rendering",
			enabled:   p.config.Components.WebUI,
			templates: []string{},
			renderOnceTemplates: []string{
				"templates/frontend/dist/index.html.tmpl",
			},
		},
		{
			title:   "Database Components Rendering",
			enabled: p.config.Components.HasDatabase(),
			templates: []string{
				"templates/wire/inject_db.go.tmpl",
				"templates/wire/inject_repositories.go.tmpl",
				"templates/internal/dbconns/connections.go.tmpl",
				"templates/internal/dbconns/connections_test.go.tmpl",
				"templates/internal/dbconns/generate_cmd.go.tmpl",
				"templates/internal/dbconns/generate_cmd_test.go.tmpl",
				"templates/internal/cmd/generate_all_cmd.go.tmpl",
				"templates/internal/migrations/migrations.go.tmpl",
				"templates/internal/migrations/migrations_test.go.tmpl",
				"templates/internal/migrations/migration_connection_test.go.tmpl",
				"templates/internal/migrations/migration_commands_test.go.tmpl",
				"templates/internal/migrations/migrate_cmd.go.tmpl",
				"templates/internal/migrations/migrate_rollback_cmd.go.tmpl",
				"templates/internal/modelgen/make_model_cmd.go.tmpl",
				"templates/internal/modelgen/make_model_mysql_integration_test.go.tmpl",
				"templates/internal/modelgen/make_model_postgres_integration_test.go.tmpl",
				"templates/internal/modelgen/make_model_sqlite_integration_test.go.tmpl",
				"templates/internal/modelgen/repository_wire_test.go.tmpl",
				"templates/internal/migrations/.goforj/placeholder.txt.tmpl",
			},
			raw: []string{"templates/internal/modelgen/model.tmpl"},
			renderOnceTemplates: []string{
				"templates/internal/migrations/2025_04_25_235625_new_user_table.up.sql.tmpl",
				"templates/internal/migrations/2025_04_25_235625_new_user_table.down.sql.tmpl",
			},
		},
		{
			title:   "Scheduler Components Rendering",
			enabled: p.config.Components.Scheduler,
			templates: []string{
				"templates/internal/scheduler/devconsole.go.tmpl",
				"templates/internal/scheduler/scheduler.go.tmpl",
				"templates/internal/scheduler/fluent_job_wrapper.go.tmpl",
				"templates/internal/scheduler/fluent_job_wrapper_test.go.tmpl",
				"templates/internal/scheduler/cmd.go.tmpl",
				"templates/wire/inject_scheduler.go.tmpl",
			},
			renderOnceTemplates: []string{
				"templates/internal/scheduler/scheduler_registry.go.tmpl",
			},
		},
		{
			title:   "Job Components Rendering",
			enabled: p.config.Components.Jobs,
			templates: []string{
				"templates/internal/jobs/example_hello_job.go.tmpl",
				"templates/internal/jobs/example_hello_job_cmd.go.tmpl",
				"templates/internal/jobs/make_job_cmd.go.tmpl",
				"templates/internal/jobs/devconsole.go.tmpl",
				"templates/internal/jobs/worker.go.tmpl",
				"templates/internal/jobs/worker_logger.go.tmpl",
				"templates/internal/jobs/worker_cmd.go.tmpl",
				"templates/wire/inject_jobs.go.tmpl",
				"templates/wire/inject_jobs_app.go.tmpl",
			},
			raw: []string{"templates/internal/jobs/job.tmpl"},
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
		return fmt.Errorf("go mod tidy: %w", err)
	}

	modCount := countTidyModules(stdout.String(), stderr.String())
	fmt.Println(renderCountsLine("go mod tidy", modCount, 0, "modules"))

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

	fmt.Println(renderCountsLine("wire generate", 1, 0, "command"))
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

// renderTemplateFile renders templates based on project configuration settings
func (p *ProjectRenderer) renderTemplateFile(destPath, tmpl string, data any) error {
	tmplBytes, err := templates.ReadFile(tmpl)
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
		dest := strings.TrimSuffix(strings.TrimPrefix(path, "templates/"), ".tmpl")
		if err := p.renderTemplateFile(dest, path, p.config); err != nil {
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
	if _, err := fs.ReadDir(templates, path); err == nil {
		return fs.WalkDir(templates, path, func(entry string, d fs.DirEntry, err error) error {
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

func (p *ProjectRenderer) copyRawFile(path string) error {
	dest := strings.TrimPrefix(path, "templates/")
	content, err := templates.ReadFile(path)
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
		dest := strings.TrimSuffix(strings.TrimPrefix(path, "templates/"), ".tmpl")

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
	fmt.Println(renderCountsLine(title, created, skipped, "files"))
}

func (p *ProjectRenderer) printOverallSummary() {
	total := p.stats.counts()

	fmt.Printf("\n %s %s\n", markCreate, summaryStyle.Render(fmt.Sprintf("Project render complete (created: %d, skipped: %d)", total.created, total.skipped)))

	if total.skipped > 0 {
		fmt.Printf("   %s existing files preserved: %d\n", markSkip, total.skipped)
	}

	next := p.nextSteps()
	if len(next) > 0 {
		fmt.Printf("   %s\n", nextStyle.Render("next steps:"))
		for _, step := range next {
			fmt.Printf("     %s %s\n", bulletStyle, step)
		}
	}
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
