package project

import (
	"fmt"
	"sort"
	"strings"
)

// DriverSelection records the active implementation and the implementations compiled into an App.
type DriverSelection struct {
	Active    string
	Supported []string
}

// ResourcePlan records concrete resource selections used by generation and rendering.
type ResourcePlan struct {
	Selections      map[ResourceKey]DriverSelection
	NamedSelections map[string]string
}

// GeneratedNamedResourceSelection records a framework-owned named resource requirement.
type GeneratedNamedResourceSelection struct {
	Resource       ResourceKey
	Name           string
	Label          string
	EnvironmentKey string
	Active         string
}

// LocalServiceMode identifies whether GoForj should manage a provisionable service locally.
type LocalServiceMode string

const (
	// LocalServiceModeLocal selects generated local service management.
	LocalServiceModeLocal LocalServiceMode = "local"
	// LocalServiceModeExternal leaves service management to the deployment environment.
	LocalServiceModeExternal LocalServiceMode = "external"
)

// LocalServiceIntent records explicit placement decisions independently from active drivers.
type LocalServiceIntent struct {
	Modes map[ServiceKey]LocalServiceMode
}

// DefaultResourcePlan returns local starting drivers with the common Redis alternatives included.
func DefaultResourcePlan(components Components) (ResourcePlan, error) {
	plan := ResourcePlan{Selections: map[ResourceKey]DriverSelection{}}
	if components.HasDatabase() {
		driver := components.DatabaseDriver()
		if components.DemoApp {
			driver = "mysql"
		}
		if driver == "" {
			return ResourcePlan{}, fmt.Errorf("database capability requires a selected database driver")
		}
		supported := []string{driver}
		if components.DemoApp {
			supported = []string{"sqlite", "mysql"}
		}
		plan.Selections[ResourceDatabase] = DriverSelection{Active: driver, Supported: supported}
	}
	plan.Selections[ResourceCache] = DriverSelection{Active: "memory", Supported: []string{"memory", "redis"}}
	if components.Events {
		plan.Selections[ResourceEvents] = DriverSelection{Active: "inproc", Supported: []string{"inproc", "redis"}}
	}
	if components.Jobs {
		plan.Selections[ResourceQueue] = DriverSelection{Active: "workerpool", Supported: []string{"workerpool", "redis"}}
	}
	plan.Selections[ResourceStorage] = DriverSelection{Active: "local", Supported: []string{"local"}}
	if components.Mail {
		active := "log"
		if components.Docker {
			active = "smtp"
		}
		plan.Selections[ResourceMail] = DriverSelection{Active: active, Supported: []string{"log", "smtp"}}
	}
	plan.NamedSelections = defaultGeneratedNamedDrivers(components)
	return plan.Normalized(components)
}

// Clone returns a deep copy so plan changes do not alias prior values.
func (p ResourcePlan) Clone() ResourcePlan {
	cloned := ResourcePlan{
		Selections:      make(map[ResourceKey]DriverSelection, len(p.Selections)),
		NamedSelections: make(map[string]string, len(p.NamedSelections)),
	}
	for key, selection := range p.Selections {
		cloned.Selections[key] = cloneDriverSelection(selection)
	}
	for key, driver := range p.NamedSelections {
		cloned.NamedSelections[key] = driver
	}
	return cloned
}

// Selection returns a defensive copy of one resource selection.
func (p ResourcePlan) Selection(key ResourceKey) (DriverSelection, bool) {
	selection, ok := p.Selections[key]
	if !ok {
		return DriverSelection{}, false
	}
	return cloneDriverSelection(selection), true
}

// WithSelection returns a cloned plan with one resource selection replaced.
func (p ResourcePlan) WithSelection(key ResourceKey, selection DriverSelection) ResourcePlan {
	cloned := p.Clone()
	if cloned.Selections == nil {
		cloned.Selections = map[ResourceKey]DriverSelection{}
	}
	cloned.Selections[key] = cloneDriverSelection(selection)
	return cloned
}

