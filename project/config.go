package project

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// DevWatch represents a command to be run in development mode.
type DevWatch struct {
	Name  string            `yaml:"name" json:"name"`
	Watch string            `yaml:"watch" json:"watch"` // wgo options
	Exec  string            `yaml:"exec" json:"exec"`   // bash command to run on change
	Env   map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
}

// DevTask represents a task to be run in development mode.
type DevTask struct {
	Name string `yaml:"name" json:"name"`
	Cmd  string `yaml:"cmd" json:"cmd"`
}

// DevConfig represents development lifecycle configuration.
type DevConfig struct {
	Pre               []DevTask  `yaml:"pre" json:"pre"`
	Down              []DevTask  `yaml:"down" json:"down"`
	AutoMigrate       bool       `yaml:"auto_migrate" json:"auto_migrate"`
	DownOnExit        bool       `yaml:"down_on_exit" json:"down_on_exit"`
	SoundOnWatchError bool       `yaml:"sound_on_watch_error" json:"sound_on_watch_error"`
	WirePaths         []string   `yaml:"wire_paths" json:"wire_paths"`
	Watches           []DevWatch `yaml:"watches" json:"watches"`
}

// RenderConfig represents render-time defaults and selections.
type RenderConfig struct {
	Components    Components `yaml:"components" json:"components"`
	StarterKit    StarterKit `yaml:"starter_kit" json:"starter_kit"`
	QueueDriver   string     `yaml:"queue_driver" json:"queue_driver"`
	GoForjVersion string     `yaml:"goforj_version" json:"goforj_version"`
	// ModuleReplaces applies optional local go.mod replace directives before dependency sync.
	ModuleReplaces map[string]string `yaml:"module_replaces,omitempty" json:"module_replaces,omitempty"`
}

// ProjectConfig represents the configuration for a project.
type ProjectConfig struct {
	ProjectName  string       `yaml:"project_name" json:"project_name"`
	GoModuleName string       `yaml:"module_name" json:"module_name"`
	UpdatedAt    string       `yaml:"updated_at" json:"updated_at"`
	Dev          DevConfig    `yaml:"dev" json:"dev"`
	Render       RenderConfig `yaml:"render" json:"render"`

	// temporary
	AppKey          string `yaml:"-" json:"-"`
	AppDiagToken    string `yaml:"-" json:"-"`
	LighthouseSecret string `yaml:"-" json:"-"`
	JWTSecretKey    string `yaml:"-" json:"-"`
}

// Config is the preferred name for project configuration.
type Config = ProjectConfig

// Components represents the components of the project.
type Components struct {
	CLI              bool `yaml:"cli" json:"cli"`
	DemoApp          bool `yaml:"demo_app" json:"demo_app"`
	Mail             bool `yaml:"mail" json:"mail"`
	Auth             bool `yaml:"auth" json:"auth"`
	OAuth            bool `yaml:"oauth" json:"oauth"`
	WebAPI           bool `yaml:"web_api" json:"web_api"`
	WebUI            bool `yaml:"web_ui" json:"web_ui"`
	Metrics          bool `yaml:"metrics" json:"metrics"`
	Observability    bool `yaml:"observability" json:"observability"`
	Grafana          bool `yaml:"grafana" json:"grafana"`
	Docker           bool `yaml:"docker" json:"docker"`
	DatabaseMySQL    bool `yaml:"database_mysql" json:"database_mysql"`
	DatabasePostgres bool `yaml:"database_postgres" json:"database_postgres"`
	DatabaseSQLite   bool `yaml:"database_sqlite" json:"database_sqlite"`
	Scheduler        bool `yaml:"scheduler" json:"scheduler"`
	Jobs             bool `yaml:"jobs" json:"jobs"`
	StressTest       bool `yaml:"stress_test" json:"stress_test"`
}

// Enabled reports whether a component is enabled.
func (c Components) Enabled(key ComponentKey) bool {
	switch key {
	case ComponentCLI:
		return c.CLI
	case ComponentDemoApp:
		return c.DemoApp
	case ComponentMail:
		return c.Mail
	case ComponentAuth:
		return c.Auth
	case ComponentOAuth:
		return c.OAuth
	case ComponentWebAPI:
		return c.WebAPI
	case ComponentWebUI:
		return c.WebUI
	case ComponentMetrics:
		return c.Metrics
	case ComponentObservability:
		return c.Observability
	case ComponentGrafana:
		return c.Grafana
	case ComponentDocker:
		return c.Docker
	case ComponentDatabaseMySQL:
		return c.DatabaseMySQL
	case ComponentDatabasePostgres:
		return c.DatabasePostgres
	case ComponentDatabaseSQLite:
		return c.DatabaseSQLite
	case ComponentScheduler:
		return c.Scheduler
	case ComponentJobs:
		return c.Jobs
	case ComponentStressTest:
		return c.StressTest
	default:
		return false
	}
}

