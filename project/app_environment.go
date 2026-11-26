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

// AppResourceKeyOwner assigns resource settings to the longest known App prefix across resource families.
func AppResourceKeyOwner(key string, knownAppPrefixes []string) string {
	owner := ""
	for _, prefix := range knownAppPrefixes {
		if prefix == "" || len(prefix) <= len(owner) {
			continue
		}
		for _, definition := range resourceCatalog {
			if strings.HasPrefix(key, prefix+"_"+definition.EnvironmentPrefix+"_") {
				owner = prefix
				break
			}
		}
	}
	return owner
}
