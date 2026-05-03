package project

import "fmt"

// StarterKit identifies an optional product scaffold layered on top of components.
type StarterKit string

const (
	StarterKitNone StarterKit = "none"
	StarterKitVue  StarterKit = "vue"
)

// StarterKitDefinition describes a selectable starter kit and its component contract.
type StarterKitDefinition struct {
	Key             StarterKit
	Label           string
	Description     string
	DefaultSelected bool
	Requires        []ComponentKey
}

var starterKitCatalog = []StarterKitDefinition{
	{
		Key:             StarterKitNone,
		Label:           "None",
		Description:     "Render a plain Web UI placeholder without an application starter",
		DefaultSelected: true,
	},
	{
		Key:         StarterKitVue,
		Label:       "Vue",
		Description: "Vue, Vite, TypeScript, Tailwind, and Shadcn-Vue app shell",
		Requires:    []ComponentKey{ComponentWebUI},
	},
}

// StarterKitCatalog returns the canonical starter-kit definitions.
func StarterKitCatalog() []StarterKitDefinition {
	out := make([]StarterKitDefinition, len(starterKitCatalog))
	copy(out, starterKitCatalog)
	return out
}

// StarterKitDefinitionByKey returns the definition for a starter-kit key.
func StarterKitDefinitionByKey(key StarterKit) (StarterKitDefinition, bool) {
	for _, definition := range starterKitCatalog {
		if definition.Key == key {
			return definition, true
		}
	}
	return StarterKitDefinition{}, false
}

// DefaultStarterKit returns the default starter-kit selection.
func DefaultStarterKit() StarterKit {
	for _, definition := range starterKitCatalog {
		if definition.DefaultSelected {
			return definition.Key
		}
	}
	return StarterKitNone
}

// NormalizeStarterKit returns a supported starter-kit value.
func NormalizeStarterKit(value StarterKit) StarterKit {
	switch value {
	case StarterKitVue:
		return StarterKitVue
	case StarterKitNone, "":
		return StarterKitNone
	default:
		return value
	}
}

// ValidateStarterKitContract reports invalid starter-kit/component combinations.
func ValidateStarterKitContract(starterKit StarterKit, components Components) error {
	starterKit = NormalizeStarterKit(starterKit)
	if starterKit == StarterKitNone {
		return nil
	}
	definition, ok := StarterKitDefinitionByKey(starterKit)
	if !ok {
		return fmt.Errorf("unknown starter kit: %s", starterKit)
	}
	components = components.WithResolvedDependencies()
	for _, required := range definition.Requires {
		if !components.Enabled(required) {
			return fmt.Errorf("%s starter kit requires %s", definition.Label, required)
		}
	}
	return nil
}
