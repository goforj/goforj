package forj

import (
	"fmt"
	"slices"
	"strings"

	"github.com/goforj/console"
	"github.com/goforj/goforj/internal/localproviders"
	"github.com/goforj/goforj/project"
)

// LocalProvidersCmd switches project resources to local providers with one private restore point.
type LocalProvidersCmd struct {
	root string
	ui   *console.Console
}

// Signature places the local-provider switch alongside the existing environment commands.
func (*LocalProvidersCmd) Signature() string {
	return `name:"env:local" help:"Choose resource drivers, use local providers, or restore settings"`
}

// Help explains which files and running processes the local-provider switch affects.
func (*LocalProvidersCmd) Help() string {
	return "Choose resource drivers or switch to SQLite and in-process providers, review the changes, and update .env.\nOne private backup allows you to restore the previous settings.\nRun forj build afterward to regenerate driver support. Database contents and existing containers stay in place."
}

// Run previews every mutation and leaves generation and process management to their existing commands.
func (c *LocalProvidersCmd) Run() error {
	root := c.root
	if root == "" {
		root = "."
	}
	ui := c.ui
	if ui == nil {
		ui = console.Default()
	}
	session, err := localproviders.Open(root)
	if err != nil {
		return err
	}
	options := []string{"Use local providers", "Choose drivers", "Cancel"}
	defaultChoice := 0
	if session.HasPrevious() {
		options = []string{"Use local providers", "Choose drivers", "Restore previous settings", "Cancel"}
		defaultChoice = 2
	}
	choice, err := ui.ChooseIndex("Resource drivers", options, defaultChoice)
	if err != nil || choice == len(options)-1 {
		return err
	}
	restore := choice == 2
	var values map[string]string
	switch choice {
	case 0:
		values, err = chooseLocalResources(ui, session)
	case 1:
		values, err = chooseResourceDrivers(ui, session)
	case 2:
		values, err = session.Previous()
	}
	if err != nil || values == nil {
		return err
	}
	return applyLocalProviders(ui, session, values, restore)
}

// chooseLocalResources offers only the local preset and per-resource exclusions.
func chooseLocalResources(ui *console.Console, s *localproviders.Session) (map[string]string, error) {
	excluded := map[string]bool{}
	for _, resource := range s.Resources {
		if resource.Definition.DefaultDriver == "" {
			excluded[resource.Key] = true
		}
	}
	for {
		choices := []string{"Review changes"}
		for _, resource := range s.Resources {
			mark := "x"
			if excluded[resource.Key] {
				mark = " "
			}
			target := resource.Definition.DefaultDriver
			if target == "" {
				target = "no shared local provider"
			}
			choices = append(choices, fmt.Sprintf("[%s] %s -> %s", mark, resource.Key, target))
		}
		choices = append(choices, "Cancel")
		choice, err := ui.ChooseIndex("Choose resources to switch to local providers", choices, 0)
		if err != nil {
			return nil, err
		}
		if choice == len(choices)-1 {
			return nil, nil
		}
		if choice == 0 {
			return s.PortableFor(excluded)
		}
		key := s.Resources[choice-1].Key
		excluded[key] = !excluded[key]
	}
}

// chooseResourceDrivers edits only catalog driver choices and leaves credentials in project configuration.
func chooseResourceDrivers(ui *console.Console, s *localproviders.Session) (map[string]string, error) {
	selections := map[string]string{}
	for {
		choices := []string{"Review changes"}
		for _, resource := range s.Resources {
			driver := "keep current"
			if selected, ok := selections[resource.Key]; ok {
				driver = selected
			}
			current := resourceDriverDisplay(resource.Definition, s.Current[resource.Key], false)
			if current == "" {
				current = "inherited/default"
			}
			choices = append(choices, fmt.Sprintf("%s: %s -> %s", resource.Key, current, driver))
		}
		choices = append(choices, "Cancel")
		choice, err := ui.ChooseIndex("Choose resource drivers", choices, 0)
		if err != nil || choice == len(choices)-1 {
			return nil, err
		}
		if choice == 0 {
			return s.Drivers(selections)
		}
		resource := s.Resources[choice-1]
		drivers := []string{"Keep current setting"}
		for _, driver := range resource.Definition.Drivers {
			drivers = append(drivers, driver.Name)
		}
		selected, err := ui.ChooseIndex(resource.Key, drivers, 0)
		if err != nil {
			return nil, err
		}
		if selected == 0 {
			delete(selections, resource.Key)
		} else {
			selections[resource.Key] = resource.Definition.Drivers[selected-1].Name
		}
	}
}

