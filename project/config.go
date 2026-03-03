package project

import (
	"os"

	"gopkg.in/yaml.v3"
)

// DevWatch represents a command to be run in development mode.
type DevWatch struct {
	Name  string `yaml:"name" json:"name"`
	Watch string `yaml:"watch" json:"watch"` // wgo options
	Exec  string `yaml:"exec" json:"exec"`   // bash command to run on change
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
	Watches           []DevWatch `yaml:"watches" json:"watches"`
}

// RenderConfig represents render-time defaults and selections.
type RenderConfig struct {
	Components    Components `yaml:"components" json:"components"`
	QueueDriver   string     `yaml:"queue_driver" json:"queue_driver"`
	GoForjVersion string     `yaml:"goforj_version" json:"goforj_version"`
}

// ProjectConfig represents the configuration for a project.
type ProjectConfig struct {
	ProjectName  string       `yaml:"project_name" json:"project_name"`
	GoModuleName string       `yaml:"module_name" json:"module_name"`
	UpdatedAt    string       `yaml:"updated_at" json:"updated_at"`
	Dev          DevConfig    `yaml:"dev" json:"dev"`
	Render       RenderConfig `yaml:"render" json:"render"`
	Components   Components   `yaml:"-" json:"components"`

	// temporary
	AppKey          string `yaml:"-" json:"-"`
	DevConsoleToken string `yaml:"-" json:"-"`
	JWTSecretKey    string `yaml:"-" json:"-"`
}

// Config is the preferred name for project configuration.
type Config = ProjectConfig

// Components represents the components of the project.
type Components struct {
	CLI              bool `yaml:"cli" json:"cli"`
	DemoApp          bool `yaml:"demo_app" json:"demo_app"`
	WebAPI           bool `yaml:"web_api" json:"web_api"`
	WebUI            bool `yaml:"web_ui" json:"web_ui"`
	Docker           bool `yaml:"docker" json:"docker"`
	DatabaseMySQL    bool `yaml:"database_mysql" json:"database_mysql"`
	DatabasePostgres bool `yaml:"database_postgres" json:"database_postgres"`
	DatabaseSQLite   bool `yaml:"database_sqlite" json:"database_sqlite"`
	Scheduler        bool `yaml:"scheduler" json:"scheduler"`
	Jobs             bool `yaml:"jobs" json:"jobs"`
	StressTest       bool `yaml:"stress_test" json:"stress_test"`
}

// HasDatabase reports whether any database component is enabled.
func (c Components) HasDatabase() bool {
	return c.DatabaseMySQL || c.DatabasePostgres || c.DatabaseSQLite
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
	config.Components = config.Render.Components

	return config, nil
}
