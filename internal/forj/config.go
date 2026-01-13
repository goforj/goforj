package forj

import (
	"gopkg.in/yaml.v3"
	"os"
)

// DevWatch represents a command to be run in development mode.
type DevWatch struct {
	Name  string `yaml:"name"`
	Watch string `yaml:"watch"` // wgo options
	Exec  string `yaml:"exec"`  // bash command to run on change
}

// DevTask represents a task to be run in development mode.
type DevTask struct {
	Name string `yaml:"name"`
	Cmd  string `yaml:"cmd"`
}

// DevConfig represents development lifecycle configuration.
type DevConfig struct {
	Pre               []DevTask  `yaml:"pre"`
	Down              []DevTask  `yaml:"down"`
	DownOnExit        bool       `yaml:"down_on_exit"`
	SoundOnWatchError bool       `yaml:"sound_on_watch_error"`
	Watches           []DevWatch `yaml:"watches"`
}

// ProjectConfig represents the configuration for a project.
type ProjectConfig struct {
	ProjectName  string     `yaml:"project_name"`
	GoModuleName string     `yaml:"module_name"`
	UpdatedAt    string     `yaml:"updated_at"`
	Dev          DevConfig  `yaml:"dev"`
	Components   Components `yaml:"components"`

	// temporary
	AppKey string `yaml:"-"`
}

// Components represents the components of the project.
type Components struct {
	CLI              bool `yaml:"cli"`
	WebAPI           bool `yaml:"web_api"`
	WebUI            bool `yaml:"web_ui"`
	Docker           bool `yaml:"docker"`
	DatabaseMySQL    bool `yaml:"database_mysql"`
	DatabasePostgres bool `yaml:"database_postgres"`
	Scheduler        bool `yaml:"scheduler"`
	Jobs             bool `yaml:"jobs"`
}

// HasDatabase reports whether any database component is enabled.
func (c Components) HasDatabase() bool {
	return c.DatabaseMySQL || c.DatabasePostgres
}

// DatabaseDriver returns the selected database driver name.
func (c Components) DatabaseDriver() string {
	if c.DatabasePostgres {
		return "postgres"
	}
	if c.DatabaseMySQL {
		return "mysql"
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
	return ""
}

// LoadProjectConfig loads the project configuration from the .goforge.yml file.
func LoadProjectConfig() (*ProjectConfig, error) {
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

	return config, nil
}
