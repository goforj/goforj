package forj

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/goforj/str/v2"
)

// openURL centralizes open url behavior so callers follow the same contract.
func openURL(raw string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", raw).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", "", raw).Start()
	default:
		return exec.Command("xdg-open", raw).Start()
	}
}

// resolveLighthouseOpenURL centralizes resolve lighthouse open url lookup for the surrounding workflow.
func resolveLighthouseOpenURL(lighthouseURL string) string {
	raw := strings.TrimSpace(lighthouseURL)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	basePath := strings.TrimRight(u.Path, "/")
	if basePath == "" {
		basePath = "/lighthouse"
	}
	u.Path = basePath + "/auth/dev-session"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

// absolutizeLighthouseURL centralizes absolutize lighthouse url behavior so callers follow the same contract.
func absolutizeLighthouseURL(baseURL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsedRaw, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if parsedRaw.IsAbs() {
		return parsedRaw.String()
	}
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return raw
	}
	return base.ResolveReference(parsedRaw).String()
}

// readEnvKey centralizes read env key lookup for the surrounding workflow.
func readEnvKey(content, key string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		working := trimmed
		if strings.HasPrefix(working, "export ") {
			working = str.Of(working).TrimPrefix("export ").Trim().String()
		}
		if strings.HasPrefix(working, key+"=") {
			return str.Of(working).TrimPrefix(key + "=").Trim().String()
		}
	}
	return ""
}

// updateEnvKey centralizes update env key behavior so callers follow the same contract.
func updateEnvKey(content, key, value string) string {
	lines := strings.Split(content, "\n")
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		exportPrefix := ""
		working := trimmed
		if strings.HasPrefix(working, "export ") {
			exportPrefix = "export "
			working = str.Of(working).TrimPrefix("export ").Trim().String()
		}
		if !strings.HasPrefix(working, key+"=") {
			continue
		}
		found = true
		comment := ""
		if idx := strings.Index(line, " #"); idx >= 0 {
			comment = line[idx:]
		}
		lines[i] = exportPrefix + key + "=" + value + comment
	}
	if !found {
		lines = append(lines, key+"="+value)
	}
	return strings.Join(lines, "\n")
}

// loadDevRuntimeSettings centralizes load dev runtime settings lookup for the surrounding workflow.
func loadDevRuntimeSettings() (bool, string) {
	content, err := os.ReadFile(".env")
	if err != nil {
		return false, "1"
	}
	queryRaw := strings.TrimSpace(readEnvKey(string(content), "DB_QUERY_LOGGING"))
	debugRaw := strings.TrimSpace(readEnvKey(string(content), "APP_DEBUG"))
	queryOn := queryRaw == "1" || strings.EqualFold(queryRaw, "true")
	if debugRaw != "0" && debugRaw != "1" && debugRaw != "2" && debugRaw != "3" {
		debugRaw = "1"
	}
	return queryOn, debugRaw
}

// toggleDevQueryLogging centralizes toggle dev query logging behavior so callers follow the same contract.
func toggleDevQueryLogging() error {
	content, err := os.ReadFile(".env")
	if err != nil {
		return err
	}
	current := strings.TrimSpace(readEnvKey(string(content), "DB_QUERY_LOGGING"))
	next := "true"
	if current == "1" || strings.EqualFold(current, "true") {
		next = "false"
	}
	return writeDevEnvKey(".env", "DB_QUERY_LOGGING", next, content)
}

// setDevAppDebugLevel centralizes set dev app debug level behavior so callers follow the same contract.
func setDevAppDebugLevel(level string) error {
	if level != "0" && level != "1" && level != "2" && level != "3" {
		return fmt.Errorf("invalid APP_DEBUG level: %s", level)
	}
	content, err := os.ReadFile(".env")
	if err != nil {
		return err
	}
	return writeDevEnvKey(".env", "APP_DEBUG", level, content)
}

// writeDevEnvKey centralizes write dev env key persistence for the surrounding workflow.
func writeDevEnvKey(path, key, value string, content []byte) error {
	updated := updateEnvKey(string(content), key, value)
	suppressNextDevEnvTrigger()
	return os.WriteFile(path, []byte(updated), 0o644)
}
