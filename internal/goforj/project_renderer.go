package goforj

import (
	"bytes"
	"embed"
	"fmt"
	"github.com/goforj/goforj/internal/crypt"
	"github.com/goforj/goforj/internal/logger"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

const (
	EmojiComponent = "📦"
	EmojiCreate    = "✨"
	EmojiSkip      = "🔘"
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
}

// NewProjectRenderer creates a new ProjectRenderer instance
func NewProjectRenderer(logger *logger.AppLogger) *ProjectRenderer {
	return &ProjectRenderer{logger: logger}
}

// Render is the main rendering function
func (p *ProjectRenderer) Render(input ComponentRenderInput) error {
	p.logger.Info().Msg("Rendering project...")

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
		title     string
		enabled   bool
		templates []string
		raw       []string
		action    func() error
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
				for _, tmpl := range envTemplates {
					name := strings.TrimSuffix(strings.TrimPrefix(tmpl, "templates/"), ".tmpl")
					if _, err := os.Stat(name); err == nil {
						fmt.Printf("  %s File already initialized [%v]\n", EmojiSkip, name)
						continue
					}
					key, err := crypt.GenerateAppKey()
					if err != nil {
						return fmt.Errorf("failed to generate app key: %w", err)
					}
					p.config.AppKey = key
					return p.writeTemplates(envTemplates)
				}
				return nil
			},
		},
		{
			title:   "Core Components Rendering",
			enabled: input.renderAll,
			templates: []string{
				"templates/internal/cmd/hello_world_cmd.go.tmpl",
				"templates/internal/cmd/kong_help_formatter.go.tmpl",
				"templates/internal/cmd/root_cmd.go.tmpl",
				"templates/internal/env/app.go.tmpl",
				"templates/internal/env/env.go.tmpl",
				"templates/internal/env/env_test.go.tmpl",
				"templates/internal/crypt/crypt.go.tmpl",
				"templates/internal/crypt/crypt_test.go.tmpl",
				"templates/internal/logger/app.go.tmpl",
				"templates/internal/logger/wire.go.tmpl",
				"templates/wire/app.go.tmpl",
				"templates/wire/inject_cmd.go.tmpl",
				"templates/wire/wire.go.tmpl",
			},
		},
		{
			title:   "Docker Components Rendering",
			enabled: p.config.Components.Docker,
			templates: append([]string{"templates/docker-compose.yml.tmpl"},
				func() []string {
					if p.config.Components.Database {
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
			templates: []string{
				"templates/wire/inject_http.go.tmpl",
				"templates/internal/http/route.go.tmpl",
				"templates/internal/http/serve_cmd.go.tmpl",
				"templates/internal/http/server.go.tmpl",
				"templates/internal/http/spa.go.tmpl",
				"templates/internal/http/types.go.tmpl",
				"templates/internal/hello/controller.go.tmpl",
			},
		},
		{
			title:     "Web UI Components Rendering",
			enabled:   p.config.Components.WebUI,
			templates: []string{"templates/frontend/dist/index.html.tmpl"},
		},
		{
			title:   "Database Components Rendering",
			enabled: p.config.Components.Database,
			templates: []string{
				"templates/wire/inject_db.go.tmpl",
				"templates/internal/migrations/migrations.go.tmpl",
				"templates/internal/migrations/migrations_test.go.tmpl",
				"templates/internal/migrations/migrate_cmd.go.tmpl",
				"templates/internal/migrations/migrate_rollback_cmd.go.tmpl",
				"templates/internal/migrations/2025_04_25_235625_new_user_table.up.sql.tmpl",
				"templates/internal/migrations/2025_04_25_235625_new_user_table.down.sql.tmpl",
				"templates/internal/modelgen/make_model_cmd.go.tmpl",
			},
			raw: []string{"templates/internal/modelgen/model.tmpl"},
		},
		{
			title:   "Scheduler Components Rendering",
			enabled: p.config.Components.Scheduler,
			templates: []string{
				"templates/internal/scheduler/scheduler.go.tmpl",
				"templates/internal/scheduler/fluent_job_wrapper.go.tmpl",
				"templates/internal/scheduler/fluent_job_wrapper_test.go.tmpl",
				"templates/internal/scheduler/cmd.go.tmpl",
				"templates/wire/inject_scheduler.go.tmpl",
			},
		},
		{
			title:   "Job Components Rendering",
			enabled: p.config.Components.Jobs,
			templates: []string{
				"templates/internal/jobs/example_hello_job.go.tmpl",
				"templates/internal/jobs/example_hello_job_cmd.go.tmpl",
				"templates/internal/jobs/make_job_cmd.go.tmpl",
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

		fmt.Printf("%s %s\n", EmojiComponent, step.title)

		if len(step.templates) > 0 {
			if err := p.writeTemplates(step.templates); err != nil {
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
	}

	p.logger.Info().Msg("Project rendered successfully.")
	return nil
}

// createGoMod initializes the go.mod for the project
func (p *ProjectRenderer) createGoMod() error {
	fmt.Printf("  %s Initializing Go module [%s]\n", EmojiCreate, p.config.GoModuleName)
	if err := exec.Command("go", "mod", "init", p.config.GoModuleName).Run(); err != nil {
		fmt.Printf("  %s Go module already initialized [%s]\n", EmojiSkip, p.config.GoModuleName)
	}
	if err := exec.Command("go", "mod", "tidy").Run(); err != nil {
		return fmt.Errorf("go mod tidy: %w", err)
	}
	fmt.Printf("  %s Go module initialized successfully.\n", EmojiSkip)
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
		fmt.Printf("  %s Skipping unchanged file [%v]\n", EmojiSkip, destPath)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(destPath, newContent, 0644); err != nil {
		return err
	}
	fmt.Printf("  %s Creating file [%v]\n", EmojiCreate, destPath)
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
		fmt.Printf("  %s Creating raw file [%v]\n", EmojiCreate, dest)
	}
	return nil
}
