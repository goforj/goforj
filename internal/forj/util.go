package forj

import (
	"fmt"
	"os"
	"strings"
)

// formatGoFile formats a Go source file in place.
func formatGoFile(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return writeGeneratorGoFile(path, src)
}

// containsLine reports whether lines contain target after trimming whitespace.
func containsLine(lines []string, target string) bool {
	for _, line := range lines {
		if strings.TrimSpace(line) == strings.TrimSpace(target) {
			return true
		}
	}
	return false
}

// insertBeforeClosingBrace inserts a line before the first closing brace after a marker.
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

// insertIntoFuncParams inserts a parameter line before a function parameter list closes.
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

			if strings.HasPrefix(trimmed, ")") {
				lines = append(lines[:i], append([]string{insert}, lines[i:]...)...)
				break
			}
		}
	}

	return lines
}

// getGoModuleName reads the module path from the current go.mod.
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

// ensureCmdSuffix appends Cmd when name does not already use that suffix.
func ensureCmdSuffix(name string) string {
	if !strings.HasSuffix(name, "Cmd") {
		return name + "Cmd"
	}
	return name
}
