package envfile

import (
	"bytes"
	"strings"

	"github.com/goforj/str/v2"
	"github.com/joho/godotenv"
)

// RedactExample derives a commit-safe environment contract without discarding runtime-shaping values or comments.
func RedactExample(source []byte) []byte {
	var output bytes.Buffer
	var multilineQuote byte
	sensitiveValues := sensitiveAssignmentValues(source)
	for _, chunk := range bytes.SplitAfter(source, []byte{'\n'}) {
		if len(chunk) == 0 {
			continue
		}
		line, ending := splitLineEnding(chunk)
		if multilineQuote != 0 {
			suffix, closed := redactMultilineContinuation(string(line), multilineQuote)
			output.WriteString(suffix)
			output.Write(ending)
			if closed {
				multilineQuote = 0
			}
			continue
		}

		key, prefix, value, ok := scanAssignment(string(line))
		if !ok || (!isSensitiveKey(key) && !sensitiveValues[key] && !valueContainsURLUserinfo(value)) {
			output.Write(chunk)
			continue
		}
		redacted, openQuote := redactAssignmentValue(prefix, value)
		output.WriteString(redacted)
		output.Write(ending)
		multilineQuote = openQuote
	}
	return output.Bytes()
}

// sensitiveAssignmentValues decodes complete assignments so multiline credential URLs cannot evade line transforms.
func sensitiveAssignmentValues(source []byte) map[string]bool {
	values, err := godotenv.Unmarshal(string(source))
	if err != nil {
		return nil
	}
	sensitive := make(map[string]bool)
	for key, value := range values {
		joined := strings.Map(func(character rune) rune {
			if character == '\r' || character == '\n' {
				return -1
			}
			return character
		}, value)
		if valueContainsURLUserinfo(joined) {
			sensitive[key] = true
		}
	}
	return sensitive
}

// MergeExample updates generated assignments while retaining existing safe comments and app-owned keys.
func MergeExample(existing []byte, source []byte) []byte {
	generated := RedactExample(source)
	lines := strings.Split(string(RedactExample(existing)), "\n")
	indices := map[string]int{}
	commented := map[string]bool{}
	for index, line := range lines {
		key, _, _, ok := scanAssignment(line)
		if !ok {
			continue
		}
		isCommented := strings.HasPrefix(strings.TrimSpace(line), "#")
		previousCommented, exists := commented[key]
		if !exists || previousCommented && !isCommented {
			indices[key] = index
			commented[key] = isCommented
		}
	}

	generatedAssignments := map[string]string{}
	generatedCommented := map[string]bool{}
	order := []string{}
	for _, line := range strings.Split(string(generated), "\n") {
		key, _, _, ok := scanAssignment(line)
		if !ok {
			continue
		}
		isCommented := strings.HasPrefix(strings.TrimSpace(line), "#")
		previousCommented, exists := generatedCommented[key]
		if !exists {
			order = append(order, key)
		}
		if !exists || previousCommented && !isCommented {
			generatedAssignments[key] = line
			generatedCommented[key] = isCommented
		}
	}
	for _, key := range order {
		line := generatedAssignments[key]
		if index, exists := indices[key]; exists {
			if (isFrameworkContractKey(key) || isSensitiveKey(key)) && (commented[key] && !generatedCommented[key] || commented[key] == generatedCommented[key]) {
				lines[index] = line
			}
			continue
		}
		if !isFrameworkContractKey(key) {
			_, prefix, value, _ := scanAssignment(line)
			line, _ = redactAssignmentValue(prefix, value)
		}
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines[len(lines)-1] = line
			lines = append(lines, "")
		} else {
			lines = append(lines, line)
		}
	}
	merged := strings.Join(lines, "\n")
	if strings.HasSuffix(string(existing), "\n") && !strings.HasSuffix(merged, "\n") {
		merged += "\n"
	}
	return []byte(merged)
}

// valueContainsURLUserinfo catches credential-bearing schemes even when an owner uses an arbitrary key name.
func valueContainsURLUserinfo(value string) bool {
	searchFrom := 0
	for searchFrom < len(value) {
		relativeScheme := strings.Index(value[searchFrom:], "://")
		if relativeScheme < 0 {
			return false
		}
		authorityStart := searchFrom + relativeScheme + len("://")
		authorityEnd := len(value)
		if relativeEnd := strings.IndexAny(value[authorityStart:], "/?# \t\r\n'\"`"); relativeEnd >= 0 {
			authorityEnd = authorityStart + relativeEnd
		}
		if strings.Contains(value[authorityStart:authorityEnd], "@") {
			return true
		}
		searchFrom = authorityStart
	}
	return false
}

