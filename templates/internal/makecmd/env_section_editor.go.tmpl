package makecmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goforj/str/v2"
)

// EnvWrite describes one env key/value pair to upsert.
type EnvWrite struct {
	Key   string
	Value string
}

// EnvChange describes how one env key was handled.
type EnvChange struct {
	Key    string
	Value  string
	Status string
}

const (
	// envChangeCreated marks a newly inserted env key.
	envChangeCreated = "created"
	// envChangeUpdated marks an existing env key whose value changed.
	envChangeUpdated = "updated"
	// envChangeSkipped marks an existing env key whose value already matched.
	envChangeSkipped = "skipped"
)

// ApplyEnvSectionWrites upserts env writes into a named section while preserving surrounding lines.
func ApplyEnvSectionWrites(path, sectionHeader, insertAfter string, writes []EnvWrite) ([]EnvChange, error) {
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	lines, trailingNewline := splitEnvContent(string(content), os.IsNotExist(err))
	sectionStart, sectionEnd := findEnvSection(lines, sectionHeader)
	if sectionStart == -1 {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, sectionHeader)
		sectionStart = len(lines) - 1
		if strings.TrimSpace(insertAfter) != "" {
			lines = append(lines, insertAfter)
		}
		sectionEnd = len(lines)
	}

	insertPoint := envSectionInsertPoint(lines, sectionStart, sectionEnd, insertAfter, writes)
	changes := make([]EnvChange, 0, len(writes))
	for _, write := range writes {
		var change EnvChange
		lines, sectionEnd, insertPoint, change = upsertEnvWrite(lines, sectionStart, sectionEnd, insertPoint, write)
		changes = append(changes, change)
	}

	if err := writeEnvFile(path, lines, trailingNewline); err != nil {
		return nil, err
	}
	return changes, nil
}

// splitEnvContent splits env content into logical lines and tracks EOF newline behavior.
func splitEnvContent(content string, missing bool) ([]string, bool) {
	if missing {
		return nil, true
	}
	trailingNewline := strings.HasSuffix(content, "\n")
	if trailingNewline {
		content = strings.TrimSuffix(content, "\n")
	}
	if content == "" {
		return nil, trailingNewline
	}
	return strings.Split(content, "\n"), trailingNewline
}

// findEnvSection returns the line range for a comment-headed env section.
func findEnvSection(lines []string, sectionHeader string) (int, int) {
	for i, line := range lines {
		if strings.TrimSpace(line) != strings.TrimSpace(sectionHeader) {
			continue
		}
		return i, findEnvSectionEnd(lines, i)
	}
	return -1, -1
}

// findEnvSectionEnd finds the next section header after start.
func findEnvSectionEnd(lines []string, start int) int {
	for i := start + 1; i < len(lines); i++ {
		if isEnvSectionHeader(lines, i) {
			return i
		}
	}
	return len(lines)
}

// isEnvSectionHeader reports whether line index looks like a top-level env section heading.
func isEnvSectionHeader(lines []string, index int) bool {
	line := strings.TrimSpace(lines[index])
	if !strings.HasPrefix(line, "# ") {
		return false
	}
	if strings.HasPrefix(strings.ToLower(line), "# available drivers:") {
		return false
	}
	if commentedEnvKey(line) != "" {
		return false
	}
	for i := index + 1; i < len(lines); i++ {
		next := strings.TrimSpace(lines[i])
		if next == "" {
			continue
		}
		return !strings.HasPrefix(next, "#")
	}
	return false
}

// envSectionInsertPoint chooses where new env keys should be inserted.
func envSectionInsertPoint(lines []string, sectionStart, sectionEnd int, insertAfter string, writes []EnvWrite) int {
	lastExisting := -1
	for i := sectionStart + 1; i < sectionEnd; i++ {
		key := activeEnvKey(lines[i])
		if key == "" {
			key = commentedEnvKey(lines[i])
		}
		if key == "" {
			continue
		}
		for _, write := range writes {
			if key == write.Key && i > lastExisting {
				lastExisting = i
			}
		}
	}
	if lastExisting != -1 {
		return lastExisting + 1
	}

	cleanInsertAfter := strings.TrimSpace(insertAfter)
	if cleanInsertAfter != "" {
		for i := sectionStart + 1; i < sectionEnd; i++ {
			if strings.TrimSpace(lines[i]) == cleanInsertAfter {
				return envInsertPointAfterAnchorBlock(lines, i+1, sectionEnd, writes)
			}
		}
	}
	return envSectionContentEnd(lines, sectionStart, sectionEnd)
}

