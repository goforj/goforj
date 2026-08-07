package testkit

import (
	"fmt"
	"os"
	"strings"
)

// ParseEnvFiles centralizes parse env files behavior so callers follow the same contract.
func ParseEnvFiles(paths ...string) (map[string]string, error) {
	values := map[string]string{}
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, rawLine := range strings.Split(string(body), "\n") {
			line := strings.TrimSpace(rawLine)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			value = strings.TrimSpace(value)
			if commentIndex := strings.Index(value, " #"); commentIndex >= 0 {
				value = strings.TrimSpace(value[:commentIndex])
			}
			values[key] = strings.Trim(value, `"'`)
		}
	}
	return values, nil
}

// ReplaceOrAppendEnvValue centralizes replace or append env value behavior so callers follow the same contract.
func ReplaceOrAppendEnvValue(path, key, value string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(content), "\n")
	prefix := key + "="
	replaced := false
	for i := range lines {
		if strings.HasPrefix(lines[i], prefix) {
			lines[i] = prefix + value
			replaced = true
		}
	}
	if !replaced {
		lines = append(lines, prefix+value)
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

// ReplaceOrAppendEnvValues centralizes replace or append env values behavior so callers follow the same contract.
func ReplaceOrAppendEnvValues(paths []string, values map[string]string) error {
	for _, path := range paths {
		for key, value := range values {
			if err := ReplaceOrAppendEnvValue(path, key, value); err != nil {
				return fmt.Errorf("update %s in %s: %w", key, path, err)
			}
		}
	}
	return nil
}
