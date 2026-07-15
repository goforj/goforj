package project

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// DevWatch represents a command to be run in development mode.
type DevWatch struct {
	Name     string            `yaml:"name" json:"name"`
	Watch    any               `yaml:"watch,omitempty" json:"watch,omitempty"`
	Include  []string          `yaml:"include,omitempty" json:"include,omitempty"`
	Ignore   []string          `yaml:"ignore,omitempty" json:"ignore,omitempty"`
	Root     string            `yaml:"root,omitempty" json:"root,omitempty"`
	Roots    []string          `yaml:"roots,omitempty" json:"roots,omitempty"`
	WorkDir  string            `yaml:"workdir,omitempty" json:"workdir,omitempty"`
	Files    DevWatchMatchers  `yaml:"files,omitempty" json:"files,omitempty"`
	Dirs     DevWatchMatchers  `yaml:"dirs,omitempty" json:"dirs,omitempty"`
	Exec     string            `yaml:"exec" json:"exec"`
	Env      map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Debounce string            `yaml:"debounce,omitempty" json:"debounce,omitempty"`
	Poll     string            `yaml:"poll,omitempty" json:"poll,omitempty"`
	Postpone bool              `yaml:"postpone,omitempty" json:"postpone,omitempty"`
	Restart  bool              `yaml:"restart,omitempty" json:"restart,omitempty"`
	Exit     bool              `yaml:"exit,omitempty" json:"exit,omitempty"`
	Stdin    bool              `yaml:"stdin,omitempty" json:"stdin,omitempty"`
	Extra    map[string]any    `yaml:",inline" json:"-"`
}

// DevWatchMatchers preserves precise file or directory controls alongside the
// shared matcher lists.
type DevWatchMatchers struct {
	Include []string `yaml:"include,omitempty" json:"include,omitempty"`
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
	// Extra preserves matcher controls introduced by newer GoForj versions during config migration.
	Extra map[string]any `yaml:",inline" json:"-"`
}

// DevTask represents a task to be run in development mode.
type DevTask struct {
	Name string `yaml:"name" json:"name"`
	Cmd  string `yaml:"cmd" json:"cmd"`
}

// DevConfig represents development lifecycle configuration.
type DevConfig struct {
	Pre               []DevTask         `yaml:"pre" json:"pre"`
	Down              []DevTask         `yaml:"down" json:"down"`
	Run               map[string]string `yaml:"run,omitempty" json:"run,omitempty"`
	AutoMigrate       bool              `yaml:"auto_migrate" json:"auto_migrate"`
	DownOnExit        bool              `yaml:"down_on_exit" json:"down_on_exit"`
	SoundOnWatchError bool              `yaml:"sound_on_watch_error" json:"sound_on_watch_error"`
	WirePaths         []string          `yaml:"wire_paths" json:"wire_paths"`
	Watches           []DevWatch        `yaml:"watches,omitempty" json:"watches,omitempty"`
	Apps              map[string]any    `yaml:"apps,omitempty" json:"apps,omitempty"`
	Extra             map[string]any    `yaml:",inline" json:"-"`
	appsConfigured    bool
}

// UnmarshalYAML preserves an explicit empty native App allowlist across config round trips.
func (c *DevConfig) UnmarshalYAML(value *yaml.Node) error {
	type devConfigFields DevConfig
	var fields devConfigFields
	if err := value.Decode(&fields); err != nil {
		return err
	}
	*c = DevConfig(fields)
	for index := 0; index+1 < len(value.Content); index += 2 {
		if value.Content[index].Value == "apps" {
			c.appsConfigured = len(c.Apps) == 0
			break
		}
	}
	return nil
}

// MarshalYAML retains explicit empty lifecycle maps whenever project settings are saved.
func (c DevConfig) MarshalYAML() (any, error) {
	type devConfigFields DevConfig
	var node yaml.Node
	if err := node.Encode(devConfigFields(c)); err != nil {
		return nil, err
	}
	if c.Run != nil && len(c.Run) == 0 {
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "run"},
			&yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"},
		)
	}
	if (c.appsConfigured || c.Apps != nil) && len(c.Apps) == 0 {
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "apps"},
			&yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"},
		)
	}
	return &node, nil
}

// SetApps replaces the native App allowlist while retaining explicit empty-map presence.
func (c *DevConfig) SetApps(apps map[string]any) {
	c.Apps = apps
	c.appsConfigured = len(apps) == 0
}

// DefaultAppName is the implicit single-app name.
const DefaultAppName = "app"

const componentYAMLInlineLimit = 120

