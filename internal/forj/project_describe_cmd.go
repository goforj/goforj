package forj

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/goforj/goforj/internal/projectlayout"
	"github.com/goforj/goforj/project"
	"github.com/goforj/goforj/version"
)

const (
	// projectDescribeSchemaVersion identifies the first static project descriptor schema.
	projectDescribeSchemaVersion = 1
	// projectDescribeCapability identifies the CLI contract implemented by this command.
	projectDescribeCapability = "project-descriptor.v1"
)

// ProjectDescribeReport is the stable static project descriptor returned to machine consumers.
type ProjectDescribeReport struct {
	SchemaVersion int                    `json:"schema_version"`
	Project       ProjectDescribeProject `json:"project"`
	GoForj        ProjectDescribeGoForj  `json:"goforj"`
	Apps          []ProjectDescribeApp   `json:"apps"`
}

// ProjectDescribeProject identifies one project without exposing its filesystem location or environment.
type ProjectDescribeProject struct {
	Name         string `json:"name"`
	Module       string `json:"module"`
	ConfigDigest string `json:"config_digest"`
}

// ProjectDescribeGoForj describes the CLI and generated-project capabilities relevant to the static contract.
type ProjectDescribeGoForj struct {
	Version          string                          `json:"version"`
	CLICapabilities  []string                        `json:"cli_capabilities"`
	GeneratedProject ProjectDescribeGeneratedProject `json:"generated_project"`
}

// ProjectDescribeGeneratedProject describes generated-project compatibility without inspecting runtime state.
type ProjectDescribeGeneratedProject struct {
	Generation   string   `json:"generation"`
	Capabilities []string `json:"capabilities"`
}

// ProjectDescribeApp describes one available App and its conventional runtime intent.
type ProjectDescribeApp struct {
	ID         string                   `json:"id"`
	Name       string                   `json:"name"`
	Entrypoint string                   `json:"entrypoint"`
	Runtimes   []ProjectDescribeRuntime `json:"runtimes"`
}

// ProjectDescribeRuntime describes one generated runtime without claiming an effective environment-selected port.
type ProjectDescribeRuntime struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	DefaultPort   int    `json:"default_port"`
	PublicURL     bool   `json:"public_url"`
	ReadinessPath string `json:"readiness_path"`
}

// ProjectDescribeCmd emits the static project descriptor for a machine consumer.
type ProjectDescribeCmd struct {
	JSON bool `name:"json" help:"Print the versioned JSON project descriptor"`

	stdout     io.Writer
	loadConfig func() (*project.Config, error)
	discover   func(string) (projectlayout.Discovery, error)
	cliVersion func() string
}

// Signature declares the machine-oriented project descriptor command.
func (*ProjectDescribeCmd) Signature() string {
	return `name:"project:describe" help:"Describe static project topology as JSON"`
}

// Run emits exactly one descriptor and never loads project dotenv files or starts generated code.
func (c *ProjectDescribeCmd) Run() error {
	if !c.JSON {
		return fmt.Errorf("project:describe requires --json")
	}
	loadConfig := c.loadConfig
	if loadConfig == nil {
		loadConfig = project.LoadProjectConfig
	}
	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load project configuration: %w", err)
	}
	discover := c.discover
	if discover == nil {
		discover = projectlayout.Discover
	}
	discovery, err := discover(".")
	if err != nil {
		return fmt.Errorf("discover project Apps: %w", err)
	}
	cliVersion := c.cliVersion
	if cliVersion == nil {
		cliVersion = version.String
	}
	report, err := newProjectDescribeReport(config, discovery, cliVersion())
	if err != nil {
		return err
	}
	return c.writeReport(report)
}

