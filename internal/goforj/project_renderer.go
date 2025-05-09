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
	EmojiBuild     = "🏗️"
	EmojiSkip      = "🔘"
)

// ProjectRenderer is responsible for rendering the project structure
type ProjectRenderer struct {
	logger *logger.AppLogger
	config *ProjectConfig
}

// NewProjectRenderer creates a new ProjectRenderer
func NewProjectRenderer(logger *logger.AppLogger) *ProjectRenderer {
	return &ProjectRenderer{
		logger: logger,
	}
}

//go:embed all:templates
var templates embed.FS

// Render generates the project structure based on the config and templates
func (p *ProjectRenderer) Render() error {
	p.logger.Info().Msg("Rendering project...")
	cfg, err := LoadProjectConfig()
	if err != nil {
		return err
	}
	p.config = cfg

	p.logger.Info().Msg("Project rendered successfully.")

	fmt.Printf("%s Go Mod ❯\n", EmojiComponent)

	// Create go module
	err = p.createGoMod()
	if err != nil {
		p.logger.Debug().Err(err).Msg("Failed to create go.mod")
	}

	fmt.Printf("%s Main ❯\n", EmojiComponent)

	// main.go
	err = p.renderTemplateFile("main.go", "templates/main.go.tmpl", p.config)
	if err != nil {
		p.logger.Error().Err(err).Msg("Failed to render main.go")
	}

	// Create directories
	err = os.MkdirAll("internal", 0755)
	if err != nil {
		p.logger.Error().Err(err).Msg("Failed to create internal directory")
	}

	err = os.MkdirAll("internal/cmd", 0755)
	if err != nil {
		p.logger.Error().Err(err).Msg("Failed to create internal/cmd directory")
	}

	// env
	var envTemplates = []string{
		"templates/.env.tmpl",
		"templates/.env.host.tmpl",
	}

	for _, envTemplate := range envTemplates {
		f := strings.TrimPrefix(envTemplate, "templates/")
		f = strings.TrimSuffix(f, ".tmpl")

		// check if file already exists
		if _, err := os.Stat(f); err == nil {
			// File exists, skip writing
			fmt.Printf("  %s File already initialized [%v]\n", EmojiSkip, f)
			continue
		}

		key, err := crypt.GenerateAppKey()
		if err != nil {
			return fmt.Errorf("failed to generate app key: %w", err)
		}

		p.config.AppKey = key

		// Create .env and .env.host files
		err = p.writeTemplates(envTemplates)
		if err != nil {
			return err
		}
	}

	// core
	fmt.Printf("%s Core ❯\n", EmojiComponent)

	var coreTemplates = []string{
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
	}

	err = p.writeTemplates(coreTemplates)
	if err != nil {
		return err
	}

	// docker
	fmt.Printf("%s Docker ❯\n", EmojiComponent)

	var dockerTemplates = []string{
		"templates/docker-compose.yml.tmpl",
	}

	if p.config.Components.Database {
		dockerTemplates = append(dockerTemplates, "templates/containers/mariadb/Dockerfile")
		dockerTemplates = append(dockerTemplates, "templates/containers/mariadb/my.cnf")
	}

	err = p.writeTemplates(dockerTemplates)
	if err != nil {
		return err
	}

	// web
	if p.config.Components.WebAPI || p.config.Components.WebUI {
		fmt.Printf("%s Web API ❯\n", EmojiComponent)
		var httpTemplates = []string{
			"templates/wire/inject_http.go.tmpl",
			"templates/internal/http/route.go.tmpl",
			"templates/internal/http/serve_cmd.go.tmpl",
			"templates/internal/http/server.go.tmpl",
			"templates/internal/http/spa.go.tmpl",
			"templates/internal/http/types.go.tmpl",
			"templates/internal/hello/controller.go.tmpl",
		}
		err = p.writeTemplates(httpTemplates)
		if err != nil {
			return err
		}
	}

	if p.config.Components.WebUI {
		fmt.Printf("%s Web UI ❯\n", EmojiComponent)
		var uiTemplates = []string{
			"templates/frontend/dist/index.html.tmpl",
		}
		err = p.writeTemplates(uiTemplates)
		if err != nil {
			return err
		}
	}

	// db
	if p.config.Components.Database {
		fmt.Printf("%s Database ❯\n", EmojiComponent)
		var httpTemplates = []string{
			"templates/wire/inject_db.go.tmpl",
			"templates/internal/migrations/migrations.go.tmpl",
			"templates/internal/migrations/migrations_test.go.tmpl",
			"templates/internal/migrations/migrate_cmd.go.tmpl",
			"templates/internal/migrations/migrate_rollback_cmd.go.tmpl",
			"templates/internal/migrations/2025_04_25_235625_new_user_table.up.sql.tmpl",
			"templates/internal/migrations/2025_04_25_235625_new_user_table.down.sql.tmpl",
			"templates/internal/modelgen/make_model_cmd.go.tmpl",
		}
		err = p.writeTemplates(httpTemplates)
		if err != nil {
			return err
		}

		// write raw files if not changed
		// don't render the template
		var rawTemplates = []string{
			"templates/internal/modelgen/model.tmpl",
		}

		err = p.writeRawFiles(rawTemplates)
		if err != nil {
			return err
		}
	}

	// scheduler
	if p.config.Components.Scheduler {
		fmt.Printf("%s Scheduler (gocron) ❯\n", EmojiComponent)
		var schedulerTemplates = []string{
			"templates/internal/scheduler/scheduler.go.tmpl",
			"templates/internal/scheduler/fluent_job_wrapper.go.tmpl",
			"templates/internal/scheduler/fluent_job_wrapper_test.go.tmpl",
			"templates/internal/scheduler/cmd.go.tmpl",
			"templates/wire/wire.go.tmpl",
		}
		err = p.writeTemplates(schedulerTemplates)
		if err != nil {
			return err
		}
	}

	// jobs
	if p.config.Components.Jobs {
		fmt.Printf("%s Jobs (Asynq) ❯\n", EmojiComponent)
		var schedulerTemplates = []string{
			"templates/internal/jobs/example_hello_job.go.tmpl",
			"templates/internal/jobs/example_hello_job_cmd.go.tmpl",
			"templates/internal/jobs/make_job_cmd.go.tmpl",
			"templates/internal/jobs/worker.go.tmpl",
			"templates/internal/jobs/worker_logger.go.tmpl",
			"templates/internal/jobs/worker_cmd.go.tmpl",
			"templates/wire/inject_jobs.go.tmpl",
			"templates/wire/inject_jobs_app.go.tmpl",
			"templates/wire/wire.go.tmpl",
		}
		err = p.writeTemplates(schedulerTemplates)
		if err != nil {
			return err
		}

		// write raw files if not changed
		// don't render the template
		var rawTemplates = []string{
			"templates/internal/jobs/job.tmpl",
		}

		err = p.writeRawFiles(rawTemplates)
		if err != nil {
			return err
		}
	}

	return nil
}