// legacyComponentContractVersion is the only retired marker whose mapping semantics this project can migrate safely.
const legacyComponentContractVersion = 1

// RenderConfig represents render-time defaults and selections.
type RenderConfig struct {
	Components     Components        `yaml:"components" json:"components"`
	StarterKit     string            `yaml:"starter_kit" json:"starter_kit"`
	HelpFormat     string            `yaml:"help_format,omitempty" json:"help_format,omitempty"`
	GoForjVersion  string            `yaml:"goforj_version" json:"goforj_version"`
	ModuleReplaces map[string]string `yaml:"module_replaces,omitempty" json:"module_replaces,omitempty"`
	Extra          map[string]any    `yaml:",inline" json:"-"`
}

// UnmarshalYAML consumes obsolete render fields so config rewrites cannot preserve them as extensions.
func (c *RenderConfig) UnmarshalYAML(value *yaml.Node) error {
	type renderConfigFields RenderConfig
	var fields renderConfigFields
	if err := value.Decode(&fields); err != nil {
		return err
	}
	*c = RenderConfig(fields)
	delete(c.Extra, "component_contract")
	delete(c.Extra, "queue_driver")
	return nil
}

// ProjectConfig represents the configuration for a project.
type ProjectConfig struct {
	ProjectName  string         `yaml:"project_name" json:"project_name"`
	GoModuleName string         `yaml:"module_name" json:"module_name"`
	UpdatedAt    string         `yaml:"updated_at" json:"updated_at"`
	Dev          DevConfig      `yaml:"dev" json:"dev"`
	Render       RenderConfig   `yaml:"render" json:"render"`
	Apps         map[string]any `yaml:"apps,omitempty" json:"apps,omitempty"`
	Extra        map[string]any `yaml:",inline" json:"-"`

	// temporary
	AppKey           string `yaml:"-" json:"-"`
	AppDiagToken     string `yaml:"-" json:"-"`
	LighthouseSecret string `yaml:"-" json:"-"`
	JWTSecretKey     string `yaml:"-" json:"-"`
}

// Config is the preferred name for project configuration.
type Config = ProjectConfig

// UnmarshalYAML uses the render component shape to distinguish legacy omission defaults from explicit modern selections.
func (c *ProjectConfig) UnmarshalYAML(value *yaml.Node) error {
	type projectConfigFields ProjectConfig
	var fields projectConfigFields
	if err := value.Decode(&fields); err != nil {
		return fmt.Errorf("decode project config: %w", err)
	}
	*c = ProjectConfig(fields)
	render := yamlMappingValue(value, "render")
	version := 0
	if marker := yamlMappingValue(render, "component_contract"); marker != nil {
		if err := marker.Decode(&version); err != nil {
			return fmt.Errorf("decode legacy component contract: %w", err)
		}
	}
	if version < 0 || version > legacyComponentContractVersion {
		return fmt.Errorf("decode project config: unsupported component contract version %d; this project supports version %d", version, legacyComponentContractVersion)
	}
	renderComponents := yamlMappingValue(render, "components")
	legacyDefaults := version < legacyComponentContractVersion && (renderComponents == nil || renderComponents.Kind == yaml.MappingNode)
	if legacyDefaults {
		c.Render.Components.enableLegacyPrimitiveDefaults()
	}
	if err := normalizeAppComponents(c.Apps, legacyDefaults); err != nil {
		return err
	}
	return nil
}

// yamlMappingValue reads raw field shapes so compatibility decisions do not depend on decoded component values.
func yamlMappingValue(value *yaml.Node, key string) *yaml.Node {
	if value == nil || value.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(value.Content); index += 2 {
		if value.Content[index].Value == key {
			return value.Content[index+1]
		}
	}
	return nil
}

// MarshalYAML preserves compact per-App component lists while leaving extension-shaped App settings untyped.
func (c ProjectConfig) MarshalYAML() (any, error) {
	type projectConfigFields ProjectConfig
	var node yaml.Node
	if err := node.Encode(projectConfigFields(c)); err != nil {
		return nil, err
	}
	setCompactAppComponentSequenceStyle(&node)
	return &node, nil
}

