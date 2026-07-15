package envfile

import (
	"strings"

	"github.com/joho/godotenv"
)

// Lookup returns the final concrete assignment for a key using dotenv override semantics.
func Lookup(lines []string, want string) (string, bool) {
	values, err := godotenv.Unmarshal(strings.Join(lines, "\n"))
	if err == nil {
		value, found := values[want]
		return value, found
	}

	// A malformed unrelated line must not hide a concrete owner value and allow
	// a generator to replace it as though the key were missing.
	var value string
	found := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		key, _, candidate, ok := scanAssignment(line)
		if !ok || key != want {
			continue
		}
		value = strings.TrimSpace(candidate)
		found = true
	}
	return value, found
}

// SetFinal updates the assignment that controls dotenv precedence or appends a missing key.
func SetFinal(lines []string, want string, value string) []string {
	index := -1
	for lineIndex, line := range lines {
		key, _, ok := ParseAssignment(line)
		if ok && key == want {
			index = lineIndex
		}
	}
	if index >= 0 {
		lines[index] = want + "=" + value
		return lines
	}
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines[len(lines)-1] = want + "=" + value
		return append(lines, "")
	}
	return append(lines, want+"="+value)
}

// ParseAssignment decodes one active dotenv assignment for line-oriented owner edits.
func ParseAssignment(line string) (string, string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", "", false
	}
	values, err := godotenv.Unmarshal(line)
	if err != nil || len(values) != 1 {
		return "", "", false
	}
	for key, value := range values {
		key = strings.TrimSpace(key)
		return key, value, key != ""
	}
	return "", "", false
}

// ScanKey recognizes the key from an active or commented dotenv assignment without decoding its value.
func ScanKey(line string) (string, bool) {
	key, _, _, ok := scanAssignment(line)
	return key, ok
}

// scanAssignment preserves the assignment prefix needed by source-aware transforms.
func scanAssignment(line string) (string, string, string, bool) {
	cursor := skipHorizontalSpace(line, 0)
	if cursor < len(line) && line[cursor] == '#' {
		cursor = skipHorizontalSpace(line, cursor+1)
	}
	if hasWordAt(line, cursor, "export") {
		cursor = skipHorizontalSpace(line, cursor+len("export"))
	}
	if cursor >= len(line) || !isKeyStart(line[cursor]) {
		return "", "", "", false
	}
	keyStart := cursor
	for cursor < len(line) && isKeyPart(line[cursor]) {
		cursor++
	}
	key := line[keyStart:cursor]
	cursor = skipHorizontalSpace(line, cursor)
	if cursor >= len(line) || line[cursor] != '=' {
		return "", "", "", false
	}
	cursor++
	return key, line[:cursor], line[cursor:], true
}

// skipHorizontalSpace advances past spacing accepted around dotenv assignments without consuming line structure.
func skipHorizontalSpace(value string, start int) int {
	for start < len(value) && (value[start] == ' ' || value[start] == '\t') {
		start++
	}
	return start
}

// hasWordAt prevents a key beginning with export from being mistaken for the optional keyword.
func hasWordAt(value string, start int, word string) bool {
	end := start + len(word)
	if end > len(value) || value[start:end] != word {
		return false
	}
	return end < len(value) && (value[end] == ' ' || value[end] == '\t')
}

// isKeyStart enforces the portable dotenv key subset used by generated GoForj projects.
func isKeyStart(value byte) bool {
	return value == '_' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

// isKeyPart keeps assignment recognition narrow enough that prose comments cannot be transformed accidentally.
func isKeyPart(value byte) bool {
	return isKeyStart(value) || value >= '0' && value <= '9'
}
