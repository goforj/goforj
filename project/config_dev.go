package project

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// devFlowStrings keeps matcher lists compact in generated lifecycle configuration.
type devFlowStrings []string

// MarshalYAML encodes matcher values inline so lifecycle configuration stays scan-friendly.
func (values devFlowStrings) MarshalYAML() (any, error) {
	node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
	for _, value := range values {
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
	}
	return node, nil
}

// UsesStructuredApps reports whether dev.apps was explicitly configured, including an empty allowlist.
func (c DevConfig) UsesStructuredApps() bool {
	return c.appsConfigured || c.Apps != nil
}

// UnmarshalYAML preserves the distinction between an absent legacy app model and an explicit empty allowlist.
func (c *DevConfig) UnmarshalYAML(value *yaml.Node) error {
	type devConfigFields DevConfig
	var fields devConfigFields
	if err := value.Decode(&fields); err != nil {
		return fmt.Errorf("decode dev config: %w", err)
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

// MarshalYAML retains explicit empty lifecycle maps whose presence disables historical discovery.
func (c DevConfig) MarshalYAML() (any, error) {
	type devConfigFields DevConfig
	var node yaml.Node
	if err := node.Encode(devConfigFields(c)); err != nil {
		return nil, fmt.Errorf("encode dev config: %w", err)
	}
	if c.Run != nil && len(c.Run) == 0 {
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "run"},
			&yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"},
		)
	}
	if c.UsesStructuredApps() && len(c.Apps) == 0 {
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "apps"},
			&yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"},
		)
	}
	return &node, nil
}

// DevWatchMatchers provides precise file or directory matching when the shared
// watch and ignore lists are not specific enough.
type DevWatchMatchers struct {
	Include []string `yaml:"include,omitempty" json:"include,omitempty"`
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
}

// Empty reports whether the matcher set has no configured rules.
func (m DevWatchMatchers) Empty() bool {
	return len(m.Include) == 0 && len(m.Exclude) == 0
}

// IsLegacy reports whether the watcher uses the historical wgo flag grammar.
func (w DevWatch) IsLegacy() bool {
	return w.Legacy || w.Watch != ""
}

// UnmarshalYAML preserves scalar watch values as legacy wgo input while list
// values opt into native matcher semantics.
func (w *DevWatch) UnmarshalYAML(value *yaml.Node) error {
	type watchFields struct {
		Name     string            `yaml:"name"`
		Watch    yaml.Node         `yaml:"watch"`
		Include  []string          `yaml:"include"`
		Ignore   []string          `yaml:"ignore"`
		Roots    []string          `yaml:"roots"`
		Root     string            `yaml:"root"`
		WorkDir  string            `yaml:"workdir"`
		Files    DevWatchMatchers  `yaml:"files"`
		Dirs     DevWatchMatchers  `yaml:"dirs"`
		Exec     string            `yaml:"exec"`
		Env      map[string]string `yaml:"env"`
		Debounce string            `yaml:"debounce"`
		Poll     string            `yaml:"poll"`
		Postpone bool              `yaml:"postpone"`
		Restart  bool              `yaml:"restart"`
		Exit     bool              `yaml:"exit"`
		Stdin    bool              `yaml:"stdin"`
	}

	var fields watchFields
	if err := value.Decode(&fields); err != nil {
		return fmt.Errorf("decode dev watcher: %w", err)
	}
	if len(fields.Include) > 0 && fields.Watch.Kind != 0 {
		return fmt.Errorf("decode dev watcher %q: watch and include cannot both be set", fields.Name)
	}

	*w = DevWatch{
		Name:     fields.Name,
		Include:  fields.Include,
		Ignore:   fields.Ignore,
		Roots:    fields.Roots,
		WorkDir:  fields.WorkDir,
		Files:    fields.Files,
		Dirs:     fields.Dirs,
		Exec:     fields.Exec,
		Env:      fields.Env,
		Debounce: fields.Debounce,
		Poll:     fields.Poll,
		Postpone: fields.Postpone,
		Restart:  fields.Restart,
		Exit:     fields.Exit,
		Stdin:    fields.Stdin,
	}
	if fields.Root != "" {
		if len(w.Roots) > 0 {
			return fmt.Errorf("decode dev watcher %q: root and roots cannot both be set", fields.Name)
		}
		w.Roots = []string{fields.Root}
	}

	switch fields.Watch.Kind {
	case 0:
		return nil
	case yaml.ScalarNode:
		if fields.Watch.Tag != "!!str" {
			return fmt.Errorf("decode dev watcher %q: scalar watch must be a string", fields.Name)
		}
		if err := fields.Watch.Decode(&w.Watch); err != nil {
			return err
		}
		w.Legacy = w.Watch == ""
		return nil
	case yaml.SequenceNode:
		if err := fields.Watch.Decode(&w.Include); err != nil {
			return fmt.Errorf("decode dev watcher %q native watch matchers: %w", fields.Name, err)
		}
		return nil
	default:
		return fmt.Errorf("decode dev watcher %q: watch must be a legacy string or matcher list", fields.Name)
	}
}