// setCompactAppComponentSequenceStyle restores the shared compact convention after untyped App maps are encoded.
func setCompactAppComponentSequenceStyle(node *yaml.Node) {
	root := node
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = root.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return
	}
	for index := 0; index+1 < len(root.Content); index += 2 {
		if root.Content[index].Value != "apps" {
			continue
		}
		apps := root.Content[index+1]
		if apps.Kind != yaml.MappingNode {
			break
		}
		for appIndex := 1; appIndex < len(apps.Content); appIndex += 2 {
			app := apps.Content[appIndex]
			if app.Kind != yaml.MappingNode {
				continue
			}
			for fieldIndex := 0; fieldIndex+1 < len(app.Content); fieldIndex += 2 {
				if app.Content[fieldIndex].Value == "components" && app.Content[fieldIndex+1].Kind == yaml.SequenceNode {
					app.Content[fieldIndex+1].Style = componentYAMLNodeSequenceStyle(app.Content[fieldIndex+1])
				}
			}
		}
		break
	}
}

// Components represents the components of the project.
type Components struct {
	CLI              bool           `yaml:"cli" json:"cli"`
	DemoApp          bool           `yaml:"demo_app" json:"demo_app"`
	Mail             bool           `yaml:"mail" json:"mail"`
	Auth             bool           `yaml:"auth" json:"auth"`
	OAuth            bool           `yaml:"oauth" json:"oauth"`
	WebAPI           bool           `yaml:"web_api" json:"web_api"`
	WebUI            bool           `yaml:"web_ui" json:"web_ui"`
	Metrics          bool           `yaml:"metrics" json:"metrics"`
	Observability    bool           `yaml:"observability" json:"observability"`
	Grafana          bool           `yaml:"grafana" json:"grafana"`
	Docker           bool           `yaml:"docker" json:"docker"`
	DatabaseMySQL    bool           `yaml:"database_mysql" json:"database_mysql"`
	DatabasePostgres bool           `yaml:"database_postgres" json:"database_postgres"`
	DatabaseSQLite   bool           `yaml:"database_sqlite" json:"database_sqlite"`
	Scheduler        bool           `yaml:"scheduler" json:"scheduler"`
	Cache            bool           `yaml:"cache" json:"cache"`
	Events           bool           `yaml:"events" json:"events"`
	Storage          bool           `yaml:"storage" json:"storage"`
	Jobs             bool           `yaml:"jobs" json:"jobs"`
	Extra            map[string]any `yaml:",inline" json:"-"`
}

// UnmarshalYAML accepts legacy boolean mappings and compact enabled-name sequences.
func (c *Components) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.MappingNode:
		type componentFields Components
		var fields componentFields
		if err := value.Decode(&fields); err != nil {
			return fmt.Errorf("decode component mapping: %w", err)
		}
		*c = Components(fields)
		return nil
	case yaml.SequenceNode:
		var decoded Components
		seen := make(map[string]struct{}, len(value.Content))
		for _, item := range value.Content {
			if item.Kind != yaml.ScalarNode || item.Tag != "!!str" {
				return fmt.Errorf("decode component sequence: entries must be strings")
			}
			name := item.Value
			if !isValidComponentName(name) {
				return fmt.Errorf("decode component sequence: invalid component name %q", name)
			}
			if _, exists := seen[name]; exists {
				return fmt.Errorf("decode component sequence: duplicate component %q", name)
			}
			seen[name] = struct{}{}
			if decoded.setComponentEnabled(name) {
				continue
			}
			if decoded.Extra == nil {
				decoded.Extra = map[string]any{}
			}
			decoded.Extra[name] = true
		}
		*c = decoded
		return nil
	default:
		return fmt.Errorf("decode components: expected a mapping or sequence")
	}
}

// MarshalYAML uses the compact form only when every extension can be represented without loss.
func (c Components) MarshalYAML() (any, error) {
	names := c.enabledComponentNames()
	unknown := make([]string, 0, len(c.Extra))
	for name, value := range c.Extra {
		enabled, ok := value.(bool)
		if !ok || !enabled || isKnownComponentName(name) || !isValidComponentName(name) {
			type componentFields Components
			return componentFields(c), nil
		}
		unknown = append(unknown, name)
	}
	sort.Strings(unknown)
	names = append(names, unknown...)

	node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: componentYAMLSequenceStyle(names)}
	for _, name := range names {
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name})
	}
	return node, nil
}

// componentYAMLNodeSequenceStyle applies the same readable line limit to untyped per-App component lists.
func componentYAMLNodeSequenceStyle(node *yaml.Node) yaml.Style {
	names := make([]string, 0, len(node.Content))
	for _, entry := range node.Content {
		names = append(names, entry.Value)
	}
	return componentYAMLSequenceStyle(names)
}

// componentYAMLSequenceStyle keeps short selections scan-friendly without creating an overlong config line.
func componentYAMLSequenceStyle(names []string) yaml.Style {
	line := "components: [" + strings.Join(names, ", ") + "]"
	if len(line) <= componentYAMLInlineLimit {
		return yaml.FlowStyle
	}
	return 0
}

