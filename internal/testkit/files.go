package testkit

import (
	"fmt"
	"os"
	"strings"
)

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