// WithoutSelection returns a cloned plan without a disabled resource.
func (p ResourcePlan) WithoutSelection(key ResourceKey) ResourcePlan {
	cloned := p.Clone()
	delete(cloned.Selections, key)
	return cloned
}

// WithNamedSelection returns a cloned plan with one generated named-resource driver replaced.
func (p ResourcePlan) WithNamedSelection(environmentKey string, driver string) ResourcePlan {
	cloned := p.Clone()
	if cloned.NamedSelections == nil {
		cloned.NamedSelections = map[string]string{}
	}
	cloned.NamedSelections[environmentKey] = normalizeDriverName(driver)
	return cloned
}

// Normalized validates the plan and returns deterministic driver names and built-in ordering.
func (p ResourcePlan) Normalized(components Components) (ResourcePlan, error) {
	knownResources := map[ResourceKey]bool{}
	knownNamedSelections := map[string]bool{}
	for _, definition := range resourceCatalog {
		knownResources[definition.Key] = true
		for _, named := range definition.NamedResources {
			knownNamedSelections[named.EnvironmentKey] = true
		}
	}
	for key := range p.Selections {
		if !knownResources[key] {
			return ResourcePlan{}, fmt.Errorf("resource plan contains unknown resource %q", key)
		}
	}
	for key := range p.NamedSelections {
		if !knownNamedSelections[key] {
			return ResourcePlan{}, fmt.Errorf("resource plan contains unknown generated named resource %q", key)
		}
	}
	normalized := ResourcePlan{Selections: map[ResourceKey]DriverSelection{}, NamedSelections: map[string]string{}}
	for _, definition := range resourceCatalog {
		selection, exists := p.Selections[definition.Key]
		if !definition.AppliesTo(components) {
			continue
		}
		if !exists {
			return ResourcePlan{}, fmt.Errorf("resource %s requires a driver selection", definition.Label)
		}
		active := CanonicalResourceDriver(definition.Key, selection.Active)
		if active == "" {
			return ResourcePlan{}, fmt.Errorf("resource %s requires an active driver", definition.Label)
		}
		if _, ok := definition.Driver(active); !ok {
			return ResourcePlan{}, fmt.Errorf("resource %s selects unknown active driver %q", definition.Label, selection.Active)
		}
		supported, err := normalizeSupportedDrivers(definition, selection.Supported)
		if err != nil {
			return ResourcePlan{}, err
		}
		if !containsDriver(supported, active) {
			return ResourcePlan{}, fmt.Errorf("resource %s active driver %q is not built into the App", definition.Label, active)
		}
		normalized.Selections[definition.Key] = DriverSelection{Active: active, Supported: supported}
	}
	if components.DemoApp {
		database, ok := normalized.Selections[ResourceDatabase]
		if !ok || database.Active != "mysql" {
			return ResourcePlan{}, fmt.Errorf("Demo App requires the MySQL database driver")
		}
		if !containsDriver(database.Supported, "sqlite") {
			return ResourcePlan{}, fmt.Errorf("Demo App requires the SQLite database fallback to be built into the App")
		}
	}
	for _, named := range p.GeneratedNamedSelections(components) {
		named.Active = CanonicalResourceDriver(named.Resource, named.Active)
		definition, ok := ResourceDefinitionByKey(named.Resource)
		if !ok {
			return ResourcePlan{}, fmt.Errorf("named resource %s refers to unknown resource %q", named.Label, named.Resource)
		}
		if _, ok := definition.Driver(named.Active); !ok {
			return ResourcePlan{}, fmt.Errorf("named resource %s selects unknown driver %q", named.Label, named.Active)
		}
		root := normalized.Selections[named.Resource]
		if !containsDriver(root.Supported, named.Active) {
			return ResourcePlan{}, fmt.Errorf("resource %s must build in %q because it is required by %s", named.Resource, named.Active, named.Label)
		}
		normalized.NamedSelections[named.EnvironmentKey] = named.Active
	}
	return normalized, nil
}

