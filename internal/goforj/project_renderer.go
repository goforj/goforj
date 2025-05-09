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

//go:embed all:templates
var templates embed.FS

type ComponentRenderInput struct {
	components Components
	renderAll  bool
}

type ProjectRenderer struct {
	logger *logger.AppLogger
	config *ProjectConfig
}

func NewProjectRenderer(logger *logger.AppLogger) *ProjectRenderer {
	return &ProjectRenderer{logger: logger}
}

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

	fmt.Printf("%s Go Mod ❯\n", EmojiComponent)
	if err := p.createGoMod(); err != nil {
		p.logger.Debug().Err(err).Msg("Failed to create go.mod")
	}

	fmt.Printf("%s Main ❯\n", EmojiComponent)
	if err := p.renderTemplateFile("main.go", "templates/main.go.tmpl", p.config); err != nil {
		p.logger.Error().Err(err).Msg("Failed to render main.go")
	}

	if err := p.initEnvFiles(); err != nil {
		return err
	}

	if err := p.renderCore(); err != nil {
		return err
	}

	if p.config.Components.Docker {
		if err := p.renderDocker(); err != nil {
			return err
		}
	}

	if p.config.Components.WebAPI || p.config.Components.WebUI {
		if err := p.renderWeb(); err != nil {
			return err
		}
	}

	if p.config.Components.WebUI {
		if err := p.renderUI(); err != nil {
			return err
		}
	}

	if p.config.Components.Database {
		if err := p.renderDatabase(); err != nil {
			return err
		}
	}

	if p.config.Components.Scheduler {
		if err := p.renderScheduler(); err != nil {
			return err
		}
	}

	if p.config.Components.Jobs {
		if err := p.renderJobs(); err != nil {
			return err
		}
	}

	p.logger.Info().Msg("Project rendered successfully.")
	return nil
}

func (p *ProjectRenderer) renderCore() error {
	fmt.Printf("%s Core ❯\n", EmojiComponent)

	coreTemplates := []string{
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
	return p.writeTemplates(coreTemplates)
}

func (p *ProjectRenderer) renderDocker() error {
	fmt.Printf("%s Docker ❯\n", EmojiComponent)

	dockerTemplates := []string{"templates/docker-compose.yml.tmpl"}
	if p.config.Components.Database {
		dockerTemplates = append(dockerTemplates,
			"templates/containers/mariadb/Dockerfile",
			"templates/containers/mariadb/my.cnf",
		)
	}
	return p.writeTemplates(dockerTemplates)
}

func (p *ProjectRenderer) renderWeb() error {
	fmt.Printf("%s Web API ❯\n", EmojiComponent)

	httpTemplates := []string{
		"templates/wire/inject_http.go.tmpl",
		"templates/internal/http/route.go.tmpl",
		"templates/internal/http/serve_cmd.go.tmpl",
		"templates/internal/http/server.go.tmpl",
		"templates/internal/http/spa.go.tmpl",
		"templates/internal/http/types.go.tmpl",
		"templates/internal/hello/controller.go.tmpl",
	}
	return p.writeTemplates(httpTemplates)
}

func (p *ProjectRenderer) renderUI() error {
	fmt.Printf("%s Web UI ❯\n", EmojiComponent)
	uiTemplates := []string{"templates/frontend/dist/index.html.tmpl"}
	return p.writeTemplates(uiTemplates)
}

func (p *ProjectRenderer) renderDatabase() error {
	fmt.Printf("%s Database ❯\n", EmojiComponent)

	dbTemplates := []string{
		"templates/wire/inject_db.go.tmpl",
		"templates/internal/migrations/migrations.go.tmpl",
		"templates/internal/migrations/migrations_test.go.tmpl",
		"templates/internal/migrations/migrate_cmd.go.tmpl",
		"templates/internal/migrations/migrate_rollback_cmd.go.tmpl",
		"templates/internal/migrations/2025_04_25_235625_new_user_table.up.sql.tmpl",
		"templates/internal/migrations/2025_04_25_235625_new_user_table.down.sql.tmpl",
		"templates/internal/modelgen/make_model_cmd.go.tmpl",
	}
	if err := p.writeTemplates(dbTemplates); err != nil {
		return err
	}

	rawTemplates := []string{"templates/internal/modelgen/model.tmpl"}
	return p.writeRawFiles(rawTemplates)
}

func (p *ProjectRenderer) renderScheduler() error {
	fmt.Printf("%s Scheduler (gocron) ❯\n", EmojiComponent)

	schedulerTemplates := []string{
		"templates/internal/scheduler/scheduler.go.tmpl",
		"templates/internal/scheduler/fluent_job_wrapper.go.tmpl",
		"templates/internal/scheduler/fluent_job_wrapper_test.go.tmpl",
		"templates/internal/scheduler/cmd.go.tmpl",
		"templates/wire/wire.go.tmpl",
	}
	return p.writeTemplates(schedulerTemplates)
}

func (p *ProjectRenderer) renderJobs() error {
	fmt.Printf("%s Jobs (Asynq) ❯\n", EmojiComponent)

	jobTemplates := []string{
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
	if err := p.writeTemplates(jobTemplates); err != nil {
		return err
	}

	rawTemplates := []string{"templates/internal/jobs/job.tmpl"}
	return p.writeRawFiles(rawTemplates)
}

func (p *ProjectRenderer) createGoMod() error {
	fmt.Printf("  %s Initializing Go module [%s]\n", EmojiCreate, p.config.GoModuleName)

	if err := exec.Command("go", "mod", "init", p.config.GoModuleName).Run(); err != nil {
		return fmt.Errorf("go mod init: %w", err)
	}
	if err := exec.Command("go", "mod", "tidy").Run(); err != nil {
		return fmt.Errorf("go mod tidy: %w", err)
	}
	fmt.Println(" ✨ Go module initialized successfully.")
	return nil
}

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

func (p *ProjectRenderer) writeTemplates(tmpls []string) error {
	for _, path := range tmpls {
		dest := strings.TrimSuffix(strings.TrimPrefix(path, "templates/"), ".tmpl")
		if err := p.renderTemplateFile(dest, path, p.config); err != nil {
			return err
		}
	}
	return nil
}

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

func (p *ProjectRenderer) initEnvFiles() error {
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

		return p.writeTemplates(envTemplates) // write both in one pass
	}

	return nil
}