// SetEnabled toggles a component by catalog key.
func (c *Components) SetEnabled(key ComponentKey, enabled bool) {
	if c == nil {
		return
	}
	switch key {
	case ComponentCLI:
		c.CLI = enabled
	case ComponentDemoApp:
		c.DemoApp = enabled
	case ComponentMail:
		c.Mail = enabled
	case ComponentAuth:
		c.Auth = enabled
	case ComponentOAuth:
		c.OAuth = enabled
	case ComponentWebAPI:
		c.WebAPI = enabled
	case ComponentWebUI:
		c.WebUI = enabled
	case ComponentMetrics:
		c.Metrics = enabled
	case ComponentObservability:
		c.Observability = enabled
	case ComponentGrafana:
		c.Grafana = enabled
	case ComponentDocker:
		c.Docker = enabled
	case ComponentDatabaseMySQL:
		c.DatabaseMySQL = enabled
	case ComponentDatabasePostgres:
		c.DatabasePostgres = enabled
	case ComponentDatabaseSQLite:
		c.DatabaseSQLite = enabled
	case ComponentScheduler:
		c.Scheduler = enabled
	case ComponentJobs:
		c.Jobs = enabled
	case ComponentStressTest:
		c.StressTest = enabled
	}
}

// ResolveDependencies applies dependency rules in-place without mutating the original config source.
func (c *Components) ResolveDependencies() {
	if c == nil {
		return
	}
	changed := true
	for changed {
		changed = false
		for _, definition := range ComponentCatalog() {
			if !c.Enabled(definition.Key) {
				continue
			}
			for _, required := range definition.Requires {
				if c.Enabled(required) {
					continue
				}
				c.SetEnabled(required, true)
				changed = true
			}
		}
	}
}

// WithResolvedDependencies returns a copy with dependency rules applied.
func (c Components) WithResolvedDependencies() Components {
	resolved := c
	resolved.ResolveDependencies()
	return resolved
}

// HasDatabase reports whether any database component is enabled.
func (c Components) HasDatabase() bool {
	return c.DatabaseMySQL || c.DatabasePostgres || c.DatabaseSQLite
}

// ValidateRenderContract reports invalid component combinations that cannot be rendered coherently.
func (c Components) ValidateRenderContract() error {
	c = c.WithResolvedDependencies()
	if c.Auth && !c.WebAPI {
		return fmt.Errorf("auth component requires web_api")
	}
	if c.Auth && !c.HasDatabase() {
		return fmt.Errorf("auth component requires a database")
	}
	if c.OAuth && !c.Auth {
		return fmt.Errorf("oauth component requires auth")
	}
	if c.OAuth && !c.HasDatabase() {
		return fmt.Errorf("oauth component requires a database")
	}
	if c.StressTest && !c.Jobs {
		return fmt.Errorf("stress_test component requires jobs")
	}
	return nil
}

// DatabaseDriver returns the selected database driver name.
func (c Components) DatabaseDriver() string {
	if c.DatabasePostgres {
		return "postgres"
	}
	if c.DatabaseMySQL {
		return "mysql"
	}
	if c.DatabaseSQLite {
		return "sqlite"
	}
	return ""
}

// DatabaseServiceName returns the docker service name for the selected database.
func (c Components) DatabaseServiceName() string {
	if c.DatabasePostgres {
		return "postgres"
	}
	if c.DatabaseMySQL {
		return "mysql"
	}
	if c.DatabaseSQLite {
		return "sqlite"
	}
	return ""
}

// LoadProjectConfig loads the project configuration from the .goforj.yml file.
func LoadProjectConfig() (*Config, error) {
	config := &ProjectConfig{}
	configFile := ".goforj.yml"
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return nil, err
	}
	file, err := os.Open(configFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(config); err != nil {
		return nil, err
	}
	if len(config.Dev.WirePaths) == 0 {
		config.Dev.WirePaths = []string{"wire"}
	}

	return config, nil
}