// resourceDriverDisplay reveals recognized driver names without echoing misplaced credentials.
func resourceDriverDisplay(definition project.ResourceDefinition, value string, multiple bool) string {
	if strings.TrimSpace(value) == "" {
		return value
	}
	drivers := []string{value}
	if multiple {
		drivers = strings.Split(value, ",")
	}
	for _, driver := range drivers {
		if multiple && strings.TrimSpace(driver) == "" {
			continue
		}
		if _, valid := definition.Driver(driver); !valid {
			return "(invalid)"
		}
	}
	return value
}

// applyLocalProviders shows exact provider and path changes while masking private connection values.
func applyLocalProviders(ui *console.Console, s *localproviders.Session, values map[string]string, restore bool) error {
	if err := s.Validate(values); err != nil {
		return err
	}
	overrides, err := s.Overrides()
	if err != nil {
		return err
	}
	for _, layer := range overrides {
		ui.Infof("Other runtime layer contains resource settings (%s). Its existing precedence still applies.", layer)
	}
	all := map[string]string{}
	for key, value := range s.Current {
		all[key] = value
	}
	for key, value := range values {
		all[key] = value
	}
	definitions := map[string]project.ResourceDefinition{}
	paths := map[string]bool{}
	for _, resource := range s.ResourcesFor(all) {
		definitions[resource.Key] = resource.Definition
		definitions[resource.SupportedKey] = resource.Definition
		if resource.Definition.Key == project.ResourceDatabase {
			paths[strings.TrimSuffix(resource.Key, "DRIVER")+"SQLITE_DATABASE"] = true
		}
	}
	var keys []string
	for key := range all {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	changed := false
	ui.Infof("Changes to .env")
	for _, key := range keys {
		before, had := s.Current[key]
		after, has := values[key]
		if had == has && before == after {
			continue
		}
		changed = true
		definition, driver := definitions[key]
		if driver {
			multiple := strings.HasSuffix(key, "_SUPPORTED_DRIVERS")
			before = resourceDriverDisplay(definition, before, multiple)
			after = resourceDriverDisplay(definition, after, multiple)
		}
		if driver || key == "COMPOSE_PROFILES" || paths[key] {
			if !had {
				before = "(unset)"
			}
			if !has {
				after = "(unset)"
			}
			ui.Infof("  %s: %s -> %s", key, before, after)
		} else {
			ui.Infof("  %s: private setting changes", key)
		}
	}
	if !changed {
		if !restore {
			ui.Infof("No changes needed")
			return nil
		}
		ui.Infof("Settings are already restored. Only the private recovery record needs cleanup.")
	}
	ui.Infof("Check connection settings for newly selected service drivers before starting the App.")
	ui.Infof("Database contents stay in place. Apply compatible migrations separately before starting the App.")
	ui.Infof("Memory and inproc providers are process-local. Existing containers keep running.")
	ui.Infof("Stop forj dev before applying changes if its startup tasks could touch the database.")
	prompt := "Apply these settings to .env and keep one private restore point?"
	if restore {
		prompt = "Restore these settings to .env and clear the restore point?"
	}
	confirmed, err := ui.Confirm(prompt, false)
	if err != nil || !confirmed {
		return err
	}
	if restore {
		err = s.Restore()
	} else {
		err = s.Apply(values)
	}
	if err != nil {
		return err
	}
	if !changed {
		ui.Successf("Cleared the private recovery record")
		return nil
	}
	ui.Successf("Updated .env")
	ui.Infof("Run forj build to regenerate driver support, then restart the App.")
	return nil
}
