package project

import "github.com/goforj/str/v2"

// HelpFormat identifies the generated Kong help formatter used by an app.
type HelpFormat string

const (
	// HelpFormatFramework keeps the framework-oriented command grouping.
	HelpFormatFramework HelpFormat = "framework"
	// HelpFormatExternalCLI uses a compact layout for user-facing CLI binaries.
	HelpFormatExternalCLI HelpFormat = "external_cli"
	// HelpFormatGuided uses examples-first help inspired by clig.dev guidance.
	HelpFormatGuided HelpFormat = "guided"
)

// HelpFormatDefinition describes a help formatter option in interactive flows.
type HelpFormatDefinition struct {
	Key         HelpFormat
	Label       string
	Description string
}

// HelpFormatCatalog returns the formatter choices exposed for app CLI help.
func HelpFormatCatalog() []HelpFormatDefinition {
	return []HelpFormatDefinition{
		{Key: HelpFormatFramework, Label: "Framework", Description: "grouped command surface for app operations"},
		{Key: HelpFormatGuided, Label: "Guided (clig.dev-inspired)", Description: "external/user-facing CLI with examples and next steps"},
		{Key: HelpFormatExternalCLI, Label: "External CLI", Description: "compact help for small command tools"},
	}
}

// DefaultHelpFormat returns the backwards-compatible formatter selection.
func DefaultHelpFormat() HelpFormat {
	return HelpFormatFramework
}

// NormalizeHelpFormat maps empty or unknown values to the framework formatter.
func NormalizeHelpFormat(value HelpFormat) HelpFormat {
	switch HelpFormat(str.Of(string(value)).Trim().ToLower().String()) {
	case HelpFormatExternalCLI:
		return HelpFormatExternalCLI
	case HelpFormatGuided:
		return HelpFormatGuided
	case HelpFormatFramework:
		return HelpFormatFramework
	default:
		return DefaultHelpFormat()
	}
}

// HelpFormatDefinitionByKey returns the display metadata for a formatter key.
func HelpFormatDefinitionByKey(key HelpFormat) (HelpFormatDefinition, bool) {
	key = NormalizeHelpFormat(key)
	for _, definition := range HelpFormatCatalog() {
		if definition.Key == key {
			return definition, true
		}
	}
	return HelpFormatDefinition{}, false
}