// setComponentEnabled enables a known component and reports whether the name is recognized.
func (c *Components) setComponentEnabled(name string) bool {
	switch name {
	case "cli":
		c.CLI = true
	case "demo_app":
		c.DemoApp = true
	case "mail":
		c.Mail = true
	case "auth":
		c.Auth = true
	case "oauth":
		c.OAuth = true
	case "web_api":
		c.WebAPI = true
	case "web_ui":
		c.WebUI = true
	case "metrics":
		c.Metrics = true
	case "observability":
		c.Observability = true
	case "grafana":
		c.Grafana = true
	case "docker":
		c.Docker = true
	case "database_mysql":
		c.DatabaseMySQL = true
	case "database_postgres":
		c.DatabasePostgres = true
	case "database_sqlite":
		c.DatabaseSQLite = true
	case "scheduler":
		c.Scheduler = true
	case "cache":
		c.Cache = true
	case "events":
		c.Events = true
	case "storage":
		c.Storage = true
	case "jobs":
		c.Jobs = true
	default:
		return false
	}
	return true
}

// enabledComponentNames returns known enabled names in canonical project order.
func (c Components) enabledComponentNames() []string {
	fields := []struct {
		name    string
		enabled bool
	}{
		{name: "cli", enabled: c.CLI},
		{name: "demo_app", enabled: c.DemoApp},
		{name: "mail", enabled: c.Mail},
		{name: "auth", enabled: c.Auth},
		{name: "oauth", enabled: c.OAuth},
		{name: "web_api", enabled: c.WebAPI},
		{name: "web_ui", enabled: c.WebUI},
		{name: "metrics", enabled: c.Metrics},
		{name: "observability", enabled: c.Observability},
		{name: "grafana", enabled: c.Grafana},
		{name: "docker", enabled: c.Docker},
		{name: "database_mysql", enabled: c.DatabaseMySQL},
		{name: "database_postgres", enabled: c.DatabasePostgres},
		{name: "database_sqlite", enabled: c.DatabaseSQLite},
		{name: "scheduler", enabled: c.Scheduler},
		{name: "cache", enabled: c.Cache},
		{name: "events", enabled: c.Events},
		{name: "storage", enabled: c.Storage},
		{name: "jobs", enabled: c.Jobs},
	}
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		if field.enabled {
			names = append(names, field.name)
		}
	}
	return names
}

// enableLegacyPrimitiveDefaults preserves capabilities that generated Apps received before they became optional.
func (c *Components) enableLegacyPrimitiveDefaults() {
	c.Cache = true
	c.Events = true
	c.Storage = true
}

// normalizeAppComponents makes every declared selection canonical while applying defaults only to genuinely legacy configs.
func normalizeAppComponents(apps map[string]any, legacyDefaults bool) error {
	for name, raw := range apps {
		settings, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		var components Components
		rawComponents, exists := settings["components"]
		if !exists && !legacyDefaults {
			continue
		}
		if exists {
			var node yaml.Node
			if err := node.Encode(rawComponents); err != nil {
				return fmt.Errorf("encode App %s components: %w", name, err)
			}
			if err := node.Decode(&components); err != nil {
				return fmt.Errorf("decode App %s components: %w", name, err)
			}
		}
		if legacyDefaults {
			components.enableLegacyPrimitiveDefaults()
		}
		settings["components"] = components
		apps[name] = settings
	}
	return nil
}

// isKnownComponentName reports whether name belongs to this generated project version.
func isKnownComponentName(name string) bool {
	var components Components
	return components.setComponentEnabled(name)
}

// isValidComponentName reports whether name uses the lowercase snake-case component grammar.
func isValidComponentName(name string) bool {
	if name == "" || name[0] < 'a' || name[0] > 'z' || name[len(name)-1] == '_' {
		return false
	}
	previousUnderscore := false
	for _, char := range name {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			previousUnderscore = false
		case char == '_' && !previousUnderscore:
			previousUnderscore = true
		default:
			return false
		}
	}
	return true
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
	if len(config.Dev.WirePaths) == 0 {
		config.Dev.WirePaths = []string{defaultWireDir()}
	}

	return config, nil
}

// defaultWireDir keeps modern projects on app/wire while retaining the legacy root wire layout.
func defaultWireDir() string {
	if info, err := os.Stat("app/wire"); err == nil && info.IsDir() {
		return "app/wire"
	}
	return "wire"
}