// newProjectDescribeReport derives a secret-free static projection from configuration and conventional App markers.
func newProjectDescribeReport(config *project.Config, discovery projectlayout.Discovery, cliVersion string) (ProjectDescribeReport, error) {
	if config == nil {
		return ProjectDescribeReport{}, fmt.Errorf("project configuration is required")
	}
	apps := projectDescribeApps(config, discovery.RuntimeApps(config))
	digest, err := projectDescribeDigest(config, apps)
	if err != nil {
		return ProjectDescribeReport{}, err
	}
	return ProjectDescribeReport{
		SchemaVersion: projectDescribeSchemaVersion,
		Project: ProjectDescribeProject{
			Name:         config.ProjectName,
			Module:       config.GoModuleName,
			ConfigDigest: digest,
		},
		GoForj: ProjectDescribeGoForj{
			Version:         cliVersion,
			CLICapabilities: []string{projectDescribeCapability},
			GeneratedProject: ProjectDescribeGeneratedProject{
				Generation:   config.Render.GoForjVersion,
				Capabilities: []string{},
			},
		},
		Apps: apps,
	}, nil
}

// projectDescribeApps preserves generated runtime ordering while making every array explicit in JSON.
func projectDescribeApps(config *project.Config, source []project.App) []ProjectDescribeApp {
	apps := make([]ProjectDescribeApp, 0, len(source))
	for index, app := range source {
		app = projectlayout.NormalizeApp(app)
		components := projectDescribeAppComponents(config, app.Name)
		runtimes := make([]ProjectDescribeRuntime, 0, 1)
		if components.WebAPI || components.WebUI {
			runtimes = append(runtimes, ProjectDescribeRuntime{
				ID:            "http",
				Kind:          "http",
				DefaultPort:   3000 + index,
				PublicURL:     true,
				ReadinessPath: "/-/ready",
			})
		}
		apps = append(apps, ProjectDescribeApp{
			ID:         app.Name,
			Name:       app.Name,
			Entrypoint: app.Entrypoint,
			Runtimes:   runtimes,
		})
	}
	return apps
}

// projectDescribeAppComponents preserves the default App's project-wide component selection.
func projectDescribeAppComponents(config *project.Config, name string) project.Components {
	if name == "" || name == project.DefaultAppName {
		return config.Render.Components.WithResolvedDependencies()
	}
	appConfig, ok := config.Apps[name]
	if !ok {
		return config.Render.Components.WithResolvedDependencies()
	}
	return project.NormalizeConfiguredAppComponents(config, appConfig.Components)
}

// projectDescribeDigest hashes normalized non-secret topology instead of raw configuration or dotenv bytes.
func projectDescribeDigest(config *project.Config, apps []ProjectDescribeApp) (string, error) {
	type appTopology struct {
		Name       string             `json:"name"`
		Components project.Components `json:"components"`
	}
	appNames := make([]string, 0, len(config.Apps))
	for name := range config.Apps {
		appNames = append(appNames, name)
	}
	sort.Strings(appNames)
	configuredApps := make([]appTopology, 0, len(appNames))
	for _, name := range appNames {
		configuredApps = append(configuredApps, appTopology{
			Name:       name,
			Components: projectDescribeAppComponents(config, name),
		})
	}
	payload := struct {
		ProjectName   string               `json:"project_name"`
		ModuleName    string               `json:"module_name"`
		Components    project.Components   `json:"components"`
		Configured    []appTopology        `json:"configured_apps"`
		AvailableApps []ProjectDescribeApp `json:"available_apps"`
	}{
		ProjectName:   config.ProjectName,
		ModuleName:    config.GoModuleName,
		Components:    config.Render.Components.WithResolvedDependencies(),
		Configured:    configuredApps,
		AvailableApps: apps,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode normalized project topology: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// writeReport writes a single JSON object and preserves empty arrays for schema consumers.
func (c *ProjectDescribeCmd) writeReport(report ProjectDescribeReport) error {
	if report.GoForj.CLICapabilities == nil {
		report.GoForj.CLICapabilities = []string{}
	}
	if report.GoForj.GeneratedProject.Capabilities == nil {
		report.GoForj.GeneratedProject.Capabilities = []string{}
	}
	if report.Apps == nil {
		report.Apps = []ProjectDescribeApp{}
	}
	for index := range report.Apps {
		if report.Apps[index].Runtimes == nil {
			report.Apps[index].Runtimes = []ProjectDescribeRuntime{}
		}
	}
	stdout := c.stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	if err := json.NewEncoder(stdout).Encode(report); err != nil {
		return fmt.Errorf("write project descriptor: %w", err)
	}
	return nil
}