// Validate reports whether the complete plan satisfies capability and built-in-driver invariants.
func (p ResourcePlan) Validate(components Components) error {
	_, err := p.Normalized(components)
	return err
}

// GeneratedNamedSelections resolves framework-owned named resources from concrete overrides or catalog defaults.
func (p ResourcePlan) GeneratedNamedSelections(components Components) []GeneratedNamedResourceSelection {
	selections := []GeneratedNamedResourceSelection{}
	for _, definition := range resourceCatalog {
		if !definition.AppliesTo(components) {
			continue
		}
		for _, named := range definition.NamedResources {
			if named.RequiredComponent != "" && !components.Enabled(named.RequiredComponent) {
				continue
			}
			driver, explicit := p.NamedSelections[named.EnvironmentKey]
			if !explicit {
				driver = named.DefaultDriver
			}
			selections = append(selections, GeneratedNamedResourceSelection{
				Resource:       named.Resource,
				Name:           named.Name,
				Label:          named.Label,
				EnvironmentKey: named.EnvironmentKey,
				Active:         normalizeDriverName(driver),
			})
		}
	}
	return selections
}

// defaultGeneratedNamedDrivers resolves catalog-owned defaults before user or environment overrides exist.
func defaultGeneratedNamedDrivers(components Components) map[string]string {
	drivers := map[string]string{}
	plan := ResourcePlan{}
	for _, selection := range plan.GeneratedNamedSelections(components) {
		drivers[selection.EnvironmentKey] = selection.Active
	}
	return drivers
}

// Mode returns an explicit local-service mode and reports whether one was selected.
func (i LocalServiceIntent) Mode(key ServiceKey) (LocalServiceMode, bool) {
	mode, ok := i.Modes[key]
	return mode, ok
}

// WithMode returns a cloned intent with one service placement replaced.
func (i LocalServiceIntent) WithMode(key ServiceKey, mode LocalServiceMode) LocalServiceIntent {
	modes := make(map[ServiceKey]LocalServiceMode, len(i.Modes)+1)
	for currentKey, currentMode := range i.Modes {
		modes[currentKey] = currentMode
	}
	modes[key] = mode
	return LocalServiceIntent{Modes: modes}
}

// cloneDriverSelection prevents supported-driver slices from aliasing across plan values.
func cloneDriverSelection(selection DriverSelection) DriverSelection {
	return DriverSelection{Active: selection.Active, Supported: append([]string(nil), selection.Supported...)}
}

// normalizeDriverName applies the environment representation used by every generator.
func normalizeDriverName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// normalizeSupportedDrivers validates names before returning catalog-stable ordering.
func normalizeSupportedDrivers(definition ResourceDefinition, drivers []string) ([]string, error) {
	seen := map[string]bool{}
	ordered := make([]DriverDefinition, 0, len(drivers))
	for _, name := range drivers {
		normalized := CanonicalResourceDriver(definition.Key, name)
		if normalized == "" || seen[normalized] {
			continue
		}
		driver, ok := definition.Driver(normalized)
		if !ok {
			return nil, fmt.Errorf("resource %s lists unknown built-in driver %q", definition.Label, name)
		}
		seen[normalized] = true
		ordered = append(ordered, driver)
	}
	if len(ordered) == 0 {
		return nil, fmt.Errorf("resource %s requires at least one built-in driver", definition.Label)
	}
	sort.SliceStable(ordered, func(left, right int) bool { return ordered[left].Order < ordered[right].Order })
	normalized := make([]string, 0, len(ordered))
	for _, driver := range ordered {
		normalized = append(normalized, driver.Name)
	}
	return normalized, nil
}

// containsDriver reports membership in an already normalized supported-driver list.
func containsDriver(drivers []string, want string) bool {
	for _, driver := range drivers {
		if driver == want {
			return true
		}
	}
	return false
}