// MarshalYAML writes the same watch key used by existing project files while
// choosing its scalar or list shape from the watcher mode.
func (w DevWatch) MarshalYAML() (any, error) {
	type watchFields struct {
		Name     string            `yaml:"name"`
		Watch    any               `yaml:"watch,omitempty"`
		Ignore   []string          `yaml:"ignore,omitempty"`
		Roots    []string          `yaml:"roots,omitempty"`
		WorkDir  string            `yaml:"workdir,omitempty"`
		Files    DevWatchMatchers  `yaml:"files,omitempty"`
		Dirs     DevWatchMatchers  `yaml:"dirs,omitempty"`
		Exec     string            `yaml:"exec"`
		Env      map[string]string `yaml:"env,omitempty"`
		Debounce string            `yaml:"debounce,omitempty"`
		Poll     string            `yaml:"poll,omitempty"`
		Postpone bool              `yaml:"postpone,omitempty"`
		Restart  bool              `yaml:"restart,omitempty"`
		Exit     bool              `yaml:"exit,omitempty"`
		Stdin    bool              `yaml:"stdin,omitempty"`
	}

	var watch any
	if w.IsLegacy() {
		watch = w.Watch
	} else if len(w.Include) > 0 {
		watch = w.Include
	}
	return watchFields{
		Name:     w.Name,
		Watch:    watch,
		Ignore:   w.Ignore,
		Roots:    w.Roots,
		WorkDir:  w.WorkDir,
		Files:    w.Files,
		Dirs:     w.Dirs,
		Exec:     w.Exec,
		Env:      w.Env,
		Debounce: w.Debounce,
		Poll:     w.Poll,
		Postpone: w.Postpone,
		Restart:  w.Restart,
		Exit:     w.Exit,
		Stdin:    w.Stdin,
	}, nil
}

// DevApp describes one app's participation in the native development loop.
type DevApp struct {
	Build *DevAppCommand    `yaml:"build,omitempty" json:"build,omitempty"`
	Run   *DevAppCommand    `yaml:"run,omitempty" json:"run,omitempty"`
	SPAs  map[string]DevSPA `yaml:"spas,omitempty" json:"spas,omitempty"`
}

// UnmarshalYAML accepts true as the concise default app or an app override mapping.
func (a *DevApp) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		if value.Tag != "!!bool" {
			return fmt.Errorf("decode dev app: expected true or an app mapping")
		}
		var included bool
		if err := value.Decode(&included); err != nil {
			return fmt.Errorf("decode dev app: %w", err)
		}
		if !included {
			return fmt.Errorf("decode dev app: false is invalid; remove the app from dev.apps to exclude it")
		}
		*a = DevApp{}
		return nil
	case yaml.MappingNode:
		type appFields struct {
			Build *DevAppCommand    `yaml:"build"`
			Run   *DevAppCommand    `yaml:"run"`
			SPAs  map[string]DevSPA `yaml:"spas"`
		}
		var fields appFields
		if err := value.Decode(&fields); err != nil {
			return fmt.Errorf("decode dev app: %w", err)
		}
		*a = DevApp{Build: fields.Build, Run: fields.Run, SPAs: fields.SPAs}
		return nil
	default:
		return fmt.Errorf("decode dev app: expected true or an app mapping")
	}
}

// MarshalYAML keeps default-only apps concise while retaining explicit lifecycle overrides.
func (a DevApp) MarshalYAML() (any, error) {
	if a.Build == nil && a.Run == nil && len(a.SPAs) == 0 {
		return true, nil
	}
	type appFields struct {
		Build *DevAppCommand    `yaml:"build,omitempty"`
		Run   *DevAppCommand    `yaml:"run,omitempty"`
		SPAs  map[string]DevSPA `yaml:"spas,omitempty"`
	}
	return appFields{Build: a.Build, Run: a.Run, SPAs: a.SPAs}, nil
}

// DevAppCommand describes a build or runtime command override for an app.
type DevAppCommand struct {
	Disabled     bool              `yaml:"-" json:"disabled,omitempty"`
	Shorthand    bool              `yaml:"-" json:"-"`
	Exec         string            `yaml:"exec,omitempty" json:"exec,omitempty"`
	Watch        []string          `yaml:"watch,omitempty" json:"watch,omitempty"`
	Ignore       []string          `yaml:"ignore,omitempty" json:"ignore,omitempty"`
	Root         string            `yaml:"root,omitempty" json:"root,omitempty"`
	WorkDir      string            `yaml:"workdir,omitempty" json:"workdir,omitempty"`
	Env          map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Debounce     string            `yaml:"debounce,omitempty" json:"debounce,omitempty"`
	Poll         string            `yaml:"poll,omitempty" json:"poll,omitempty"`
	Postpone     bool              `yaml:"postpone,omitempty" json:"postpone,omitempty"`
	PostponeSet  bool              `yaml:"-" json:"-"`
	conventional bool
}

