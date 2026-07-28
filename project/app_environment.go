package project

import "strings"

// AppEnvironmentPrefix converts an app name into the canonical prefix used by per-app environment overrides.
func AppEnvironmentPrefix(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || name == DefaultAppName {
		return ""
	}
	var builder strings.Builder
	lastWasSeparator := true
	for _, character := range name {
		switch {
		case character >= 'a' && character <= 'z':
			builder.WriteRune(character - 'a' + 'A')
			lastWasSeparator = false
		case character >= 'A' && character <= 'Z', character >= '0' && character <= '9':
			builder.WriteRune(character)
			lastWasSeparator = false
		default:
			if !lastWasSeparator {
				builder.WriteByte('_')
				lastWasSeparator = true
			}
		}
	}
	return strings.Trim(builder.String(), "_")
}