// splitLineEnding separates content from its original newline convention so examples retain stable diffs.
func splitLineEnding(line []byte) ([]byte, []byte) {
	if len(line) == 0 || line[len(line)-1] != '\n' {
		return line, nil
	}
	if len(line) >= 2 && line[len(line)-2] == '\r' {
		return line[:len(line)-2], line[len(line)-2:]
	}
	return line[:len(line)-1], line[len(line)-1:]
}

// isSensitiveKey identifies values that must never be copied from an owner-controlled environment into a committed example.
func isSensitiveKey(key string) bool {
	normalized := str.Of(key).Trim().ToUpper().String()
	if normalized == "APP_KEY" {
		return true
	}
	for _, marker := range []string{
		"SECRET",
		"PRIVATE_KEY",
		"CREDENTIAL",
		"API_KEY",
		"APIKEY",
		"ACCESS_KEY",
		"DSN",
		"CONNECTION_STRING",
		"ENCRYPTION_KEY",
		"SIGNING_KEY",
		"WEBHOOK",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	if normalized == "SSH_KEY" || strings.HasSuffix(normalized, "_SSH_KEY") || normalized == "HMAC_KEY" || strings.HasSuffix(normalized, "_HMAC_KEY") {
		return true
	}
	if normalized == "PAT" || strings.HasSuffix(normalized, "_PAT") {
		return true
	}
	if isSecretMetadataKey(normalized) {
		return false
	}
	if strings.Contains(normalized, "TOKEN") || strings.Contains(normalized, "PASSWORD") {
		return true
	}
	return isCredentialBearingURL(normalized)
}

// isSecretMetadataKey retains policy settings whose names mention passwords or tokens but whose values are not credentials.
func isSecretMetadataKey(key string) bool {
	for _, marker := range []string{
		"_PASSWORD_MIN_LENGTH",
		"_PASSWORD_MAX_LENGTH",
		"_PASSWORD_REQUIRE_",
		"_TOKEN_TTL",
		"_TOKEN_TIMEOUT",
		"_TOKEN_DURATION",
		"_TOKEN_LIFETIME",
		"_TOKEN_EXPIRY",
		"_TOKEN_EXPIRATION",
		"_TOKEN_MAX_AGE",
	} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return strings.HasSuffix(key, "_RETURN_TOKEN")
}

// isCredentialBearingURL covers connection URLs that commonly embed credentials while leaving public app endpoints intact.
func isCredentialBearingURL(key string) bool {
	for _, candidate := range []string{
		"DATABASE_URL",
		"DB_URL",
		"REDIS_URL",
		"CACHE_URL",
		"QUEUE_URL",
		"EVENTS_URL",
		"STORAGE_URL",
		"BROKER_URL",
		"AMQP_URL",
		"SMTP_URL",
	} {
		if key == candidate || strings.HasSuffix(key, "_"+candidate) {
			return true
		}
	}
	return false
}

// redactAssignmentValue removes an assignment value and its inline comment because comments can repeat credentials.
func redactAssignmentValue(prefix string, value string) (string, byte) {
	if strings.TrimSpace(value) == "" {
		return prefix + value, 0
	}
	valueStart := skipHorizontalSpace(value, 0)
	leadingSpace := value[:valueStart]
	if valueStart < len(value) && isQuote(value[valueStart]) {
		quote := value[valueStart]
		closing := findClosingQuote(value, valueStart+1, quote)
		if closing < 0 {
			return prefix + leadingSpace + string([]byte{quote, quote}), quote
		}
		return prefix + leadingSpace + string([]byte{quote, quote}), 0
	}
	return prefix, 0
}

// isQuote limits multiline handling to quoting conventions supported by common dotenv parsers.
func isQuote(value byte) bool {
	return value == '\'' || value == '"' || value == '`'
}

// findClosingQuote locates an unescaped terminator so secret fragments following escaped quotes are not retained.
func findClosingQuote(value string, start int, quote byte) int {
	backslashes := 0
	for index := start; index < len(value); index++ {
		if value[index] == '\\' {
			backslashes++
			continue
		}
		if value[index] == quote && backslashes%2 == 0 {
			return index
		}
		backslashes = 0
	}
	return -1
}

// redactMultilineContinuation blanks quoted payload lines and any trailing comment through their terminator.
func redactMultilineContinuation(line string, quote byte) (string, bool) {
	closing := findClosingQuote(line, 0, quote)
	if closing < 0 {
		return "", false
	}
	return "", true
}