// envSectionContentEnd keeps inserted values above the blank separator owned by the next section.
func envSectionContentEnd(lines []string, sectionStart, sectionEnd int) int {
	for sectionEnd > sectionStart+1 && strings.TrimSpace(lines[sectionEnd-1]) == "" {
		sectionEnd--
	}
	return sectionEnd
}

// envInsertPointAfterAnchorBlock skips comments and related env assignments that belong to an anchor.
func envInsertPointAfterAnchorBlock(lines []string, start, sectionEnd int, writes []EnvWrite) int {
	insertPoint := start
	for insertPoint < sectionEnd {
		clean := strings.TrimSpace(lines[insertPoint])
		if clean == "" {
			break
		}
		if strings.HasPrefix(clean, "#") {
			insertPoint++
			continue
		}
		if key := activeEnvKey(clean); key != "" && envKeyMatchesWriteFamily(key, writes) {
			insertPoint++
			continue
		}
		break
	}
	return insertPoint
}

// envKeyMatchesWriteFamily reports whether key belongs to the same root env family as writes.
func envKeyMatchesWriteFamily(key string, writes []EnvWrite) bool {
	keyRoot := envKeyRoot(key)
	if keyRoot == "" {
		return false
	}
	for _, write := range writes {
		if keyRoot == envKeyRoot(write.Key) {
			return true
		}
	}
	return false
}

// envKeyRoot returns the root prefix before the first underscore.
func envKeyRoot(key string) string {
	root, _, ok := strings.Cut(strings.TrimSpace(key), "_")
	if !ok {
		return ""
	}
	return root
}

// upsertEnvWrite updates, uncomments, or inserts one env key/value pair.
func upsertEnvWrite(lines []string, sectionStart, sectionEnd, insertPoint int, write EnvWrite) ([]string, int, int, EnvChange) {
	assignment := envAssignment(write)
	for i := sectionStart + 1; i < sectionEnd; i++ {
		if activeEnvKey(lines[i]) != write.Key {
			continue
		}
		if strings.TrimSpace(lines[i]) == assignment {
			return lines, sectionEnd, insertPoint, EnvChange{Key: write.Key, Value: write.Value, Status: envChangeSkipped}
		}
		lines[i] = assignment
		return lines, sectionEnd, insertPoint, EnvChange{Key: write.Key, Value: write.Value, Status: envChangeUpdated}
	}

	for i := sectionStart + 1; i < sectionEnd; i++ {
		if commentedEnvKey(lines[i]) != write.Key {
			continue
		}
		lines[i] = assignment
		return lines, sectionEnd, insertPoint, EnvChange{Key: write.Key, Value: write.Value, Status: envChangeUpdated}
	}

	lines = append(lines[:insertPoint], append([]string{assignment}, lines[insertPoint:]...)...)
	return lines, sectionEnd + 1, insertPoint + 1, EnvChange{Key: write.Key, Value: write.Value, Status: envChangeCreated}
}

// activeEnvKey returns the key for an active env assignment line.
func activeEnvKey(line string) string {
	clean := strings.TrimSpace(line)
	if clean == "" || strings.HasPrefix(clean, "#") {
		return ""
	}
	key, _, ok := strings.Cut(clean, "=")
	if !ok {
		return ""
	}
	return strings.TrimSpace(key)
}

// commentedEnvKey returns the key for a commented example env assignment line.
func commentedEnvKey(line string) string {
	clean := strings.TrimSpace(line)
	if !strings.HasPrefix(clean, "#") {
		return ""
	}
	clean = str.Of(clean).TrimPrefix("#").Trim().String()
	key, _, ok := strings.Cut(clean, "=")
	if !ok {
		return ""
	}
	return strings.TrimSpace(key)
}

// envAssignment formats one env assignment.
func envAssignment(write EnvWrite) string {
	return fmt.Sprintf("%s=%s", strings.TrimSpace(write.Key), strings.TrimSpace(write.Value))
}

// writeEnvFile writes env content using a temporary file and rename.
func writeEnvFile(path string, lines []string, trailingNewline bool) error {
	content := strings.Join(lines, "\n")
	if trailingNewline {
		content += "\n"
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil && dir != "." {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	temp, err := os.CreateTemp(dir, ".env-write-*")
	if err != nil {
		return fmt.Errorf("create temp env file: %w", err)
	}
	tempPath := temp.Name()
	if _, err := temp.WriteString(content); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("write temp env file: %w", err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("close temp env file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
