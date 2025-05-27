package goforj

import (
	"fmt"
	"go/format"
	"os"
	"strings"
)

func formatGoFile(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	formatted, err := format.Source(src)
	if err != nil {
		return err
	}
	return os.WriteFile(path, formatted, 0644)
}

func containsLine(lines []string, target string) bool {
	for _, line := range lines {
		if strings.TrimSpace(line) == strings.TrimSpace(target) {
			return true
		}
	}
	return false
}

func insertBeforeClosingBrace(lines []string, structStartMarker, insert string) []string {
	inStruct := false
	for i, line := range lines {
		if strings.Contains(line, structStartMarker) {
			inStruct = true
			continue
		}
		if inStruct && strings.TrimSpace(line) == "}" {
			return append(lines[:i], append([]string{insert}, lines[i:]...)...)
		}
	}
	return lines
}

func insertIntoFuncParams(lines []string, funcName string, insert string) []string {
	foundFunc := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		if !foundFunc && strings.Contains(line, "func "+funcName+"(") {
			foundFunc = true
			continue
		}

		if foundFunc {
			trimmed := strings.TrimSpace(line)

			// check if line contains closing param paren before return type
			if strings.HasSuffix(trimmed, ")") || strings.HasSuffix(trimmed, ") *AppCommands {") || strings.HasSuffix(trimmed, ") error {") {
				lines = append(lines[:i], append([]string{insert}, lines[i:]...)...)
				break
			}
		}
	}

	return lines
}

func snakeCase(s string) string {
	var out []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			out = append(out, '_')
		}
		out = append(out, r)
	}
	return strings.ToLower(string(out))
}

func getGoModuleName() (string, error) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", fmt.Errorf("module name not found")
}

func ensureCmdSuffix(name string) string {
	if !strings.HasSuffix(name, "Cmd") {
		return name + "Cmd"
	}
	return name
}
