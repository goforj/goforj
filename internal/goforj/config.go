package goforj

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

// ProjectConfig represents the configuration for a project.
type ProjectConfig struct {
	ProjectName  string     `yaml:"project_name"`
	GoModuleName string     `yaml:"module_name"`
	UpdatedAt    string     `yaml:"updated_at"`
	PreDev       []DevTask  `yaml:"pre_dev"`
	DevWatches   []DevWatch `yaml:"dev_watches"`
	Components   Components `yaml:"components"`

	// temporary
	AppKey string `yaml:"-"`
}

// Components represents the components of the project.
type Components struct {
	CLI       bool `yaml:"cli"`
	WebAPI    bool `yaml:"web_api"`
	WebUI     bool `yaml:"web_ui"`
	Docker    bool `yaml:"docker"`
	Database  bool `yaml:"database"`
	Scheduler bool `yaml:"scheduler"`
	Jobs      bool `yaml:"jobs"`
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
