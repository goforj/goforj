package envfile

import (
	"encoding/base64"
	"path/filepath"
	"strings"
)

const testingContractHeader = "# Test environment. Safe to commit; process variables override these values."

// MergeTesting derives a runnable test profile from the safe environment contract while preserving project-owned test values.
func MergeTesting(existing []byte, example []byte) []byte {
	generated := testingContract(example)
	if len(existing) == 0 {
		return generated
	}

	lines := strings.Split(string(RedactExample(existing)), "\n")
	generatedKeys := assignmentKeys(generated)
	retained := lines[:0]
	for _, line := range lines {
		key, _, ok := ParseAssignment(line)
		if ok && isFrameworkTestingKey(key) && !generatedKeys[key] {
			continue
		}
		retained = append(retained, line)
	}
	lines = retained
	indices := activeAssignmentIndices(lines)
	for _, line := range strings.Split(string(generated), "\n") {
		key, _, ok := ParseAssignment(line)
		if !ok {
			continue
		}
		if index, exists := indices[key]; exists {
			if isFrameworkTestingKey(key) {
				lines[index] = line
			}
			continue
		}
		lines = appendBeforeTerminalEmpty(lines, line)
		indices[key] = len(lines) - 1
	}
	if !containsExactLine(lines, testingContractHeader) {
		lines = append([]string{testingContractHeader, ""}, lines...)
	}
	merged := strings.Join(lines, "\n")
	if strings.HasSuffix(string(existing), "\n") && !strings.HasSuffix(merged, "\n") {
		merged += "\n"
	}
	return []byte(merged)
}

// assignmentKeys identifies the exact framework contract keys that remain owned by synchronization.
func assignmentKeys(content []byte) map[string]bool {
	keys := make(map[string]bool)
	for _, line := range strings.Split(string(content), "\n") {
		key, _, ok := ParseAssignment(line)
		if ok {
			keys[key] = true
		}
	}
	return keys
}

// testingContract converts every active example assignment into its deterministic test representation.
func testingContract(example []byte) []byte {
	lines := strings.Split(string(example), "\n")
	out := make([]string, 0, len(lines)+2)
	out = append(out, testingContractHeader, "")
	for _, line := range lines {
		key, value, ok := ParseAssignment(line)
		if !ok {
			out = append(out, line)
			continue
		}
		out = append(out, key+"="+EncodeValue(testingValue(key, value)))
	}
	return []byte(strings.Join(out, "\n"))
}

// testingValue selects public, deterministic values that cannot be mistaken for deployment credentials.
func testingValue(key string, value string) string {
	normalized := strings.ToUpper(strings.TrimSpace(key))
	policyKey := testingPolicyKey(normalized)
	switch policyKey {
	case "APP_ENV":
		return "testing"
	case "APP_KEY":
		return "base64:" + base64.StdEncoding.EncodeToString([]byte("goforj-public-testing-key-000001"))
	case "APP_DIAG_TOKEN":
		return "goforj-public-testing-diagnostics-token"
	case "LIGHTHOUSE_SECRET":
		return "goforj-public-testing-lighthouse-secret"
	case "API_JWT_SECRET_KEY":
		return "goforj-public-testing-jwt-signing-key-000000000000"
	case "AUTH_BOOTSTRAP_USERNAME", "AUTH_BOOTSTRAP_EMAIL", "AUTH_BOOTSTRAP_PASSWORD":
		return ""
	case "DB_HOST", "REDIS_HOST", "MAIL_SMTP_HOST":
		return "127.0.0.1"
	case "DB_USERNAME":
		return "test"
	case "DB_PASSWORD", "DB_ROOT_PASSWORD":
		return "test"
	case "QUEUE_ACCESS_KEY", "QUEUE_SECRET_KEY":
		return "test"
	case "DB_DATABASE", "DB_SQLITE_DATABASE":
		return testingDatabase(value)
	}
	if isSensitiveKey(normalized) {
		return ""
	}
	return value
}

// testingPolicyKey applies root policies to app-prefixed forms such as BILLING_DB_PASSWORD.
func testingPolicyKey(key string) string {
	for _, candidate := range []string{
		"APP_ENV", "APP_KEY", "APP_DIAG_TOKEN", "LIGHTHOUSE_SECRET", "API_JWT_SECRET_KEY",
		"AUTH_BOOTSTRAP_USERNAME", "AUTH_BOOTSTRAP_EMAIL", "AUTH_BOOTSTRAP_PASSWORD",
		"DB_HOST", "REDIS_HOST", "MAIL_SMTP_HOST", "DB_USERNAME", "DB_PASSWORD", "DB_ROOT_PASSWORD",
		"QUEUE_ACCESS_KEY", "QUEUE_SECRET_KEY", "DB_DATABASE", "DB_SQLITE_DATABASE",
	} {
		if key == candidate || strings.HasSuffix(key, "_"+candidate) {
			return candidate
		}
	}
	return key
}

// testingDatabase isolates test data without changing the configured database driver.
func testingDatabase(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "app_testing"
	}
	extension := filepath.Ext(value)
	switch strings.ToLower(extension) {
	case ".db", ".sqlite", ".sqlite3":
		return strings.TrimSuffix(value, extension) + "_testing" + extension
	}
	if strings.HasSuffix(value, "_testing") {
		return value
	}
	return value + "_testing"
}

// isFrameworkTestingKey identifies values whose test policy remains framework-owned across synchronization.
func isFrameworkTestingKey(key string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(key))
	for _, prefix := range []string{
		"APP_", "API_", "HTTP_", "AUTH_", "DB_", "REDIS_", "CACHE_", "QUEUE_", "EVENTS_",
		"STORAGE_", "MAIL_", "METRICS_", "LIGHTHOUSE_", "OBSERVABILITY_", "GRAFANA_", "SCHEDULER_",
	} {
		if strings.HasPrefix(normalized, prefix) || strings.Contains(normalized, "_"+prefix) {
			return true
		}
	}
	return normalized == "TZ" || normalized == "PORT"
}

// activeAssignmentIndices records the controlling assignment for each active key.
func activeAssignmentIndices(lines []string) map[string]int {
	indices := make(map[string]int)
	for index, line := range lines {
		key, _, ok := ParseAssignment(line)
		if ok {
			indices[key] = index
		}
	}
	return indices
}

// appendBeforeTerminalEmpty retains the existing newline convention while adding a missing assignment.
func appendBeforeTerminalEmpty(lines []string, line string) []string {
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines[len(lines)-1] = line
		return append(lines, "")
	}
	return append(lines, line)
}

// containsExactLine reports whether a generated ownership note is already present.
func containsExactLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}
