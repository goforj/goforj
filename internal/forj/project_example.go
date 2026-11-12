package forj

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/goforj/goforj/project"
)

// configureGopherEats gives each executable only the capabilities used by its role.
func configureGopherEats(config *project.Config) {
	config.Render.Example = "gopher_eats"
	config.Render.StarterKit = project.StarterKitVue
	config.Render.StarterKitOptions = project.NewStarterKitOptions(false)
	config.Render.Components = project.Components{CLI: true, WebAPI: true, WebUI: true, Events: true, Metrics: true, Observability: true, Grafana: true, Docker: true}
	if config.Apps == nil {
		config.Apps = make(map[string]project.AppConfig)
	}
	for name, components := range map[string]project.Components{
		"kitchen":  {CLI: true, WebAPI: true, Events: true, Jobs: true},
		"delivery": {CLI: true, WebAPI: true, Events: true},
	} {
		if _, exists := config.Apps[name]; !exists {
			config.Apps[name] = project.AppConfig{Components: components, StarterKit: project.StarterKitNone}
		}
	}
}

// scaffoldGopherEats creates editable example code without replacing application-owned changes on rerender.
func (p *ProjectRenderer) scaffoldGopherEats() error {
	return fs.WalkDir(templatesFS, "gopher_eats/internal", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".tmpl") {
			return nil
		}
		destination := strings.TrimSuffix(strings.TrimPrefix(path, "gopher_eats/"), ".tmpl")
		return p.writeTemplateMappingsOnce([]templateMapping{mapTemplateTo(path, destination)})
	})
}

// scaffoldGopherEatsFrontend overlays the reference UI once while retaining the Vue starter's pinned toolchain.
func (p *ProjectRenderer) scaffoldGopherEatsFrontend() error {
	if exists, err := p.workspace.exists("cmd/app/frontend/src/eats/model.ts"); err != nil {
		return err
	} else if exists {
		return nil
	}
	return p.copyRawPathToDest("gopher_eats/frontend", "cmd/app/frontend")
}

// prepareExample applies only explicitly selected example defaults before resource and App discovery.
func (p *ProjectRenderer) prepareExample(input *ComponentRenderInput) error {
	switch p.config.Render.Example {
	case "":
		return nil
	case "gopher_eats":
		if p.config.Render.Components.DemoApp {
			return fmt.Errorf("Gopher Eats and Uptime Gopher cannot be selected together")
		}
	default:
		return fmt.Errorf("unknown reference example %q", p.config.Render.Example)
	}
	if _, exists := p.config.Apps["kitchen"]; !exists {
		configureGopherEats(p.config)
	}
	if !p.config.Render.Components.WebUI || !p.config.Render.Components.WebAPI || !p.config.Render.Components.Events || p.config.Render.StarterKit != project.StarterKitVue || !p.config.Apps["kitchen"].Components.WebAPI || !p.config.Apps["kitchen"].Components.Jobs || !p.config.Apps["kitchen"].Components.Events || !p.config.Apps["delivery"].Components.WebAPI || !p.config.Apps["delivery"].Components.Events {
		return fmt.Errorf("Gopher Eats requires the Vue storefront, Web API and Events in each App, and kitchen Jobs")
	}
	if p.config.Dev.Apps == nil {
		p.config.Dev.Apps = make(map[string]project.DevApp)
	}
	for _, name := range []string{"app", "kitchen", "delivery"} {
		if _, exists := p.config.Dev.Apps[name]; !exists {
			p.config.Dev.Apps[name] = generatedDevAppConfig(p.config, project.AppForName(name), "")
		}
	}
	if len(p.config.Dev.Pre) == 0 {
		p.config.Dev.Pre = []project.DevTask{{Name: "Run Docker Compose", Cmd: dockerComposeUpDevCommand(p.config.Render.Components)}}
		if len(p.config.Dev.Down) == 0 {
			p.config.Dev.Down = []project.DevTask{{Name: "Docker Compose Down", Cmd: dockerComposeDownDevCommand()}}
		}
	}
	for _, name := range []string{"kitchen", "delivery"} {
		path := filepath.Join("cmd", name, "main.go")
		if exists, err := p.workspace.exists(path); err != nil {
			return err
		} else if !exists {
			if err := os.MkdirAll(p.workspace.path(filepath.Dir(path)), 0755); err != nil {
				return err
			}
			if err := p.workspace.writeFile(path, []byte("package main\n"), 0644); err != nil {
				return err
			}
		}
	}
	if exists, err := p.workspace.exists(".env"); err != nil {
		return err
	} else if !exists {
		plan, err := project.DefaultResourcePlan(project.ProjectComponents(p.config))
		if err != nil {
			return err
		}
		for _, key := range []project.ResourceKey{project.ResourceQueue, project.ResourceEvents} {
			plan = plan.WithSelection(key, project.DriverSelection{Active: "nats", Supported: []string{"nats"}})
		}
		input.resourcePlan = plan
	}
	return nil
}

// selectedDemoSummary keeps wizard review labels aligned with the selected reference application.
func (m model) selectedDemoSummary() string {
	if m.config.Render.Example == "gopher_eats" {
		return "Gopher Eats (multi-app pizza delivery)"
	}
	if m.config.Render.Components.DemoApp {
		return "Uptime Gopher (monitoring)"
	}
	return "Off"
}