// IsMapping reports whether the command was explicitly expressed as a full override mapping.
func (c DevAppCommand) IsMapping() bool {
	return !c.Disabled && !c.Shorthand && !c.conventional
}

// IsConventional reports whether the command requests framework-derived behavior without overrides.
func (c DevAppCommand) IsConventional() bool {
	return c.conventional
}

// UnmarshalYAML accepts command strings for the common case, booleans for
// default or disabled behavior, and mappings for native watcher controls.
func (c *DevAppCommand) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		switch value.Tag {
		case "!!bool":
			var enabled bool
			if err := value.Decode(&enabled); err != nil {
				return fmt.Errorf("decode dev app command: %w", err)
			}
			*c = DevAppCommand{Disabled: !enabled, conventional: enabled}
			return nil
		case "!!str":
			var exec string
			if err := value.Decode(&exec); err != nil {
				return fmt.Errorf("decode dev app command: %w", err)
			}
			*c = DevAppCommand{Exec: exec, Shorthand: true}
			return nil
		default:
			return fmt.Errorf("decode dev app command: expected a command string, boolean, or mapping")
		}
	case yaml.MappingNode:
		type commandFields DevAppCommand
		var fields commandFields
		if err := value.Decode(&fields); err != nil {
			return fmt.Errorf("decode dev app command: %w", err)
		}
		*c = DevAppCommand(fields)
		for index := 0; index+1 < len(value.Content); index += 2 {
			if value.Content[index].Value == "postpone" {
				c.PostponeSet = true
				break
			}
		}
		return nil
	default:
		return fmt.Errorf("decode dev app command: expected a command string, boolean, or mapping")
	}
}

// MarshalYAML emits command-only overrides as scalars and expanded overrides
// as mappings so generated config stays readable.
func (c DevAppCommand) MarshalYAML() (any, error) {
	if c.Disabled {
		return false, nil
	}
	if c.IsConventional() {
		return true, nil
	}
	if c.Shorthand && c.Exec != "" && len(c.Watch) == 0 && len(c.Ignore) == 0 && c.Root == "" && c.WorkDir == "" && len(c.Env) == 0 && c.Debounce == "" && c.Poll == "" && !c.PostponeSet {
		return c.Exec, nil
	}
	type commandFields struct {
		Exec     string            `yaml:"exec,omitempty"`
		Watch    devFlowStrings    `yaml:"watch,omitempty"`
		Ignore   devFlowStrings    `yaml:"ignore,omitempty"`
		Root     string            `yaml:"root,omitempty"`
		WorkDir  string            `yaml:"workdir,omitempty"`
		Env      map[string]string `yaml:"env,omitempty"`
		Debounce string            `yaml:"debounce,omitempty"`
		Poll     string            `yaml:"poll,omitempty"`
		Postpone *bool             `yaml:"postpone,omitempty"`
	}
	var postpone *bool
	if c.PostponeSet {
		postpone = new(bool)
		*postpone = c.Postpone
	}
	return commandFields{
		Exec: c.Exec, Watch: devFlowStrings(c.Watch), Ignore: devFlowStrings(c.Ignore), Root: c.Root,
		WorkDir: c.WorkDir, Env: c.Env, Debounce: c.Debounce, Poll: c.Poll,
		Postpone: postpone,
	}, nil
}

// DevSPA describes a frontend build owned by an app.
type DevSPA struct {
	Path   string   `yaml:"path" json:"path"`
	Build  string   `yaml:"build,omitempty" json:"build,omitempty"`
	Watch  []string `yaml:"watch,omitempty" json:"watch,omitempty"`
	Ignore []string `yaml:"ignore,omitempty" json:"ignore,omitempty"`
}

// UnmarshalYAML accepts a path scalar for conventional SPAs or an override mapping.
func (s *DevSPA) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		if value.Tag != "!!str" {
			return fmt.Errorf("decode dev SPA: expected a path string or mapping")
		}
		if err := value.Decode(&s.Path); err != nil {
			return fmt.Errorf("decode dev SPA path: %w", err)
		}
		return nil
	case yaml.MappingNode:
		type spaFields DevSPA
		var fields spaFields
		if err := value.Decode(&fields); err != nil {
			return fmt.Errorf("decode dev SPA: %w", err)
		}
		*s = DevSPA(fields)
		return nil
	default:
		return fmt.Errorf("decode dev SPA: expected a path string or mapping")
	}
}

// MarshalYAML emits conventional path-only SPAs as scalars.
func (s DevSPA) MarshalYAML() (any, error) {
	if s.Build == "" && len(s.Watch) == 0 && len(s.Ignore) == 0 {
		return s.Path, nil
	}
	type spaFields struct {
		Path   string         `yaml:"path"`
		Build  string         `yaml:"build,omitempty"`
		Watch  devFlowStrings `yaml:"watch,omitempty"`
		Ignore devFlowStrings `yaml:"ignore,omitempty"`
	}
	return spaFields{
		Path: s.Path, Build: s.Build,
		Watch: devFlowStrings(s.Watch), Ignore: devFlowStrings(s.Ignore),
	}, nil
}