// renderTemplateFile renders a template with the given data and writes it to destPath
func (p *ProjectRenderer) renderTemplateFile(destPath, tmpl string, data any) error {
	// Read template
	file, err := templates.ReadFile(tmpl)
	if err != nil {
		return err
	}

	// Parse template
	t, err := template.New("").Parse(string(file))
	if err != nil {
		return err
	}

	// Execute template into memory
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return err
	}

	newContent := buf.Bytes()

	// Check if file already exists
	existingContent, err := os.ReadFile(destPath)
	if err == nil {
		// File exists, compare contents
		if bytes.Equal(existingContent, newContent) {
			// Content is the same, skip writing
			fmt.Printf("  %s Skipping unchanged file [%v]\n", EmojiSkip, destPath)
			return nil
		}
	}

	// Create destination directory if needed
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	// Write new content with explicit 0644
	if err := os.WriteFile(destPath, newContent, 0644); err != nil {
		return err
	}

	fmt.Printf("  %s Creating file [%v]\n", EmojiCreate, destPath)
	return nil
}

func (p *ProjectRenderer) createGoMod() error {
	moduleName := p.config.GoModuleName

	fmt.Printf("  %s Initializing Go module [%s]\n", EmojiCreate, moduleName)

	// Run `go mod init`
	cmd := exec.Command("go", "mod", "init", moduleName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run go mod init: %w", err)
	}

	// Run `go mod tidy`
	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Stdout = os.Stdout
	tidyCmd.Stderr = os.Stderr
	if err := tidyCmd.Run(); err != nil {
		return fmt.Errorf("failed to run go mod tidy: %w", err)
	}

	fmt.Println(" ✨ Go module initialized successfully.")

	return nil
}

// writeTemplates writes the core templates to the project directory
func (p *ProjectRenderer) writeTemplates(templates []string) error {
	for _, templatePath := range templates {
		relPath := strings.TrimPrefix(templatePath, "templates/")
		destPath := strings.TrimSuffix(relPath, ".tmpl")
		//destPath := strings.ReplaceAll(relPath, ".go.tmpl", ".go")

		destDir := filepath.Dir(destPath)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", destDir, err)
		}

		err := p.renderTemplateFile(destPath, templatePath, p.config)
		if err != nil {
			return fmt.Errorf("failed to render template %s: %w", templatePath, err)
		}
	}

	return nil
}

func (p *ProjectRenderer) writeRawFiles(paths []string) error {
	for _, templatePath := range paths {
		relPath := strings.TrimPrefix(templatePath, "templates/")
		destPath := relPath // no .tmpl stripping

		// Read raw content from embed
		rawContent, err := templates.ReadFile(templatePath)
		if err != nil {
			return fmt.Errorf("failed to read raw template %s: %w", templatePath, err)
		}

		// Create destination directory if needed
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", destPath, err)
		}

		if err := os.WriteFile(destPath, rawContent, 0644); err != nil {
			return fmt.Errorf("failed to write raw file %s: %w", destPath, err)
		}

		fmt.Printf("  %s Creating raw file [%v]\n", EmojiCreate, destPath)
	}

	return nil
}
