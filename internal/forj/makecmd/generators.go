package makecmd

import (
	"fmt"
	"go/format"
	"os"
	"strconv"
	"strings"
)

type goImportSpec struct {
	alias string
	path  string
}

// readGeneratorGoFile reads a Go file as both lines and raw content for generator edits.
func readGeneratorGoFile(path string) ([]string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	content := string(data)
	return strings.Split(content, "\n"), content, nil
}

// writeGeneratorGoFile formats and writes generated Go source.
func writeGeneratorGoFile(path string, src []byte) error {
	formatted, err := format.Source(src)
	if err != nil {
		return fmt.Errorf("gofmt error: %w", err)
	}
	return os.WriteFile(path, formatted, 0644)
}

// writeGeneratorGoLines joins, formats, and writes edited Go source lines.
func writeGeneratorGoLines(path string, lines []string) error {
	return writeGeneratorGoFile(path, []byte(strings.Join(lines, "\n")))
}

// insertImportIfMissing adds an import path with an optional alias and normalizes imports.
func insertImportIfMissing(lines []string, importPath string, aliases ...string) []string {
	alias := ""
	if len(aliases) > 0 {
		alias = strings.TrimSpace(aliases[0])
	}
	spec := goImportSpec{alias: alias, path: importPath}

	hasImport := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "import (") {
			hasImport = true
			for j := i + 1; j < len(lines); j++ {
				if strings.TrimSpace(lines[j]) == ")" {
					for _, imp := range lines[i+1 : j] {
						existing, ok := parseGoImportSpec(imp)
						if ok && existing.path == importPath {
							return lines
						}
					}
					lines = append(lines[:j], append([]string{renderGoImportSpec(spec)}, lines[j:]...)...)
					break
				}
			}
			break
		}
	}
	if hasImport {
		return normalizeImports(lines)
	}

	var importLines []int
	var imports []goImportSpec
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "import ") {
			importLines = append(importLines, i)
			existing, ok := parseGoImportSpec(line)
			if ok {
				imports = append(imports, existing)
			}
		}
	}
	if len(importLines) > 0 {
		for _, imp := range imports {
			if imp.path == importPath {
				return normalizeImports(lines)
			}
		}
		imports = append(imports, spec)
		block := []string{"import ("}
		for _, imp := range imports {
			block = append(block, renderGoImportSpec(imp))
		}
		block = append(block, ")")

		start := importLines[0]
		lines = append(lines[:start], append(block, lines[start+1:]...)...)
		for i := len(importLines) - 1; i > 0; i-- {
			idx := importLines[i]
			lines = append(lines[:idx], lines[idx+1:]...)
		}
		return normalizeImports(lines)
	}

	for i, line := range lines {
		if strings.HasPrefix(line, "package ") {
			insert := []string{"", strings.TrimSpace("import " + strings.TrimSpace(renderGoImportSpec(spec))), ""}
			lines = append(lines[:i+1], append(insert, lines[i+1:]...)...)
			return normalizeImports(lines)
		}
	}
	return lines
}

// parseGoImportSpec parses a single Go import line or import block entry.
func parseGoImportSpec(line string) (goImportSpec, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "//") {
		return goImportSpec{}, false
	}
	if strings.HasPrefix(trimmed, "import ") {
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "import "))
	}
	if trimmed == "(" || trimmed == ")" {
		return goImportSpec{}, false
	}

	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return goImportSpec{}, false
	}
	if path, err := strconv.Unquote(fields[0]); err == nil {
		return goImportSpec{path: path}, true
	}
	if len(fields) >= 2 {
		path, err := strconv.Unquote(fields[1])
		if err == nil {
			return goImportSpec{alias: fields[0], path: path}, true
		}
	}
	return goImportSpec{}, false
}

// renderGoImportSpec renders an import block entry with its alias when present.
func renderGoImportSpec(spec goImportSpec) string {
	if spec.alias == "" {
		return fmt.Sprintf("\t%q", spec.path)
	}
	return fmt.Sprintf("\t%s %q", spec.alias, spec.path)
}

// normalizeImports converts imports to one de-duplicated import block.
func normalizeImports(lines []string) []string {
	var blockStart int = -1
	var blockEnd int = -1
	var imports []goImportSpec
	var singleImportLines []int
	seen := make(map[string]struct{})

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if blockStart == -1 && strings.HasPrefix(trimmed, "import (") {
			blockStart = i
			continue
		}
		if blockStart != -1 && blockEnd == -1 {
			if strings.TrimSpace(line) == ")" {
				blockEnd = i
				continue
			}
			spec, ok := parseGoImportSpec(line)
			if ok {
				key := spec.alias + "\x00" + spec.path
				if _, ok := seen[key]; !ok {
					seen[key] = struct{}{}
					imports = append(imports, spec)
				}
			}
			continue
		}

		if strings.HasPrefix(trimmed, "import ") && !strings.HasPrefix(trimmed, "import (") {
			singleImportLines = append(singleImportLines, i)
			spec, ok := parseGoImportSpec(line)
			if ok {
				key := spec.alias + "\x00" + spec.path
				if _, ok := seen[key]; !ok {
					seen[key] = struct{}{}
					imports = append(imports, spec)
				}
			}
		}
	}

	if blockStart != -1 {
		for i := len(singleImportLines) - 1; i >= 0; i-- {
			idx := singleImportLines[i]
			lines = append(lines[:idx], lines[idx+1:]...)
		}
		block := []string{"import ("}
		for _, imp := range imports {
			block = append(block, renderGoImportSpec(imp))
		}
		block = append(block, ")")
		lines = append(lines[:blockStart], append(block, lines[blockEnd+1:]...)...)
		return lines
	}

	if len(singleImportLines) <= 1 {
		return lines
	}

	block := []string{"import ("}
	for _, imp := range imports {
		block = append(block, renderGoImportSpec(imp))
	}
	block = append(block, ")")

	start := singleImportLines[0]
	lines = append(lines[:start], append(block, lines[start+1:]...)...)
	for i := len(singleImportLines) - 1; i > 0; i-- {
		idx := singleImportLines[i]
		lines = append(lines[:idx], lines[idx+1:]...)
	}
	return lines
}

// insertIntoCallBlock inserts a line before the closing paren of a call block.
func insertIntoCallBlock(lines []string, startMarker, insert string) []string {
	if containsLine(lines, insert) {
		return lines
	}
	for i, line := range lines {
		if !strings.Contains(line, startMarker) {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == ")" {
				return append(lines[:j], append([]string{insert}, lines[j:]...)...)
			}
		}
	}
	return lines
}

// insertAfterMarkerIfMissing inserts a line after a marker unless the line already exists.
func insertAfterMarkerIfMissing(lines []string, marker, insert string) []string {
	if containsLine(lines, insert) {
		return lines
	}
	for i, line := range lines {
		if strings.Contains(line, marker) {
			return append(lines[:i+1], append([]string{insert}, lines[i+1:]...)...)
		}
	}
	return lines
}
